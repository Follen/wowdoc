package store

import (
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/follenfang/wowdoc/internal/home"
	"github.com/follenfang/wowdoc/internal/lock"
	"github.com/follenfang/wowdoc/internal/objectstore"
	"github.com/follenfang/wowdoc/internal/result"
	_ "modernc.org/sqlite"
)

type Branch struct {
	DB          *sql.DB
	Content     *sql.DB
	Path        string
	ContentPath string
	contentLock string
	Layout      home.Layout
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

func generation(parserSchema, indexSchema string) string {
	sum := sha256.Sum256([]byte(parserSchema + "\x00" + indexSchema))
	return hex.EncodeToString(sum[:])[:12]
}

func BranchPath(layout home.Layout, sourceID, productID, parserSchema, indexSchema string) string {
	return filepath.Join(layout.Indexes, sourceID, productID+"-"+generation(parserSchema, indexSchema)+".sqlite")
}

func ContentPath(layout home.Layout, sourceID, parserSchema, indexSchema string) string {
	return filepath.Join(layout.Indexes, sourceID, "_content-"+generation(parserSchema, indexSchema)+".sqlite")
}

func OpenBranch(layout home.Layout, sourceID, productID, parserSchema, indexSchema string) (*Branch, error) {
	path := BranchPath(layout, sourceID, productID, parserSchema, indexSchema)
	contentPath := ContentPath(layout, sourceID, parserSchema, indexSchema)
	contentLock := sharedContentLockPath(layout, sourceID, parserSchema, indexSchema)
	if err := ensureParent(path); err != nil {
		return nil, err
	}
	guard, err := acquireSharedContentLock(contentLock)
	if err != nil {
		return nil, err
	}
	defer guard.Release()
	content, err := openSQLite(contentPath, false)
	if err != nil {
		return nil, fmt.Errorf("open shared content DB: %w", err)
	}
	if _, err = content.Exec(contentSchema); err != nil {
		content.Close()
		return nil, fmt.Errorf("initialize shared content schema: %w", err)
	}
	db, err := openSQLite(path, false)
	if err != nil {
		content.Close()
		return nil, fmt.Errorf("open branch DB: %w", err)
	}
	if err = attachContent(db, contentPath, false); err != nil {
		db.Close()
		content.Close()
		return nil, fmt.Errorf("attach shared content DB: %w", err)
	}
	if _, err = db.Exec(branchSchema); err != nil {
		db.Close()
		content.Close()
		return nil, fmt.Errorf("initialize branch schema: %w", err)
	}
	return &Branch{DB: db, Content: content, Path: path, ContentPath: contentPath, contentLock: contentLock, Layout: layout}, nil
}

func OpenBranchRead(layout home.Layout, sourceID, productID, parserSchema, indexSchema string) (*Branch, error) {
	return OpenBranchReadPath(layout, sourceID, BranchPath(layout, sourceID, productID, parserSchema, indexSchema), parserSchema, indexSchema)
}

func OpenBranchReadPath(layout home.Layout, sourceID, path, parserSchema, indexSchema string) (*Branch, error) {
	if _, err := os.Stat(path); err != nil {
		return nil, err
	}
	contentPath := ContentPath(layout, sourceID, parserSchema, indexSchema)
	content, err := openSQLite(contentPath, true)
	if err != nil {
		return nil, err
	}
	db, err := openSQLite(path, true)
	if err != nil {
		content.Close()
		return nil, err
	}
	if err = attachContent(db, contentPath, true); err != nil {
		db.Close()
		content.Close()
		return nil, err
	}
	return &Branch{DB: db, Content: content, Path: path, ContentPath: contentPath, contentLock: sharedContentLockPath(layout, sourceID, parserSchema, indexSchema), Layout: layout}, nil
}

func openSQLite(path string, readOnly bool) (*sql.DB, error) {
	if err := ensureParent(path); err != nil {
		return nil, err
	}
	dsn := path
	if readOnly {
		dsn = "file:" + filepath.ToSlash(path) + "?mode=ro"
	}
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, err
	}
	db.SetMaxOpenConns(1)
	db.SetMaxIdleConns(1)
	if _, err = db.Exec("PRAGMA busy_timeout=30000"); err != nil {
		db.Close()
		return nil, fmt.Errorf("set SQLite busy timeout: %w", err)
	}
	pragmas := []string{"PRAGMA foreign_keys=ON"}
	if !readOnly {
		var journalMode string
		if err = db.QueryRow("PRAGMA journal_mode").Scan(&journalMode); err != nil {
			db.Close()
			return nil, fmt.Errorf("read SQLite journal mode: %w", err)
		}
		if !strings.EqualFold(journalMode, "wal") {
			if err = db.QueryRow("PRAGMA journal_mode=WAL").Scan(&journalMode); err != nil {
				db.Close()
				return nil, fmt.Errorf("enable SQLite WAL: %w", err)
			}
		}
		pragmas = append([]string{"PRAGMA synchronous=NORMAL"}, pragmas...)
	} else {
		pragmas = append(pragmas, "PRAGMA query_only=ON")
	}
	for _, pragma := range pragmas {
		if _, err = db.Exec(pragma); err != nil {
			db.Close()
			return nil, fmt.Errorf("apply %s: %w", pragma, err)
		}
	}
	return db, nil
}

