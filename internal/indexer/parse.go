package indexer

import (
	"bytes"
	"encoding/json"
	"encoding/xml"
	"fmt"
	"regexp"
	"strings"

	"github.com/follenfang/wowdoc/internal/objectstore"
	"github.com/follenfang/wowdoc/internal/store"
	"github.com/yuin/gopher-lua/ast"
	"github.com/yuin/gopher-lua/parse"
)

var assetRE = regexp.MustCompile(`(?i)["']([^"']+\.(?:blp|tga|png|jpe?g|dds|ttf|otf|mp3|ogg|wav|m2|wmo))["']`)
var generatedNameRE = regexp.MustCompile(`^\s*Name\s*=\s*["']([^"']+)["']`)
var generatedTypeRE = regexp.MustCompile(`^\s*Type\s*=\s*["']([^"']+)["']`)
var generatedNamespaceRE = regexp.MustCompile(`^\s*Namespace\s*=\s*["']([^"']+)["']`)

func parseLua(path string, data []byte, role string) (any, []store.SymbolFact, []store.EdgeFact, []store.AssetRefFact, error) {
	clean := bytes.TrimPrefix(data, []byte{0xef, 0xbb, 0xbf})
	if bytes.HasPrefix(clean, []byte("#!")) {
		if i := bytes.IndexByte(clean, '\n'); i >= 0 {
			clean = append(bytes.Repeat([]byte{' '}, i), clean[i:]...)
		}
	}
	tree, err := parse.Parse(bytes.NewReader(clean), "<content>")
	if err != nil {
		return nil, nil, nil, nil, err
	}
	symbols, edges := extractLuaFacts(tree, path, strings.Split(string(clean), "\n"))
	var refs []store.AssetRefFact
	lines := strings.Split(string(clean), "\n")
	for i, line := range lines {
		for _, m := range assetRE.FindAllStringSubmatch(line, -1) {
			refs = append(refs, store.AssetRefFact{SourcePath: path, Line: i + 1, Kind: "lua-resource", Value: m[1], NormalizedValue: objectstore.NormalizePath(m[1])})
		}
	}
	apiSymbols, apiEdges := parseGeneratedAPI(path, lines)
	for i := range apiSymbols {
		apiSymbols[i].RequiredRole = "official-generated-api"
	}
	for i := range apiEdges {
		apiEdges[i].RequiredRole = "official-generated-api"
	}
	symbols = append(symbols, apiSymbols...)
	edges = append(edges, apiEdges...)
	return tree, symbols, edges, refs, nil
}

