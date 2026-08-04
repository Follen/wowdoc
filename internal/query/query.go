package query

import (
	"bufio"
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/follenfang/wowdoc/internal/home"
	"github.com/follenfang/wowdoc/internal/objectstore"
	"github.com/follenfang/wowdoc/internal/result"
	"github.com/follenfang/wowdoc/internal/schema"
	"github.com/follenfang/wowdoc/internal/store"
)

type Context struct{ SourceID, ProductID, RequestedRef, MatchedTag, Commit, SnapshotID, DBPath string }
type Match struct {
	Kind        string         `json:"kind"`
	Name        string         `json:"name,omitempty"`
	Path        string         `json:"path"`
	MatchedBy   string         `json:"matchedBy"`
	Role        string         `json:"role"`
	Confidence  string         `json:"confidence,omitempty"`
	Excerpt     string         `json:"excerpt"`
	ContentHash string         `json:"contentHash"`
	Line        int            `json:"line"`
	Score       int            `json:"score"`
	ScoreParts  map[string]int `json:"scoreParts,omitempty"`
}
type Relation struct {
	Source     string `json:"source,omitempty"`
	Target     string `json:"target"`
	Kind       string `json:"kind"`
	Confidence string `json:"confidence"`
	Path       string `json:"path"`
	Line       int    `json:"line"`
}
type Response struct {
	SourceID       string     `json:"sourceId"`
	Product        string     `json:"product"`
	RequestedRef   string     `json:"requestedRef,omitempty"`
	MatchedTag     any        `json:"matchedTag"`
	ResolvedCommit string     `json:"resolvedCommit"`
	SnapshotID     string     `json:"snapshotId"`
	Results        []Match    `json:"results"`
	Relations      []Relation `json:"relations,omitempty"`
	Suggestions    []string   `json:"suggestions,omitempty"`
}

func openBranch(layout home.Layout, ctx Context) (*store.Branch, error) {
	var branch *store.Branch
	var err error
	currentPath := store.BranchPath(layout, ctx.SourceID, ctx.ProductID, schema.Parser, schema.Index)
	if ctx.DBPath != "" && filepath.Clean(ctx.DBPath) == filepath.Clean(currentPath) {
		branch, err = store.OpenBranchReadPath(layout, ctx.SourceID, ctx.DBPath, schema.Parser, schema.Index)
	} else {
		branch, err = store.OpenBranchRead(layout, ctx.SourceID, ctx.ProductID, schema.Parser, schema.Index)
	}
	if err != nil && os.IsNotExist(err) {
		return nil, snapshotNotReady("the selected snapshot is not indexed for the current schema")
	}
	return branch, err
}

