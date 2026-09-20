package query

import (
	"database/sql"
	"fmt"
	"sort"
	"strings"

	"github.com/follenfang/wowdoc/internal/home"
	"github.com/follenfang/wowdoc/internal/objectstore"
	"github.com/follenfang/wowdoc/internal/result"
)

// Search stops at the strongest available evidence tier. Explore explicitly
// includes weaker tiers; neither mode fills an exact answer with unrelated text.
func Search(layout home.Layout, ctx Context, text, topic string, limit int) (Response, error) {
	return search(layout, ctx, text, topic, limit, false)
}

func Explore(layout home.Layout, ctx Context, text, topic string, limit int) (Response, error) {
	return search(layout, ctx, text, topic, limit, true)
}

type searchCandidate struct {
	kind, name, path, matched, role string
	line, endLine, rank             int
}

func search(layout home.Layout, ctx Context, text, topic string, limit int, broad bool) (Response, error) {
	text, topic = strings.TrimSpace(text), strings.ToLower(strings.TrimSpace(topic))
	if text == "" {
		return Response{}, result.E("query_required", "search text must not be empty", 2)
	}
	filter, err := searchTopicFilter(topic)
	if err != nil {
		return Response{}, err
	}
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
	var tag any = ctx.MatchedTag
	if ctx.MatchedTag == "" {
		tag = nil
	}
	response := Response{SourceID: ctx.SourceID, Product: ctx.ProductID, RequestedRef: ctx.RequestedRef, MatchedTag: tag, ResolvedCommit: ctx.Commit, SnapshotID: ctx.SnapshotID, Results: []Match{}}
	var candidates []searchCandidate
	// Filtering and role ranking happen before LIMIT, so off-topic or vendor
	// hits cannot crowd a relevant project definition out of the candidate set.
	collect := func(statement string, args ...any) error {
		statement = `SELECT * FROM (` + statement + `) WHERE ` + filter + ` ORDER BY rank-CASE role WHEN 'vendor' THEN 15 WHEN 'locale' THEN 20 WHEN 'generated-data' THEN 20 WHEN 'tool' THEN 20 ELSE 0 END DESC,path,line,kind,name LIMIT ?`
		args = append(args, limit*3)
		rows, e := branch.DB.Query(statement, args...)
		if e != nil {
			return e
		}
		defer rows.Close()
		for rows.Next() {
			var c searchCandidate
			if e = rows.Scan(&c.kind, &c.name, &c.path, &c.line, &c.endLine, &c.matched, &c.role, &c.rank); e != nil {
				return e
			}
			candidates = append(candidates, c)
		}
		return rows.Err()
	}
	const symbolBase = `SELECT s.kind AS kind,qualified.value AS name,sf.path AS path,s.line AS line,s.end_line AS endLine,'exact_symbol' AS matched,sf.role AS role,100 AS rank FROM content.symbols s JOIN content.strings qualified ON qualified.id=s.qualified_id JOIN snapshot_files sf ON sf.content_id=s.content_id WHERE sf.snapshot_id=? AND (s.required_role='' OR s.required_role=sf.role) AND `
	const docBase = `SELECT d.kind AS kind,name.value AS name,sf.path AS path,d.line AS line,0 AS endLine,'exact_fact' AS matched,sf.role AS role,90 AS rank FROM content.search_docs d JOIN content.strings name ON name.id=d.name_id JOIN snapshot_files sf ON sf.content_id=d.content_id WHERE sf.snapshot_id=? AND (d.required_role='' OR d.required_role=sf.role) AND `
	if topic != "asset" {
		err = collect(symbolBase+`s.qualified_id=(SELECT id FROM content.strings WHERE value=?) UNION `+symbolBase+`s.name_id=(SELECT id FROM content.strings WHERE value=?) UNION ALL `+docBase+`name.value=?`, ctx.SnapshotID, text, ctx.SnapshotID, text, ctx.SnapshotID, text)
		if err != nil {
			return Response{}, err
		}
		if broad || len(candidates) == 0 {
			prefix := escapeLike(text) + "%"
			symbolPrefix := strings.NewReplacer("'exact_symbol'", "'symbol_prefix'", "100 AS rank", "80 AS rank").Replace(symbolBase)
			docPrefix := strings.NewReplacer("'exact_fact'", "'name_prefix'", "90 AS rank", "80 AS rank").Replace(docBase)
			err = collect(symbolPrefix+`(qualified.value LIKE ? ESCAPE '\' OR s.name_id IN (SELECT id FROM content.strings WHERE value LIKE ? ESCAPE '\')) UNION ALL `+docPrefix+`name.value LIKE ? ESCAPE '\'`, ctx.SnapshotID, prefix, prefix, ctx.SnapshotID, prefix)
			if err != nil {
				return Response{}, err
			}
		}
		if broad || len(candidates) == 0 {
			fts := `SELECT d.kind AS kind,name.value AS name,sf.path AS path,d.line AS line,0 AS endLine,'fts5' AS matched,sf.role AS role,CAST(70+min(9,abs(bm25(search_fts))) AS INTEGER) AS rank FROM search_fts JOIN content.search_docs d ON d.id=search_fts.rowid JOIN content.strings name ON name.id=d.name_id JOIN snapshot_files sf ON sf.content_id=d.content_id WHERE sf.snapshot_id=? AND (d.required_role='' OR d.required_role=sf.role) AND search_fts MATCH ?`
			terms := ftsQuery(text)
			if broad {
				terms = strings.ReplaceAll(terms, " AND ", " OR ")
			}
			if err = collect(fts, ctx.SnapshotID, terms); err != nil {
				return Response{}, err
			}
		}
	}
	if topic == "asset" {
		assetSQL := `SELECT 'asset' AS kind,sa.path AS name,sa.path AS path,1 AS line,0 AS endLine,'asset_path' AS matched,'project' AS role,CASE WHEN sa.normalized_path=lower(?) THEN 100 ELSE 70 END AS rank FROM snapshot_assets sa WHERE sa.snapshot_id=? AND sa.normalized_path LIKE lower(?) ESCAPE '\'`
		normalized := objectstore.NormalizePath(text)
		if err = collect(assetSQL, normalized, ctx.SnapshotID, "%"+escapeLike(normalized)+"%"); err != nil {
			return Response{}, err
		}
	}
	sort.SliceStable(candidates, func(i, j int) bool {
		a, b := candidates[i], candidates[j]
		return a.rank-rolePenalty(a.role) > b.rank-rolePenalty(b.role)
	})
	seen := map[string]bool{}
	seenLines := map[string]bool{}
	for _, c := range candidates {
		if c.kind == "source" && c.line == 0 {
			c.line, err = sourceMatchLine(branch.DB, layout, ctx.SnapshotID, c.path, text)
			if err != nil {
				return Response{}, err
			}
		}
		// Fold redundant facts and source excerpts, while preserving distinct
		// definitions that happen to share one line (e.g. compact Lua files).
		location := fmt.Sprintf("%s:%d", c.path, c.line)
		if c.kind == "source" && seenLines[location] {
			continue
		}
		key := location + "\x00" + c.name
		if seen[key] {
			continue
		}
		seen[key] = true
		seenLines[location] = true
		var hash, snippet string
		if c.kind == "asset" {
			err = branch.DB.QueryRow(`SELECT c.content_hash FROM snapshot_assets sa JOIN content.contents c ON c.id=sa.content_id WHERE sa.snapshot_id=? AND sa.path=?`, ctx.SnapshotID, c.path).Scan(&hash)
			snippet = c.path
		} else if c.matched == "exact_symbol" && c.endLine >= c.line {
			hash, snippet, err = excerptRange(branch.DB, layout, ctx.SnapshotID, c.path, c.line, c.endLine, 80)
		} else {
			hash, snippet, err = excerpt(branch.DB, layout, ctx.SnapshotID, c.path, c.line, 3)
		}
		if err != nil {
			return Response{}, err
		}
		response.Results = append(response.Results, Match{Kind: c.kind, Name: c.name, Path: c.path, Line: c.line, MatchedBy: c.matched, Role: c.role, Score: c.rank - rolePenalty(c.role), ScoreParts: map[string]int{"match": c.rank, "rolePenalty": -rolePenalty(c.role)}, ContentHash: hash, Excerpt: snippet})
		if len(response.Results) >= limit {
			break
		}
	}
	// Relations are exact by default and share the requested bound. Broad
	// substring relations belong to Explore and no longer bypass --limit.
	if err = searchRelations(branch.DB, ctx, text, limit, broad, &response); err != nil {
		return Response{}, err
	}
	if len(response.Results) == 0 {
		response.Suggestions = []string{"use explore for broader text and symbol matches", "check the topic, product and ref"}
	}
	return response, nil
}

