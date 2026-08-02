package indexer_test

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/follenfang/wowdoc/internal/home"
	"github.com/follenfang/wowdoc/internal/indexer"
	"github.com/follenfang/wowdoc/internal/query"
)

func TestBuildReusesObjectsAndReturnsExactSourceEvidence(t *testing.T) {
	root := t.TempDir()
	t.Setenv("WOWDOC_HOME", root)
	layout, err := home.Resolve()
	if err != nil {
		t.Fatal(err)
	}
	if err = layout.Ensure(); err != nil {
		t.Fatal(err)
	}
	fixture := filepath.Join("..", "..", "testdata", "sources", "valid-retail")
	commit := "1111111111111111111111111111111111111111"
	opts := indexer.BuildOptions{Layout: layout, SourceID: "wow-ui-source", ProductID: "retail", Commit: commit, RequestedRef: "fixture", Input: indexer.DirectoryInput{Root: fixture}, Workers: 4}
	first, err := indexer.Build(context.Background(), opts)
	if err != nil {
		t.Fatal(err)
	}
	if first.ParsedLua != 2 {
		t.Fatalf("parsedLua=%d", first.ParsedLua)
	}
	opts.Input = failingInput{}
	second, err := indexer.Build(context.Background(), opts)
	if err != nil {
		t.Fatal(err)
	}
	if second.ReusedObjects != second.Files {
		t.Fatalf("reused objects=%d files=%d", second.ReusedObjects, second.Files)
	}
	if second.ReusedAST < 2 {
		t.Fatalf("reused AST=%d", second.ReusedAST)
	}
	response, err := query.Search(layout, query.Context{SourceID: "wow-ui-source", ProductID: "retail", RequestedRef: "fixture", Commit: commit, SnapshotID: first.SnapshotID}, "C_AuctionHouse.GetItemSearchResultInfo", "api", 3)
	if err != nil {
		t.Fatal(err)
	}
	if len(response.Results) == 0 {
		t.Fatal("no results")
	}
	got := response.Results[0]
	if got.MatchedBy != "exact_symbol" || got.Line != 16 {
		t.Fatalf("unexpected top result: %#v", got)
	}
	if got.ContentHash == "" || got.Excerpt == "" {
		t.Fatalf("missing evidence: %#v", got)
	}
	if _, err := os.Stat(filepath.Join(layout.Objects, got.ContentHash[:2], got.ContentHash)); err != nil {
		t.Fatal(err)
	}
}

type failingInput struct{}

func (failingInput) Entries(context.Context) ([]indexer.Entry, error) {
	return nil, errors.New("ready snapshot unexpectedly read its input")
}
func (failingInput) Read(context.Context, indexer.Entry) ([]byte, error) {
	return nil, errors.New("ready snapshot unexpectedly read its input")
}
func (failingInput) ReadRaw(context.Context, indexer.Entry) ([]byte, error) {
	return nil, errors.New("ready snapshot unexpectedly read its input")
}

func TestXMLInheritanceAndMixinRelations(t *testing.T) {
	t.Setenv("WOWDOC_HOME", t.TempDir())
	layout, _ := home.Resolve()
	if err := layout.Ensure(); err != nil {
		t.Fatal(err)
	}
	fixture := filepath.Join("..", "..", "testdata", "sources", "partial-classic")
	commit := "2222222222222222222222222222222222222222"
	stats, err := indexer.Build(context.Background(), indexer.BuildOptions{Layout: layout, SourceID: "wow-ui-source", ProductID: "classic", Commit: commit, Input: indexer.DirectoryInput{Root: fixture}, Workers: 4})
	if err != nil {
		t.Fatal(err)
	}
	response, err := query.Search(layout, query.Context{SourceID: "wow-ui-source", ProductID: "classic", Commit: commit, SnapshotID: stats.SnapshotID}, "TestAddonMixin", "lua", 5)
	if err != nil {
		t.Fatal(err)
	}
	if len(response.Results) == 0 || response.Results[0].Kind != "mixin" {
		t.Fatalf("unexpected results: %#v", response.Results)
	}
	if len(response.Relations) == 0 {
		t.Fatal("expected inheritance relations")
	}
}
