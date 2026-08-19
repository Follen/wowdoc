package validator

import (
	"fmt"
	"sort"
	"strings"
)

// MergeMatrix combines independently validated targets without changing their
// order. All summary collections are initialized so the result has a stable
// JSON shape, including when no targets are supplied.
func MergeMatrix(path string, targets []Result) MatrixResult {
	result := MatrixResult{
		Path:    path,
		Valid:   true,
		Targets: append([]Result{}, targets...),
		Summary: newMatrixSummary(),
	}

	for _, target := range targets {
		if !target.Valid {
			result.Valid = false
		}
		result.Summary.Unresolved[target.ID] = sortedUnresolved(target.Unresolved)
		result.Summary.Interfaces[target.ID] = interfaceSummary(target)
	}

	result.Summary.APIs = mergeFacts(targets, map[string]bool{"api": true})
	result.Summary.Events = mergeFacts(targets, map[string]bool{"event": true})
	result.Summary.XML = mergeFacts(targets, map[string]bool{
		"template":   true,
		"mixin":      true,
		"frame-type": true,
	})
	result.Summary.SharedFiles, result.Summary.TargetOnlyFiles = mergeFiles(targets)
	result.Summary.SharedDiagnostics, result.Summary.TargetOnlyDiagnostics = mergeDiagnostics(targets)

	return result
}

func newMatrixSummary() MatrixSummary {
	return MatrixSummary{
		APIs:                  FactSummary{Shared: []string{}, Differences: []FactDifference{}},
		Events:                FactSummary{Shared: []string{}, Differences: []FactDifference{}},
		XML:                   FactSummary{Shared: []string{}, Differences: []FactDifference{}},
		Interfaces:            map[string]any{},
		SharedFiles:           []string{},
		TargetOnlyFiles:       map[string][]string{},
		SharedDiagnostics:     []Diagnostic{},
		TargetOnlyDiagnostics: map[string][]Diagnostic{},
		Unresolved:            map[string][]Unresolved{},
	}
}

func mergeFacts(targets []Result, includedKinds map[string]bool) FactSummary {
	summary := FactSummary{Shared: []string{}, Differences: []FactDifference{}}
	if len(targets) == 0 {
		return summary
	}

	// A target can contain the same fact for several usage sites. Treat those
	// as one value while retaining genuine per-target signature differences.
	valuesByName := map[string]map[string]FactValue{}
	for _, target := range targets {
		for _, fact := range target.Facts {
			kind := strings.ToLower(fact.Kind)
			if !includedKinds[kind] {
				continue
			}
			if valuesByName[fact.Name] == nil {
				valuesByName[fact.Name] = map[string]FactValue{}
			}
			valuesByName[fact.Name][target.ID] = FactValue{
				Exists:    fact.Exists,
				Signature: fact.Signature,
			}
		}
	}

	names := matrixSortedKeys(valuesByName)
	for _, name := range names {
		targetValues := make(map[string]FactValue, len(targets))
		var sharedValue FactValue
		shared := true
		for i, target := range targets {
			value, observed := valuesByName[name][target.ID]
			targetValues[target.ID] = value
			if i == 0 {
				sharedValue = value
			} else if value != sharedValue {
				shared = false
			}
			if !observed {
				shared = false
			}
		}

		// A fact must be observed in every target to be considered shared.
		if shared {
			summary.Shared = append(summary.Shared, name)
			continue
		}
		summary.Differences = append(summary.Differences, FactDifference{
			Name:    name,
			Targets: targetValues,
		})
	}

	return summary
}

