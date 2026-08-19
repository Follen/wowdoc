package query

import (
	"database/sql"
	"fmt"
	"sort"
	"strings"

	"github.com/follenfang/wowdoc/internal/home"
)

// CompatibilityUsage is a statically resolved AddOn reference to snapshot data.
type CompatibilityUsage struct {
	Kind       string `json:"kind"`
	Name       string `json:"name"`
	Expression string `json:"expression,omitempty"`
	File       string `json:"file"`
	Line       int    `json:"line"`
	Column     int    `json:"column"`
}

type CompatibilityEvidence map[string]any

type CompatibilityFact struct {
	Kind      string                `json:"kind"`
	Name      string                `json:"name"`
	Exists    bool                  `json:"exists"`
	Signature string                `json:"signature"`
	Evidence  CompatibilityEvidence `json:"evidence"`
}

type CompatibilityUnresolved struct {
	Kind       string                `json:"kind"`
	Expression string                `json:"expression"`
	File       string                `json:"file"`
	Line       int                   `json:"line"`
	Column     int                   `json:"column"`
	Reason     string                `json:"reason"`
	Evidence   CompatibilityEvidence `json:"evidence"`
}

type CompatibilityDiagnostic struct {
	Severity string                `json:"severity"`
	Code     string                `json:"code"`
	File     string                `json:"file"`
	Line     int                   `json:"line"`
	Column   int                   `json:"column"`
	Message  string                `json:"message"`
	Evidence CompatibilityEvidence `json:"evidence"`
}

type compatibilityMatch struct {
	Path, Role, Signature, Detail string
	Line                          int
}

// LookupCompatibility resolves static AddOn usages against one immutable indexed
// snapshot. It does not fall back to facts from another snapshot in the shared
// content database.
func LookupCompatibility(layout home.Layout, ctx Context, usages []CompatibilityUsage, interfaceValue string) ([]CompatibilityFact, []CompatibilityUnresolved, []CompatibilityDiagnostic, error) {
	facts := make([]CompatibilityFact, 0, len(usages)+1)
	unresolved := make([]CompatibilityUnresolved, 0)
	diagnostics := make([]CompatibilityDiagnostic, 0)

	branch, err := openBranch(layout, ctx)
	if err != nil {
		return nil, nil, nil, err
	}
	defer branch.Close()
	if err = ensureReady(branch.DB, ctx.SnapshotID); err != nil {
		return nil, nil, nil, err
	}

	interfaceSeen := false
	for _, usage := range usages {
		candidate := strings.EqualFold(strings.TrimSpace(usage.Kind), "api-candidate")
		kind := normalizeCompatibilityKind(usage.Kind)
		if kind == "interface" && strings.TrimSpace(usage.Name) == strings.TrimSpace(interfaceValue) {
			interfaceSeen = true
		}
		if kind == "" {
			unresolved = append(unresolved, unresolvedUsage(ctx, usage, "unsupported compatibility evidence kind"))
			continue
		}
		if strings.TrimSpace(usage.Name) == "" {
			unresolved = append(unresolved, unresolvedUsage(ctx, usage, "dynamic or empty reference cannot be resolved statically"))
			continue
		}
		fact, resolved, lookupErr := lookupCompatibilityUsage(branch.DB, ctx, usage, kind)
		if lookupErr != nil {
			return nil, nil, nil, lookupErr
		}
		if resolved {
			if candidate && !fact.Exists {
				unresolved = append(unresolved, unresolvedUsage(ctx, usage, "the call is not an indexed API and may be an AddOn-defined global"))
			} else {
				facts = append(facts, fact)
			}
		} else {
			unresolved = append(unresolved, unresolvedUsage(ctx, usage, "the indexed snapshot cannot prove presence or absence for this reference"))
		}
	}
	if value := strings.TrimSpace(interfaceValue); value != "" && !interfaceSeen {
		usage := CompatibilityUsage{Kind: "interface", Name: value, Expression: value}
		fact, resolved, lookupErr := lookupCompatibilityUsage(branch.DB, ctx, usage, "interface")
		if lookupErr != nil {
			return nil, nil, nil, lookupErr
		}
		if resolved {
			facts = append(facts, fact)
		} else {
			unresolved = append(unresolved, unresolvedUsage(ctx, usage, "the indexed snapshot has no authoritative Interface evidence"))
		}
	}
	return facts, unresolved, diagnostics, nil
}

