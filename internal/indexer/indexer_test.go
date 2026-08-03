package indexer_test

import (
	"context"
	"database/sql"
	"errors"
	"os"
	"path/filepath"
	"sort"
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

func TestUnchangedContentReusesFactsAndFTSAcrossSnapshots(t *testing.T) {
	t.Setenv("WOWDOC_HOME", t.TempDir())
	layout, _ := home.Resolve()
	if err := layout.Ensure(); err != nil {
		t.Fatal(err)
	}
	fixture := filepath.Join("..", "..", "testdata", "sources", "valid-retail")
	first, err := indexer.Build(context.Background(), indexer.BuildOptions{Layout: layout, SourceID: "wow-ui-source", ProductID: "retail", Commit: "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", RequestedRef: "v1", Input: indexer.DirectoryInput{Root: fixture}, Workers: 2})
	if err != nil {
		t.Fatal(err)
	}
	db, err := sql.Open("sqlite", first.DBPath)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	contentsBefore := countRows(t, db, "contents")
	symbolsBefore := countRows(t, db, "symbols")
	searchBefore := countRows(t, db, "search_docs")

	second, err := indexer.Build(context.Background(), indexer.BuildOptions{Layout: layout, SourceID: "wow-ui-source", ProductID: "retail", Commit: "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb", RequestedRef: "v2", Input: indexer.DirectoryInput{Root: fixture}, Workers: 2})
	if err != nil {
		t.Fatal(err)
	}
	if second.ParsedLua != 0 || second.ParsedXML != 0 || second.ParsedTOC != 0 {
		t.Fatalf("unchanged snapshot was reparsed: %#v", second)
	}
	if second.ReusedObjects != second.Files {
		t.Fatalf("reused=%d files=%d", second.ReusedObjects, second.Files)
	}
	if got := countRows(t, db, "contents"); got != contentsBefore {
		t.Fatalf("contents=%d, want %d", got, contentsBefore)
	}
	if got := countRows(t, db, "symbols"); got != symbolsBefore {
		t.Fatalf("symbols=%d, want %d", got, symbolsBefore)
	}
	if got := countRows(t, db, "search_docs"); got != searchBefore {
		t.Fatalf("search_docs=%d, want %d", got, searchBefore)
	}
	if got := countRows(t, db, "snapshot_files"); got != first.Files+second.Files {
		t.Fatalf("snapshot_files=%d, want %d", got, first.Files+second.Files)
	}
	response, err := query.Search(layout, query.Context{SourceID: "wow-ui-source", ProductID: "retail", Commit: second.Commit, SnapshotID: second.SnapshotID, DBPath: second.DBPath}, "C_AuctionHouse.GetItemSearchResultInfo", "api", 3)
	if err != nil || len(response.Results) == 0 || response.Results[0].Line != 16 {
		t.Fatalf("reused snapshot query err=%v response=%#v", err, response)
	}
}

func TestOneContentUnitCanMapToMultipleSnapshotPaths(t *testing.T) {
	t.Setenv("WOWDOC_HOME", t.TempDir())
	layout, _ := home.Resolve()
	if err := layout.Ensure(); err != nil {
		t.Fatal(err)
	}
	fixture := t.TempDir()
	source := []byte("function SharedFunction()\n  return true\nend\n")
	for _, name := range []string{"A.lua", "nested/B.lua"} {
		path := filepath.Join(fixture, filepath.FromSlash(name))
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, source, 0o644); err != nil {
			t.Fatal(err)
		}
	}
	stats, err := indexer.Build(context.Background(), indexer.BuildOptions{Layout: layout, SourceID: "fixture", ProductID: "main", Commit: "cccccccccccccccccccccccccccccccccccccccc", Input: indexer.DirectoryInput{Root: fixture}, Workers: 2})
	if err != nil {
		t.Fatal(err)
	}
	db, err := sql.Open("sqlite", stats.DBPath)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if got := countRows(t, db, "contents"); got != 1 {
		rows, _ := db.Query(`SELECT content_hash,language,parser_schema,index_schema FROM contents ORDER BY id`)
		var details []string
		if rows != nil {
			defer rows.Close()
			for rows.Next() {
				var hash, language, parser, index string
				_ = rows.Scan(&hash, &language, &parser, &index)
				details = append(details, hash+"|"+language+"|"+parser+"|"+index)
			}
		}
		t.Fatalf("contents=%d, want 1: %v stats=%#v", got, details, stats)
	}
	if got := countRows(t, db, "symbols"); got != 1 {
		t.Fatalf("symbols=%d, want 1", got)
	}
	if got := countRows(t, db, "snapshot_files"); got != 2 {
		t.Fatalf("snapshot_files=%d, want 2", got)
	}
	response, err := query.Search(layout, query.Context{SourceID: "fixture", ProductID: "main", Commit: stats.Commit, SnapshotID: stats.SnapshotID, DBPath: stats.DBPath}, "SharedFunction", "lua", 10)
	if err != nil {
		t.Fatal(err)
	}
	var paths []string
	for _, match := range response.Results {
		if match.MatchedBy == "exact_symbol" {
			paths = append(paths, filepath.ToSlash(match.Path))
		}
	}
	sort.Strings(paths)
	if len(paths) != 2 || paths[0] != "A.lua" || paths[1] != "nested/B.lua" {
		t.Fatalf("paths=%v", paths)
	}
}

func TestSameBytesUseDifferentParseUnitsForDifferentLanguages(t *testing.T) {
	t.Setenv("WOWDOC_HOME", t.TempDir())
	layout, _ := home.Resolve()
	if err := layout.Ensure(); err != nil {
		t.Fatal(err)
	}
	fixture := t.TempDir()
	for _, name := range []string{"same.lua", "same.toc"} {
		if err := os.WriteFile(filepath.Join(fixture, name), []byte("-- shared bytes\n"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	stats, err := indexer.Build(context.Background(), indexer.BuildOptions{Layout: layout, SourceID: "fixture", ProductID: "languages", Commit: "dddddddddddddddddddddddddddddddddddddddd", Input: indexer.DirectoryInput{Root: fixture}, Workers: 2})
	if err != nil {
		t.Fatal(err)
	}
	db, err := sql.Open("sqlite", stats.DBPath)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if got := countRows(t, db, "contents"); got != 2 {
		t.Fatalf("contents=%d, want one parse unit per language", got)
	}
}

func countRows(t *testing.T, db *sql.DB, table string) int {
	t.Helper()
	var count int
	if err := db.QueryRow("SELECT count(*) FROM " + table).Scan(&count); err != nil {
		t.Fatal(err)
	}
	return count
}