func mergeFiles(targets []Result) ([]string, map[string][]string) {
	shared := []string{}
	targetOnly := map[string][]string{}
	if len(targets) == 0 {
		return shared, targetOnly
	}

	filesByTarget := make(map[string]map[string]struct{}, len(targets))
	allFiles := map[string]struct{}{}
	for _, target := range targets {
		files := map[string]struct{}{}
		for _, file := range target.LoadClosure {
			files[file.Path] = struct{}{}
			allFiles[file.Path] = struct{}{}
		}
		filesByTarget[target.ID] = files
		targetOnly[target.ID] = []string{}
	}

	for _, path := range matrixSortedKeys(allFiles) {
		inAll := true
		for _, target := range targets {
			if _, ok := filesByTarget[target.ID][path]; !ok {
				inAll = false
				break
			}
		}
		if inAll {
			shared = append(shared, path)
			continue
		}
		for _, target := range targets {
			if _, ok := filesByTarget[target.ID][path]; ok {
				targetOnly[target.ID] = append(targetOnly[target.ID], path)
			}
		}
	}

	return shared, targetOnly
}

func mergeDiagnostics(targets []Result) ([]Diagnostic, map[string][]Diagnostic) {
	shared := []Diagnostic{}
	targetOnly := map[string][]Diagnostic{}
	if len(targets) == 0 {
		return shared, targetOnly
	}

	diagnosticsByTarget := make(map[string]map[string]Diagnostic, len(targets))
	allKeys := map[string]struct{}{}
	for _, target := range targets {
		items := map[string]Diagnostic{}
		for _, diagnostic := range target.Diagnostics {
			key := diagnosticKey(diagnostic)
			if _, exists := items[key]; !exists {
				items[key] = diagnostic
			}
			allKeys[key] = struct{}{}
		}
		diagnosticsByTarget[target.ID] = items
		targetOnly[target.ID] = []Diagnostic{}
	}

	for _, key := range matrixSortedKeys(allKeys) {
		inAll := true
		var representative Diagnostic
		for i, target := range targets {
			diagnostic, ok := diagnosticsByTarget[target.ID][key]
			if !ok {
				inAll = false
				continue
			}
			if i == 0 {
				representative = diagnostic
			}
		}
		if inAll {
			shared = append(shared, representative)
			continue
		}
		for _, target := range targets {
			if diagnostic, ok := diagnosticsByTarget[target.ID][key]; ok {
				targetOnly[target.ID] = append(targetOnly[target.ID], diagnostic)
			}
		}
	}

	return shared, targetOnly
}

func diagnosticKey(d Diagnostic) string {
	return fmt.Sprintf("%s\x00%s\x00%s\x00%010d\x00%010d\x00%s",
		d.Severity, d.Code, d.File, d.Line, d.Column, d.Message)
}

func interfaceSummary(target Result) map[string]any {
	facts := make([]CompatibilityFact, 0)
	for _, fact := range target.Facts {
		if strings.Contains(strings.ToLower(fact.Kind), "interface") {
			facts = append(facts, fact)
		}
	}
	sort.Slice(facts, func(i, j int) bool {
		left, right := facts[i], facts[j]
		if left.Kind != right.Kind {
			return left.Kind < right.Kind
		}
		if left.Name != right.Name {
			return left.Name < right.Name
		}
		if left.Exists != right.Exists {
			return !left.Exists
		}
		return left.Signature < right.Signature
	})

	statuses := make([]Diagnostic, 0)
	for _, diagnostic := range target.Diagnostics {
		if strings.Contains(strings.ToLower(diagnostic.Code), "interface") {
			statuses = append(statuses, diagnostic)
		}
	}
	sort.Slice(statuses, func(i, j int) bool {
		return diagnosticKey(statuses[i]) < diagnosticKey(statuses[j])
	})

	return map[string]any{
		"declared":    target.Interface,
		"facts":       facts,
		"diagnostics": statuses,
	}
}

func sortedUnresolved(items []Unresolved) []Unresolved {
	result := append([]Unresolved{}, items...)
	sort.Slice(result, func(i, j int) bool {
		left, right := result[i], result[j]
		if left.Kind != right.Kind {
			return left.Kind < right.Kind
		}
		if left.Expression != right.Expression {
			return left.Expression < right.Expression
		}
		if left.File != right.File {
			return left.File < right.File
		}
		if left.Line != right.Line {
			return left.Line < right.Line
		}
		if left.Column != right.Column {
			return left.Column < right.Column
		}
		return left.Reason < right.Reason
	})
	return result
}

func matrixSortedKeys[V any](values map[string]V) []string {
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}
