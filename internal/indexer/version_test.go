package indexer_test

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/follenfang/wowdoc/internal/home"
	"github.com/follenfang/wowdoc/internal/indexer"
	"github.com/follenfang/wowdoc/internal/query"
	"github.com/follenfang/wowdoc/internal/store"
)

func TestBuildCachesMissingOrUnusableVersion(t *testing.T) {
	for _, tc := range []struct{ name, payload string }{{"missing", ""}, {"unusable", "not a game version"}} {
		t.Run(tc.name, func(t *testing.T) {
			opts := versionBuildOptions(t, "wow-ui-source", tc.payload)
			if _, err := indexer.Build(context.Background(), opts); err != nil {
				t.Fatal(err)
			}
			opts.Input = failingInput{}
			for i := 0; i < 2; i++ {
				stats, err := indexer.Build(context.Background(), opts)
				if err != nil {
					t.Fatalf("ready snapshot accessed input: %v", err)
				}
				if stats.BuildInterface != "" {
					t.Fatalf("unexpected Interface %q", stats.BuildInterface)
				}
			}
		})
	}
}

// failingVersionInput lists the real tree but cannot read the root version.txt,
// simulating a transient worktree or IO failure.
type failingVersionInput struct {
	root   string
	locked bool
}

func (f failingVersionInput) Entries(ctx context.Context) ([]indexer.Entry, error) {
	return indexer.DirectoryInput{Root: f.root}.Entries(ctx)
}

func (f failingVersionInput) Read(ctx context.Context, e indexer.Entry) ([]byte, error) {
	if f.locked && strings.EqualFold(e.Path, "version.txt") {
		return nil, errors.New("simulated version read failure")
	}
	return indexer.DirectoryInput{Root: f.root}.Read(ctx, e)
}

func (f failingVersionInput) ReadRaw(ctx context.Context, e indexer.Entry) ([]byte, error) {
	return f.Read(ctx, e)
}

// A failed version.txt read must not be cached as "this snapshot has no
// Interface evidence", or the snapshot stays permanently unresolvable.
func TestUnreadableVersionIsNotCachedAsAbsent(t *testing.T) {
	opts := versionBuildOptions(t, "wow-ui-source", "1.60.1.69893")
	failing := failingVersionInput{root: opts.Input.(indexer.DirectoryInput).Root, locked: true}
	if _, err := indexer.Build(context.Background(), indexer.BuildOptions{Layout: opts.Layout, SourceID: opts.SourceID, ProductID: opts.ProductID, Commit: opts.Commit, Input: failing, Workers: 2}); err == nil {
		t.Fatal("a failed version.txt read must fail the build, not publish silently")
	}
	// The retry must still observe the build version rather than a cached empty.
	stats, err := indexer.Build(context.Background(), opts)
	if err != nil {
		t.Fatal(err)
	}
	if stats.BuildInterface != "16001" {
		t.Fatalf("buildInterface=%q, want 16001 after a failed read", stats.BuildInterface)
	}
	opts.Input = failingInput{}
	if _, err = indexer.Build(context.Background(), opts); err != nil {
		t.Fatalf("ready snapshot accessed input: %v", err)
	}
}

func TestLegacyBuildEvidenceIsBackfilledOnce(t *testing.T) {
	for _, tc := range []struct{ source, payload, want string }{
		{"wow-ui-source", "1.60.1.69893", "16001"},
		{"wow-ui-source", "", ""},
		{"weakauras", "5.20.3", ""},
	} {
		t.Run(tc.source+"/"+tc.payload, func(t *testing.T) {
			opts := versionBuildOptions(t, tc.source, tc.payload)
			stats, err := indexer.Build(context.Background(), opts)
			if err != nil {
				t.Fatal(err)
			}
			branch, err := store.OpenBranch(opts.Layout, opts.SourceID, opts.ProductID, indexer.ParserSchema, indexer.IndexSchema)
			if err != nil {
				t.Fatal(err)
			}
			// Recreate v0.0.10 metadata, including an incorrectly derived value.
			_, err = branch.DB.Exec(`ALTER TABLE snapshots DROP COLUMN build_interface_checked`)
			if err == nil {
				_, err = branch.DB.Exec(`UPDATE snapshots SET build_interface='52003'`)
			}
			branch.Close()
			if err != nil {
				t.Fatal(err)
			}
			stats, err = indexer.Build(context.Background(), opts)
			if err != nil {
				t.Fatal(err)
			}
			if stats.BuildInterface != tc.want {
				t.Fatalf("backfilled Interface=%q, want %q", stats.BuildInterface, tc.want)
			}
			opts.Input = failingInput{}
			if _, err = indexer.Build(context.Background(), opts); err != nil {
				t.Fatalf("backfill repeated: %v", err)
			}
		})
	}
}

