package query_test

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/follenfang/wowdoc/internal/home"
	"github.com/follenfang/wowdoc/internal/indexer"
	"github.com/follenfang/wowdoc/internal/query"
)

func TestLookupCompatibilityReturnsSnapshotEvidenceAndSignatures(t *testing.T) {
	layout, fixture := compatibilityFixture(t)
	writeCompatibilityFixture(t, fixture, "KnownAPI", "KNOWN_EVENT")
	stats, err := indexer.Build(context.Background(), indexer.BuildOptions{
		Layout: layout, SourceID: "wow-ui-source", ProductID: "retail",
		Commit: "1111111111111111111111111111111111111111", RequestedRef: "12.1.0",
		Input: indexer.DirectoryInput{Root: fixture}, Workers: 2,
	})
	if err != nil {
		t.Fatal(err)
	}
	ctx := query.Context{
		SourceID: "wow-ui-source", ProductID: "retail", RequestedRef: "12.1.0",
		MatchedTag: "12.1.0", Commit: stats.Commit, SnapshotID: stats.SnapshotID, DBPath: stats.DBPath,
	}
	usages := []query.CompatibilityUsage{
		{Kind: "api", Name: "C_Test.KnownAPI", File: "Addon.lua", Line: 1, Column: 1},
		{Kind: "event", Name: "KNOWN_EVENT", File: "Addon.lua", Line: 2, Column: 2},
		{Kind: "template", Name: "KnownTemplate", File: "Addon.xml", Line: 3, Column: 3},
		{Kind: "mixin", Name: "KnownMixin", File: "Addon.lua", Line: 4, Column: 4},
		{Kind: "frame-type", Name: "Button", File: "Addon.xml", Line: 5, Column: 5},
		{Kind: "api", Name: "C_Test.MissingAPI", File: "Addon.lua", Line: 6, Column: 6},
		{Kind: "event", Name: "MISSING_EVENT", File: "Addon.lua", Line: 7, Column: 7},
		{Kind: "template", Name: "MissingTemplate", File: "Addon.xml", Line: 8, Column: 8},
		{Kind: "mixin", Name: "MissingMixin", File: "Addon.lua", Line: 9, Column: 9},
		{Kind: "frame-type", Name: "MissingFrame", File: "Addon.xml", Line: 10, Column: 10},
		{Kind: "api", Expression: "C_Test[name]", File: "Addon.lua", Line: 11, Column: 11},
		{Kind: "api-candidate", Name: "AddonGlobal", File: "Addon.lua", Line: 12, Column: 12},
	}
	facts, unresolved, diagnostics, err := query.LookupCompatibility(layout, ctx, usages, "120100")
	if err != nil {
		t.Fatal(err)
	}
	if diagnostics == nil || len(diagnostics) != 0 {
		t.Fatalf("diagnostics=%#v, want stable empty slice", diagnostics)
	}
	byKindAndName := make(map[string]query.CompatibilityFact)
	for _, fact := range facts {
		byKindAndName[fact.Kind+":"+fact.Name] = fact
	}
	for _, key := range []string{"api:C_Test.KnownAPI", "event:KNOWN_EVENT", "template:KnownTemplate", "mixin:KnownMixin", "frame-type:Button", "interface:120100"} {
		if fact, ok := byKindAndName[key]; !ok || !fact.Exists {
			t.Fatalf("fact %s=%#v, want exists", key, fact)
		}
	}
	for _, key := range []string{"api:C_Test.MissingAPI", "event:MISSING_EVENT", "template:MissingTemplate"} {
		if fact, ok := byKindAndName[key]; !ok || fact.Exists {
			t.Fatalf("fact %s=%#v, want proven absent", key, fact)
		}
	}
	if got := byKindAndName["api:C_Test.KnownAPI"].Signature; got != "C_Test.KnownAPI(value: number) -> result?: string" {
		t.Fatalf("API signature=%q", got)
	}
	if got := byKindAndName["event:KNOWN_EVENT"].Signature; got != "C_Test.KNOWN_EVENT(payload: string)" {
		t.Fatalf("event signature=%q", got)
	}
	if evidence := byKindAndName["api:C_Test.KnownAPI"].Evidence; evidence["snapshotId"] != stats.SnapshotID || evidence["matchedTag"] != "12.1.0" || evidence["resolvedCommit"] != stats.Commit {
		t.Fatalf("incomplete immutable evidence: %#v", evidence)
	}
	if len(unresolved) != 4 {
		t.Fatalf("unresolved=%#v, want missing mixin, missing frame type, dynamic API, and AddOn global", unresolved)
	}
}