func attachContent(db *sql.DB, path string, readOnly bool) error {
	if err := ensureParent(path); err != nil {
		return err
	}
	value := filepath.ToSlash(path)
	if readOnly {
		value = "file:" + value + "?mode=ro"
	}
	_, err := db.Exec(`ATTACH DATABASE '` + strings.ReplaceAll(value, "'", "''") + `' AS content`)
	return err
}

func (b *Branch) Close() error {
	dbErr := b.DB.Close()
	contentErr := b.Content.Close()
	if dbErr != nil {
		return dbErr
	}
	return contentErr
}

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
		{&summary.Lua, `SELECT count(*) FROM snapshot_files sf JOIN content.contents c ON c.id=sf.content_id WHERE sf.snapshot_id=? AND c.language='lua'`},
		{&summary.XML, `SELECT count(*) FROM snapshot_files sf JOIN content.contents c ON c.id=sf.content_id WHERE sf.snapshot_id=? AND c.language='xml'`},
		{&summary.TOC, `SELECT count(*) FROM snapshot_files sf JOIN content.contents c ON c.id=sf.content_id WHERE sf.snapshot_id=? AND c.language='toc'`},
		{&summary.Assets, `SELECT count(*) FROM snapshot_assets WHERE snapshot_id=?`},
		{&summary.AST, `SELECT count(*) FROM snapshot_files sf JOIN content.contents c ON c.id=sf.content_id WHERE sf.snapshot_id=? AND c.ast_hash<>''`},
	}
	for _, item := range queries {
		if err := b.DB.QueryRow(item.query, snapshotID).Scan(item.value); err != nil {
			return SnapshotSummary{}, false, err
		}
	}
	return summary, true, nil
}

func (b *Branch) LookupOID(oid, language, parserSchema, indexSchema string) (ContentRecord, bool, error) {
	return b.lookupContent(`JOIN content.blob_aliases ba ON ba.content_id=c.id WHERE ba.git_oid=? AND ba.language=? AND c.parser_schema=? AND c.index_schema=?`, oid, language, parserSchema, indexSchema)
}

func (b *Branch) LookupHash(hash, language, parserSchema, indexSchema string) (ContentRecord, bool, error) {
	return b.lookupContent(`WHERE c.content_hash=? AND c.language=? AND c.parser_schema=? AND c.index_schema=?`, hash, language, parserSchema, indexSchema)
}

