package objectstore

import (
	"bytes"
	"compress/gzip"
	"crypto/sha256"
	"database/sql"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/follenfang/wowdoc/internal/home"
	_ "modernc.org/sqlite"
)

type Kind string

const (
	Source Kind = "source"
	Asset  Kind = "asset"
	AST    Kind = "ast"

	packMagic             = "WOWPACK1"
	packHeaderSize        = 8
	recordHeaderSize      = 52
	codecRaw         byte = 0
	codecGzip        byte = 1
)

type pendingRecord struct {
	Kind, Hash                             string
	Offset, CompressedBytes, OriginalBytes int64
	Codec                                  byte
}

var catalogMu sync.Mutex
var catalogCache = map[string]map[string]recordLocation{}

// Store accumulates one build's new objects in a single sequential staging pack.
type Store struct {
	Layout home.Layout
	TaskID string

	mu          sync.Mutex
	file        *os.File
	stagingPath string
	records     []pendingRecord
	local       map[string]bool
	published   bool
}

func New(layout home.Layout, taskID string) *Store {
	return &Store{Layout: layout, TaskID: sanitize(taskID), local: map[string]bool{}}
}

func Hash(data []byte) string { sum := sha256.Sum256(data); return hex.EncodeToString(sum[:]) }

func (s *Store) PutSource(data []byte) (string, string, bool, error) {
	return s.put(Source, data, true)
}

func (s *Store) PutAsset(data []byte) (string, string, bool, error) {
	return s.put(Asset, data, false)
}

func (s *Store) PutAST(schema, contentHash string, value any) (string, string, bool, error) {
	data, err := json.Marshal(value)
	if err != nil {
		return "", "", false, err
	}
	hash, locator, reused, err := s.put(AST, data, true)
	_ = schema
	_ = contentHash
	return hash, locator, reused, err
}

func (s *Store) put(kind Kind, data []byte, compress bool) (string, string, bool, error) {
	hash := Hash(data)
	key := string(kind) + ":" + hash
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.published {
		return "", "", false, fmt.Errorf("object pack is already published")
	}
	if s.local[key] {
		return hash, "pack:staging", true, nil
	}
	if location, ok, err := lookup(s.Layout, kind, hash); err != nil {
		return "", "", false, err
	} else if ok {
		s.local[key] = true
		return hash, location, true, nil
	}
	payload := data
	codec := codecRaw
	if compress {
		var err error
		payload, err = gzipBytes(data)
		if err != nil {
			return "", "", false, err
		}
		codec = codecGzip
	}
	if err := s.openStaging(); err != nil {
		return "", "", false, err
	}
	offset, err := s.file.Seek(0, io.SeekCurrent)
	if err != nil {
		return "", "", false, err
	}
	header, err := encodeRecordHeader(kind, codec, hash, int64(len(data)), int64(len(payload)))
	if err != nil {
		return "", "", false, err
	}
	if _, err = s.file.Write(header); err == nil {
		_, err = s.file.Write(payload)
	}
	if err != nil {
		return "", "", false, err
	}
	s.records = append(s.records, pendingRecord{Kind: string(kind), Hash: hash, Offset: offset, CompressedBytes: int64(len(payload)), OriginalBytes: int64(len(data)), Codec: codec})
	s.local[key] = true
	return hash, "pack:staging", false, nil
}

func (s *Store) openStaging() error {
	if s.file != nil {
		return nil
	}
	if err := os.MkdirAll(filepath.Join(s.Layout.Temp, "packs"), 0o755); err != nil {
		return err
	}
	file, err := os.CreateTemp(filepath.Join(s.Layout.Temp, "packs"), s.TaskID+"-*.pack.tmp")
	if err != nil {
		return err
	}
	if _, err = file.Write([]byte(packMagic)); err != nil {
		_ = file.Close()
		_ = os.Remove(file.Name())
		return err
	}
	s.file = file
	s.stagingPath = file.Name()
	return nil
}

