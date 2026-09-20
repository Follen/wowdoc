package query_test

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/follenfang/wowdoc/internal/indexer"
	"github.com/follenfang/wowdoc/internal/query"
)

func TestSearchPrecisionAndExplore(t *testing.T) {
	layout, root := compatibilityFixture(t)
	writeCompatibilityFixture(t, root, "KnownAPI", "KNOWN_EVENT")
	generatedPath := filepath.Join(root, "Interface", "AddOns", "Blizzard_APIDocumentationGenerated", "GeneratedDocumentation.lua")
	generated, err := os.ReadFile(generatedPath)
	if err != nil {
		t.Fatal(err)
	}
	generated = []byte(strings.ReplaceAll(string(generated), `Name = "KNOWN_EVENT",`, "Name = \"KnownEvent\",\n LiteralName = \"KNOWN_EVENT\","))
	if err = os.WriteFile(generatedPath, generated, 0644); err != nil {
		t.Fatal(err)
	}
	files := map[string]string{
		"A.lua":               "function Needle() end\nfunction NeedleMore() end\nlocal x = 'scarlet'\n",
		"B.lua":               "local x = 'scarlet orchid'\n",
		"C.lua":               "local x = 'orchid'\n",
		"Topic.xml":           "<Ui><Frame name=\"Needle\"/></Ui>",
		"Vendor/First.lua":    "function Ranked() end\n",
		"ZProject.lua":        "function Ranked() end\nfunction LiteralXName() end\n",
		"Literal.lua":         "function Literal_Name() end\n",
		"Media/Icon_name.tga": "fixture image bytes",
		"Media/IconXname.tga": "other fixture image bytes",
		"Together.lua":        "function GroupOne() end; function GroupTwo() end\n",
	}
	for i := 0; i < 15; i++ {
		files[fmt.Sprintf("Noise%02d.lua", i)] = "function KnownAPI() end\n"
	}
	for name, data := range files {
		path := filepath.Join(root, name)
		if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(data), 0644); err != nil {
			t.Fatal(err)
		}
	}
	stats, err := indexer.Build(context.Background(), indexer.BuildOptions{Layout: layout, SourceID: "wow-ui-source", ProductID: "retail", Commit: "7777777777777777777777777777777777777777", Input: indexer.DirectoryInput{Root: root}, Workers: 2})
	if err != nil {
		t.Fatal(err)
	}
	ctx := query.Context{SourceID: "wow-ui-source", ProductID: "retail", Commit: stats.Commit, SnapshotID: stats.SnapshotID, DBPath: stats.DBPath}
	search := func(text, topic string, limit int) query.Response {
		t.Helper()
		r, e := query.Search(layout, ctx, text, topic, limit)
		if e != nil {
			t.Fatal(e)
		}
		return r
	}
	t.Run("literal event and documentation alias", func(t *testing.T) {
		for _, name := range []string{"KNOWN_EVENT", "KnownEvent", "C_Test.KnownEvent"} {
			r := search(name, "api", 10)
			if len(r.Results) != 1 || r.Results[0].Kind != "api-event" {
				t.Fatalf("%s: %+v", name, r.Results)
			}
		}
		facts, unresolved, _, e := query.LookupCompatibility(layout, ctx, []query.CompatibilityUsage{{Kind: "event", Name: "KNOWN_EVENT"}}, "")
		if e != nil || len(unresolved) != 0 || len(facts) != 1 || !facts[0].Exists || facts[0].Signature != "KNOWN_EVENT(payload: string)" {
			t.Fatalf("facts=%+v unresolved=%+v err=%v", facts, unresolved, e)
		}
	})
	t.Run("exact does not fill quota", func(t *testing.T) {
		r := search("Needle", "lua", 10)
		if len(r.Results) != 1 || r.Results[0].Name != "Needle" {
			t.Fatalf("results=%+v", r.Results)
		}
		b, e := query.Explore(layout, ctx, "Needle", "lua", 10)
		if e != nil {
			t.Fatal(e)
		}
		if len(b.Results) <= len(r.Results) {
			t.Fatalf("explore=%+v", b.Results)
		}
	})
	t.Run("topic before limit", func(t *testing.T) {
		r := search("KnownAPI", "api", 1)
		if len(r.Results) != 1 || r.Results[0].Kind != "api-function" {
			t.Fatalf("results=%+v", r.Results)
		}
		r = search("Needle", "xml", 1)
		if len(r.Results) != 1 || r.Results[0].Path != "Topic.xml" {
			t.Fatalf("results=%+v", r.Results)
		}
	})
	t.Run("all words required", func(t *testing.T) {
		r := search("scarlet orchid", "lua", 10)
		if len(r.Results) != 1 || r.Results[0].Path != "B.lua" {
			t.Fatalf("results=%+v", r.Results)
		}
	})
	t.Run("rank before limit", func(t *testing.T) {
		r := search("Ranked", "lua", 1)
		if len(r.Results) != 1 || r.Results[0].Path != "ZProject.lua" {
			t.Fatalf("results=%+v", r.Results)
		}
	})
	t.Run("literal prefix", func(t *testing.T) {
		r := search("Literal_", "lua", 10)
		if len(r.Results) != 1 || r.Results[0].Name != "Literal_Name" {
			t.Fatalf("results=%+v", r.Results)
		}
	})
	t.Run("distinct definitions on one line", func(t *testing.T) {
		r := search("Group", "lua", 10)
		if len(r.Results) != 2 {
			t.Fatalf("results=%+v", r.Results)
		}
	})
	t.Run("invalid topic", func(t *testing.T) {
		if _, e := query.Search(layout, ctx, "Needle", "typo", 1); e == nil {
			t.Fatal("expected topic error")
		}
	})
	t.Run("asset path separators and literal underscore", func(t *testing.T) {
		for _, name := range []string{"Media/Icon_name.tga", `Media\Icon_name.tga`, "Icon_"} {
			r := search(name, "asset", 10)
			if len(r.Results) != 1 || r.Results[0].Path != "Media/Icon_name.tga" || r.Results[0].ContentHash == "" {
				t.Fatalf("%s: %+v", name, r.Results)
			}
		}
	})
}

