package app

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/follenfang/wowdoc/internal/home"
	"github.com/follenfang/wowdoc/internal/query"
	"github.com/follenfang/wowdoc/internal/result"
	"github.com/follenfang/wowdoc/internal/validator"
	"github.com/spf13/cobra"
	"github.com/yuin/gopher-lua/parse"
)

func validateCommand() *cobra.Command {
	var path, toc, sourceID, productID, ref string
	cmd := &cobra.Command{Use: "validate", RunE: func(cmd *cobra.Command, args []string) error {
		if err := require(path, "path_required", "--path is required"); err != nil {
			return err
		}
		if toc == "" {
			return validateDirectory(cmd, path, sourceID, productID, ref)
		}
		if err := require(sourceID, "source_required", "--source is required with --toc"); err != nil {
			return err
		}
		if err := require(productID, "product_required", "--product is required with --toc"); err != nil {
			return err
		}
		sel, err := selectSnapshot(sourceID, productID, ref)
		if err != nil {
			return err
		}
		defer sel.cat.Close()
		value, err := validator.Validate(validator.Options{
			Path: path, TOC: toc, SourceID: sel.source.ID, Product: sel.product.ID,
			RequestedRef: ref, MatchedTag: sel.ctx.MatchedTag, ResolvedCommit: sel.ctx.Commit,
			Evidence: snapshotEvidence{layout: sel.layout, ctx: sel.ctx},
		})
		return writeResult(cmd, value, err)
	}}
	cmd.Flags().StringVar(&path, "path", "", "AddOn directory")
	cmd.Flags().StringVar(&toc, "toc", "", "TOC file whose load closure is validated")
	cmd.Flags().StringVar(&sourceID, "source", "", "target source id")
	cmd.Flags().StringVar(&productID, "product", "", "target product id")
	cmd.Flags().StringVar(&ref, "ref", "latest", "target ref")
	return cmd
}

func validateDirectory(cmd *cobra.Command, path, sourceID, productID, ref string) error {
	diagnostics := make([]result.Diagnostic, 0)
	files := 0
	err := filepath.WalkDir(path, func(p string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() || !strings.EqualFold(filepath.Ext(p), ".lua") {
			return nil
		}
		files++
		data, readErr := os.ReadFile(p)
		if readErr != nil {
			return readErr
		}
		if _, parseErr := parse.Parse(bytes.NewReader(data), p); parseErr != nil {
			diagnostics = append(diagnostics, result.Diagnostic{Code: "lua_parse_failed", Message: parseErr.Error(), Path: p})
		}
		return nil
	})
	if err != nil {
		return err
	}
	return result.Write(cmd.OutOrStdout(), map[string]any{"path": path, "sourceId": sourceID, "product": productID, "ref": ref, "checkedLua": files, "valid": len(diagnostics) == 0, "diagnostics": diagnostics})
}

func validateMatrixCommand() *cobra.Command {
	var configPath string
	cmd := &cobra.Command{Use: "validate-matrix", RunE: func(cmd *cobra.Command, args []string) error {
		if err := require(configPath, "config_required", "--config is required"); err != nil {
			return err
		}
		config, addonPath, err := readMatrixConfig(configPath)
		if err != nil {
			return err
		}
		targets := make([]validator.Result, 0, len(config.Targets))
		seen := map[string]bool{}
		for _, target := range config.Targets {
			if target.ID == "" || target.TOC == "" || target.Product == "" || target.Ref == "" {
				return result.E("invalid_matrix_config", "each target requires id, toc, product, and ref", 2)
			}
			if seen[target.ID] {
				return result.E("duplicate_target_id", "duplicate matrix target id: "+target.ID, 2)
			}
			seen[target.ID] = true
			sourceID := target.Source
			if sourceID == "" {
				sourceID = "wow-ui-source"
			}
			sel, selectErr := selectSnapshot(sourceID, target.Product, target.Ref)
			if selectErr != nil {
				return selectErr
			}
			value, validateErr := validator.Validate(validator.Options{
				ID: target.ID, Path: addonPath, TOC: target.TOC, SourceID: sel.source.ID, Product: sel.product.ID,
				RequestedRef: target.Ref, MatchedTag: sel.ctx.MatchedTag, ResolvedCommit: sel.ctx.Commit,
				Evidence: snapshotEvidence{layout: sel.layout, ctx: sel.ctx},
			})
			sel.cat.Close()
			if validateErr != nil {
				return validateErr
			}
			targets = append(targets, value)
		}
		return result.Write(cmd.OutOrStdout(), validator.MergeMatrix(addonPath, targets))
	}}
	cmd.Flags().StringVar(&configPath, "config", "", "matrix configuration JSON")
	return cmd
}