func Search(layout home.Layout, ctx Context, text, topic string, limit int) (Response, error) {
	if limit <= 0 {
		limit = 10
	}
	branch, err := openBranch(layout, ctx)
	if err != nil {
		return Response{}, err
	}
	defer branch.Close()
	if err = ensureReady(branch.DB, ctx.SnapshotID); err != nil {
		return Response{}, err
	}
	type candidate struct {
		kind, name, path, matched, role, confidence string
		line, endLine, score                        int
	}
	var candidates []candidate
	rows, err := branch.DB.Query(`SELECT s.kind,qualified.value,sf.path,s.line,s.end_line,'exact_symbol',sf.role,100 AS rank FROM content.symbols s JOIN content.strings name ON name.id=s.name_id JOIN content.strings qualified ON qualified.id=s.qualified_id JOIN snapshot_files sf ON sf.content_id=s.content_id WHERE sf.snapshot_id=? AND (s.required_role='' OR s.required_role=sf.role) AND (qualified.value=? OR name.value=?) UNION ALL SELECT d.kind,name.value,sf.path,d.line,0,'exact_fact',sf.role,90 AS rank FROM content.search_docs d JOIN content.strings name ON name.id=d.name_id JOIN snapshot_files sf ON sf.content_id=d.content_id WHERE sf.snapshot_id=? AND (d.required_role='' OR d.required_role=sf.role) AND name.value=? UNION ALL SELECT s.kind,qualified.value,sf.path,s.line,s.end_line,'symbol_prefix',sf.role,80 AS rank FROM content.symbols s JOIN content.strings name ON name.id=s.name_id JOIN content.strings qualified ON qualified.id=s.qualified_id JOIN snapshot_files sf ON sf.content_id=s.content_id WHERE sf.snapshot_id=? AND (s.required_role='' OR s.required_role=sf.role) AND (qualified.value LIKE ? OR name.value LIKE ?) ORDER BY rank DESC,3,4,1,2 LIMIT ?`, ctx.SnapshotID, text, text, ctx.SnapshotID, text, ctx.SnapshotID, text+"%", text+"%", limit*3)
	if err != nil {
		return Response{}, err
	}
	for rows.Next() {
		var c candidate
		if err = rows.Scan(&c.kind, &c.name, &c.path, &c.line, &c.endLine, &c.matched, &c.role, &c.score); err != nil {
			rows.Close()
			return Response{}, err
		}
		candidates = append(candidates, c)
	}
	rows.Close()
	if len(candidates) < limit {
		rows, err = branch.DB.Query(`SELECT d.kind,name.value,sf.path,d.line,0,'fts5',sf.role,CAST(70-min(20,abs(bm25(search_fts))) AS INTEGER) FROM search_fts JOIN content.search_docs d ON d.id=search_fts.rowid JOIN content.strings name ON name.id=d.name_id JOIN snapshot_files sf ON sf.content_id=d.content_id WHERE sf.snapshot_id=? AND (d.required_role='' OR d.required_role=sf.role) AND search_fts MATCH ? ORDER BY bm25(search_fts),CASE sf.role WHEN 'project' THEN 0 WHEN 'official-generated-api' THEN 0 WHEN 'vendor' THEN 2 ELSE 1 END,sf.path,d.line LIMIT ?`, ctx.SnapshotID, ftsQuery(text), limit*3)
		if err != nil {
			like := "%" + text + "%"
			rows, err = branch.DB.Query(`SELECT d.kind,name.value,sf.path,d.line,0,'name_fallback',sf.role,60 FROM content.search_docs d JOIN content.strings name ON name.id=d.name_id JOIN snapshot_files sf ON sf.content_id=d.content_id WHERE sf.snapshot_id=? AND (d.required_role='' OR d.required_role=sf.role) AND name.value LIKE ? ORDER BY sf.path,d.line LIMIT ?`, ctx.SnapshotID, like, limit*3)
			if err != nil {
				return Response{}, err
			}
		}
		for rows.Next() {
			var c candidate
			if err = rows.Scan(&c.kind, &c.name, &c.path, &c.line, &c.endLine, &c.matched, &c.role, &c.score); err != nil {
				rows.Close()
				return Response{}, err
			}
			candidates = append(candidates, c)
		}
		rows.Close()
	}
	seen := map[string]bool{}
	var matches []Match
	for _, c := range candidates {
		if c.kind == "source" && c.line == 0 {
			resolvedLine, resolveErr := sourceMatchLine(branch.DB, layout, ctx.SnapshotID, c.path, text)
			if resolveErr != nil {
				continue
			}
			c.line = resolvedLine
		}
		key := fmt.Sprintf("%s:%d:%s", c.path, c.line, c.kind)
		if seen[key] {
			continue
		}
		seen[key] = true
		var hash, excerptText string
		var e error
		if c.matched == "exact_symbol" && c.endLine >= c.line {
			hash, excerptText, e = excerptRange(branch.DB, layout, ctx.SnapshotID, c.path, c.line, c.endLine, 80)
		} else {
			hash, excerptText, e = excerpt(branch.DB, layout, ctx.SnapshotID, c.path, c.line, 3)
		}
		if e != nil {
			continue
		}
		score := c.score - rolePenalty(c.role)
		matches = append(matches, Match{Kind: c.kind, Name: c.name, Path: c.path, Line: c.line, MatchedBy: c.matched, Role: c.role, Confidence: c.confidence, Score: score, ScoreParts: map[string]int{"match": c.score, "rolePenalty": -rolePenalty(c.role)}, ContentHash: hash, Excerpt: excerptText})
		if len(matches) >= limit {
			break
		}
	}
	var tag any = ctx.MatchedTag
	if ctx.MatchedTag == "" {
		tag = nil
	}
	response := Response{SourceID: ctx.SourceID, Product: ctx.ProductID, RequestedRef: ctx.RequestedRef, MatchedTag: tag, ResolvedCommit: ctx.Commit, SnapshotID: ctx.SnapshotID, Results: matches}
	relationRows, relationErr := branch.DB.Query(`SELECT source.value,target.value,e.kind,e.confidence,sf.path,e.line FROM content.edges e JOIN content.strings source ON source.id=e.source_id JOIN content.strings target ON target.id=e.target_id JOIN snapshot_files sf ON sf.content_id=e.content_id WHERE sf.snapshot_id=? AND (e.required_role='' OR e.required_role=sf.role) AND (source.value=? OR target.value=? OR source.value LIKE ? OR target.value LIKE ?) ORDER BY CASE e.confidence WHEN 'exact' THEN 0 WHEN 'inferred' THEN 1 ELSE 2 END,sf.path,e.line LIMIT 50`, ctx.SnapshotID, text, text, "%"+text+"%", "%"+text+"%")
	if relationErr == nil {
		defer relationRows.Close()
		for relationRows.Next() {
			var relation Relation
			if relationRows.Scan(&relation.Source, &relation.Target, &relation.Kind, &relation.Confidence, &relation.Path, &relation.Line) == nil {
				relation.Source = strings.ReplaceAll(relation.Source, "{path}", relation.Path)
				response.Relations = append(response.Relations, relation)
			}
		}
	}
	if len(matches) == 0 {
		response.Suggestions = []string{"use explore with a shorter symbol or path prefix", "check source list for the selected product and ref"}
	}
	return response, nil
}

