package store

import (
	"database/sql"
	"os"
	"path/filepath"

	"github.com/follenfang/wowdoc/internal/home"
	_ "modernc.org/sqlite"
)

type Branch struct {
	DB   *sql.DB
	Path string
}

type FileFact struct {
	Path, ContentHash, ASTHash, Language, Role string
	Size                                       int64
}
type SymbolFact struct {
	Name, Qualified, Kind, Path, Signature string
	Line, EndLine                          int
}
type EdgeFact struct {
	Source, Target, Kind, Confidence, Path string
	Line                                   int
}
type XMLFact struct {
	Name, Kind, Path string
	Line             int
	Attributes       string
}
type TOCFact struct {
	Path, Key, Value string
	Line             int
}
type SearchFact struct {
	Path, Kind, Name, Text string
	Line                   int
	Role                   string
}
type AssetFact struct {
	Path, NormalizedPath, ContentHash, GitOID, Extension, MIME, Format string
	Size                                                               int64
	Width, Height                                                      int
}
type AssetRefFact struct {
	SourcePath, Value, NormalizedValue, Kind string
	Line                                     int
}

func OpenBranch(layout home.Layout, sourceID, productID string) (*Branch, error) {
	path := filepath.Join(layout.Indexes, sourceID, productID+".sqlite")
	if err := ensureParent(path); err != nil {
		return nil, err
	}
	db, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, err
	}
	db.SetMaxOpenConns(8)
	for _, p := range []string{"PRAGMA journal_mode=WAL", "PRAGMA foreign_keys=ON", "PRAGMA busy_timeout=5000"} {
		if _, err = db.Exec(p); err != nil {
			db.Close()
			return nil, err
		}
	}
	if _, err = db.Exec(branchSchema); err != nil {
		db.Close()
		return nil, err
	}
	_, _ = db.Exec(`ALTER TABLE symbols ADD COLUMN end_line INTEGER NOT NULL DEFAULT 0`)
	_, _ = db.Exec(`ALTER TABLE snapshots ADD COLUMN parser_schema TEXT NOT NULL DEFAULT ''`)
	_, _ = db.Exec(`ALTER TABLE snapshots ADD COLUMN index_schema TEXT NOT NULL DEFAULT ''`)
	return &Branch{DB: db, Path: path}, nil
}

func OpenBranchRead(layout home.Layout, sourceID, productID string) (*Branch, error) {
	path := filepath.Join(layout.Indexes, sourceID, productID+".sqlite")
	if _, err := os.Stat(path); err != nil {
		return nil, err
	}
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
	return &Branch{DB: db, Path: path}, nil
}

func (b *Branch) Close() error { return b.DB.Close() }

type SnapshotBatch struct {
	Files     []FileFact
	Symbols   []SymbolFact
	Edges     []EdgeFact
	XML       []XMLFact
	TOC       []TOCFact
	Search    []SearchFact
	Assets    []AssetFact
	AssetRefs []AssetRefFact
}

type SnapshotSummary struct {
	Files, Lua, XML, TOC, Assets, AST int
}

func (b *Branch) ReadySnapshot(snapshotID, parserSchema, indexSchema string) (SnapshotSummary, bool, error) {
	var status, storedParser, storedIndex string
	if err := b.DB.QueryRow(`SELECT status,parser_schema,index_schema FROM snapshots WHERE id=?`, snapshotID).Scan(&status, &storedParser, &storedIndex); err == sql.ErrNoRows {
		return SnapshotSummary{}, false, nil
	} else if err != nil {
		return SnapshotSummary{}, false, err
	}
	if status != "ready" || storedParser != parserSchema || storedIndex != indexSchema {
		return SnapshotSummary{}, false, nil
	}
	var summary SnapshotSummary
	queries := []struct {
		value *int
		query string
	}{
		{&summary.Files, `SELECT count(*) FROM snapshot_files WHERE snapshot_id=?`},
		{&summary.Lua, `SELECT count(*) FROM snapshot_files sf JOIN files f ON f.content_hash=sf.content_hash WHERE sf.snapshot_id=? AND f.language='lua'`},
		{&summary.XML, `SELECT count(*) FROM snapshot_files sf JOIN files f ON f.content_hash=sf.content_hash WHERE sf.snapshot_id=? AND f.language='xml'`},
		{&summary.TOC, `SELECT count(*) FROM snapshot_files sf JOIN files f ON f.content_hash=sf.content_hash WHERE sf.snapshot_id=? AND f.language='toc'`},
		{&summary.Assets, `SELECT count(*) FROM snapshot_assets WHERE snapshot_id=?`},
		{&summary.AST, `SELECT count(*) FROM snapshot_files sf JOIN files f ON f.content_hash=sf.content_hash WHERE sf.snapshot_id=? AND f.ast_hash<>''`},
	}
	for _, item := range queries {
		if err := b.DB.QueryRow(item.query, snapshotID).Scan(item.value); err != nil {
			return SnapshotSummary{}, false, err
		}
	}
	return summary, true, nil
}