func readMatrixConfig(configPath string) (validator.MatrixConfig, string, error) {
	data, err := os.ReadFile(configPath)
	if err != nil {
		return validator.MatrixConfig{}, "", err
	}
	var config validator.MatrixConfig
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if err = decoder.Decode(&config); err != nil {
		return validator.MatrixConfig{}, "", result.E("invalid_matrix_config", "cannot parse matrix config: "+err.Error(), 2)
	}
	if len(config.Targets) == 0 {
		return validator.MatrixConfig{}, "", result.E("invalid_matrix_config", "matrix targets must not be empty", 2)
	}
	base, err := filepath.Abs(filepath.Dir(configPath))
	if err != nil {
		return validator.MatrixConfig{}, "", err
	}
	addonPath := config.Path
	if addonPath == "" {
		addonPath = "."
	}
	if !filepath.IsAbs(addonPath) {
		addonPath = filepath.Join(base, addonPath)
	}
	addonPath, err = filepath.Abs(addonPath)
	if err != nil {
		return validator.MatrixConfig{}, "", err
	}
	return config, filepath.Clean(addonPath), nil
}

type snapshotEvidence struct {
	layout home.Layout
	ctx    query.Context
}

func (lookup snapshotEvidence) Lookup(usages []validator.Usage, interfaceValue string) ([]validator.CompatibilityFact, []validator.Unresolved, []validator.Diagnostic, error) {
	queryUsages := make([]query.CompatibilityUsage, 0, len(usages))
	for _, usage := range usages {
		queryUsages = append(queryUsages, query.CompatibilityUsage{Kind: usage.Kind, Name: usage.Name, Expression: usage.Expression, File: usage.File, Line: usage.Line, Column: usage.Column})
	}
	facts, unresolved, diagnostics, err := query.LookupCompatibility(lookup.layout, lookup.ctx, queryUsages, interfaceValue)
	if err != nil {
		return nil, nil, nil, err
	}
	outFacts := make([]validator.CompatibilityFact, 0, len(facts))
	outUnresolved := make([]validator.Unresolved, 0, len(unresolved))
	outDiagnostics := make([]validator.Diagnostic, 0, len(diagnostics))
	for _, fact := range facts {
		evidence := validator.Evidence(fact.Evidence)
		outFacts = append(outFacts, validator.CompatibilityFact{Kind: fact.Kind, Name: fact.Name, Exists: fact.Exists, Signature: fact.Signature, Evidence: evidence})
		if !fact.Exists {
			code, message := compatibilityMissing(fact.Kind, fact.Name)
			outDiagnostics = append(outDiagnostics, validator.Diagnostic{Severity: "error", Code: code, File: usageFile(evidence), Line: usageLine(evidence), Column: usageColumn(evidence), Message: message, Evidence: evidence})
		}
	}
	for _, item := range unresolved {
		outUnresolved = append(outUnresolved, validator.Unresolved{Kind: item.Kind, Expression: item.Expression, File: item.File, Line: item.Line, Column: item.Column, Reason: item.Reason, Evidence: validator.Evidence(item.Evidence)})
	}
	for _, item := range diagnostics {
		outDiagnostics = append(outDiagnostics, validator.Diagnostic{Severity: item.Severity, Code: item.Code, File: item.File, Line: item.Line, Column: item.Column, Message: item.Message, Evidence: validator.Evidence(item.Evidence)})
	}
	return outFacts, outUnresolved, outDiagnostics, nil
}

func compatibilityMissing(kind, name string) (string, string) {
	switch kind {
	case "api":
		return "api_not_found", fmt.Sprintf("API %s does not exist in the target snapshot", name)
	case "event":
		return "event_not_found", fmt.Sprintf("event %s does not exist in the target snapshot", name)
	case "template":
		return "xml_template_not_found", fmt.Sprintf("XML template %s does not exist in the target snapshot", name)
	case "interface":
		return "toc_interface_mismatch", fmt.Sprintf("TOC Interface %s does not match the target snapshot", name)
	default:
		return "compatibility_reference_not_found", fmt.Sprintf("%s %s does not exist in the target snapshot", kind, name)
	}
}

func usageEvidence(evidence validator.Evidence) map[string]any {
	value, _ := evidence["usage"].(map[string]any)
	return value
}
func usageFile(evidence validator.Evidence) string {
	value, _ := usageEvidence(evidence)["file"].(string)
	return value
}
func usageLine(evidence validator.Evidence) int {
	return numericEvidence(usageEvidence(evidence)["line"])
}
func usageColumn(evidence validator.Evidence) int {
	return numericEvidence(usageEvidence(evidence)["column"])
}
func numericEvidence(value any) int {
	switch number := value.(type) {
	case int:
		return number
	case int64:
		return int(number)
	case float64:
		return int(number)
	}
	return 0
}