// Publish atomically makes the immutable pack visible, then indexes its records.
func (s *Store) Publish() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.published {
		return nil
	}
	if s.file == nil {
		s.published = true
		return nil
	}
	if err := s.file.Sync(); err != nil {
		return err
	}
	if err := s.file.Close(); err != nil {
		return err
	}
	s.file = nil
	if err := os.MkdirAll(s.Layout.Packs, 0o755); err != nil {
		return err
	}
	packID := strings.TrimSuffix(filepath.Base(s.stagingPath), ".pack.tmp")
	packID = strings.TrimSuffix(packID, ".tmp")
	finalPath := filepath.Join(s.Layout.Packs, packID+".pack")
	if err := publishIndex(s.Layout, packID, s.stagingPath, finalPath, s.records); err != nil {
		return err
	}
	s.stagingPath = ""
	s.published = true
	return nil
}

func (s *Store) Abort() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	var err error
	if s.file != nil {
		err = s.file.Close()
		s.file = nil
	}
	if s.stagingPath != "" {
		if removeErr := os.Remove(s.stagingPath); err == nil && !os.IsNotExist(removeErr) {
			err = removeErr
		}
		s.stagingPath = ""
	}
	return err
}

func Exists(layout home.Layout, kind Kind, hash string) bool {
	if _, ok, err := lookup(layout, kind, hash); err == nil && ok {
		return true
	}
	_, err := os.Stat(legacyPath(layout, kind, hash))
	return err == nil
}

func Open(layout home.Layout, kind Kind, hash string) (io.ReadCloser, error) {
	location, ok, err := lookupRecord(layout, kind, hash)
	if err != nil {
		return nil, err
	}
	if ok {
		return openPackRecord(location, kind, hash)
	}
	return OpenSource(legacyPath(layout, kind, hash))
}

func OpenAST(layout home.Layout, schema, contentHash, astHash string) (io.ReadCloser, error) {
	if location, ok, err := lookupRecord(layout, AST, astHash); err != nil {
		return nil, err
	} else if ok {
		return openPackRecord(location, AST, astHash)
	}
	if len(contentHash) < 2 {
		return nil, fmt.Errorf("invalid AST content hash")
	}
	return OpenSource(filepath.Join(layout.AST, schema, contentHash[:2], contentHash+".json"))
}

func Location(layout home.Layout, kind Kind, hash string) string {
	if location, ok, err := lookup(layout, kind, hash); err == nil && ok {
		return location
	}
	return legacyPath(layout, kind, hash)
}

type recordLocation struct {
	Path                                   string
	Offset, CompressedBytes, OriginalBytes int64
	Codec                                  byte
}

func lookup(layout home.Layout, kind Kind, hash string) (string, bool, error) {
	r, ok, err := lookupRecord(layout, kind, hash)
	if !ok || err != nil {
		return "", ok, err
	}
	return fmt.Sprintf("%s#%d", r.Path, r.Offset), true, nil
}

func lookupRecord(layout home.Layout, kind Kind, hash string) (recordLocation, bool, error) {
	catalogMu.Lock()
	defer catalogMu.Unlock()
	path := catalogPath(layout)
	key := string(kind) + ":" + hash
	if cached, loaded := catalogCache[path]; loaded {
		r, ok := cached[key]
		return r, ok, nil
	}
	if _, err := os.Stat(path); err != nil {
		if os.IsNotExist(err) {
			catalogCache[path] = map[string]recordLocation{}
			return recordLocation{}, false, nil
		}
		return recordLocation{}, false, err
	}
	db, err := sql.Open("sqlite", "file:"+filepath.ToSlash(path)+"?mode=ro")
	if err != nil {
		return recordLocation{}, false, err
	}
	defer db.Close()
	db.SetMaxOpenConns(1)
	if _, err = db.Exec("PRAGMA busy_timeout=30000"); err != nil {
		return recordLocation{}, false, err
	}
	rows, err := db.Query(`SELECT o.kind,o.hash,p.path,o.offset,o.compressed_bytes,o.original_bytes,o.codec FROM objects o JOIN packs p ON p.id=o.pack_id`)
	if err != nil {
		return recordLocation{}, false, err
	}
	loadedRecords := map[string]recordLocation{}
	for rows.Next() {
		var objectKind, objectHash string
		var codec int
		var item recordLocation
		if err = rows.Scan(&objectKind, &objectHash, &item.Path, &item.Offset, &item.CompressedBytes, &item.OriginalBytes, &codec); err != nil {
			rows.Close()
			return recordLocation{}, false, err
		}
		item.Codec = byte(codec)
		loadedRecords[objectKind+":"+objectHash] = item
	}
	if err = rows.Close(); err != nil {
		return recordLocation{}, false, err
	}
	catalogCache[path] = loadedRecords
	r, ok := loadedRecords[key]
	return r, ok, nil
}