func (b *Branch) lookupContent(clause string, args ...any) (ContentRecord, bool, error) {
	var record ContentRecord
	var ast sql.NullString
	var ext, mime, format sql.NullString
	var assetBytes, width, height sql.NullInt64
	err := b.DB.QueryRow(`SELECT c.id,c.content_hash,c.ast_hash,c.language,c.bytes,a.extension,a.mime,a.bytes,a.width,a.height,a.format FROM content.contents c LEFT JOIN content.assets a ON a.content_id=c.id `+clause, args...).Scan(&record.ID, &record.ContentHash, &ast, &record.Language, &record.Size, &ext, &mime, &assetBytes, &width, &height, &format)
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
	guard, err := acquireSharedContentLock(b.contentLock)
	if err != nil {
		return err
	}
	defer guard.Release()
	pathContent, err := b.publishContent(parserSchema, indexSchema, batch)
	if err != nil {
		return fmt.Errorf("merge shared content facts: %w", err)
	}
	tx, err := b.DB.Begin()
	if err != nil {
		return fmt.Errorf("begin branch snapshot transaction: %w", err)
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
	for _, f := range batch.Files {
		contentID := pathContent[f.Path]
		if contentID == 0 {
			return fmt.Errorf("content mapping missing for %s", f.Path)
		}
		if _, err = tx.Exec(`INSERT INTO snapshot_files(snapshot_id,path,content_id,role) VALUES(?,?,?,?)`, snapshotID, f.Path, contentID, f.Role); err != nil {
			return err
		}
	}
	if err = b.ensureBranchFTS(tx, pathContent); err != nil {
		return err
	}
	for _, a := range batch.Assets {
		contentID := pathContent[a.Path]
		if contentID == 0 {
			continue
		}
		if _, err = tx.Exec(`INSERT INTO snapshot_assets(snapshot_id,path,normalized_path,content_id) VALUES(?,?,?,?)`, snapshotID, a.Path, a.NormalizedPath, contentID); err != nil {
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

func (b *Branch) publishContent(parserSchema, indexSchema string, batch SnapshotBatch) (map[string]int64, error) {
	tx, err := b.Content.Begin()
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()
	stringIDs := make(map[string]int64)
	rows, err := tx.Query(`SELECT id,value FROM strings`)
	if err != nil {
		return nil, err
	}
	for rows.Next() {
		var id int64
		var value string
		if err = rows.Scan(&id, &value); err != nil {
			rows.Close()
			return nil, err
		}
		stringIDs[value] = id
	}
	if err = rows.Close(); err != nil {
		return nil, err
	}
	stringID := func(value string) (int64, error) {
		if id, ok := stringIDs[value]; ok {
			return id, nil
		}
		if _, insertErr := tx.Exec(`INSERT OR IGNORE INTO strings(value) VALUES(?)`, value); insertErr != nil {
			return 0, insertErr
		}
		var id int64
		if selectErr := tx.QueryRow(`SELECT id FROM strings WHERE value=?`, value).Scan(&id); selectErr != nil {
			return 0, selectErr
		}
		stringIDs[value] = id
		return id, nil
	}
	pathContent := make(map[string]int64, len(batch.Files))
	newContent := make(map[int64]bool)
	factPath := make(map[int64]string)
	for _, f := range batch.Files {
		result, execErr := tx.Exec(`INSERT OR IGNORE INTO contents(content_hash,parser_schema,index_schema,ast_hash,language,bytes) VALUES(?,?,?,?,?,?)`, f.ContentHash, parserSchema, indexSchema, f.ASTHash, f.Language, f.Size)
		if execErr != nil {
			return nil, execErr
		}
		var contentID int64
		if err = tx.QueryRow(`SELECT id FROM contents WHERE content_hash=? AND language=? AND parser_schema=? AND index_schema=?`, f.ContentHash, f.Language, parserSchema, indexSchema).Scan(&contentID); err != nil {
			return nil, err
		}
		if affected, _ := result.RowsAffected(); affected > 0 {
			newContent[contentID] = true
			factPath[contentID] = f.Path
		}
		pathContent[f.Path] = contentID
		if f.GitOID != "" {
			if _, err = tx.Exec(`INSERT OR IGNORE INTO blob_aliases(git_oid,language,content_id) VALUES(?,?,?)`, f.GitOID, f.Language, contentID); err != nil {
				return nil, err
			}
		}
	}
	for _, s := range batch.Symbols {
		if id := pathContent[s.Path]; id != 0 && newContent[id] && factPath[id] == s.Path {
			nameID, e := stringID(s.Name)
			if e != nil {
				return nil, e
			}
			qualifiedID, e := stringID(s.Qualified)
			if e != nil {
				return nil, e
			}
			signatureID, e := stringID(s.Signature)
			if e != nil {
				return nil, e
			}
			if _, err = tx.Exec(`INSERT INTO symbols(content_id,name_id,qualified_id,kind,line,end_line,signature_id,required_role) VALUES(?,?,?,?,?,?,?,?)`, id, nameID, qualifiedID, s.Kind, s.Line, s.EndLine, signatureID, s.RequiredRole); err != nil {
				return nil, err
			}
		}
	}
	for _, e := range batch.Edges {
		if id := pathContent[e.Path]; id != 0 && newContent[id] && factPath[id] == e.Path {
			sourceID, se := stringID(e.Source)
			if se != nil {
				return nil, se
			}
			targetID, se := stringID(e.Target)
			if se != nil {
				return nil, se
			}
			if _, err = tx.Exec(`INSERT INTO edges(content_id,source_id,target_id,kind,confidence,line,required_role) VALUES(?,?,?,?,?,?,?)`, id, sourceID, targetID, e.Kind, e.Confidence, e.Line, e.RequiredRole); err != nil {
				return nil, err
			}
		}
	}
	for _, x := range batch.XML {
		if id := pathContent[x.Path]; id != 0 && newContent[id] && factPath[id] == x.Path {
			nameID, e := stringID(x.Name)
			if e != nil {
				return nil, e
			}
			kindID, e := stringID(x.Kind)
			if e != nil {
				return nil, e
			}
			attributesID, e := stringID(x.Attributes)
			if e != nil {
				return nil, e
			}
			if _, err = tx.Exec(`INSERT INTO xml_nodes(content_id,name_id,kind_id,line,attributes_id,required_role) VALUES(?,?,?,?,?,?)`, id, nameID, kindID, x.Line, attributesID, x.RequiredRole); err != nil {
				return nil, err
			}
		}
	}
	for _, v := range batch.TOC {
		if id := pathContent[v.Path]; id != 0 && newContent[id] && factPath[id] == v.Path {
			if _, err = tx.Exec(`INSERT INTO toc_entries(content_id,line,key,value,required_role) VALUES(?,?,?,?,?)`, id, v.Line, v.Key, v.Value, v.RequiredRole); err != nil {
				return nil, err
			}
		}
	}
	for _, d := range batch.Search {
		if id := pathContent[d.Path]; id != 0 && newContent[id] && factPath[id] == d.Path {
			nameID, e := stringID(d.Name)
			if e != nil {
				return nil, e
			}
			text := strings.TrimSpace(d.Name + " " + d.Text)
			if d.Kind == "source" {
				text = ""
			}
			if _, err = tx.Exec(`INSERT INTO search_docs(content_id,line,kind,name_id,text,required_role) VALUES(?,?,?,?,?,?)`, id, d.Line, d.Kind, nameID, text, d.RequiredRole); err != nil {
				return nil, err
			}
		}
	}
	for _, a := range batch.Assets {
		if id := pathContent[a.Path]; id != 0 && newContent[id] && factPath[id] == a.Path {
			if _, err = tx.Exec(`INSERT OR IGNORE INTO assets(content_id,extension,mime,bytes,width,height,format) VALUES(?,?,?,?,?,?,?)`, id, a.Extension, a.MIME, a.Size, a.Width, a.Height, a.Format); err != nil {
				return nil, err
			}
		}
	}
	for _, a := range batch.AssetRefs {
		if id := pathContent[a.SourcePath]; id != 0 && newContent[id] && factPath[id] == a.SourcePath {
			if _, err = tx.Exec(`INSERT INTO asset_refs(content_id,line,kind,value,normalized_value,required_role) VALUES(?,?,?,?,?,?)`, id, a.Line, a.Kind, a.Value, a.NormalizedValue, a.RequiredRole); err != nil {
				return nil, err
			}
		}
	}
	if err = tx.Commit(); err != nil {
		return nil, err
	}
	return pathContent, nil
}

func (b *Branch) ensureBranchFTS(tx *sql.Tx, paths map[string]int64) error {
	for _, contentID := range uniqueContentIDs(paths) {
		result, err := tx.Exec(`INSERT OR IGNORE INTO branch_contents(content_id) VALUES(?)`, contentID)
		if err != nil {
			return err
		}
		affected, _ := result.RowsAffected()
		if affected == 0 {
			continue
		}
		rows, err := b.Content.Query(`SELECT d.id,d.text,d.kind,c.content_hash FROM search_docs d JOIN contents c ON c.id=d.content_id WHERE d.content_id=? ORDER BY d.id`, contentID)
		if err != nil {
			return err
		}
		for rows.Next() {
			var id int64
			var text, kind, hash string
			if err = rows.Scan(&id, &text, &kind, &hash); err != nil {
				rows.Close()
				return err
			}
			if kind == "source" && text == "" {
				reader, openErr := objectstore.Open(b.Layout, objectstore.Source, hash)
				if openErr != nil {
					rows.Close()
					return openErr
				}
				data, readErr := io.ReadAll(reader)
				closeErr := reader.Close()
				if readErr == nil {
					readErr = closeErr
				}
				if readErr != nil {
					rows.Close()
					return readErr
				}
				text = strings.TrimSpace(string(data))
			}
			if _, err = tx.Exec(`INSERT INTO search_fts(rowid,text) VALUES(?,?)`, id, text); err != nil {
				rows.Close()
				return err
			}
		}
		if err = rows.Close(); err != nil {
			return err
		}
	}
	return nil
}

func uniqueContentIDs(paths map[string]int64) []int64 {
	seen := make(map[int64]bool, len(paths))
	out := make([]int64, 0, len(paths))
	for _, id := range paths {
		if id != 0 && !seen[id] {
			seen[id] = true
			out = append(out, id)
		}
	}
	return out
}

func ensureParent(path string) error { return mkdirAll(filepath.Dir(path)) }

func sharedContentLockPath(layout home.Layout, sourceID, parserSchema, indexSchema string) string {
	return filepath.Join(layout.Locks, "content-"+sourceID+"-"+generation(parserSchema, indexSchema)+".lock")
}

func acquireSharedContentLock(path string) (*lock.Lock, error) {
	deadline := time.Now().Add(5 * time.Minute)
	for {
		guard, err := lock.Acquire(path, "shared-content-publish", 10*time.Minute)
		if err == nil {
			return guard, nil
		}
		var appErr *result.Error
		if !result.As(err, &appErr) || appErr.Code != "operation_in_progress" || time.Now().After(deadline) {
			return nil, err
		}
		time.Sleep(50 * time.Millisecond)
	}
}

const branchSchema = `
PRAGMA user_version=6;
CREATE TABLE IF NOT EXISTS branch_state(key TEXT PRIMARY KEY,value TEXT NOT NULL);
CREATE TABLE IF NOT EXISTS snapshots(id TEXT PRIMARY KEY,commit_hash TEXT NOT NULL UNIQUE,requested_ref TEXT NOT NULL,tag TEXT,status TEXT NOT NULL,created_at TEXT NOT NULL,published_at TEXT,parser_schema TEXT NOT NULL,index_schema TEXT NOT NULL);
CREATE TABLE IF NOT EXISTS branch_contents(content_id INTEGER PRIMARY KEY);
CREATE TABLE IF NOT EXISTS snapshot_files(snapshot_id TEXT NOT NULL REFERENCES snapshots(id),path TEXT NOT NULL,content_id INTEGER NOT NULL,role TEXT NOT NULL,PRIMARY KEY(snapshot_id,path));
CREATE INDEX IF NOT EXISTS snapshot_files_content ON snapshot_files(snapshot_id,content_id);
CREATE INDEX IF NOT EXISTS content_snapshots ON snapshot_files(content_id,snapshot_id);
CREATE VIRTUAL TABLE IF NOT EXISTS search_fts USING fts5(text,content='',detail=column,tokenize='unicode61 tokenchars ''._:''');
CREATE TABLE IF NOT EXISTS snapshot_assets(snapshot_id TEXT NOT NULL REFERENCES snapshots(id),path TEXT NOT NULL,normalized_path TEXT NOT NULL,content_id INTEGER NOT NULL,PRIMARY KEY(snapshot_id,path));
CREATE INDEX IF NOT EXISTS asset_path ON snapshot_assets(snapshot_id,normalized_path);
`

const contentSchema = `
PRAGMA user_version=1;
CREATE TABLE IF NOT EXISTS contents(id INTEGER PRIMARY KEY,content_hash TEXT NOT NULL,parser_schema TEXT NOT NULL,index_schema TEXT NOT NULL,ast_hash TEXT NOT NULL DEFAULT '',language TEXT NOT NULL,bytes INTEGER NOT NULL,UNIQUE(content_hash,language,parser_schema,index_schema));
CREATE TABLE IF NOT EXISTS blob_aliases(git_oid TEXT NOT NULL,language TEXT NOT NULL,content_id INTEGER NOT NULL REFERENCES contents(id),PRIMARY KEY(git_oid,language));
CREATE TABLE IF NOT EXISTS strings(id INTEGER PRIMARY KEY,value TEXT NOT NULL UNIQUE);
CREATE TABLE IF NOT EXISTS symbols(id INTEGER PRIMARY KEY,content_id INTEGER NOT NULL REFERENCES contents(id),name_id INTEGER NOT NULL REFERENCES strings(id),qualified_id INTEGER NOT NULL REFERENCES strings(id),kind TEXT NOT NULL,line INTEGER NOT NULL,end_line INTEGER NOT NULL DEFAULT 0,signature_id INTEGER NOT NULL REFERENCES strings(id),required_role TEXT NOT NULL DEFAULT '');
CREATE INDEX IF NOT EXISTS symbols_exact ON symbols(content_id,qualified_id);
CREATE INDEX IF NOT EXISTS symbols_name ON symbols(content_id,name_id);
CREATE TABLE IF NOT EXISTS edges(id INTEGER PRIMARY KEY,content_id INTEGER NOT NULL REFERENCES contents(id),source_id INTEGER NOT NULL REFERENCES strings(id),target_id INTEGER NOT NULL REFERENCES strings(id),kind TEXT NOT NULL,confidence TEXT NOT NULL,line INTEGER NOT NULL,required_role TEXT NOT NULL DEFAULT '');
CREATE INDEX IF NOT EXISTS edges_target ON edges(content_id,target_id);
CREATE TABLE IF NOT EXISTS xml_nodes(id INTEGER PRIMARY KEY,content_id INTEGER NOT NULL REFERENCES contents(id),name_id INTEGER NOT NULL REFERENCES strings(id),kind_id INTEGER NOT NULL REFERENCES strings(id),line INTEGER NOT NULL,attributes_id INTEGER NOT NULL REFERENCES strings(id),required_role TEXT NOT NULL DEFAULT '');
CREATE TABLE IF NOT EXISTS toc_entries(id INTEGER PRIMARY KEY,content_id INTEGER NOT NULL REFERENCES contents(id),line INTEGER NOT NULL,key TEXT NOT NULL,value TEXT NOT NULL,required_role TEXT NOT NULL DEFAULT '');
CREATE TABLE IF NOT EXISTS search_docs(id INTEGER PRIMARY KEY,content_id INTEGER NOT NULL REFERENCES contents(id),line INTEGER NOT NULL,kind TEXT NOT NULL,name_id INTEGER NOT NULL REFERENCES strings(id),text TEXT NOT NULL,required_role TEXT NOT NULL DEFAULT '');
CREATE INDEX IF NOT EXISTS search_docs_content ON search_docs(content_id);
CREATE TABLE IF NOT EXISTS assets(content_id INTEGER PRIMARY KEY REFERENCES contents(id),extension TEXT,mime TEXT,bytes INTEGER,width INTEGER,height INTEGER,format TEXT);
CREATE TABLE IF NOT EXISTS asset_refs(id INTEGER PRIMARY KEY,content_id INTEGER NOT NULL REFERENCES contents(id),line INTEGER NOT NULL,kind TEXT NOT NULL,value TEXT NOT NULL,normalized_value TEXT NOT NULL,required_role TEXT NOT NULL DEFAULT '');
`