func (b *Branch) Publish(snapshotID, commit, requestedRef, tag, parserSchema, indexSchema string, batch SnapshotBatch) error {
	tx, err := b.DB.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err = tx.Exec(`INSERT INTO snapshots(id,commit_hash,requested_ref,tag,status,created_at,parser_schema,index_schema) VALUES(?,?,?,?, 'building',datetime('now'),?,?) ON CONFLICT(id) DO UPDATE SET status='building',requested_ref=excluded.requested_ref,tag=excluded.tag,parser_schema=excluded.parser_schema,index_schema=excluded.index_schema`, snapshotID, commit, requestedRef, tag, parserSchema, indexSchema); err != nil {
		return err
	}
	for _, table := range []string{"snapshot_files", "symbols", "edges", "xml_nodes", "toc_entries", "search_docs", "snapshot_assets", "asset_refs"} {
		if _, err = tx.Exec(`DELETE FROM `+table+` WHERE snapshot_id=?`, snapshotID); err != nil {
			return err
		}
	}
	for _, f := range batch.Files {
		if _, err = tx.Exec(`INSERT OR IGNORE INTO files(content_hash,ast_hash,language,bytes) VALUES(?,?,?,?)`, f.ContentHash, f.ASTHash, f.Language, f.Size); err != nil {
			return err
		}
		if _, err = tx.Exec(`INSERT INTO snapshot_files(snapshot_id,path,content_hash,role) VALUES(?,?,?,?)`, snapshotID, f.Path, f.ContentHash, f.Role); err != nil {
			return err
		}
	}
	for _, s := range batch.Symbols {
		if _, err = tx.Exec(`INSERT INTO symbols(snapshot_id,name,qualified_name,kind,path,line,end_line,signature) VALUES(?,?,?,?,?,?,?,?)`, snapshotID, s.Name, s.Qualified, s.Kind, s.Path, s.Line, s.EndLine, s.Signature); err != nil {
			return err
		}
	}
	for _, e := range batch.Edges {
		if _, err = tx.Exec(`INSERT INTO edges(snapshot_id,source,target,kind,confidence,path,line) VALUES(?,?,?,?,?,?,?)`, snapshotID, e.Source, e.Target, e.Kind, e.Confidence, e.Path, e.Line); err != nil {
			return err
		}
	}
	for _, x := range batch.XML {
		if _, err = tx.Exec(`INSERT INTO xml_nodes(snapshot_id,name,kind,path,line,attributes) VALUES(?,?,?,?,?,?)`, snapshotID, x.Name, x.Kind, x.Path, x.Line, x.Attributes); err != nil {
			return err
		}
	}
	for _, v := range batch.TOC {
		if _, err = tx.Exec(`INSERT INTO toc_entries(snapshot_id,path,line,key,value) VALUES(?,?,?,?,?)`, snapshotID, v.Path, v.Line, v.Key, v.Value); err != nil {
			return err
		}
	}
	for _, d := range batch.Search {
		if _, err = tx.Exec(`INSERT INTO search_docs(snapshot_id,path,line,kind,name,text,role) VALUES(?,?,?,?,?,?,?)`, snapshotID, d.Path, d.Line, d.Kind, d.Name, d.Text, d.Role); err != nil {
			return err
		}
	}
	for _, a := range batch.Assets {
		if _, err = tx.Exec(`INSERT OR IGNORE INTO assets(content_hash,git_oid,extension,mime,bytes,width,height,format) VALUES(?,?,?,?,?,?,?,?)`, a.ContentHash, a.GitOID, a.Extension, a.MIME, a.Size, a.Width, a.Height, a.Format); err != nil {
			return err
		}
		if _, err = tx.Exec(`INSERT INTO snapshot_assets(snapshot_id,path,normalized_path,content_hash) VALUES(?,?,?,?)`, snapshotID, a.Path, a.NormalizedPath, a.ContentHash); err != nil {
			return err
		}
	}
	for _, a := range batch.AssetRefs {
		if _, err = tx.Exec(`INSERT INTO asset_refs(snapshot_id,source_path,line,kind,value,normalized_value) VALUES(?,?,?,?,?,?)`, snapshotID, a.SourcePath, a.Line, a.Kind, a.Value, a.NormalizedValue); err != nil {
			return err
		}
	}
	if _, err = tx.Exec(`UPDATE snapshots SET status='ready',published_at=datetime('now') WHERE id=?`, snapshotID); err != nil {
		return err
	}
	if _, err = tx.Exec(`INSERT INTO branch_state(key,value) VALUES('active_snapshot',?) ON CONFLICT(key) DO UPDATE SET value=excluded.value`, snapshotID); err != nil {
		return err
	}
	return tx.Commit()
}

