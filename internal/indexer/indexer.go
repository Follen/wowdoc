package indexer

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/follenfang/wowdoc/internal/gitstore"
	"github.com/follenfang/wowdoc/internal/home"
	"github.com/follenfang/wowdoc/internal/lock"
	"github.com/follenfang/wowdoc/internal/objectstore"
	"github.com/follenfang/wowdoc/internal/result"
	"github.com/follenfang/wowdoc/internal/schema"
	"github.com/follenfang/wowdoc/internal/store"
)

const ParserSchema = schema.Parser
const IndexSchema = schema.Index

type Input interface {
	Entries(context.Context) ([]Entry, error)
	Read(context.Context, Entry) ([]byte, error)
	ReadRaw(context.Context, Entry) ([]byte, error)
}
type Entry struct {
	Path, OID string
	Size      int64
}
type GitInput struct {
	Manager          gitstore.Manager
	SourceID, Commit string
	ProductID        string
	worktree         *gitstore.Worktree
	blobs            *gitstore.BlobReader
}

func (g *GitInput) Entries(ctx context.Context) ([]Entry, error) {
	taskID := fmt.Sprintf("%s-%s-%.12s-%d-%d", g.SourceID, g.ProductID, g.Commit, os.Getpid(), time.Now().UnixNano())
	worktree, err := g.Manager.CreateWorktree(ctx, g.SourceID, g.Commit, taskID)
	if err != nil {
		return nil, err
	}
	g.worktree = worktree
	entries, err := (DirectoryInput{Root: worktree.Path()}).Entries(ctx)
	if err != nil {
		return nil, err
	}
	tree, err := g.Manager.Tree(ctx, g.SourceID, g.Commit)
	if err != nil {
		return nil, err
	}
	oids := make(map[string]string, len(tree))
	for _, item := range tree {
		oids[item.Path] = item.OID
	}
	for i := range entries {
		entries[i].OID = oids[entries[i].Path]
	}
	g.blobs, err = g.Manager.OpenBlobReader(ctx, g.SourceID)
	if err != nil {
		return nil, err
	}
	return entries, nil
}
func (g *GitInput) Read(_ context.Context, e Entry) ([]byte, error) {
	if g.worktree == nil {
		return nil, result.E("worktree_not_ready", "parser worktree has not been created", 5)
	}
	return os.ReadFile(filepath.Join(g.worktree.Path(), filepath.FromSlash(e.Path)))
}
func (g *GitInput) ReadRaw(ctx context.Context, e Entry) ([]byte, error) {
	if g.blobs != nil && e.OID != "" {
		return g.blobs.Read(e.OID)
	}
	return g.Manager.Blob(ctx, g.SourceID, g.Commit, e.Path)
}
func (g *GitInput) Close(ctx context.Context) error {
	var blobErr error
	if g.blobs != nil {
		blobErr = g.blobs.Close()
		g.blobs = nil
	}
	if g.worktree != nil {
		if err := g.worktree.Close(ctx); err != nil {
			return err
		}
		g.worktree = nil
	}
	return blobErr
}

type DirectoryInput struct{ Root string }

func (d DirectoryInput) Entries(_ context.Context) ([]Entry, error) {
	var out []Entry
	err := filepath.WalkDir(d.Root, func(path string, e fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if e.Name() == ".git" || strings.HasPrefix(e.Name(), ".wowdoc-task") {
			if e.IsDir() {
				return fs.SkipDir
			}
			return nil
		}
		if e.IsDir() {
			if e.Name() == ".git" {
				return fs.SkipDir
			}
			return nil
		}
		info, err := e.Info()
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(d.Root, path)
		if err != nil {
			return err
		}
		out = append(out, Entry{Path: filepath.ToSlash(rel), Size: info.Size()})
		return nil
	})
	sort.Slice(out, func(i, j int) bool { return out[i].Path < out[j].Path })
	return out, err
}
func (d DirectoryInput) Read(_ context.Context, e Entry) ([]byte, error) {
	return os.ReadFile(filepath.Join(d.Root, filepath.FromSlash(e.Path)))
}
func (d DirectoryInput) ReadRaw(ctx context.Context, e Entry) ([]byte, error) {
	return d.Read(ctx, e)
}