func extractLuaFacts(tree []ast.Stmt, path string, lines []string) ([]store.SymbolFact, []store.EdgeFact) {
	var symbols []store.SymbolFact
	var edges []store.EdgeFact
	lineText := func(line int) string {
		if line > 0 && line <= len(lines) {
			return strings.TrimSpace(lines[line-1])
		}
		return ""
	}
	var walkExpr func(ast.Expr, string)
	var walkStmts func([]ast.Stmt, string)
	walkExpr = func(expr ast.Expr, current string) {
		if expr == nil {
			return
		}
		switch value := expr.(type) {
		case *ast.FuncCallExpr:
			target := callName(value)
			if current == "" {
				current = fmt.Sprintf("<top-level@{path}:%d>", value.Line())
			}
			confidence := "inferred"
			if target == "" {
				target, confidence = "<dynamic>", "dynamic-unresolved"
			}
			edges = append(edges, store.EdgeFact{Source: current, Target: target, Kind: "call", Confidence: confidence, Path: path, Line: value.Line()})
			if (strings.HasSuffix(target, ":RegisterEvent") || strings.HasSuffix(target, ":RegisterUnitEvent")) && len(value.Args) > 0 {
				if event, ok := value.Args[0].(*ast.StringExpr); ok {
					edges = append(edges, store.EdgeFact{Source: current, Target: event.Value, Kind: "register-event", Confidence: "exact", Path: path, Line: value.Line()})
				}
			}
			walkExpr(value.Func, current)
			walkExpr(value.Receiver, current)
			for _, arg := range value.Args {
				walkExpr(arg, current)
			}
		case *ast.FunctionExpr:
			if current == "" {
				current = fmt.Sprintf("<anonymous@{path}:%d>", value.Line())
			}
			walkStmts(value.Stmts, current)
		case *ast.AttrGetExpr:
			walkExpr(value.Object, current)
			walkExpr(value.Key, current)
		case *ast.TableExpr:
			for _, field := range value.Fields {
				walkExpr(field.Key, current)
				walkExpr(field.Value, current)
			}
		case *ast.LogicalOpExpr:
			walkExpr(value.Lhs, current)
			walkExpr(value.Rhs, current)
		case *ast.RelationalOpExpr:
			walkExpr(value.Lhs, current)
			walkExpr(value.Rhs, current)
		case *ast.StringConcatOpExpr:
			walkExpr(value.Lhs, current)
			walkExpr(value.Rhs, current)
		case *ast.ArithmeticOpExpr:
			walkExpr(value.Lhs, current)
			walkExpr(value.Rhs, current)
		case *ast.UnaryMinusOpExpr:
			walkExpr(value.Expr, current)
		case *ast.UnaryNotOpExpr:
			walkExpr(value.Expr, current)
		case *ast.UnaryLenOpExpr:
			walkExpr(value.Expr, current)
		}
	}
	walkStmts = func(stmts []ast.Stmt, current string) {
		for _, statement := range stmts {
			switch value := statement.(type) {
			case *ast.FuncDefStmt:
				name := funcName(value.Name)
				symbols = append(symbols, store.SymbolFact{Name: lastName(name), Qualified: name, Kind: "function", Path: path, Line: value.Line(), EndLine: value.LastLine(), Signature: lineText(value.Line())})
				walkStmts(value.Func.Stmts, name)
			case *ast.LocalAssignStmt:
				for i, expr := range value.Exprs {
					if fn, ok := expr.(*ast.FunctionExpr); ok && i < len(value.Names) {
						name := value.Names[i]
						symbols = append(symbols, store.SymbolFact{Name: name, Qualified: name, Kind: "function", Path: path, Line: value.Line(), EndLine: value.LastLine(), Signature: lineText(value.Line())})
						walkStmts(fn.Stmts, name)
					} else {
						walkExpr(expr, current)
					}
				}
			case *ast.AssignStmt:
				for i, expr := range value.Rhs {
					lhs := ""
					if i < len(value.Lhs) {
						lhs = exprName(value.Lhs[i])
					}
					if fn, ok := expr.(*ast.FunctionExpr); ok && lhs != "" {
						symbols = append(symbols, store.SymbolFact{Name: lastName(lhs), Qualified: lhs, Kind: "function", Path: path, Line: value.Line(), EndLine: value.LastLine(), Signature: lineText(value.Line())})
						walkStmts(fn.Stmts, lhs)
					} else {
						if call, ok := expr.(*ast.FuncCallExpr); ok && callName(call) == "CreateFromMixins" && lhs != "" {
							symbols = append(symbols, store.SymbolFact{Name: lastName(lhs), Qualified: lhs, Kind: "mixin", Path: path, Line: value.Line(), EndLine: value.LastLine(), Signature: lineText(value.Line())})
							for _, arg := range call.Args {
								if target := exprName(arg); target != "" {
									edges = append(edges, store.EdgeFact{Source: lhs, Target: target, Kind: "inherits", Confidence: "exact", Path: path, Line: value.Line()})
								}
							}
						}
						walkExpr(expr, current)
					}
				}
			case *ast.FuncCallStmt:
				walkExpr(value.Expr, current)
			case *ast.DoBlockStmt:
				walkStmts(value.Stmts, current)
			case *ast.WhileStmt:
				walkExpr(value.Condition, current)
				walkStmts(value.Stmts, current)
			case *ast.RepeatStmt:
				walkExpr(value.Condition, current)
				walkStmts(value.Stmts, current)
			case *ast.IfStmt:
				walkExpr(value.Condition, current)
				walkStmts(value.Then, current)
				walkStmts(value.Else, current)
			case *ast.NumberForStmt:
				walkExpr(value.Init, current)
				walkExpr(value.Limit, current)
				walkExpr(value.Step, current)
				walkStmts(value.Stmts, current)
			case *ast.GenericForStmt:
				for _, expr := range value.Exprs {
					walkExpr(expr, current)
				}
				walkStmts(value.Stmts, current)
			case *ast.ReturnStmt:
				for _, expr := range value.Exprs {
					walkExpr(expr, current)
				}
			}
		}
	}
	walkStmts(tree, "")
	return symbols, edges
}