func publishIndex(layout home.Layout, packID, stagingPath, finalPath string, records []pendingRecord) error {
	catalogMu.Lock()
	defer catalogMu.Unlock()
	if err := os.MkdirAll(layout.Packs, 0o755); err != nil {
		return err
	}
	db, err := sql.Open("sqlite", "file:"+filepath.ToSlash(catalogPath(layout))+"?_txlock=immediate")
	if err != nil {
		return err
	}
	defer db.Close()
	db.SetMaxOpenConns(1)
	if _, err = db.Exec("PRAGMA busy_timeout=30000"); err != nil {
		return err
	}
	var journalMode string
	if err = db.QueryRow("PRAGMA journal_mode").Scan(&journalMode); err != nil {
		return err
	}
	pragmas := []string{"PRAGMA synchronous=FULL"}
	if !strings.EqualFold(journalMode, "wal") {
		pragmas = append([]string{"PRAGMA journal_mode=WAL"}, pragmas...)
	}
	for _, pragma := range pragmas {
		if _, err = db.Exec(pragma); err != nil {
			return err
		}
	}
	if _, err = db.Exec(packSchema); err != nil {
		return err
	}
	tx, err := db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	lookupStmt, err := tx.Prepare(`SELECT 1 FROM objects WHERE kind=? AND hash=?`)
	if err != nil {
		return err
	}
	defer lookupStmt.Close()
	unique := make([]pendingRecord, 0, len(records))
	for _, r := range records {
		var present int
		err = lookupStmt.QueryRow(r.Kind, r.Hash).Scan(&present)
		if err == sql.ErrNoRows {
			unique = append(unique, r)
			continue
		}
		if err != nil {
			return err
		}
	}
	if len(unique) == 0 {
		if err = tx.Commit(); err != nil {
			return err
		}
		return os.Remove(stagingPath)
	}
	packPath := stagingPath
	if len(unique) != len(records) {
		packPath, unique, err = compactPack(stagingPath, unique)
		if err != nil {
			return err
		}
	}
	if err = os.Rename(packPath, finalPath); err != nil {
		if packPath != stagingPath {
			_ = os.Remove(packPath)
		}
		return err
	}
	if packPath != stagingPath {
		_ = os.Remove(stagingPath)
	}
	info, err := os.Stat(finalPath)
	if err != nil {
		return err
	}
	if _, err = tx.Exec(`INSERT INTO packs(id,path,bytes,created_at) VALUES(?,?,?,?)`, packID, finalPath, info.Size(), time.Now().UTC().Format(time.RFC3339)); err != nil {
		return err
	}
	stmt, err := tx.Prepare(`INSERT INTO objects(kind,hash,pack_id,offset,compressed_bytes,original_bytes,codec) VALUES(?,?,?,?,?,?,?)`)
	if err != nil {
		return err
	}
	for _, r := range unique {
		if _, err = stmt.Exec(r.Kind, r.Hash, packID, r.Offset, r.CompressedBytes, r.OriginalBytes, r.Codec); err != nil {
			stmt.Close()
			return err
		}
	}
	if err = stmt.Close(); err != nil {
		return err
	}
	if err = tx.Commit(); err != nil {
		return err
	}
	cached := catalogCache[catalogPath(layout)]
	if cached == nil {
		cached = map[string]recordLocation{}
		catalogCache[catalogPath(layout)] = cached
	}
	for _, r := range unique {
		key := r.Kind + ":" + r.Hash
		if _, exists := cached[key]; !exists {
			cached[key] = recordLocation{Path: finalPath, Offset: r.Offset, CompressedBytes: r.CompressedBytes, OriginalBytes: r.OriginalBytes, Codec: r.Codec}
		}
	}
	return nil
}

