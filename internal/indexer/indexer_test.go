package indexer_test

import (
	"context"
	"database/sql"
	"errors"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"testing"

	"github.com/follenfang/wowdoc/internal/home"
	"github.com/follenfang/wowdoc/internal/indexer"
	"github.com/follenfang/wowdoc/internal/objectstore"
	"github.com/follenfang/wowdoc/internal/query"
	"github.com/follenfang/wowdoc/internal/schema"
	"github.com/follenfang/wowdoc/internal/store"
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
	if !objectstore.Exists(layout, objectstore.Source, got.ContentHash) {
		t.Fatal("source evidence object is missing")
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

func TestExactXMLDefinitionRanksBeforeInheritanceReference(t *testing.T) {
	t.Setenv("WOWDOC_HOME", t.TempDir())
	layout, _ := home.Resolve()
	if err := layout.Ensure(); err != nil {
		t.Fatal(err)
	}
	fixture := t.TempDir()
	xmlSource := []byte("<Ui>\n  <Frame name=\"Consumer\" inherits=\"TargetTemplate\"/>\n  <Frame name=\"TargetTemplate\" inherits=\"BaseTemplate\"/>\n</Ui>\n")
	if err := os.WriteFile(filepath.Join(fixture, "Templates.xml"), xmlSource, 0o644); err != nil {
		t.Fatal(err)
	}
	stats, err := indexer.Build(context.Background(), indexer.BuildOptions{Layout: layout, SourceID: "fixture", ProductID: "xml-rank", Commit: "eeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeee", Input: indexer.DirectoryInput{Root: fixture}, Workers: 2})
	if err != nil {
		t.Fatal(err)
	}
	response, err := query.Search(layout, query.Context{SourceID: "fixture", ProductID: "xml-rank", Commit: stats.Commit, SnapshotID: stats.SnapshotID, DBPath: stats.DBPath}, "TargetTemplate", "xml", 1)
	if err != nil {
		t.Fatal(err)
	}
	if len(response.Results) != 1 || response.Results[0].Name != "TargetTemplate" || response.Results[0].Line != 3 || response.Results[0].MatchedBy != "exact_fact" {
		t.Fatalf("unexpected exact XML result: %#v", response.Results)
	}
}

func TestStructuredFileKeepsCompletePlainTextSearch(t *testing.T) {
	t.Setenv("WOWDOC_HOME", t.TempDir())
	layout, _ := home.Resolve()
	if err := layout.Ensure(); err != nil {
		t.Fatal(err)
	}
	fixture := t.TempDir()
	source := []byte("function IndexedFunction()\n  local status = \"obscure regression marker\"\n  return status\nend\n")
	if err := os.WriteFile(filepath.Join(fixture, "Search.lua"), source, 0o644); err != nil {
		t.Fatal(err)
	}
	stats, err := indexer.Build(context.Background(), indexer.BuildOptions{Layout: layout, SourceID: "fixture", ProductID: "fulltext", Commit: "ffffffffffffffffffffffffffffffffffffffff", Input: indexer.DirectoryInput{Root: fixture}, Workers: 2})
	if err != nil {
		t.Fatal(err)
	}
	response, err := query.Search(layout, query.Context{SourceID: "fixture", ProductID: "fulltext", Commit: stats.Commit, SnapshotID: stats.SnapshotID, DBPath: stats.DBPath}, "obscure regression marker", "lua", 1)
	if err != nil {
		t.Fatal(err)
	}
	if len(response.Results) != 1 || response.Results[0].Kind != "source" || response.Results[0].Line != 2 || !strings.Contains(response.Results[0].Excerpt, "obscure regression marker") {
		t.Fatalf("unexpected full-text result: %#v", response.Results)
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
	db, err := sql.Open("sqlite", store.ContentPath(layout, "wow-ui-source", schema.Parser, schema.Index))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	branchDB, err := sql.Open("sqlite", first.DBPath)
	if err != nil {
		t.Fatal(err)
	}
	defer branchDB.Close()
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
	if got := countRows(t, branchDB, "snapshot_files"); got != first.Files+second.Files {
		t.Fatalf("snapshot_files=%d, want %d", got, first.Files+second.Files)
	}
	response, err := query.Search(layout, query.Context{SourceID: "wow-ui-source", ProductID: "retail", Commit: second.Commit, SnapshotID: second.SnapshotID, DBPath: second.DBPath}, "C_AuctionHouse.GetItemSearchResultInfo", "api", 3)
	if err != nil || len(response.Results) == 0 || response.Results[0].Line != 16 {
		t.Fatalf("reused snapshot query err=%v response=%#v", err, response)
	}
}

func TestFactsAreSharedAcrossProductsWhileFTSRemainsBranchLocal(t *testing.T) {
	t.Setenv("WOWDOC_HOME", t.TempDir())
	layout, _ := home.Resolve()
	if err := layout.Ensure(); err != nil {
		t.Fatal(err)
	}
	fixture := filepath.Join("..", "..", "testdata", "sources", "valid-retail")
	first, err := indexer.Build(context.Background(), indexer.BuildOptions{Layout: layout, SourceID: "wow-ui-source", ProductID: "retail", Commit: "1212121212121212121212121212121212121212", RequestedRef: "retail", Input: indexer.DirectoryInput{Root: fixture}, Workers: 2})
	if err != nil {
		t.Fatal(err)
	}
	contentDB, err := sql.Open("sqlite", store.ContentPath(layout, "wow-ui-source", schema.Parser, schema.Index))
	if err != nil {
		t.Fatal(err)
	}
	defer contentDB.Close()
	contents := countRows(t, contentDB, "contents")
	symbols := countRows(t, contentDB, "symbols")
	searchDocs := countRows(t, contentDB, "search_docs")
	second, err := indexer.Build(context.Background(), indexer.BuildOptions{Layout: layout, SourceID: "wow-ui-source", ProductID: "ptr", Commit: "3434343434343434343434343434343434343434", RequestedRef: "ptr", Input: indexer.DirectoryInput{Root: fixture}, Workers: 2})
	if err != nil {
		t.Fatal(err)
	}
	if got := countRows(t, contentDB, "contents"); got != contents {
		t.Fatalf("shared contents=%d, want %d", got, contents)
	}
	if got := countRows(t, contentDB, "symbols"); got != symbols {
		t.Fatalf("shared symbols=%d, want %d", got, symbols)
	}
	if got := countRows(t, contentDB, "search_docs"); got != searchDocs {
		t.Fatalf("shared search docs=%d, want %d", got, searchDocs)
	}
	for _, dbPath := range []string{first.DBPath, second.DBPath} {
		db, openErr := sql.Open("sqlite", dbPath)
		if openErr != nil {
			t.Fatal(openErr)
		}
		if got := countRows(t, db, "branch_contents"); got != contents {
			db.Close()
			t.Fatalf("branch contents=%d, want %d", got, contents)
		}
		if got := countRows(t, db, "search_fts"); got != searchDocs {
			db.Close()
			t.Fatalf("branch FTS docs=%d, want %d", got, searchDocs)
		}
		db.Close()
	}
	left, err := query.Search(layout, query.Context{SourceID: "wow-ui-source", ProductID: "retail", Commit: first.Commit, SnapshotID: first.SnapshotID, DBPath: first.DBPath}, "C_AuctionHouse.GetItemSearchResultInfo", "api", 3)
	if err != nil {
		t.Fatal(err)
	}
	right, err := query.Search(layout, query.Context{SourceID: "wow-ui-source", ProductID: "ptr", Commit: second.Commit, SnapshotID: second.SnapshotID, DBPath: second.DBPath}, "C_AuctionHouse.GetItemSearchResultInfo", "api", 3)
	if err != nil {
		t.Fatal(err)
	}
	if len(left.Results) != len(right.Results) || len(left.Results) == 0 {
		t.Fatalf("left=%#v right=%#v", left.Results, right.Results)
	}
	for i := range left.Results {
		if left.Results[i].Path != right.Results[i].Path || left.Results[i].Line != right.Results[i].Line || left.Results[i].MatchedBy != right.Results[i].MatchedBy || left.Results[i].Score != right.Results[i].Score || left.Results[i].ContentHash != right.Results[i].ContentHash {
			t.Fatalf("result %d differs: left=%#v right=%#v", i, left.Results[i], right.Results[i])
		}
	}
}

func TestConcurrentProductBuildsSerializeSharedContentPublish(t *testing.T) {
	t.Setenv("WOWDOC_HOME", t.TempDir())
	layout, _ := home.Resolve()
	if err := layout.Ensure(); err != nil {
		t.Fatal(err)
	}
	fixture := filepath.Join("..", "..", "testdata", "sources", "valid-retail")
	products := []string{"retail", "ptr", "beta", "classic"}
	start := make(chan struct{})
	results := make(chan struct {
		stats indexer.Stats
		err   error
	}, len(products))
	var wg sync.WaitGroup
	for i, product := range products {
		wg.Add(1)
		go func(i int, product string) {
			defer wg.Done()
			<-start
			commit := strings.Repeat(string(rune('a'+i)), 40)
			stats, err := indexer.Build(context.Background(), indexer.BuildOptions{
				Layout: layout, SourceID: "wow-ui-source", ProductID: product,
				Commit: commit, RequestedRef: product, Input: indexer.DirectoryInput{Root: fixture}, Workers: 2,
			})
			results <- struct {
				stats indexer.Stats
				err   error
			}{stats: stats, err: err}
		}(i, product)
	}
	close(start)
	wg.Wait()
	close(results)

	var built []indexer.Stats
	for result := range results {
		if result.err != nil {
			t.Fatalf("concurrent build: %v", result.err)
		}
		built = append(built, result.stats)
	}
	if len(built) != len(products) {
		t.Fatalf("built=%d, want %d", len(built), len(products))
	}
	contentDB, err := sql.Open("sqlite", store.ContentPath(layout, "wow-ui-source", schema.Parser, schema.Index))
	if err != nil {
		t.Fatal(err)
	}
	defer contentDB.Close()
	contents := countRows(t, contentDB, "contents")
	if contents == 0 {
		t.Fatal("shared content DB is empty")
	}
	for _, stats := range built {
		db, openErr := sql.Open("sqlite", stats.DBPath)
		if openErr != nil {
			t.Fatal(openErr)
		}
		got := countRows(t, db, "branch_contents")
		db.Close()
		if got != contents {
			t.Fatalf("%s branch contents=%d, want %d", stats.SnapshotID, got, contents)
		}
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
	db, err := sql.Open("sqlite", store.ContentPath(layout, "fixture", schema.Parser, schema.Index))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	branchDB, err := sql.Open("sqlite", stats.DBPath)
	if err != nil {
		t.Fatal(err)
	}
	defer branchDB.Close()
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
	if got := countRows(t, branchDB, "snapshot_files"); got != 2 {
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
	_, err := indexer.Build(context.Background(), indexer.BuildOptions{Layout: layout, SourceID: "fixture", ProductID: "languages", Commit: "dddddddddddddddddddddddddddddddddddddddd", Input: indexer.DirectoryInput{Root: fixture}, Workers: 2})
	if err != nil {
		t.Fatal(err)
	}
	db, err := sql.Open("sqlite", store.ContentPath(layout, "fixture", schema.Parser, schema.Index))
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