func funcName(name *ast.FuncName) string {
	if name == nil {
		return ""
	}
	if name.Receiver != nil {
		base := exprName(name.Receiver)
		if name.Method != "" {
			return base + ":" + name.Method
		}
		return base
	}
	return exprName(name.Func)
}
func callName(call *ast.FuncCallExpr) string {
	if call == nil {
		return ""
	}
	if call.Receiver != nil {
		base := exprName(call.Receiver)
		if base != "" && call.Method != "" {
			return base + ":" + call.Method
		}
	}
	return exprName(call.Func)
}
func exprName(expr ast.Expr) string {
	switch value := expr.(type) {
	case *ast.IdentExpr:
		return value.Value
	case *ast.StringExpr:
		return value.Value
	case *ast.AttrGetExpr:
		base := exprName(value.Object)
		key := exprName(value.Key)
		if base != "" && key != "" {
			return base + "." + key
		}
	}
	return ""
}

func parseGeneratedAPI(path string, lines []string) ([]store.SymbolFact, []store.EdgeFact) {
	var symbols []store.SymbolFact
	var edges []store.EdgeFact
	name, namespace := "", ""
	nameLine := 0
	for i, line := range lines {
		if match := generatedNamespaceRE.FindStringSubmatch(line); match != nil {
			namespace = match[1]
			continue
		}
		if match := generatedNameRE.FindStringSubmatch(line); match != nil {
			name, nameLine = match[1], i+1
			continue
		}
		match := generatedTypeRE.FindStringSubmatch(line)
		if match == nil || name == "" {
			continue
		}
		kind := strings.ToLower(match[1])
		if kind == "system" {
			namespace = name
			symbols = append(symbols, store.SymbolFact{Name: name, Qualified: name, Kind: "api-system", Path: path, Line: nameLine, EndLine: findLuaTableEnd(lines, nameLine), Signature: name})
		} else {
			qualified := name
			if namespace != "" {
				qualified = namespace + "." + name
			}
			symbols = append(symbols, store.SymbolFact{Name: name, Qualified: qualified, Kind: "api-" + kind, Path: path, Line: nameLine, EndLine: findLuaTableEnd(lines, nameLine), Signature: qualified})
			if namespace != "" {
				edges = append(edges, store.EdgeFact{Source: namespace, Target: qualified, Kind: "contains", Confidence: "exact", Path: path, Line: nameLine})
			}
		}
		name = ""
	}
	return symbols, edges
}

func findLuaTableEnd(lines []string, nameLine int) int {
	start := nameLine - 1
	for start > 0 && !strings.Contains(lines[start], "{") {
		start--
	}
	depth := 0
	seen := false
	for i := start; i < len(lines); i++ {
		depth += strings.Count(lines[i], "{")
		depth -= strings.Count(lines[i], "}")
		if strings.Contains(lines[i], "{") {
			seen = true
		}
		if seen && depth <= 0 {
			return i + 1
		}
	}
	return nameLine
}

type xmlTree struct {
	Nodes    []xmlTreeNode    `json:"nodes"`
	Handlers []map[string]any `json:"handlers,omitempty"`
}

type xmlTreeNode struct {
	Name, Kind, Attributes string
	Line                   int
}

