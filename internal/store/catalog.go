package store

import (
	"database/sql"
	"fmt"
	"path/filepath"
	"strings"
	"time"

	"github.com/follenfang/wowdoc/internal/catalog"
	"github.com/follenfang/wowdoc/internal/home"
	_ "modernc.org/sqlite"
)

type Catalog struct {
	db *sql.DB
}

type RefRecord struct {
	SourceID, ProductID, Branch, HeadCommit string
	SyncedAt                                string
}

type TagRecord struct {
	Name, Commit string
	CommittedAt  int64
}

type SnapshotRecord struct {
	ID, SourceID, ProductID, Commit, RequestedRef, Tag, Status, DBPath, ManifestPath, IndexedAt string
}

func OpenCatalog(layout home.Layout) (*Catalog, error) {
	db, err := sql.Open("sqlite", filepath.Join(layout.State, "catalog.sqlite"))
	if err != nil {
		return nil, err
	}
	db.SetMaxOpenConns(1)
	for _, pragma := range []string{"PRAGMA journal_mode=WAL", "PRAGMA foreign_keys=ON", "PRAGMA busy_timeout=5000"} {
		if _, err := db.Exec(pragma); err != nil {
			db.Close()
			return nil, err
		}
	}
	if _, err := db.Exec(catalogSchema); err != nil {
		db.Close()
		return nil, err
	}
	return &Catalog{db: db}, nil
}

func OpenCatalogRead(layout home.Layout) (*Catalog, error) {
	path := filepath.Join(layout.State, "catalog.sqlite")
	db, err := sql.Open("sqlite", "file:"+filepath.ToSlash(path)+"?mode=ro")
	if err != nil {
		return nil, err
	}
	db.SetMaxOpenConns(8)
	for _, pragma := range []string{"PRAGMA foreign_keys=ON", "PRAGMA busy_timeout=5000", "PRAGMA query_only=ON"} {
		if _, err = db.Exec(pragma); err != nil {
			db.Close()
			return nil, err
		}
	}
	return &Catalog{db: db}, nil
}

func (c *Catalog) Close() error { return c.db.Close() }

func (c *Catalog) Seed(sources []catalog.Source) error {
	tx, err := c.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	for _, source := range sources {
		if _, err := tx.Exec(`INSERT INTO sources(id,name,repository,official) VALUES(?,?,?,?) ON CONFLICT(id) DO UPDATE SET name=excluded.name,repository=excluded.repository,official=excluded.official`, source.ID, source.Name, source.Repository, source.Official); err != nil {
			return err
		}
		for _, product := range source.Products {
			if _, err := tx.Exec(`INSERT INTO products(source_id,id,branch) VALUES(?,?,?) ON CONFLICT(source_id,id) DO UPDATE SET branch=excluded.branch`, source.ID, product.ID, product.Branch); err != nil {
				return err
			}
		}
	}
	return tx.Commit()
}

func (c *Catalog) PublishRef(sourceID string, product catalog.Product, head string, tags []TagRecord) error {
	tx, err := c.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	now := time.Now().UTC().Format(time.RFC3339)
	if _, err := tx.Exec(`INSERT INTO refs(source_id,product_id,branch,head_commit,synced_at) VALUES(?,?,?,?,?) ON CONFLICT(source_id,product_id) DO UPDATE SET branch=excluded.branch,head_commit=excluded.head_commit,synced_at=excluded.synced_at`, sourceID, product.ID, product.Branch, head, now); err != nil {
		return err
	}
	if _, err := tx.Exec(`DELETE FROM tags WHERE source_id=? AND product_id=?`, sourceID, product.ID); err != nil {
		return err
	}
	for _, tag := range tags {
		if _, err := tx.Exec(`INSERT INTO tags(source_id,product_id,name,commit_hash,committed_at) VALUES(?,?,?,?,?)`, sourceID, product.ID, tag.Name, tag.Commit, tag.CommittedAt); err != nil {
			return err
		}
	}
	return tx.Commit()
}