func compactPack(path string, records []pendingRecord) (string, []pendingRecord, error) {
	input, err := os.Open(path)
	if err != nil {
		return "", nil, err
	}
	defer input.Close()
	compactPath := path + ".compact"
	output, err := os.Create(compactPath)
	if err != nil {
		return "", nil, err
	}
	removeOnError := true
	defer func() {
		_ = output.Close()
		if removeOnError {
			_ = os.Remove(compactPath)
		}
	}()
	if _, err = output.Write([]byte(packMagic)); err != nil {
		return "", nil, err
	}
	updated := make([]pendingRecord, 0, len(records))
	for _, record := range records {
		if _, err = input.Seek(record.Offset, io.SeekStart); err != nil {
			return "", nil, err
		}
		newOffset, err := output.Seek(0, io.SeekCurrent)
		if err != nil {
			return "", nil, err
		}
		header := make([]byte, recordHeaderSize)
		if _, err = io.ReadFull(input, header); err != nil {
			return "", nil, err
		}
		if _, err = output.Write(header); err != nil {
			return "", nil, err
		}
		if _, err = io.CopyN(output, input, record.CompressedBytes); err != nil {
			return "", nil, err
		}
		record.Offset = newOffset
		updated = append(updated, record)
	}
	if err = output.Sync(); err != nil {
		return "", nil, err
	}
	if err = output.Close(); err != nil {
		return "", nil, err
	}
	removeOnError = false
	return compactPath, updated, nil
}

func openPackRecord(r recordLocation, expectedKind Kind, expectedHash string) (io.ReadCloser, error) {
	file, err := os.Open(r.Path)
	if err != nil {
		return nil, err
	}
	magic := make([]byte, packHeaderSize)
	if _, err = file.ReadAt(magic, 0); err != nil || string(magic) != packMagic || r.Offset < packHeaderSize {
		_ = file.Close()
		if err != nil {
			return nil, fmt.Errorf("pack header: %w", err)
		}
		return nil, fmt.Errorf("invalid pack header")
	}
	header := make([]byte, recordHeaderSize)
	if _, err = file.ReadAt(header, r.Offset); err != nil {
		_ = file.Close()
		return nil, fmt.Errorf("pack record header: %w", err)
	}
	kind, codec, hash, original, compressed, err := decodeRecordHeader(header)
	if original < 0 || compressed < 0 {
		_ = file.Close()
		return nil, fmt.Errorf("invalid pack record length")
	}
	if err != nil || kind != expectedKind || hash != expectedHash || original != r.OriginalBytes || compressed != r.CompressedBytes || codec != r.Codec {
		_ = file.Close()
		if err != nil {
			return nil, err
		}
		return nil, fmt.Errorf("pack record metadata mismatch for %s", expectedHash)
	}
	payload := make([]byte, compressed)
	if _, err = file.ReadAt(payload, r.Offset+recordHeaderSize); err != nil {
		_ = file.Close()
		return nil, fmt.Errorf("pack record payload: %w", err)
	}
	_ = file.Close()
	data := payload
	if codec == codecGzip {
		reader, gzipErr := gzip.NewReader(bytes.NewReader(payload))
		if gzipErr != nil {
			return nil, gzipErr
		}
		data, err = io.ReadAll(reader)
		closeErr := reader.Close()
		if err == nil {
			err = closeErr
		}
		if err != nil {
			return nil, err
		}
	} else if codec != codecRaw {
		return nil, fmt.Errorf("unsupported pack codec %d", codec)
	}
	if int64(len(data)) != original || Hash(data) != expectedHash {
		return nil, fmt.Errorf("pack record hash mismatch for %s", expectedHash)
	}
	return io.NopCloser(bytes.NewReader(data)), nil
}

func encodeRecordHeader(kind Kind, codec byte, hash string, original, compressed int64) ([]byte, error) {
	hashBytes, err := hex.DecodeString(hash)
	if err != nil || len(hashBytes) != sha256.Size {
		return nil, fmt.Errorf("invalid object hash %q", hash)
	}
	header := make([]byte, recordHeaderSize)
	header[0] = kindCode(kind)
	header[1] = codec
	binary.BigEndian.PutUint64(header[4:12], uint64(original))
	binary.BigEndian.PutUint64(header[12:20], uint64(compressed))
	copy(header[20:], hashBytes)
	return header, nil
}