func parseXML(path string, data []byte, role string) (any, []store.XMLFact, []store.EdgeFact, []store.AssetRefFact, error) {
	decoder := xml.NewDecoder(bytes.NewReader(data))
	var nodes []store.XMLFact
	var treeNodes []xmlTreeNode
	var edges []store.EdgeFact
	var refs []store.AssetRefFact
	var stack []string
	var handler strings.Builder
	var handlerStart int
	var handlers []map[string]any
	for {
		offset := decoder.InputOffset()
		token, err := decoder.Token()
		if err != nil {
			if err.Error() == "EOF" {
				break
			}
			return nil, nil, nil, nil, err
		}
		line := 1 + bytes.Count(data[:min64(offset, int64(len(data)))], []byte{'\n'})
		switch t := token.(type) {
		case xml.StartElement:
			stack = append(stack, t.Name.Local)
			attrs := map[string]string{}
			name := ""
			for _, a := range t.Attr {
				attrs[a.Name.Local] = a.Value
				if strings.EqualFold(a.Name.Local, "name") {
					name = a.Value
				}
				if isXMLResourceAttr(a.Name.Local, a.Value) {
					refs = append(refs, store.AssetRefFact{SourcePath: path, Line: line, Kind: "xml-" + a.Name.Local, Value: a.Value, NormalizedValue: objectstore.NormalizePath(a.Value)})
				}
				if strings.EqualFold(a.Name.Local, "inherits") || strings.EqualFold(a.Name.Local, "function") || strings.EqualFold(a.Name.Local, "method") {
					edges = append(edges, store.EdgeFact{Source: name, Target: a.Value, Kind: "xml-" + strings.ToLower(a.Name.Local), Confidence: "exact", Path: path, Line: line})
				}
			}
			raw, _ := json.Marshal(attrs)
			nodes = append(nodes, store.XMLFact{Name: name, Kind: t.Name.Local, Path: path, Line: line, Attributes: string(raw)})
			treeNodes = append(treeNodes, xmlTreeNode{Name: name, Kind: t.Name.Local, Line: line, Attributes: string(raw)})
			if isHandler(t.Name.Local) {
				handler.Reset()
				handlerStart = line
			}
		case xml.CharData:
			if len(stack) > 0 && isHandler(stack[len(stack)-1]) {
				handler.Write([]byte(t))
			}
		case xml.EndElement:
			if isHandler(t.Name.Local) {
				code := handler.String()
				if strings.TrimSpace(code) != "" {
					_, symbols, handlerEdges, handlerRefs, e := parseLua(path+"#"+t.Name.Local, []byte(code), role)
					entry := map[string]any{"kind": t.Name.Local, "line": handlerStart, "valid": e == nil}
					if e != nil {
						entry["diagnostic"] = e.Error()
					} else {
						for i := range symbols {
							symbols[i].Line += handlerStart - 1
						}
						for i := range handlerEdges {
							handlerEdges[i].Line += handlerStart - 1
							handlerEdges[i].Path = path
						}
						for i := range handlerRefs {
							handlerRefs[i].Line += handlerStart - 1
							handlerRefs[i].SourcePath = path
						}
						edges = append(edges, handlerEdges...)
						refs = append(refs, handlerRefs...)
					}
					handlers = append(handlers, entry)
				}
			}
			if len(stack) > 0 {
				stack = stack[:len(stack)-1]
			}
		}
	}
	return xmlTree{Nodes: treeNodes, Handlers: handlers}, nodes, edges, refs, nil
}

func parseTOC(path string, data []byte) (any, []store.TOCFact, error) {
	var facts []store.TOCFact
	var entries []map[string]any
	var files []string
	for i, line := range strings.Split(string(bytes.TrimPrefix(data, []byte{0xef, 0xbb, 0xbf})), "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		if strings.HasPrefix(line, "##") {
			parts := strings.SplitN(strings.TrimSpace(strings.TrimPrefix(line, "##")), ":", 2)
			if len(parts) == 2 {
				key, value := strings.TrimSpace(parts[0]), strings.TrimSpace(parts[1])
				facts = append(facts, store.TOCFact{Path: path, Line: i + 1, Key: key, Value: value})
				entries = append(entries, map[string]any{"line": i + 1, "key": key, "value": value})
			}
		} else if !strings.HasPrefix(line, "#") {
			files = append(files, line)
			facts = append(facts, store.TOCFact{Path: path, Line: i + 1, Key: "File", Value: line})
			entries = append(entries, map[string]any{"line": i + 1, "key": "File", "value": line})
		}
	}
	return map[string]any{"entries": entries, "loadOrder": files}, facts, nil
}
func lastName(v string) string {
	v = strings.ReplaceAll(v, ":", ".")
	parts := strings.Split(v, ".")
	return parts[len(parts)-1]
}
func isHandler(v string) bool {
	switch strings.ToLower(v) {
	case "onload", "onevent", "onclick", "onshow", "onhide", "onupdate", "onenter", "onleave":
		return true
	}
	return false
}
func isXMLResourceAttr(name, value string) bool {
	n := strings.ToLower(name)
	if n == "file" || n == "texture" || n == "font" {
		return true
	}
	return assetRE.MatchString(fmt.Sprintf("%q", value))
}
func min64(a, b int64) int {
	if a < b {
		return int(a)
	}
	return int(b)
}
