package validator

import (
	"encoding/json"
	"reflect"
	"testing"
)

func TestMergeMatrixThreeTargets(t *testing.T) {
	sharedDiagnostic := Diagnostic{
		Severity: "warning", Code: "API_DEPRECATED", File: "Core.lua",
		Line: 8, Column: 3, Message: "old API",
	}
	pairDiagnostic := Diagnostic{
		Severity: "error", Code: "API_MISSING", File: "Feature.lua",
		Line: 4, Column: 2, Message: "missing API",
	}

	targets := []Result{
		{
			ID: "retail", Valid: true, Interface: "120100",
			LoadClosure: []LoadFile{
				{Path: "Retail.lua"}, {Path: "Shared.xml"}, {Path: "Core.lua"},
			},
			Facts: []CompatibilityFact{
				{Kind: "api", Name: "C_Common.Shared", Exists: true, Signature: "()"},
				{Kind: "api", Name: "C_Common.Shape", Exists: true, Signature: "(unit)"},
				{Kind: "api", Name: "C_Retail.Only", Exists: true, Signature: "()"},
				{Kind: "event", Name: "SHARED_EVENT", Exists: true, Signature: "(unit)"},
				{Kind: "event", Name: "CHANGED_EVENT", Exists: true, Signature: "(one)"},
				{Kind: "mixin", Name: "SharedMixin", Exists: true, Signature: "SharedMixin"},
				{Kind: "template", Name: "ChangedXML", Exists: true, Signature: "Button"},
				{Kind: "toc-interface", Name: "120100", Exists: true, Signature: "matched"},
			},
			Diagnostics: []Diagnostic{
				{Severity: "info", Code: "RETAIL_ONLY", File: "Retail.lua", Line: 1, Column: 1, Message: "retail"},
				pairDiagnostic,
				sharedDiagnostic,
			},
			Unresolved: []Unresolved{
				{Kind: "event", Expression: "z", File: "Retail.lua", Line: 9},
				{Kind: "api", Expression: "a", File: "Retail.lua", Line: 3},
			},
		},
		{
			ID: "classic", Valid: true, Interface: "50504",
			LoadClosure: []LoadFile{
				{Path: "Classic.lua"}, {Path: "Core.lua"}, {Path: "Shared.xml"},
			},
			Facts: []CompatibilityFact{
				{Kind: "api", Name: "C_Common.Shared", Exists: true, Signature: "()"},
				{Kind: "api", Name: "C_Common.Shape", Exists: true, Signature: "(unit, exact)"},
				{Kind: "event", Name: "SHARED_EVENT", Exists: true, Signature: "(unit)"},
				{Kind: "event", Name: "CHANGED_EVENT", Exists: false, Signature: ""},
				{Kind: "mixin", Name: "SharedMixin", Exists: true, Signature: "SharedMixin"},
				{Kind: "template", Name: "ChangedXML", Exists: true, Signature: "Frame"},
			},
			Diagnostics: []Diagnostic{sharedDiagnostic, pairDiagnostic},
			Unresolved: []Unresolved{
				{Kind: "template", Expression: "dynamicTemplate", File: "Classic.lua", Line: 7},
			},
		},
		{
			ID: "titan", Valid: false, Interface: "30802",
			LoadClosure: []LoadFile{
				{Path: "Shared.xml"}, {Path: "Titan.lua"}, {Path: "Core.lua"},
			},
			Facts: []CompatibilityFact{
				{Kind: "api", Name: "C_Common.Shared", Exists: true, Signature: "()"},
				{Kind: "api", Name: "C_Common.Shape", Exists: true, Signature: "(unit)"},
				{Kind: "event", Name: "SHARED_EVENT", Exists: true, Signature: "(unit)"},
				{Kind: "event", Name: "CHANGED_EVENT", Exists: true, Signature: "(one)"},
				{Kind: "mixin", Name: "SharedMixin", Exists: true, Signature: "SharedMixin"},
				{Kind: "template", Name: "ChangedXML", Exists: false, Signature: ""},
			},
			Diagnostics: []Diagnostic{sharedDiagnostic},
			Unresolved:  []Unresolved{},
		},
	}

	got := MergeMatrix(".", targets)

	if got.Valid {
		t.Fatal("matrix should be invalid when any target is invalid")
	}
	if got.Path != "." {
		t.Fatalf("path = %q, want .", got.Path)
	}
	if ids := []string{got.Targets[0].ID, got.Targets[1].ID, got.Targets[2].ID}; !reflect.DeepEqual(ids, []string{"retail", "classic", "titan"}) {
		t.Fatalf("target order = %v", ids)
	}

	assertStrings(t, got.Summary.APIs.Shared, []string{"C_Common.Shared"})
	assertDifferenceNames(t, got.Summary.APIs.Differences, []string{"C_Common.Shape", "C_Retail.Only"})
	shape := got.Summary.APIs.Differences[0].Targets
	if shape["retail"] == shape["classic"] || shape["retail"] != shape["titan"] {
		t.Fatalf("API signature difference not preserved: %#v", shape)
	}
	if only := got.Summary.APIs.Differences[1].Targets; !only["retail"].Exists || only["classic"].Exists || only["titan"].Exists {
		t.Fatalf("target-only API values = %#v", only)
	}

	assertStrings(t, got.Summary.Events.Shared, []string{"SHARED_EVENT"})
	assertDifferenceNames(t, got.Summary.Events.Differences, []string{"CHANGED_EVENT"})
	assertStrings(t, got.Summary.XML.Shared, []string{"SharedMixin"})
	assertDifferenceNames(t, got.Summary.XML.Differences, []string{"ChangedXML"})

	assertStrings(t, got.Summary.SharedFiles, []string{"Core.lua", "Shared.xml"})
	assertStrings(t, got.Summary.TargetOnlyFiles["retail"], []string{"Retail.lua"})
	assertStrings(t, got.Summary.TargetOnlyFiles["classic"], []string{"Classic.lua"})
	assertStrings(t, got.Summary.TargetOnlyFiles["titan"], []string{"Titan.lua"})

	if len(got.Summary.SharedDiagnostics) != 1 || diagnosticKey(got.Summary.SharedDiagnostics[0]) != diagnosticKey(sharedDiagnostic) {
		t.Fatalf("shared diagnostics = %#v", got.Summary.SharedDiagnostics)
	}
	if codes := diagnosticCodes(got.Summary.TargetOnlyDiagnostics["retail"]); !reflect.DeepEqual(codes, []string{"API_MISSING", "RETAIL_ONLY"}) {
		t.Fatalf("retail-only diagnostic codes = %v", codes)
	}
	if codes := diagnosticCodes(got.Summary.TargetOnlyDiagnostics["classic"]); !reflect.DeepEqual(codes, []string{"API_MISSING"}) {
		t.Fatalf("classic-only diagnostic codes = %v", codes)
	}
	if got.Summary.TargetOnlyDiagnostics["titan"] == nil || len(got.Summary.TargetOnlyDiagnostics["titan"]) != 0 {
		t.Fatalf("titan-only diagnostics = %#v", got.Summary.TargetOnlyDiagnostics["titan"])
	}

	if unresolved := got.Summary.Unresolved["retail"]; len(unresolved) != 2 || unresolved[0].Kind != "api" || unresolved[1].Kind != "event" {
		t.Fatalf("retail unresolved = %#v", unresolved)
	}
	if got.Summary.Unresolved["titan"] == nil {
		t.Fatal("empty target unresolved must be initialized")
	}

	retailInterface, ok := got.Summary.Interfaces["retail"].(map[string]any)
	if !ok {
		t.Fatalf("retail interface summary type = %T", got.Summary.Interfaces["retail"])
	}
	if retailInterface["declared"] != "120100" {
		t.Fatalf("retail declared interface = %#v", retailInterface["declared"])
	}
	if facts, ok := retailInterface["facts"].([]CompatibilityFact); !ok || len(facts) != 1 || facts[0].Kind != "toc-interface" {
		t.Fatalf("retail interface facts = %#v", retailInterface["facts"])
	}
	if diagnostics, ok := retailInterface["diagnostics"].([]Diagnostic); !ok || diagnostics == nil {
		t.Fatalf("retail interface diagnostics = %#v", retailInterface["diagnostics"])
	}
}