func escapeLike(text string) string {
	return strings.NewReplacer(`\`, `\\`, "%", `\%`, "_", `\_`).Replace(text)
}

func searchTopicFilter(topic string) (string, error) {
	switch topic {
	case "":
		return "1=1", nil
	case "api":
		return "kind LIKE 'api-%'", nil
	case "lua", "xml", "toc":
		return "lower(path) LIKE '%." + topic + "'", nil
	case "asset":
		return "kind='asset'", nil
	default:
		return "", result.E("invalid_topic", fmt.Sprintf("unsupported topic %q: use api, lua, xml, toc, or asset", topic), 2)
	}
}

func searchRelations(db *sql.DB, ctx Context, text string, limit int, broad bool, response *Response) error {
	where := `(source.value=? OR target.value=?)`
	args := []any{ctx.SnapshotID, text, text}
	if broad {
		where = `(source.value LIKE ? ESCAPE '\' OR target.value LIKE ? ESCAPE '\')`
		args = []any{ctx.SnapshotID, "%" + escapeLike(text) + "%", "%" + escapeLike(text) + "%"}
	}
	args = append(args, limit)
	rows, err := db.Query(`SELECT source.value,target.value,e.kind,e.confidence,sf.path,e.line FROM content.edges e JOIN content.strings source ON source.id=e.source_id JOIN content.strings target ON target.id=e.target_id JOIN snapshot_files sf ON sf.content_id=e.content_id WHERE sf.snapshot_id=? AND (e.required_role='' OR e.required_role=sf.role) AND `+where+` ORDER BY CASE e.confidence WHEN 'exact' THEN 0 WHEN 'inferred' THEN 1 ELSE 2 END,sf.path,e.line LIMIT ?`, args...)
	if err != nil {
		return err
	}
	defer rows.Close()
	for rows.Next() {
		var r Relation
		if err = rows.Scan(&r.Source, &r.Target, &r.Kind, &r.Confidence, &r.Path, &r.Line); err != nil {
			return err
		}
		r.Source = strings.ReplaceAll(r.Source, "{path}", r.Path)
		response.Relations = append(response.Relations, r)
	}
	return rows.Err()
}