type BuildOptions struct {
	Layout                                         home.Layout
	SourceID, ProductID, Commit, RequestedRef, Tag string
	Input                                          Input
	Workers                                        int
}
type Stats struct {
	SnapshotID    string `json:"snapshotId"`
	Commit        string `json:"resolvedCommit"`
	Files         int    `json:"files,omitempty"`
	ParsedLua     int    `json:"parsedLua,omitempty"`
	ParsedXML     int    `json:"parsedXml,omitempty"`
	ParsedTOC     int    `json:"parsedToc,omitempty"`
	Assets        int    `json:"assets,omitempty"`
	ReusedObjects int    `json:"reusedObjects,omitempty"`
	ReusedAST     int    `json:"reusedAst,omitempty"`
	Diagnostics   int    `json:"diagnostics,omitempty"`
	DurationMS    int64  `json:"durationMs"`
	DBPath        string `json:"dbPath,omitempty"`
	ManifestPath  string `json:"manifestPath,omitempty"`
}
type parsed struct {
	file                    store.FileFact
	symbols                 []store.SymbolFact
	edges                   []store.EdgeFact
	xml                     []store.XMLFact
	toc                     []store.TOCFact
	search                  []store.SearchFact
	assets                  []store.AssetFact
	assetRefs               []store.AssetRefFact
	diagnostics             []result.Diagnostic
	reusedObject, reusedAST bool
	kind                    string
}

func Build(ctx context.Context, opts BuildOptions) (Stats, error) {
	started := time.Now()
	schemaID := objectstore.Hash([]byte(ParserSchema + "|" + IndexSchema))[:12]
	guard, err := lock.Acquire(filepath.Join(opts.Layout.Locks, "index-"+opts.SourceID+"-"+opts.ProductID+"-"+opts.Commit+"-"+schemaID+".lock"), "index-build", 24*time.Hour)
	if err != nil {
		return Stats{}, err
	}
	defer guard.Release()
	snapshotID := opts.SourceID + "-" + opts.ProductID + "-" + opts.Commit
	branch, err := store.OpenBranch(opts.Layout, opts.SourceID, opts.ProductID)
	if err != nil {
		return Stats{}, err
	}
	defer branch.Close()
	manifestPath := filepath.Join(opts.Layout.Manifests, opts.SourceID, opts.ProductID, opts.Commit+".json")
	if summary, ready, readyErr := branch.ReadySnapshot(snapshotID, ParserSchema, IndexSchema); readyErr != nil {
		return Stats{}, readyErr
	} else if ready {
		if _, statErr := os.Stat(manifestPath); statErr == nil {
			return Stats{SnapshotID: snapshotID, Commit: opts.Commit, Files: summary.Files, ParsedLua: summary.Lua, ParsedXML: summary.XML, ParsedTOC: summary.TOC, Assets: summary.Assets, ReusedObjects: summary.Files, ReusedAST: summary.AST, DurationMS: time.Since(started).Milliseconds(), DBPath: branch.Path, ManifestPath: manifestPath}, nil
		}
	}
	if opts.Workers <= 0 {
		opts.Workers = runtime.NumCPU()
		if opts.Workers > 8 {
			opts.Workers = 8
		}
		if opts.Workers < 4 {
			opts.Workers = 4
		}
	}
	closer, hasCloser := opts.Input.(interface{ Close(context.Context) error })
	closed := false
	if hasCloser {
		defer func() {
			if !closed {
				_ = closer.Close(context.Background())
			}
		}()
	}
	entries, err := opts.Input.Entries(ctx)
	if err != nil {
		return Stats{}, err
	}
	objects := objectstore.Store{Layout: opts.Layout}
	jobs := make(chan Entry)
	results := make(chan parsed)
	errCh := make(chan error, 1)
	var wg sync.WaitGroup
	worker := func() {
		defer wg.Done()
		for entry := range jobs {
			parseData, e := opts.Input.Read(ctx, entry)
			if e != nil {
				select {
				case errCh <- e:
				default:
				}
				return
			}
			rawData, e := opts.Input.ReadRaw(ctx, entry)
			if e != nil {
				select {
				case errCh <- e:
				default:
				}
				return
			}
			p, e := parseEntry(objects, entry, parseData, rawData)
			if e != nil {
				p.diagnostics = append(p.diagnostics, result.Diagnostic{Code: "parse_failed", Message: e.Error(), Path: entry.Path})
			}
			select {
			case results <- p:
			case <-ctx.Done():
				return
			}
		}
	}
	for i := 0; i < opts.Workers; i++ {
		wg.Add(1)
		go worker()
	}
	go func() {
		defer close(jobs)
		for _, e := range entries {
			select {
			case jobs <- e:
			case <-ctx.Done():
				return
			}
		}
	}()
	go func() { wg.Wait(); close(results) }()
	var batch store.SnapshotBatch
	stats := Stats{SnapshotID: snapshotID, Commit: opts.Commit}
	for p := range results {
		stats.Files++
		batch.Files = append(batch.Files, p.file)
		batch.Symbols = append(batch.Symbols, p.symbols...)
		batch.Edges = append(batch.Edges, p.edges...)
		batch.XML = append(batch.XML, p.xml...)
		batch.TOC = append(batch.TOC, p.toc...)
		batch.Search = append(batch.Search, p.search...)
		batch.Assets = append(batch.Assets, p.assets...)
		batch.AssetRefs = append(batch.AssetRefs, p.assetRefs...)
		stats.Diagnostics += len(p.diagnostics)
		if p.reusedObject {
			stats.ReusedObjects++
		}
		if p.reusedAST {
			stats.ReusedAST++
		}
		switch p.kind {
		case "lua":
			stats.ParsedLua++
		case "xml":
			stats.ParsedXML++
		case "toc":
			stats.ParsedTOC++
		case "asset":
			stats.Assets++
		}
	}
	select {
	case e := <-errCh:
		return Stats{}, e
	default:
	}
	if err := branch.Publish(snapshotID, opts.Commit, opts.RequestedRef, opts.Tag, ParserSchema, IndexSchema, batch); err != nil {
		return Stats{}, err
	}
	if err := writeManifest(manifestPath, opts, batch); err != nil {
		return Stats{}, err
	}
	stats.DBPath = branch.Path
	stats.ManifestPath = manifestPath
	stats.DurationMS = time.Since(started).Milliseconds()
	if hasCloser {
		if err := closer.Close(context.Background()); err != nil {
			return Stats{}, err
		}
		closed = true
	}
	return stats, nil
}

