package analyze

import (
	"bytes"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"wowdoc/internal/shared/contracts"
)

var versionPattern = regexp.MustCompile(`\d+\.\d+\.\d+\.\d+`)

type Capabilities struct {
	APIDocumentation bool `json:"apiDocumentation"`
	FrameXML         bool `json:"frameXML"`
	WidgetDocs       bool `json:"widgetDocs"`
	Constants        bool `json:"constants"`
	Mixins           bool `json:"mixins"`
	CVars            bool `json:"cvars"`
}

type Repository struct {
	Alias        string                 `json:"alias"`
	Path         string                 `json:"path"`
	Version      string                 `json:"version"`
	RequestedRef string                 `json:"requestedRef,omitempty"`
	ResolvedRef  string                 `json:"resolvedRef,omitempty"`
	Valid        bool                   `json:"valid"`
	Capabilities Capabilities           `json:"capabilities"`
	Diagnostics  []contracts.Diagnostic `json:"diagnostics"`
}

func DetectRepository(path, alias string) Repository {
	repo := Repository{Alias: alias, Path: path}
	missing := make([]string, 0)
	if !exists(filepath.Join(path, "Interface")) {
		missing = append(missing, "Interface/")
	}
	if !hasAny(path,
		"Interface/ui-code-list.txt",
		"Interface/ui-toc-list.txt",
		"Interface/ui-gen-addon-list.txt",
		"Interface/AddOns",
	) {
		missing = append(missing, "source-list-or-addons")
	}
	versionPath := filepath.Join(path, "version.txt")
	if !exists(versionPath) {
		repo.Version = inferRepositoryVersion(path)
		if repo.Version == "" {
			missing = append(missing, "version.txt")
		}
	} else if b, err := os.ReadFile(versionPath); err == nil {
		repo.Version = stringTrim(b)
	}
	if len(missing) > 0 {
		repo.Diagnostics = append(repo.Diagnostics, contracts.Diagnostic{
			Path: path, Message: "source_invalid", Missing: missing,
		})
		return repo
	}
	repo.Valid = true
	addons := filepath.Join(path, "Interface", "AddOns")
	apiDocs := filepath.Join(addons, "Blizzard_APIDocumentationGenerated")
	repo.Capabilities.APIDocumentation = exists(apiDocs)
	repo.Capabilities.WidgetDocs = hasFileWithExt(apiDocs, ".lua")
	repo.Capabilities.Constants = hasContentSignal(apiDocs, "Enumeration", "Enum.", "Constants", "ConstantsDocumentation")
	repo.Capabilities.FrameXML, repo.Capabilities.Mixins, repo.Capabilities.CVars = scanAddOnCapabilities(addons)
	return repo
}

func inferRepositoryVersion(path string) string {
	for _, name := range []string{"build.info", ".build.info", "build.txt", "metadata.txt", ".metadata"} {
		b, err := os.ReadFile(filepath.Join(path, name))
		if err != nil {
			continue
		}
		if match := versionPattern.Find(b); len(match) > 0 {
			return string(match)
		}
	}
	return ""
}

func hasAny(root string, rels ...string) bool {
	for _, rel := range rels {
		if exists(filepath.Join(root, rel)) {
			return true
		}
	}
	return false
}

func exists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

func stringTrim(b []byte) string {
	s := string(b)
	for len(s) > 0 {
		last := s[len(s)-1]
		if last != '\n' && last != '\r' && last != ' ' && last != '\t' {
			break
		}
		s = s[:len(s)-1]
	}
	return s
}

func hasFileWithExt(root, ext string) bool {
	found := false
	_ = filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
		if err != nil || d.IsDir() || found {
			return nil
		}
		if strings.EqualFold(filepath.Ext(path), ext) {
			found = true
		}
		return nil
	})
	return found
}

func scanAddOnCapabilities(root string) (frameXML, mixins, cvars bool) {
	_ = filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return nil
		}
		ext := strings.ToLower(filepath.Ext(path))
		if ext != ".lua" && ext != ".xml" {
			return nil
		}
		if isFrameXMLSignal(path) {
			frameXML = true
		}
		if frameXML && mixins && cvars {
			return filepath.SkipAll
		}
		b, err := os.ReadFile(path)
		if err != nil {
			return nil
		}
		if bytes.Contains(b, []byte("Mixin")) || bytes.Contains(b, []byte("Template")) {
			mixins = true
		}
		if bytes.Contains(b, []byte("C_CVar")) || bytes.Contains(b, []byte("CVar")) {
			cvars = true
		}
		if frameXML && mixins && cvars {
			return filepath.SkipAll
		}
		return nil
	})
	return frameXML, mixins, cvars
}

func isFrameXMLSignal(path string) bool {
	normalized := filepath.ToSlash(path)
	return strings.Contains(normalized, "Blizzard_FrameXML/") ||
		strings.Contains(normalized, "FrameXML") ||
		strings.Contains(filepath.Base(path), "FrameXML")
}

func hasContentSignal(root string, needles ...string) bool {
	found := false
	_ = filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
		if err != nil || d.IsDir() || found {
			return nil
		}
		ext := strings.ToLower(filepath.Ext(path))
		if ext != ".lua" && ext != ".xml" {
			return nil
		}
		b, readErr := os.ReadFile(path)
		if readErr != nil {
			return nil
		}
		for _, needle := range needles {
			if bytes.Contains(b, []byte(needle)) || strings.Contains(filepath.Base(path), needle) {
				found = true
				return filepath.SkipAll
			}
		}
		return nil
	})
	return found
}