func sourceMatchLine(db *sql.DB, layout home.Layout, snapshotID, path, text string) (int, error) {
	var hash string
	if err := db.QueryRow(`SELECT c.content_hash FROM snapshot_files sf JOIN content.contents c ON c.id=sf.content_id WHERE sf.snapshot_id=? AND sf.path=?`, snapshotID, path).Scan(&hash); err != nil {
		return 0, err
	}
	file, err := objectstore.Open(layout, objectstore.Source, hash)
	if err != nil {
		return 0, err
	}
	defer file.Close()
	needle := strings.ToLower(strings.TrimSpace(text))
	tokens := strings.Fields(needle)
	allTokensLine := 0
	firstTokenLine := 0
	scanner := bufio.NewScanner(file)
	for line := 1; scanner.Scan(); line++ {
		value := strings.ToLower(scanner.Text())
		if needle != "" && strings.Contains(value, needle) {
			return line, nil
		}
		if allTokensLine == 0 && len(tokens) > 1 {
			containsAll := true
			for _, token := range tokens {
				if !strings.Contains(value, token) {
					containsAll = false
					break
				}
			}
			if containsAll {
				allTokensLine = line
			}
		}
		if firstTokenLine == 0 {
			for _, token := range tokens {
				if strings.Contains(value, token) {
					firstTokenLine = line
					break
				}
			}
		}
	}
	if err := scanner.Err(); err != nil {
		return 0, err
	}
	if allTokensLine > 0 {
		return allTokensLine, nil
	}
	if firstTokenLine > 0 {
		return firstTokenLine, nil
	}
	return 1, nil
}

