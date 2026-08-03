package store

import (
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
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
	Path, ContentHash, ASTHash, Language, Role, GitOID string
	Size                                               int64
}
type SymbolFact struct {
	Name, Qualified, Kind, Path, Signature, RequiredRole string
	Line, EndLine                                        int
}
type EdgeFact struct {
	Source, Target, Kind, Confidence, Path, RequiredRole string
	Line                                                 int
}
type XMLFact struct {
	Name, Kind, Path, RequiredRole string
	Line                           int
	Attributes                     string
}
type TOCFact struct {
	Path, Key, Value, RequiredRole string
	Line                           int
}
type SearchFact struct {
	Path, Kind, Name, Text, RequiredRole string
	Line                                 int
}
type AssetFact struct {
	Path, NormalizedPath, ContentHash, GitOID, Extension, MIME, Format string
	Size                                                               int64
	Width, Height                                                      int
}
type AssetRefFact struct {
	SourcePath, Value, NormalizedValue, Kind, RequiredRole string
	Line                                                   int
}

type ContentRecord struct {
	ID                             int64
	ContentHash, ASTHash, Language string
	Size                           int64
	Asset                          *AssetFact
}

func BranchPath(layout home.Layout, sourceID, productID, parserSchema, indexSchema string) string {
	sum := sha256.Sum256([]byte(parserSchema + "\x00" + indexSchema))
	generation := hex.EncodeToString(sum[:])[:12]
	return filepath.Join(layout.Indexes, sourceID, productID+"-"+generation+".sqlite")
}

func OpenBranch(layout home.Layout, sourceID, productID, parserSchema, indexSchema string) (*Branch, error) {
	path := BranchPath(layout, sourceID, productID, parserSchema, indexSchema)
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
	return &Branch{DB: db, Path: path}, nil
}

func OpenBranchRead(layout home.Layout, sourceID, productID, parserSchema, indexSchema string) (*Branch, error) {
	return OpenBranchReadPath(BranchPath(layout, sourceID, productID, parserSchema, indexSchema))
}

func OpenBranchReadPath(path string) (*Branch, error) {
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
		{&summary.Lua, `SELECT count(*) FROM snapshot_files sf JOIN contents c ON c.id=sf.content_id WHERE sf.snapshot_id=? AND c.language='lua'`},
		{&summary.XML, `SELECT count(*) FROM snapshot_files sf JOIN contents c ON c.id=sf.content_id WHERE sf.snapshot_id=? AND c.language='xml'`},
		{&summary.TOC, `SELECT count(*) FROM snapshot_files sf JOIN contents c ON c.id=sf.content_id WHERE sf.snapshot_id=? AND c.language='toc'`},
		{&summary.Assets, `SELECT count(*) FROM snapshot_assets WHERE snapshot_id=?`},
		{&summary.AST, `SELECT count(*) FROM snapshot_files sf JOIN contents c ON c.id=sf.content_id WHERE sf.snapshot_id=? AND c.ast_hash<>''`},
	}
	for _, item := range queries {
		if err := b.DB.QueryRow(item.query, snapshotID).Scan(item.value); err != nil {
			return SnapshotSummary{}, false, err
		}
	}
	return summary, true, nil
}

func (b *Branch) LookupOID(oid, language, parserSchema, indexSchema string) (ContentRecord, bool, error) {
	return b.lookupContent(`JOIN blob_aliases ba ON ba.content_id=c.id WHERE ba.git_oid=? AND ba.language=? AND c.parser_schema=? AND c.index_schema=?`, oid, language, parserSchema, indexSchema)
}

func (b *Branch) LookupHash(hash, language, parserSchema, indexSchema string) (ContentRecord, bool, error) {
	return b.lookupContent(`WHERE c.content_hash=? AND c.language=? AND c.parser_schema=? AND c.index_schema=?`, hash, language, parserSchema, indexSchema)
}