func TestMergeMatrixEmptyHasStableJSONShape(t *testing.T) {
	got := MergeMatrix("addon", nil)
	if !got.Valid {
		t.Fatal("an empty matrix should be vacuously valid")
	}

	raw, err := json.Marshal(got)
	if err != nil {
		t.Fatalf("marshal matrix: %v", err)
	}
	var decoded map[string]any
	if err := json.Unmarshal(raw, &decoded); err != nil {
		t.Fatalf("unmarshal matrix: %v", err)
	}
	assertJSONCollectionNotNil(t, decoded, "targets")
	summary := decoded["summary"].(map[string]any)
	for _, key := range []string{
		"interfaces", "sharedFiles", "targetOnlyFiles", "sharedDiagnostics",
		"targetOnlyDiagnostics", "unresolved",
	} {
		assertJSONCollectionNotNil(t, summary, key)
	}
	for _, key := range []string{"apis", "events", "xml"} {
		facts := summary[key].(map[string]any)
		assertJSONCollectionNotNil(t, facts, "shared")
		assertJSONCollectionNotNil(t, facts, "differences")
	}
}

func assertStrings(t *testing.T, got, want []string) {
	t.Helper()
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %v, want %v", got, want)
	}
}

func assertDifferenceNames(t *testing.T, got []FactDifference, want []string) {
	t.Helper()
	names := make([]string, len(got))
	for i, difference := range got {
		names[i] = difference.Name
	}
	assertStrings(t, names, want)
}

func diagnosticCodes(items []Diagnostic) []string {
	result := make([]string, len(items))
	for i, item := range items {
		result[i] = item.Code
	}
	return result
}

func assertJSONCollectionNotNil(t *testing.T, object map[string]any, key string) {
	t.Helper()
	if value, exists := object[key]; !exists || value == nil {
		t.Fatalf("JSON field %q is missing or null: %s", key, mustJSON(object))
	}
}

func mustJSON(value any) string {
	raw, _ := json.Marshal(value)
	return string(raw)
}