func parseEntry(objects objectstore.Store, entry Entry, parseData, rawData []byte) (parsed, error) {
	ext := strings.ToLower(filepath.Ext(entry.Path))
	role := roleFor(entry.Path)
	p := parsed{file: store.FileFact{Path: entry.Path, Language: languageFor(ext), Role: role, Size: int64(len(rawData))}}
	if isAsset(ext) {
		hash, _, reused, err := objects.PutAsset(rawData)
		if err != nil {
			return p, err
		}
		p.file.ContentHash = hash
		p.reusedObject = reused
		w, h, format := imageInfo(rawData, ext)
		p.assets = append(p.assets, store.AssetFact{Path: entry.Path, NormalizedPath: objectstore.NormalizePath(entry.Path), ContentHash: hash, GitOID: entry.OID, Extension: ext, MIME: mimeFor(ext), Size: int64(len(rawData)), Width: w, Height: h, Format: format})
		p.search = append(p.search, store.SearchFact{Path: entry.Path, Line: 1, Kind: "asset", Name: filepath.Base(entry.Path), Text: entry.Path, Role: role})
		p.kind = "asset"
		return p, nil
	}
	hash, _, reused, err := objects.PutSource(rawData)
	if err != nil {
		return p, err
	}
	p.file.ContentHash = hash
	p.reusedObject = reused
	var tree any
	switch ext {
	case ".lua":
		tree, p.symbols, p.edges, p.assetRefs, err = parseLua(entry.Path, parseData, role)
		p.kind = "lua"
	case ".xml":
		tree, p.xml, p.edges, p.assetRefs, err = parseXML(entry.Path, parseData, role)
		p.kind = "xml"
	case ".toc":
		tree, p.toc, err = parseTOC(entry.Path, parseData)
		p.kind = "toc"
	default:
		p.kind = "text"
		tree = map[string]any{"language": p.file.Language, "bytes": len(rawData)}
	}
	if err != nil {
		return p, err
	}
	if ext == ".lua" || ext == ".xml" || ext == ".toc" {
		astHash, _, astReused, e := objects.PutAST(ParserSchema, hash, map[string]any{"schema": ParserSchema, "inputHash": hash, "path": entry.Path, "language": p.file.Language, "tree": tree})
		if e != nil {
			return p, e
		}
		p.file.ASTHash = astHash
		p.reusedAST = astReused
	}
	for _, s := range p.symbols {
		p.search = append(p.search, store.SearchFact{Path: s.Path, Line: s.Line, Kind: s.Kind, Name: s.Qualified, Text: s.Signature, Role: role})
	}
	for _, x := range p.xml {
		p.search = append(p.search, store.SearchFact{Path: x.Path, Line: x.Line, Kind: x.Kind, Name: x.Name, Text: x.Attributes, Role: role})
	}
	for _, t := range p.toc {
		p.search = append(p.search, store.SearchFact{Path: t.Path, Line: t.Line, Kind: "toc", Name: t.Key, Text: t.Value, Role: role})
	}
	if (ext == ".lua" || ext == ".xml") && len(rawData) < 1<<20 {
		for i, line := range strings.Split(string(rawData), "\n") {
			line = strings.TrimSpace(line)
			if line != "" {
				p.search = append(p.search, store.SearchFact{Path: entry.Path, Line: i + 1, Kind: "source", Text: line, Role: role})
			}
		}
	}
	return p, nil
}