func TestCompatibilityRepeatedLocationsAndEvidenceSize(t *testing.T) {
	layout, root := compatibilityFixture(t)
	writeCompatibilityFixture(t, root, "KnownAPI", "KNOWN_EVENT")
	// A multi-file snapshot reproduces the harmful XML join shape and many
	// matching nodes reproduce the old per-call evidence explosion.
	for i := 0; i < 100; i++ {
		data := fmt.Sprintf("<Ui><Frame name=\"Frame%d\"/></Ui>", i)
		if err := os.WriteFile(filepath.Join(root, fmt.Sprintf("Frame%d.xml", i)), []byte(data), 0644); err != nil {
			t.Fatal(err)
		}
	}
	stats, err := indexer.Build(context.Background(), indexer.BuildOptions{Layout: layout, SourceID: "wow-ui-source", ProductID: "retail", Commit: "8888888888888888888888888888888888888888", Input: indexer.DirectoryInput{Root: root}, Workers: 2})
	if err != nil {
		t.Fatal(err)
	}
	ctx := query.Context{SourceID: "wow-ui-source", ProductID: "retail", Commit: stats.Commit, SnapshotID: stats.SnapshotID, DBPath: stats.DBPath}
	var usages []query.CompatibilityUsage
	for i := 1; i <= 100; i++ {
		usages = append(usages, query.CompatibilityUsage{Kind: "frame-type", Name: "Frame", File: "Addon.lua", Line: i})
	}
	facts, unresolved, _, err := query.LookupCompatibility(layout, ctx, usages, "")
	if err != nil {
		t.Fatal(err)
	}
	if len(facts) != 100 || len(unresolved) != 0 {
		t.Fatalf("facts=%d unresolved=%d", len(facts), len(unresolved))
	}
	for i, fact := range facts {
		if !fact.Exists || fact.Evidence["usage"].(map[string]any)["line"] != i+1 {
			t.Fatalf("location lost: %+v", fact)
		}
		if fact.Evidence["matchCount"] != 101 || fact.Evidence["matchesTruncated"] != true || len(fact.Evidence["matches"].([]map[string]any)) != 5 {
			t.Fatalf("unbounded evidence: %+v", fact.Evidence)
		}
	}
}