func decodeRecordHeader(header []byte) (Kind, byte, string, int64, int64, error) {
	if len(header) != recordHeaderSize {
		return "", 0, "", 0, 0, fmt.Errorf("invalid pack record header size")
	}
	kind := codeKind(header[0])
	if kind == "" {
		return "", 0, "", 0, 0, fmt.Errorf("invalid pack object kind")
	}
	return kind, header[1], hex.EncodeToString(header[20:52]), int64(binary.BigEndian.Uint64(header[4:12])), int64(binary.BigEndian.Uint64(header[12:20])), nil
}

func kindCode(kind Kind) byte {
	switch kind {
	case Source:
		return 1
	case Asset:
		return 2
	case AST:
		return 3
	default:
		return 0
	}
}

func codeKind(code byte) Kind {
	switch code {
	case 1:
		return Source
	case 2:
		return Asset
	case 3:
		return AST
	default:
		return ""
	}
}

func legacyPath(layout home.Layout, kind Kind, hash string) string {
	if len(hash) < 2 {
		return ""
	}
	switch kind {
	case Asset:
		return filepath.Join(layout.Assets, hash[:2], hash)
	case AST:
		return filepath.Join(layout.AST, hash[:2], hash+".json")
	default:
		return filepath.Join(layout.Objects, hash[:2], hash)
	}
}

func catalogPath(layout home.Layout) string { return filepath.Join(layout.Packs, "index.sqlite") }

func sanitize(value string) string {
	if value == "" {
		return "task"
	}
	return strings.Map(func(r rune) rune {
		if r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9' || r == '-' || r == '_' {
			return r
		}
		return '-'
	}, value)
}

func gzipBytes(data []byte) ([]byte, error) {
	var out bytes.Buffer
	w := gzip.NewWriter(&out)
	if _, err := w.Write(data); err != nil {
		return nil, err
	}
	if err := w.Close(); err != nil {
		return nil, err
	}
	return out.Bytes(), nil
}

// OpenSource opens legacy gzip-compressed and raw single-object files.
func OpenSource(path string) (io.ReadCloser, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	header := make([]byte, 2)
	n, readErr := io.ReadFull(f, header)
	if readErr != nil && readErr != io.ErrUnexpectedEOF {
		_ = f.Close()
		return nil, readErr
	}
	if n == 2 && header[0] == 0x1f && header[1] == 0x8b {
		if _, err := f.Seek(0, io.SeekStart); err != nil {
			_ = f.Close()
			return nil, err
		}
		gz, err := gzip.NewReader(f)
		if err != nil {
			_ = f.Close()
			return nil, err
		}
		return &gzipReadCloser{Reader: gz, file: f, gz: gz}, nil
	}
	if _, err := f.Seek(0, io.SeekStart); err != nil {
		_ = f.Close()
		return nil, err
	}
	return f, nil
}

type gzipReadCloser struct {
	*gzip.Reader
	file *os.File
	gz   *gzip.Reader
}

func (r *gzipReadCloser) Close() error {
	err := r.gz.Close()
	if closeErr := r.file.Close(); err == nil {
		err = closeErr
	}
	return err
}

func NormalizePath(path string) string {
	return strings.ToLower(strings.ReplaceAll(filepath.ToSlash(path), "/", "\\"))
}

const packSchema = `
CREATE TABLE IF NOT EXISTS packs(id TEXT PRIMARY KEY,path TEXT NOT NULL UNIQUE,bytes INTEGER NOT NULL,created_at TEXT NOT NULL);
CREATE TABLE IF NOT EXISTS objects(kind TEXT NOT NULL,hash TEXT NOT NULL,pack_id TEXT NOT NULL REFERENCES packs(id),offset INTEGER NOT NULL,compressed_bytes INTEGER NOT NULL,original_bytes INTEGER NOT NULL,codec INTEGER NOT NULL,PRIMARY KEY(kind,hash));
CREATE INDEX IF NOT EXISTS objects_pack ON objects(pack_id);
`