func Inspect(layout home.Layout, ctx Context, symbol, path string) (Response, error) {
	if symbol != "" {
		return Search(layout, ctx, symbol, "", 25)
	}
	branch, err := openBranch(layout, ctx)
	if err != nil {
		return Response{}, err
	}
	defer branch.Close()
	if err = ensureReady(branch.DB, ctx.SnapshotID); err != nil {
		return Response{}, err
	}
	var tag any = ctx.MatchedTag
	if ctx.MatchedTag == "" {
		tag = nil
	}
	response := Response{SourceID: ctx.SourceID, Product: ctx.ProductID, RequestedRef: ctx.RequestedRef, MatchedTag: tag, ResolvedCommit: ctx.Commit, SnapshotID: ctx.SnapshotID}
	rows, err := branch.DB.Query(`SELECT sf.path,c.content_hash,sf.role FROM snapshot_files sf JOIN content.contents c ON c.id=sf.content_id WHERE sf.snapshot_id=? AND (sf.path=? OR lower(sf.path) LIKE lower(?)) ORDER BY CASE WHEN sf.path=? THEN 0 ELSE 1 END,sf.path LIMIT 25`, ctx.SnapshotID, path, "%"+path+"%", path)
	if err != nil {
		return Response{}, err
	}
	for rows.Next() {
		var filePath, hash, role string
		if rows.Scan(&filePath, &hash, &role) == nil {
			_, text, e := excerpt(branch.DB, layout, ctx.SnapshotID, filePath, 1, 6)
			if e == nil {
				response.Results = append(response.Results, Match{Kind: "file", Name: filepath.Base(filePath), Path: filePath, Line: 1, MatchedBy: "path", Role: role, Score: 100, ScoreParts: map[string]int{"match": 100}, ContentHash: hash, Excerpt: text})
			}
		}
	}
	rows.Close()
	assetRows, assetErr := branch.DB.Query(`SELECT sa.path,c.content_hash,a.extension,a.mime,a.bytes,a.width,a.height,a.format FROM snapshot_assets sa JOIN content.contents c ON c.id=sa.content_id JOIN content.assets a ON a.content_id=sa.content_id WHERE sa.snapshot_id=? AND (sa.path=? OR lower(sa.path) LIKE lower(?)) ORDER BY CASE WHEN sa.path=? THEN 0 ELSE 1 END,sa.path LIMIT 25`, ctx.SnapshotID, path, "%"+path+"%", path)
	if assetErr == nil {
		defer assetRows.Close()
		for assetRows.Next() {
			var p, hash, ext, mime, format string
			var size int64
			var width, height int
			if assetRows.Scan(&p, &hash, &ext, &mime, &size, &width, &height, &format) == nil {
				meta := fmt.Sprintf("format=%s mime=%s bytes=%d width=%d height=%d local=%s", format, mime, size, width, height, objectstore.Location(layout, objectstore.Asset, hash))
				response.Results = append(response.Results, Match{Kind: "asset", Name: filepath.Base(p), Path: p, Line: 1, MatchedBy: "path", Role: "project", Score: 100, ContentHash: hash, Excerpt: meta})
			}
		}
	}
	if len(response.Results) == 0 {
		response.Suggestions = []string{"use query --topic asset with a filename or a shorter path"}
	}
	return response, nil
}

type DiffItem struct{ Key, Kind, Before, After, Path string }

func Diff(layout home.Layout, a, b Context) (map[string]any, error) {
	left, err := openBranch(layout, a)
	if err != nil {
		return nil, err
	}
	defer left.Close()
	right, err := openBranch(layout, b)
	if err != nil {
		return nil, err
	}
	defer right.Close()
	load := func(db *sql.DB, id string) (map[string]string, error) {
		rows, e := db.Query(`SELECT qualified.value,signature.value||'@'||sf.path||':'||s.line FROM content.symbols s JOIN content.strings qualified ON qualified.id=s.qualified_id JOIN content.strings signature ON signature.id=s.signature_id JOIN snapshot_files sf ON sf.content_id=s.content_id WHERE sf.snapshot_id=? AND (s.required_role='' OR s.required_role=sf.role)`, id)
		if e != nil {
			return nil, e
		}
		defer rows.Close()
		m := map[string]string{}
		for rows.Next() {
			var k, v string
			if e = rows.Scan(&k, &v); e != nil {
				return nil, e
			}
			m[k] = v
		}
		return m, rows.Err()
	}
	lm, err := load(left.DB, a.SnapshotID)
	if err != nil {
		return nil, err
	}
	rm, err := load(right.DB, b.SnapshotID)
	if err != nil {
		return nil, err
	}
	var added, removed, changed []DiffItem
	for k, v := range lm {
		if rv, ok := rm[k]; !ok {
			removed = append(removed, DiffItem{Key: k, Kind: "symbol", Before: v})
		} else if rv != v {
			changed = append(changed, DiffItem{Key: k, Kind: "symbol", Before: v, After: rv})
		}
	}
	for k, v := range rm {
		if _, ok := lm[k]; !ok {
			added = append(added, DiffItem{Key: k, Kind: "symbol", After: v})
		}
	}
	sort.Slice(added, func(i, j int) bool { return added[i].Key < added[j].Key })
	sort.Slice(removed, func(i, j int) bool { return removed[i].Key < removed[j].Key })
	sort.Slice(changed, func(i, j int) bool { return changed[i].Key < changed[j].Key })
	return map[string]any{"from": a, "to": b, "added": added, "removed": removed, "changed": changed}, nil
}