func (c *Catalog) Refs(sourceID string) ([]RefRecord, error) {
	query := `SELECT source_id,product_id,branch,COALESCE(head_commit,''),COALESCE(synced_at,'') FROM refs`
	args := []any{}
	if sourceID != "" {
		query += ` WHERE source_id=?`
		args = append(args, sourceID)
	}
	query += ` ORDER BY source_id,product_id`
	rows, err := c.db.Query(query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []RefRecord
	for rows.Next() {
		var row RefRecord
		if err := rows.Scan(&row.SourceID, &row.ProductID, &row.Branch, &row.HeadCommit, &row.SyncedAt); err != nil {
			return nil, err
		}
		out = append(out, row)
	}
	return out, rows.Err()
}

func (c *Catalog) Tags(sourceID, productID string) ([]TagRecord, error) {
	rows, err := c.db.Query(`SELECT name,commit_hash,committed_at FROM tags WHERE source_id=? AND product_id=? ORDER BY committed_at DESC,name DESC`, sourceID, productID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []TagRecord
	for rows.Next() {
		var t TagRecord
		if err := rows.Scan(&t.Name, &t.Commit, &t.CommittedAt); err != nil {
			return nil, err
		}
		out = append(out, t)
	}
	return out, rows.Err()
}

func (c *Catalog) SaveSnapshot(s SnapshotRecord) error {
	_, err := c.db.Exec(`INSERT INTO snapshots(id,source_id,product_id,commit_hash,requested_ref,tag,status,db_path,manifest_path,indexed_at) VALUES(?,?,?,?,?,?,?,?,?,?) ON CONFLICT(id) DO UPDATE SET requested_ref=excluded.requested_ref,tag=excluded.tag,status=excluded.status,db_path=excluded.db_path,manifest_path=excluded.manifest_path,indexed_at=excluded.indexed_at`, s.ID, s.SourceID, s.ProductID, s.Commit, s.RequestedRef, s.Tag, s.Status, s.DBPath, s.ManifestPath, s.IndexedAt)
	return err
}

func (c *Catalog) Snapshot(sourceID, productID, commit string) (SnapshotRecord, bool, error) {
	var s SnapshotRecord
	err := c.db.QueryRow(`SELECT id,source_id,product_id,commit_hash,requested_ref,COALESCE(tag,''),status,db_path,manifest_path,COALESCE(indexed_at,'') FROM snapshots WHERE source_id=? AND product_id=? AND commit_hash=?`, sourceID, productID, commit).Scan(&s.ID, &s.SourceID, &s.ProductID, &s.Commit, &s.RequestedRef, &s.Tag, &s.Status, &s.DBPath, &s.ManifestPath, &s.IndexedAt)
	if err == sql.ErrNoRows {
		return SnapshotRecord{}, false, nil
	}
	if err != nil {
		return SnapshotRecord{}, false, err
	}
	return s, true, nil
}

func (c *Catalog) Resolve(source catalog.Source, product catalog.Product, ref string) (commit, tag string, err error) {
	if ref == "" || strings.EqualFold(ref, "latest") {
		err = c.db.QueryRow(`SELECT head_commit FROM refs WHERE source_id=? AND product_id=?`, source.ID, product.ID).Scan(&commit)
		return commit, "", err
	}
	if len(ref) == 40 && isHex(ref) {
		return strings.ToLower(ref), "", nil
	}
	var candidates []TagRecord
	tags, e := c.Tags(source.ID, product.ID)
	if e != nil {
		return "", "", e
	}
	for _, t := range tags {
		if t.Name == ref {
			candidates = append(candidates, t)
			continue
		}
		for _, prefix := range source.VersionPrefixes {
			if t.Name == prefix+ref {
				candidates = append(candidates, t)
			}
		}
	}
	if len(candidates) == 1 {
		return candidates[0].Commit, candidates[0].Name, nil
	}
	if len(candidates) > 1 {
		return "", "", fmt.Errorf("ambiguous version: %s", ref)
	}
	return "", "", sql.ErrNoRows
}

func isHex(s string) bool {
	for _, r := range s {
		if !strings.ContainsRune("0123456789abcdefABCDEF", r) {
			return false
		}
	}
	return true
}

const catalogSchema = `
CREATE TABLE IF NOT EXISTS sources(id TEXT PRIMARY KEY,name TEXT NOT NULL,repository TEXT NOT NULL,official INTEGER NOT NULL);
CREATE TABLE IF NOT EXISTS products(source_id TEXT NOT NULL,id TEXT NOT NULL,branch TEXT NOT NULL,PRIMARY KEY(source_id,id),FOREIGN KEY(source_id) REFERENCES sources(id));
CREATE TABLE IF NOT EXISTS refs(source_id TEXT NOT NULL,product_id TEXT NOT NULL,branch TEXT NOT NULL,head_commit TEXT NOT NULL,synced_at TEXT NOT NULL,PRIMARY KEY(source_id,product_id));
CREATE TABLE IF NOT EXISTS tags(source_id TEXT NOT NULL,product_id TEXT NOT NULL,name TEXT NOT NULL,commit_hash TEXT NOT NULL,committed_at INTEGER NOT NULL,PRIMARY KEY(source_id,product_id,name));
CREATE INDEX IF NOT EXISTS tags_commit ON tags(source_id,product_id,commit_hash);
CREATE TABLE IF NOT EXISTS snapshots(id TEXT PRIMARY KEY,source_id TEXT NOT NULL,product_id TEXT NOT NULL,commit_hash TEXT NOT NULL,requested_ref TEXT NOT NULL,tag TEXT,status TEXT NOT NULL,db_path TEXT NOT NULL,manifest_path TEXT NOT NULL,indexed_at TEXT,UNIQUE(source_id,product_id,commit_hash));
CREATE TABLE IF NOT EXISTS tasks(id TEXT PRIMARY KEY,task_key TEXT UNIQUE NOT NULL,status TEXT NOT NULL,stage TEXT NOT NULL,progress INTEGER NOT NULL,lease_until TEXT,updated_at TEXT NOT NULL,error_code TEXT);
`