func writeManifest(path string, opts BuildOptions, b store.SnapshotBatch) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	data, err := json.MarshalIndent(map[string]any{"schema": "wowdoc.snapshot.v1", "sourceId": opts.SourceID, "productId": opts.ProductID, "resolvedCommit": opts.Commit, "requestedRef": opts.RequestedRef, "tag": opts.Tag, "parserSchema": ParserSchema, "indexSchema": IndexSchema, "files": b.Files, "assets": b.Assets}, "", "  ")
	if err != nil {
		return err
	}
	tmp := path + ".tmp"
	if err = os.WriteFile(tmp, data, 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}
func languageFor(ext string) string {
	switch ext {
	case ".lua":
		return "lua"
	case ".xml":
		return "xml"
	case ".toc":
		return "toc"
	default:
		if isAsset(ext) {
			return "asset"
		}
		return "text"
	}
}
func isAsset(ext string) bool {
	switch ext {
	case ".blp", ".tga", ".png", ".jpg", ".jpeg", ".dds", ".ttf", ".otf", ".mp3", ".ogg", ".wav", ".m2", ".wmo":
		return true
	}
	return false
}
func mimeFor(ext string) string {
	switch ext {
	case ".png":
		return "image/png"
	case ".jpg", ".jpeg":
		return "image/jpeg"
	case ".tga":
		return "image/x-tga"
	case ".blp":
		return "image/x-blp"
	case ".dds":
		return "image/vnd-ms.dds"
	case ".ttf":
		return "font/ttf"
	case ".otf":
		return "font/otf"
	case ".ogg":
		return "audio/ogg"
	case ".mp3":
		return "audio/mpeg"
	case ".wav":
		return "audio/wav"
	default:
		return "application/octet-stream"
	}
}
func roleFor(path string) string {
	p := strings.ToLower(filepath.ToSlash(path))
	switch {
	case strings.Contains(p, "apidocumentationgenerated"):
		return "official-generated-api"
	case strings.Contains(p, "locale") || strings.Contains(p, "localization"):
		return "locale"
	case strings.Contains(p, "libs/") || strings.Contains(p, "vendor/"):
		return "vendor"
	case strings.Contains(p, "modelpaths") || strings.Contains(p, "generated"):
		return "generated-data"
	case strings.Contains(p, "tools/") || strings.HasSuffix(p, "babelfish.lua"):
		return "tool"
	default:
		return "project"
	}
}
func imageInfo(data []byte, ext string) (int, int, string) {
	if ext == ".png" && len(data) >= 24 && bytes.Equal(data[:8], []byte{137, 80, 78, 71, 13, 10, 26, 10}) {
		return int(be32(data[16:20])), int(be32(data[20:24])), "png"
	}
	if (ext == ".jpg" || ext == ".jpeg") && len(data) > 4 {
		return 0, 0, "jpeg"
	}
	if ext == ".blp" && len(data) >= 20 {
		return int(le32(data[12:16])), int(le32(data[16:20])), "blp"
	}
	return 0, 0, strings.TrimPrefix(ext, ".")
}
func be32(b []byte) uint32 {
	return uint32(b[0])<<24 | uint32(b[1])<<16 | uint32(b[2])<<8 | uint32(b[3])
}
func le32(b []byte) uint32 {
	return uint32(b[0]) | uint32(b[1])<<8 | uint32(b[2])<<16 | uint32(b[3])<<24
}

var _ = fmt.Sprintf