func Status(layout home.Layout, sourceID, productID string) (map[string]any, error) {
	branch, err := store.OpenBranchRead(layout, sourceID, productID, schema.Parser, schema.Index)
	if err != nil {
		return nil, err
	}
	defer branch.Close()
	var active string
	_ = branch.DB.QueryRow(`SELECT value FROM branch_state WHERE key='active_snapshot'`).Scan(&active)
	var snapshots, files, asts int
	_ = branch.DB.QueryRow(`SELECT count(*) FROM snapshots WHERE status='ready'`).Scan(&snapshots)
	_ = branch.DB.QueryRow(`SELECT count(*) FROM snapshot_files`).Scan(&files)
	_ = branch.DB.QueryRow(`SELECT count(*) FROM content.contents WHERE ast_hash<>''`).Scan(&asts)
	return map[string]any{"sourceId": sourceID, "product": productID, "activeSnapshot": active, "readySnapshots": snapshots, "snapshotFiles": files, "astFiles": asts, "parserSchema": schema.Parser, "indexSchema": schema.Index, "database": branch.Path, "contentDatabase": branch.ContentPath, "journalMode": "wal"}, nil
}

func ensureReady(db *sql.DB, id string) error {
	var status, parserSchema, indexSchema string
	err := db.QueryRow(`SELECT status,parser_schema,index_schema FROM snapshots WHERE id=?`, id).Scan(&status, &parserSchema, &indexSchema)
	if err == sql.ErrNoRows {
		return snapshotNotReady("the selected snapshot is not indexed")
	}
	if err != nil {
		if strings.Contains(strings.ToLower(err.Error()), "no such column") {
			return snapshotNotReady("the selected snapshot uses an older index schema")
		}
		return err
	}
	if status != "ready" || parserSchema != schema.Parser || indexSchema != schema.Index {
		return snapshotNotReady("the selected snapshot must be rebuilt for the current schema")
	}
	return nil
}

func snapshotNotReady(message string) error {
	e := result.E("snapshot_not_ready", message, 3)
	e.NextSteps = []string{"wowdoc source sync --source SOURCE --product PRODUCT", "wowdoc index build --source SOURCE --product PRODUCT --ref REF"}
	return e
}
func rolePenalty(role string) int {
	switch role {
	case "vendor":
		return 15
	case "locale", "generated-data", "tool":
		return 20
	default:
		return 0
	}
}
func ftsQuery(text string) string {
	parts := strings.Fields(text)
	if len(parts) == 0 {
		return `""`
	}
	for i, part := range parts {
		parts[i] = `"` + strings.ReplaceAll(part, `"`, `""`) + `"`
	}
	return strings.Join(parts, " OR ")
}
func excerpt(db *sql.DB, layout home.Layout, snapshotID, path string, line, context int) (string, string, error) {
	var hash string
	if err := db.QueryRow(`SELECT c.content_hash FROM snapshot_files sf JOIN content.contents c ON c.id=sf.content_id WHERE sf.snapshot_id=? AND sf.path=?`, snapshotID, path).Scan(&hash); err != nil {
		return "", "", err
	}
	file, err := objectstore.Open(layout, objectstore.Source, hash)
	if err != nil {
		return "", "", err
	}
	defer file.Close()
	start := line - context
	if start < 1 {
		start = 1
	}
	end := line + context
	scanner := bufio.NewScanner(file)
	var lines []string
	for n := 1; scanner.Scan(); n++ {
		if n >= start && n <= end {
			lines = append(lines, fmt.Sprintf("%d: %s", n, scanner.Text()))
		}
		if n > end {
			break
		}
	}
	return hash, strings.Join(lines, "\n"), scanner.Err()
}

func excerptRange(db *sql.DB, layout home.Layout, snapshotID, path string, start, end, maxLines int) (string, string, error) {
	if end < start {
		end = start
	}
	if maxLines > 0 && end-start+1 > maxLines {
		end = start + maxLines - 1
	}
	var hash string
	if err := db.QueryRow(`SELECT c.content_hash FROM snapshot_files sf JOIN content.contents c ON c.id=sf.content_id WHERE sf.snapshot_id=? AND sf.path=?`, snapshotID, path).Scan(&hash); err != nil {
		return "", "", err
	}
	file, err := objectstore.Open(layout, objectstore.Source, hash)
	if err != nil {
		return "", "", err
	}
	defer file.Close()
	scanner := bufio.NewScanner(file)
	var lines []string
	for n := 1; scanner.Scan(); n++ {
		if n >= start && n <= end {
			lines = append(lines, fmt.Sprintf("%d: %s", n, scanner.Text()))
		}
		if n > end {
			break
		}
	}
	return hash, strings.Join(lines, "\n"), scanner.Err()
}