func ensureParent(path string) error { return mkdirAll(filepath.Dir(path)) }

const branchSchema = `
CREATE TABLE IF NOT EXISTS branch_state(key TEXT PRIMARY KEY,value TEXT NOT NULL);
CREATE TABLE IF NOT EXISTS snapshots(id TEXT PRIMARY KEY,commit_hash TEXT NOT NULL UNIQUE,requested_ref TEXT NOT NULL,tag TEXT,status TEXT NOT NULL,created_at TEXT NOT NULL,published_at TEXT,parser_schema TEXT NOT NULL DEFAULT '',index_schema TEXT NOT NULL DEFAULT '');
CREATE TABLE IF NOT EXISTS files(content_hash TEXT PRIMARY KEY,ast_hash TEXT,language TEXT NOT NULL,bytes INTEGER NOT NULL);
CREATE TABLE IF NOT EXISTS snapshot_files(snapshot_id TEXT NOT NULL,path TEXT NOT NULL,content_hash TEXT NOT NULL,role TEXT NOT NULL,PRIMARY KEY(snapshot_id,path));
CREATE TABLE IF NOT EXISTS symbols(id INTEGER PRIMARY KEY,snapshot_id TEXT NOT NULL,name TEXT NOT NULL,qualified_name TEXT NOT NULL,kind TEXT NOT NULL,path TEXT NOT NULL,line INTEGER NOT NULL,end_line INTEGER NOT NULL DEFAULT 0,signature TEXT);
CREATE INDEX IF NOT EXISTS symbols_exact ON symbols(snapshot_id,qualified_name);
CREATE INDEX IF NOT EXISTS symbols_name ON symbols(snapshot_id,name);
CREATE TABLE IF NOT EXISTS edges(id INTEGER PRIMARY KEY,snapshot_id TEXT NOT NULL,source TEXT,target TEXT NOT NULL,kind TEXT NOT NULL,confidence TEXT NOT NULL,path TEXT NOT NULL,line INTEGER NOT NULL);
CREATE INDEX IF NOT EXISTS edges_target ON edges(snapshot_id,target);
CREATE TABLE IF NOT EXISTS xml_nodes(id INTEGER PRIMARY KEY,snapshot_id TEXT NOT NULL,name TEXT,kind TEXT NOT NULL,path TEXT NOT NULL,line INTEGER NOT NULL,attributes TEXT);
CREATE TABLE IF NOT EXISTS toc_entries(id INTEGER PRIMARY KEY,snapshot_id TEXT NOT NULL,path TEXT NOT NULL,line INTEGER NOT NULL,key TEXT NOT NULL,value TEXT NOT NULL);
CREATE TABLE IF NOT EXISTS search_docs(id INTEGER PRIMARY KEY,snapshot_id TEXT NOT NULL,path TEXT NOT NULL,line INTEGER NOT NULL,kind TEXT NOT NULL,name TEXT,text TEXT NOT NULL,role TEXT NOT NULL);
CREATE VIRTUAL TABLE IF NOT EXISTS search_fts USING fts5(name,text,content='search_docs',content_rowid='id',tokenize='unicode61 tokenchars ''._:''');
CREATE TRIGGER IF NOT EXISTS search_ai AFTER INSERT ON search_docs BEGIN INSERT INTO search_fts(rowid,name,text) VALUES(new.id,new.name,new.text); END;
CREATE TRIGGER IF NOT EXISTS search_ad AFTER DELETE ON search_docs BEGIN INSERT INTO search_fts(search_fts,rowid,name,text) VALUES('delete',old.id,old.name,old.text); END;
CREATE TABLE IF NOT EXISTS assets(content_hash TEXT PRIMARY KEY,git_oid TEXT,extension TEXT,mime TEXT,bytes INTEGER,width INTEGER,height INTEGER,format TEXT);
CREATE TABLE IF NOT EXISTS snapshot_assets(snapshot_id TEXT NOT NULL,path TEXT NOT NULL,normalized_path TEXT NOT NULL,content_hash TEXT NOT NULL,PRIMARY KEY(snapshot_id,path));
CREATE INDEX IF NOT EXISTS asset_path ON snapshot_assets(snapshot_id,normalized_path);
CREATE TABLE IF NOT EXISTS asset_refs(id INTEGER PRIMARY KEY,snapshot_id TEXT NOT NULL,source_path TEXT NOT NULL,line INTEGER NOT NULL,kind TEXT NOT NULL,value TEXT NOT NULL,normalized_value TEXT NOT NULL);
`