func TestAddonVersionIsNotGameInterface(t *testing.T) {
	opts := versionBuildOptions(t, "weakauras", "5.20.3")
	stats, err := indexer.Build(context.Background(), opts)
	if err != nil {
		t.Fatal(err)
	}
	if stats.BuildInterface != "" {
		t.Errorf("AddOn version became game Interface: %q", stats.BuildInterface)
	}
	// Simulate metadata already written by v0.0.10: read-only queries must
	// not trust an AddOn version even before the snapshot is rebuilt.
	branch, err := store.OpenBranch(opts.Layout, opts.SourceID, opts.ProductID, indexer.ParserSchema, indexer.IndexSchema)
	if err != nil {
		t.Fatal(err)
	}
	_, err = branch.DB.Exec(`UPDATE snapshots SET build_interface='52003' WHERE id=?`, stats.SnapshotID)
	branch.Close()
	if err != nil {
		t.Fatal(err)
	}
	ctx := query.Context{SourceID: opts.SourceID, ProductID: opts.ProductID, Commit: opts.Commit, SnapshotID: stats.SnapshotID, DBPath: stats.DBPath}
	for _, value := range []string{"52003", "120000"} {
		facts, unresolved, _, err := query.LookupCompatibility(opts.Layout, ctx, nil, value)
		if err != nil {
			t.Fatal(err)
		}
		if len(facts) != 0 || len(unresolved) != 1 {
			t.Errorf("Interface %s: facts=%#v unresolved=%#v", value, facts, unresolved)
		}
	}
}

func versionBuildOptions(t *testing.T, source, payload string) indexer.BuildOptions {
	t.Helper()
	t.Setenv("WOWDOC_HOME", t.TempDir())
	layout, err := home.Resolve()
	if err != nil {
		t.Fatal(err)
	}
	if err = layout.Ensure(); err != nil {
		t.Fatal(err)
	}
	root := t.TempDir()
	if err = os.WriteFile(filepath.Join(root, "Addon.lua"), []byte("local value = 1\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if payload != "" {
		if err = os.WriteFile(filepath.Join(root, "version.txt"), []byte(payload), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return indexer.BuildOptions{Layout: layout, SourceID: source, ProductID: "main", Commit: "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", Input: indexer.DirectoryInput{Root: root}, Workers: 2}
}

func TestBuildInterfaceFromVersion(t *testing.T) {
	cases := []struct{ payload, want string }{
		{"1.60.1.69893", "16001"},
		{"1.60.1.69893\n", "16001"},
		{"12.1.0.69814", "120100"},
		{"12.0.0.60000", "120000"},
		{"5.5.4.69585", "50504"},
		{"3.80.2.69874", "38002"},
		{"1.15.9.69722", "11509"},
		{"2.5.6.69795", "20506"},
		{"1.60.1", "16001"},
		{"\xef\xbb\xbf1.60.1.69893", "16001"},
		{"", ""},
		{"empty", ""},
		{"latest", ""},
		{"1.60", ""},
		{"a.b.c", ""},
		{"1.60.x", ""},
	}
	for _, tc := range cases {
		if got := indexer.BuildInterfaceFromVersion([]byte(tc.payload)); got != tc.want {
			t.Fatalf("payload %q: got %q, want %q", tc.payload, got, tc.want)
		}
	}
}