func TestLookupCompatibilityFiltersSharedFactsBySnapshotMembership(t *testing.T) {
	layout, fixture := compatibilityFixture(t)
	writeCompatibilityFixture(t, fixture, "FirstAPI", "FIRST_EVENT")
	first, err := indexer.Build(context.Background(), indexer.BuildOptions{
		Layout: layout, SourceID: "wow-ui-source", ProductID: "retail",
		Commit: "2222222222222222222222222222222222222222", RequestedRef: "first",
		Input: indexer.DirectoryInput{Root: fixture}, Workers: 2,
	})
	if err != nil {
		t.Fatal(err)
	}
	writeCompatibilityFixture(t, fixture, "SecondAPI", "SECOND_EVENT")
	second, err := indexer.Build(context.Background(), indexer.BuildOptions{
		Layout: layout, SourceID: "wow-ui-source", ProductID: "retail",
		Commit: "3333333333333333333333333333333333333333", RequestedRef: "second",
		Input: indexer.DirectoryInput{Root: fixture}, Workers: 2,
	})
	if err != nil {
		t.Fatal(err)
	}

	firstContext := query.Context{SourceID: "wow-ui-source", ProductID: "retail", Commit: first.Commit, SnapshotID: first.SnapshotID, DBPath: first.DBPath}
	facts, unresolved, _, err := query.LookupCompatibility(layout, firstContext, []query.CompatibilityUsage{{Kind: "api", Name: "C_Test.SecondAPI"}}, "")
	if err != nil {
		t.Fatal(err)
	}
	if len(unresolved) != 0 || len(facts) != 1 || facts[0].Exists {
		t.Fatalf("first snapshot leaked second snapshot fact: facts=%#v unresolved=%#v", facts, unresolved)
	}

	secondContext := query.Context{SourceID: "wow-ui-source", ProductID: "retail", Commit: second.Commit, SnapshotID: second.SnapshotID, DBPath: second.DBPath}
	facts, unresolved, _, err = query.LookupCompatibility(layout, secondContext, []query.CompatibilityUsage{{Kind: "api", Name: "C_Test.SecondAPI"}}, "")
	if err != nil {
		t.Fatal(err)
	}
	if len(unresolved) != 0 || len(facts) != 1 || !facts[0].Exists {
		t.Fatalf("second snapshot did not resolve its fact: facts=%#v unresolved=%#v", facts, unresolved)
	}
}

func compatibilityFixture(t *testing.T) (home.Layout, string) {
	t.Helper()
	t.Setenv("WOWDOC_HOME", t.TempDir())
	layout, err := home.Resolve()
	if err != nil {
		t.Fatal(err)
	}
	if err = layout.Ensure(); err != nil {
		t.Fatal(err)
	}
	return layout, t.TempDir()
}

func writeCompatibilityFixture(t *testing.T, root, apiName, eventName string) {
	t.Helper()
	generatedDir := filepath.Join(root, "Interface", "AddOns", "Blizzard_APIDocumentationGenerated")
	if err := os.MkdirAll(generatedDir, 0o755); err != nil {
		t.Fatal(err)
	}
	generated := `APIDocumentation:AddDocumentationTable({
	Name = "C_Test",
	Type = "System",
	Functions = {
		{
			Name = "` + apiName + `",
			Type = "Function",
			Arguments = {
				{Name = "value", Type = "number", Nilable = false},
			},
			Returns = {
				{Name = "result", Type = "string", Nilable = true},
			},
		},
	},
	Events = {
		{
			Name = "` + eventName + `",
			Type = "Event",
			Payload = {
				{Name = "payload", Type = "string", Nilable = false},
			},
		},
	},
})
`
	files := map[string]string{
		filepath.Join(generatedDir, "GeneratedDocumentation.lua"): generated,
		filepath.Join(root, "Mixins.lua"):                         "KnownMixin = CreateFromMixins(BaseMixin)\n",
		filepath.Join(root, "Templates.xml"):                      "<Ui>\n  <Frame name=\"KnownTemplate\" virtual=\"true\"/>\n  <Button name=\"KnownButton\"/>\n</Ui>\n",
		filepath.Join(root, "Addon.toc"):                          "## Interface: 120100\nMixins.lua\nTemplates.xml\n",
	}
	for path, data := range files {
		if err := os.WriteFile(path, []byte(data), 0o644); err != nil {
			t.Fatal(err)
		}
	}
}