func (b *Branch) lookupContent(clause string, args ...any) (ContentRecord, bool, error) {
	var record ContentRecord
	var ast sql.NullString
	var ext, mime, format sql.NullString
	var assetBytes, width, height sql.NullInt64
	err := b.DB.QueryRow(`SELECT c.id,c.content_hash,c.ast_hash,c.language,c.bytes,a.extension,a.mime,a.bytes,a.width,a.height,a.format FROM contents c LEFT JOIN assets a ON a.content_id=c.id `+clause, args...).Scan(&record.ID, &record.ContentHash, &ast, &record.Language, &record.Size, &ext, &mime, &assetBytes, &width, &height, &format)
	if err == sql.ErrNoRows {
		return ContentRecord{}, false, nil
	}
	if err != nil {
		return ContentRecord{}, false, err
	}
	record.ASTHash = ast.String
	if ext.Valid {
		record.Asset = &AssetFact{ContentHash: record.ContentHash, Extension: ext.String, MIME: mime.String, Size: assetBytes.Int64, Width: int(width.Int64), Height: int(height.Int64), Format: format.String}
	}
	return record, true, nil
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
	for _, table := range []string{"snapshot_assets", "snapshot_files"} {
		if _, err = tx.Exec(`DELETE FROM `+table+` WHERE snapshot_id=?`, snapshotID); err != nil {
			return err
		}
	}
	pathContent := make(map[string]int64, len(batch.Files))
	newContent := make(map[int64]bool)
	factPath := make(map[int64]string)
	for _, f := range batch.Files {
		result, execErr := tx.Exec(`INSERT OR IGNORE INTO contents(content_hash,parser_schema,index_schema,ast_hash,language,bytes) VALUES(?,?,?,?,?,?)`, f.ContentHash, parserSchema, indexSchema, f.ASTHash, f.Language, f.Size)
		if execErr != nil {
			return execErr
		}
		var contentID int64
		if err = tx.QueryRow(`SELECT id FROM contents WHERE content_hash=? AND language=? AND parser_schema=? AND index_schema=?`, f.ContentHash, f.Language, parserSchema, indexSchema).Scan(&contentID); err != nil {
			return err
		}
		if affected, _ := result.RowsAffected(); affected > 0 {
			newContent[contentID] = true
			factPath[contentID] = f.Path
		}
		pathContent[f.Path] = contentID
		if f.GitOID != "" {
			if _, err = tx.Exec(`INSERT OR IGNORE INTO blob_aliases(git_oid,language,content_id) VALUES(?,?,?)`, f.GitOID, f.Language, contentID); err != nil {
				return err
			}
		}
		if _, err = tx.Exec(`INSERT INTO snapshot_files(snapshot_id,path,content_id,role) VALUES(?,?,?,?)`, snapshotID, f.Path, contentID, f.Role); err != nil {
			return err
		}
	}
	for _, s := range batch.Symbols {
		if id := pathContent[s.Path]; id != 0 && newContent[id] && factPath[id] == s.Path {
			if _, err = tx.Exec(`INSERT INTO symbols(content_id,name,qualified_name,kind,line,end_line,signature,required_role) VALUES(?,?,?,?,?,?,?,?)`, id, s.Name, s.Qualified, s.Kind, s.Line, s.EndLine, s.Signature, s.RequiredRole); err != nil {
				return err
			}
		}
	}
	for _, e := range batch.Edges {
		if id := pathContent[e.Path]; id != 0 && newContent[id] && factPath[id] == e.Path {
			if _, err = tx.Exec(`INSERT INTO edges(content_id,source,target,kind,confidence,line,required_role) VALUES(?,?,?,?,?,?,?)`, id, e.Source, e.Target, e.Kind, e.Confidence, e.Line, e.RequiredRole); err != nil {
				return err
			}
		}
	}
	for _, x := range batch.XML {
		if id := pathContent[x.Path]; id != 0 && newContent[id] && factPath[id] == x.Path {
			if _, err = tx.Exec(`INSERT INTO xml_nodes(content_id,name,kind,line,attributes,required_role) VALUES(?,?,?,?,?,?)`, id, x.Name, x.Kind, x.Line, x.Attributes, x.RequiredRole); err != nil {
				return err
			}
		}
	}
	for _, v := range batch.TOC {
		if id := pathContent[v.Path]; id != 0 && newContent[id] && factPath[id] == v.Path {
			if _, err = tx.Exec(`INSERT INTO toc_entries(content_id,line,key,value,required_role) VALUES(?,?,?,?,?)`, id, v.Line, v.Key, v.Value, v.RequiredRole); err != nil {
				return err
			}
		}
	}
	for _, d := range batch.Search {
		if id := pathContent[d.Path]; id != 0 && newContent[id] && factPath[id] == d.Path {
			if _, err = tx.Exec(`INSERT INTO search_docs(content_id,line,kind,name,text,required_role) VALUES(?,?,?,?,?,?)`, id, d.Line, d.Kind, d.Name, d.Text, d.RequiredRole); err != nil {
				return err
			}
		}
	}
	for _, a := range batch.Assets {
		contentID := pathContent[a.Path]
		if contentID == 0 {
			continue
		}
		if _, err = tx.Exec(`INSERT OR IGNORE INTO assets(content_id,extension,mime,bytes,width,height,format) VALUES(?,?,?,?,?,?,?)`, contentID, a.Extension, a.MIME, a.Size, a.Width, a.Height, a.Format); err != nil {
			return err
		}
		if _, err = tx.Exec(`INSERT INTO snapshot_assets(snapshot_id,path,normalized_path,content_id) VALUES(?,?,?,?)`, snapshotID, a.Path, a.NormalizedPath, contentID); err != nil {
			return err
		}
	}
	for _, a := range batch.AssetRefs {
		if id := pathContent[a.SourcePath]; id != 0 && newContent[id] && factPath[id] == a.SourcePath {
			if _, err = tx.Exec(`INSERT INTO asset_refs(content_id,line,kind,value,normalized_value,required_role) VALUES(?,?,?,?,?,?)`, id, a.Line, a.Kind, a.Value, a.NormalizedValue, a.RequiredRole); err != nil {
				return err
			}
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
PRAGMA user_version=4;
CREATE TABLE IF NOT EXISTS branch_state(key TEXT PRIMARY KEY,value TEXT NOT NULL);
CREATE TABLE IF NOT EXISTS snapshots(id TEXT PRIMARY KEY,commit_hash TEXT NOT NULL UNIQUE,requested_ref TEXT NOT NULL,tag TEXT,status TEXT NOT NULL,created_at TEXT NOT NULL,published_at TEXT,parser_schema TEXT NOT NULL,index_schema TEXT NOT NULL);
CREATE TABLE IF NOT EXISTS contents(id INTEGER PRIMARY KEY,content_hash TEXT NOT NULL,parser_schema TEXT NOT NULL,index_schema TEXT NOT NULL,ast_hash TEXT NOT NULL DEFAULT '',language TEXT NOT NULL,bytes INTEGER NOT NULL,UNIQUE(content_hash,language,parser_schema,index_schema));
CREATE TABLE IF NOT EXISTS blob_aliases(git_oid TEXT NOT NULL,language TEXT NOT NULL,content_id INTEGER NOT NULL REFERENCES contents(id),PRIMARY KEY(git_oid,language));
CREATE TABLE IF NOT EXISTS snapshot_files(snapshot_id TEXT NOT NULL REFERENCES snapshots(id),path TEXT NOT NULL,content_id INTEGER NOT NULL REFERENCES contents(id),role TEXT NOT NULL,PRIMARY KEY(snapshot_id,path));
CREATE INDEX IF NOT EXISTS snapshot_files_content ON snapshot_files(snapshot_id,content_id);
CREATE INDEX IF NOT EXISTS content_snapshots ON snapshot_files(content_id,snapshot_id);
CREATE TABLE IF NOT EXISTS symbols(id INTEGER PRIMARY KEY,content_id INTEGER NOT NULL REFERENCES contents(id),name TEXT NOT NULL,qualified_name TEXT NOT NULL,kind TEXT NOT NULL,line INTEGER NOT NULL,end_line INTEGER NOT NULL DEFAULT 0,signature TEXT,required_role TEXT NOT NULL DEFAULT '');
CREATE INDEX IF NOT EXISTS symbols_exact ON symbols(content_id,qualified_name);
CREATE INDEX IF NOT EXISTS symbols_name ON symbols(content_id,name);
CREATE TABLE IF NOT EXISTS edges(id INTEGER PRIMARY KEY,content_id INTEGER NOT NULL REFERENCES contents(id),source TEXT,target TEXT NOT NULL,kind TEXT NOT NULL,confidence TEXT NOT NULL,line INTEGER NOT NULL,required_role TEXT NOT NULL DEFAULT '');
CREATE INDEX IF NOT EXISTS edges_target ON edges(content_id,target);
CREATE TABLE IF NOT EXISTS xml_nodes(id INTEGER PRIMARY KEY,content_id INTEGER NOT NULL REFERENCES contents(id),name TEXT,kind TEXT NOT NULL,line INTEGER NOT NULL,attributes TEXT,required_role TEXT NOT NULL DEFAULT '');
CREATE TABLE IF NOT EXISTS toc_entries(id INTEGER PRIMARY KEY,content_id INTEGER NOT NULL REFERENCES contents(id),line INTEGER NOT NULL,key TEXT NOT NULL,value TEXT NOT NULL,required_role TEXT NOT NULL DEFAULT '');
CREATE TABLE IF NOT EXISTS search_docs(id INTEGER PRIMARY KEY,content_id INTEGER NOT NULL REFERENCES contents(id),line INTEGER NOT NULL,kind TEXT NOT NULL,name TEXT,text TEXT NOT NULL,required_role TEXT NOT NULL DEFAULT '');
CREATE INDEX IF NOT EXISTS search_docs_content ON search_docs(content_id);
CREATE VIRTUAL TABLE IF NOT EXISTS search_fts USING fts5(name,text,content='search_docs',content_rowid='id',tokenize='unicode61 tokenchars ''._:''');
CREATE TRIGGER IF NOT EXISTS search_ai AFTER INSERT ON search_docs BEGIN INSERT INTO search_fts(rowid,name,text) VALUES(new.id,new.name,new.text); END;
CREATE TRIGGER IF NOT EXISTS search_ad AFTER DELETE ON search_docs BEGIN INSERT INTO search_fts(search_fts,rowid,name,text) VALUES('delete',old.id,old.name,old.text); END;
CREATE TABLE IF NOT EXISTS assets(content_id INTEGER PRIMARY KEY REFERENCES contents(id),extension TEXT,mime TEXT,bytes INTEGER,width INTEGER,height INTEGER,format TEXT);
CREATE TABLE IF NOT EXISTS snapshot_assets(snapshot_id TEXT NOT NULL REFERENCES snapshots(id),path TEXT NOT NULL,normalized_path TEXT NOT NULL,content_id INTEGER NOT NULL REFERENCES contents(id),PRIMARY KEY(snapshot_id,path));
CREATE INDEX IF NOT EXISTS asset_path ON snapshot_assets(snapshot_id,normalized_path);
CREATE TABLE IF NOT EXISTS asset_refs(id INTEGER PRIMARY KEY,content_id INTEGER NOT NULL REFERENCES contents(id),line INTEGER NOT NULL,kind TEXT NOT NULL,value TEXT NOT NULL,normalized_value TEXT NOT NULL,required_role TEXT NOT NULL DEFAULT '');
`