func lookupCompatibilityUsage(db *sql.DB, ctx Context, usage CompatibilityUsage, kind string) (CompatibilityFact, bool, error) {
	name := strings.TrimSpace(usage.Name)
	matches, categoryKnown, err := compatibilityMatches(db, ctx.SnapshotID, kind, name)
	if err != nil {
		return CompatibilityFact{}, false, err
	}
	if len(matches) == 0 && (!categoryKnown || kind == "mixin" || kind == "frame-type") {
		return CompatibilityFact{}, false, nil
	}
	evidence := compatibilityBaseEvidence(ctx, usage)
	evidence["matches"] = compatibilityMatchEvidence(matches)
	if len(matches) == 0 {
		evidence["categoryPresent"] = true
	}
	signature := ""
	if len(matches) > 0 {
		signature = matches[0].Signature
	}
	return CompatibilityFact{Kind: kind, Name: name, Exists: len(matches) > 0, Signature: signature, Evidence: evidence}, true, nil
}

func compatibilityMatches(db *sql.DB, snapshotID, kind, name string) ([]compatibilityMatch, bool, error) {
	var query string
	var args []any
	var categoryQuery string
	switch kind {
	case "api":
		query = `SELECT sf.path,s.line,sf.role,signature.value,s.kind FROM content.symbols s JOIN content.strings name ON name.id=s.name_id JOIN content.strings qualified ON qualified.id=s.qualified_id JOIN content.strings signature ON signature.id=s.signature_id JOIN snapshot_files sf ON sf.content_id=s.content_id WHERE sf.snapshot_id=? AND (s.required_role='' OR s.required_role=sf.role) AND s.kind IN ('api-function','api-scriptobject') AND (qualified.value=? OR name.value=?) ORDER BY sf.path,s.line,s.kind`
		args = []any{snapshotID, name, name}
		categoryQuery = `SELECT EXISTS(SELECT 1 FROM content.symbols s JOIN snapshot_files sf ON sf.content_id=s.content_id WHERE sf.snapshot_id=? AND (s.required_role='' OR s.required_role=sf.role) AND s.kind IN ('api-function','api-scriptobject'))`
	case "event":
		query = `SELECT sf.path,s.line,sf.role,signature.value,s.kind FROM content.symbols s JOIN content.strings name ON name.id=s.name_id JOIN content.strings qualified ON qualified.id=s.qualified_id JOIN content.strings signature ON signature.id=s.signature_id JOIN snapshot_files sf ON sf.content_id=s.content_id WHERE sf.snapshot_id=? AND (s.required_role='' OR s.required_role=sf.role) AND s.kind='api-event' AND (qualified.value=? OR name.value=?) ORDER BY sf.path,s.line`
		args = []any{snapshotID, name, name}
		categoryQuery = `SELECT EXISTS(SELECT 1 FROM content.symbols s JOIN snapshot_files sf ON sf.content_id=s.content_id WHERE sf.snapshot_id=? AND (s.required_role='' OR s.required_role=sf.role) AND s.kind='api-event')`
	case "mixin":
		query = `SELECT sf.path,s.line,sf.role,signature.value,s.kind FROM content.symbols s JOIN content.strings qualified ON qualified.id=s.qualified_id JOIN content.strings signature ON signature.id=s.signature_id JOIN snapshot_files sf ON sf.content_id=s.content_id WHERE sf.snapshot_id=? AND (s.required_role='' OR s.required_role=sf.role) AND s.kind='mixin' AND qualified.value=? ORDER BY sf.path,s.line`
		args = []any{snapshotID, name}
		categoryQuery = `SELECT EXISTS(SELECT 1 FROM content.symbols s JOIN snapshot_files sf ON sf.content_id=s.content_id WHERE sf.snapshot_id=? AND (s.required_role='' OR s.required_role=sf.role) AND s.kind='mixin')`
	case "template":
		query = `SELECT sf.path,x.line,sf.role,kind.value,attributes.value FROM content.xml_nodes x JOIN content.strings node_name ON node_name.id=x.name_id JOIN content.strings kind ON kind.id=x.kind_id JOIN content.strings attributes ON attributes.id=x.attributes_id JOIN snapshot_files sf ON sf.content_id=x.content_id WHERE sf.snapshot_id=? AND (x.required_role='' OR x.required_role=sf.role) AND node_name.value=? ORDER BY sf.path,x.line`
		args = []any{snapshotID, name}
		categoryQuery = `SELECT EXISTS(SELECT 1 FROM content.xml_nodes x JOIN content.strings node_name ON node_name.id=x.name_id JOIN snapshot_files sf ON sf.content_id=x.content_id WHERE sf.snapshot_id=? AND (x.required_role='' OR x.required_role=sf.role) AND node_name.value<>'')`
	case "frame-type":
		query = `SELECT sf.path,x.line,sf.role,kind.value,attributes.value FROM content.xml_nodes x JOIN content.strings kind ON kind.id=x.kind_id JOIN content.strings attributes ON attributes.id=x.attributes_id JOIN snapshot_files sf ON sf.content_id=x.content_id WHERE sf.snapshot_id=? AND (x.required_role='' OR x.required_role=sf.role) AND kind.value=? ORDER BY sf.path,x.line`
		args = []any{snapshotID, name}
		categoryQuery = `SELECT EXISTS(SELECT 1 FROM content.xml_nodes x JOIN snapshot_files sf ON sf.content_id=x.content_id WHERE sf.snapshot_id=? AND (x.required_role='' OR x.required_role=sf.role))`
	case "interface":
		query = `SELECT sf.path,t.line,sf.role,t.value,t.key FROM content.toc_entries t JOIN snapshot_files sf ON sf.content_id=t.content_id WHERE sf.snapshot_id=? AND (t.required_role='' OR t.required_role=sf.role) AND lower(t.key) LIKE 'interface%' AND t.value=? ORDER BY sf.path,t.line`
		args = []any{snapshotID, name}
		categoryQuery = `SELECT EXISTS(SELECT 1 FROM content.toc_entries t JOIN snapshot_files sf ON sf.content_id=t.content_id WHERE sf.snapshot_id=? AND (t.required_role='' OR t.required_role=sf.role) AND lower(t.key) LIKE 'interface%')`
	default:
		return nil, false, fmt.Errorf("unsupported compatibility kind %q", kind)
	}
	rows, err := db.Query(query, args...)
	if err != nil {
		return nil, false, err
	}
	defer rows.Close()
	matches := make([]compatibilityMatch, 0)
	for rows.Next() {
		var match compatibilityMatch
		if err = rows.Scan(&match.Path, &match.Line, &match.Role, &match.Signature, &match.Detail); err != nil {
			return nil, false, err
		}
		matches = append(matches, match)
	}
	if err = rows.Err(); err != nil {
		return nil, false, err
	}
	var known bool
	if err = db.QueryRow(categoryQuery, snapshotID).Scan(&known); err != nil {
		return nil, false, err
	}
	return matches, known, nil
}

