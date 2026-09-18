package store_test

import (
	"database/sql"
	"os"
	"path/filepath"
	"testing"

	"github.com/follenfang/wowdoc/internal/home"
	"github.com/follenfang/wowdoc/internal/schema"
	"github.com/follenfang/wowdoc/internal/store"
	_ "modernc.org/sqlite"
)

func TestOpenBranchMigratesBuildInterfaceColumn(t *testing.T) {
	t.Setenv("WOWDOC_HOME", t.TempDir())
	layout, err := home.Resolve()
	if err != nil {
		t.Fatal(err)
	}
	if err = layout.Ensure(); err != nil {
		t.Fatal(err)
	}
	// Pre-create a branch database with the pre-migration snapshots schema.
	path := store.BranchPath(layout, "wow-ui-source", "forever", schema.Parser, schema.Index)
	if err = os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	legacy, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = legacy.Exec(`CREATE TABLE snapshots(id TEXT PRIMARY KEY,commit_hash TEXT NOT NULL UNIQUE,requested_ref TEXT NOT NULL,tag TEXT,status TEXT NOT NULL,created_at TEXT NOT NULL,published_at TEXT,parser_schema TEXT NOT NULL,index_schema TEXT NOT NULL)`); err != nil {
		legacy.Close()
		t.Fatal(err)
	}
	if _, err = legacy.Exec(`INSERT INTO snapshots(id,commit_hash,requested_ref,status,created_at,parser_schema,index_schema) VALUES('legacy','old','latest','ready',datetime('now'),?,?)`, schema.Parser, schema.Index); err != nil {
		legacy.Close()
		t.Fatal(err)
	}
	if err = legacy.Close(); err != nil {
		t.Fatal(err)
	}

	branch, err := store.OpenBranch(layout, "wow-ui-source", "forever", schema.Parser, schema.Index)
	if err != nil {
		t.Fatal(err)
	}
	defer branch.Close()
	value, checked, err := branch.SnapshotBuildInterfaceState("legacy")
	if err != nil || checked || value != "" {
		t.Fatalf("legacy state=(%q, %v, %v), want empty and unchecked", value, checked, err)
	}
	if err = branch.Publish("legacy", "old", "latest", "", "", schema.Parser, schema.Index, store.SnapshotBatch{}); err != nil {
		t.Fatal(err)
	}
	value, checked, err = branch.SnapshotBuildInterfaceState("legacy")
	if err != nil || !checked || value != "" {
		t.Fatalf("published empty state=(%q, %v, %v), want empty and checked", value, checked, err)
	}
	var columns int
	if err = branch.DB.QueryRow(`SELECT COUNT(*) FROM pragma_table_info('snapshots') WHERE name='build_interface'`).Scan(&columns); err != nil {
		t.Fatal(err)
	}
	if columns != 1 {
		t.Fatal("build_interface column was not migrated")
	}
	snapshotID := "wow-ui-source-forever-4444444444444444444444444444444444444444"
	if err = branch.Publish(snapshotID, "4444444444444444444444444444444444444444", "latest", "", "16001", schema.Parser, schema.Index, store.SnapshotBatch{}); err != nil {
		t.Fatal(err)
	}
	got, err := branch.SnapshotBuildInterface(snapshotID)
	if err != nil {
		t.Fatal(err)
	}
	if got != "16001" {
		t.Fatalf("buildInterface=%q, want 16001", got)
	}
}