func normalizeCompatibilityKind(kind string) string {
	switch strings.ToLower(strings.TrimSpace(kind)) {
	case "api", "api-candidate", "function":
		return "api"
	case "event":
		return "event"
	case "template", "xml-template":
		return "template"
	case "mixin":
		return "mixin"
	case "frame-type", "frametype", "frame":
		return "frame-type"
	case "interface":
		return "interface"
	default:
		return ""
	}
}

func unresolvedUsage(ctx Context, usage CompatibilityUsage, reason string) CompatibilityUnresolved {
	expression := strings.TrimSpace(usage.Expression)
	if expression == "" {
		expression = strings.TrimSpace(usage.Name)
	}
	return CompatibilityUnresolved{Kind: usage.Kind, Expression: expression, File: usage.File, Line: usage.Line, Column: usage.Column, Reason: reason, Evidence: compatibilityBaseEvidence(ctx, usage)}
}

func compatibilityBaseEvidence(ctx Context, usage CompatibilityUsage) CompatibilityEvidence {
	evidence := CompatibilityEvidence{
		"sourceId":       ctx.SourceID,
		"product":        ctx.ProductID,
		"requestedRef":   ctx.RequestedRef,
		"matchedTag":     ctx.MatchedTag,
		"resolvedCommit": ctx.Commit,
		"snapshotId":     ctx.SnapshotID,
	}
	if usage.File != "" {
		evidence["usage"] = map[string]any{"file": usage.File, "line": usage.Line, "column": usage.Column}
	}
	return evidence
}

func compatibilityMatchEvidence(matches []compatibilityMatch) []map[string]any {
	sort.SliceStable(matches, func(i, j int) bool {
		if matches[i].Path != matches[j].Path {
			return matches[i].Path < matches[j].Path
		}
		return matches[i].Line < matches[j].Line
	})
	values := make([]map[string]any, 0, len(matches))
	for _, match := range matches {
		values = append(values, map[string]any{"path": match.Path, "line": match.Line, "role": match.Role, "signature": match.Signature, "detail": match.Detail})
	}
	return values
}
