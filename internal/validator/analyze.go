package validator

import (
	"bytes"
	"encoding/xml"
	"fmt"
	"strings"

	"github.com/yuin/gopher-lua/ast"
	"github.com/yuin/gopher-lua/parse"
)

func Analyze(closure Closure) ([]Usage, []Unresolved, []Diagnostic) {
	usages := make([]Usage, 0)
	unresolved := make([]Unresolved, 0)
	diagnostics := make([]Diagnostic, 0)
	for _, file := range closure.Files {
		data := closure.Contents[file.Path]
		switch file.Type {
		case "lua":
			u, x, d := analyzeLua(file.Path, data)
			usages = append(usages, u...)
			unresolved = append(unresolved, x...)
			diagnostics = append(diagnostics, d...)
		case "xml":
			u, x := analyzeXML(file.Path, data)
			usages = append(usages, u...)
			unresolved = append(unresolved, x...)
		}
	}
	return dedupeUsages(usages), unresolved, diagnostics
}

func analyzeLua(path string, data []byte) ([]Usage, []Unresolved, []Diagnostic) {
	clean := bytes.TrimPrefix(data, []byte{0xef, 0xbb, 0xbf})
	tree, err := parse.Parse(bytes.NewReader(clean), path)
	if err != nil {
		return nil, nil, []Diagnostic{{Severity: "error", Code: "lua_parse_failed", File: path, Line: parseErrorLine(err), Column: 0, Message: err.Error(), Evidence: Evidence{"parser": "gopher-lua"}}}
	}
	var usages []Usage
	var unresolved []Unresolved
	var walkExpr func(ast.Expr)
	var walkStmts func([]ast.Stmt)
	walkExpr = func(expr ast.Expr) {
		if expr == nil {
			return
		}
		switch value := expr.(type) {
		case *ast.FuncCallExpr:
			name := callName(value)
			line := value.Line()
			if name == "" {
				unresolved = append(unresolved, unresolvedAt("api", "<dynamic call>", path, line, "call target is computed"))
			} else if strings.HasPrefix(name, "C_") && strings.Contains(name, ".") {
				usages = append(usages, Usage{Kind: "api", Name: name, File: path, Line: line})
			} else if !strings.Contains(name, ":") {
				usages = append(usages, Usage{Kind: "api-candidate", Name: name, File: path, Line: line})
			}
			switch {
			case strings.HasSuffix(name, ":RegisterEvent") || strings.HasSuffix(name, ":RegisterUnitEvent"):
				if len(value.Args) > 0 {
					if event, ok := value.Args[0].(*ast.StringExpr); ok {
						usages = append(usages, Usage{Kind: "event", Name: event.Value, File: path, Line: line})
					} else {
						unresolved = append(unresolved, unresolvedAt("event", expressionName(value.Args[0]), path, line, "event name is computed"))
					}
				}
			case name == "CreateFrame":
				if len(value.Args) > 0 {
					appendLiteralUsage(&usages, &unresolved, "frame-type", value.Args[0], path, line)
				}
				if len(value.Args) > 3 {
					appendCSVUsages(&usages, &unresolved, "template", value.Args[3], path, line)
				}
			case name == "CreateFromMixins":
				for _, arg := range value.Args {
					if target := expressionName(arg); target != "" {
						usages = append(usages, Usage{Kind: "mixin", Name: target, File: path, Line: line})
					} else {
						unresolved = append(unresolved, unresolvedAt("mixin", "<dynamic>", path, line, "Mixin is computed"))
					}
				}
			}
			walkExpr(value.Func)
			walkExpr(value.Receiver)
			for _, arg := range value.Args {
				walkExpr(arg)
			}
		case *ast.FunctionExpr:
			walkStmts(value.Stmts)
		case *ast.AttrGetExpr:
			walkExpr(value.Object)
			walkExpr(value.Key)
		case *ast.TableExpr:
			for _, field := range value.Fields {
				walkExpr(field.Key)
				walkExpr(field.Value)
			}
		case *ast.LogicalOpExpr:
			walkExpr(value.Lhs)
			walkExpr(value.Rhs)
		case *ast.RelationalOpExpr:
			walkExpr(value.Lhs)
			walkExpr(value.Rhs)
		case *ast.StringConcatOpExpr:
			walkExpr(value.Lhs)
			walkExpr(value.Rhs)
		case *ast.ArithmeticOpExpr:
			walkExpr(value.Lhs)
			walkExpr(value.Rhs)
		case *ast.UnaryMinusOpExpr:
			walkExpr(value.Expr)
		case *ast.UnaryNotOpExpr:
			walkExpr(value.Expr)
		case *ast.UnaryLenOpExpr:
			walkExpr(value.Expr)
		}
	}
	walkStmts = func(stmts []ast.Stmt) {
		for _, statement := range stmts {
			switch value := statement.(type) {
			case *ast.FuncDefStmt:
				walkStmts(value.Func.Stmts)
			case *ast.LocalAssignStmt:
				for _, expr := range value.Exprs {
					walkExpr(expr)
				}
			case *ast.AssignStmt:
				for _, expr := range value.Rhs {
					walkExpr(expr)
				}
			case *ast.FuncCallStmt:
				walkExpr(value.Expr)
			case *ast.DoBlockStmt:
				walkStmts(value.Stmts)
			case *ast.WhileStmt:
				walkExpr(value.Condition)
				walkStmts(value.Stmts)
			case *ast.RepeatStmt:
				walkExpr(value.Condition)
				walkStmts(value.Stmts)
			case *ast.IfStmt:
				walkExpr(value.Condition)
				walkStmts(value.Then)
				walkStmts(value.Else)
			case *ast.NumberForStmt:
				walkExpr(value.Init)
				walkExpr(value.Limit)
				walkExpr(value.Step)
				walkStmts(value.Stmts)
			case *ast.GenericForStmt:
				for _, expr := range value.Exprs {
					walkExpr(expr)
				}
				walkStmts(value.Stmts)
			case *ast.ReturnStmt:
				for _, expr := range value.Exprs {
					walkExpr(expr)
				}
			}
		}
	}
	walkStmts(tree)
	return usages, unresolved, nil
}

func analyzeXML(path string, data []byte) ([]Usage, []Unresolved) {
	decoder := xml.NewDecoder(bytes.NewReader(data))
	var usages []Usage
	var unresolved []Unresolved
	for {
		offset := decoder.InputOffset()
		token, err := decoder.Token()
		if err != nil {
			break
		}
		start, ok := token.(xml.StartElement)
		if !ok {
			continue
		}
		line := 1 + bytes.Count(data[:minInt64(offset, int64(len(data)))], []byte{'\n'})
		kind := strings.ToLower(start.Name.Local)
		if kind != "ui" && kind != "script" && kind != "include" && kind != "scripts" && kind != "layers" && kind != "frames" && kind != "anchors" && kind != "size" {
			usages = append(usages, Usage{Kind: "frame-type", Name: start.Name.Local, File: path, Line: line})
		}
		for _, attr := range start.Attr {
			if strings.EqualFold(attr.Name.Local, "inherits") {
				for _, name := range splitCSV(attr.Value) {
					if strings.ContainsAny(name, "$%+") {
						unresolved = append(unresolved, unresolvedAt("template", name, path, line, "XML inheritance is dynamic"))
					} else {
						usages = append(usages, Usage{Kind: "template", Name: name, File: path, Line: line})
					}
				}
			}
		}
	}
	return usages, unresolved
}

func appendLiteralUsage(usages *[]Usage, unresolved *[]Unresolved, kind string, expr ast.Expr, path string, line int) {
	if value, ok := expr.(*ast.StringExpr); ok {
		*usages = append(*usages, Usage{Kind: kind, Name: value.Value, File: path, Line: line})
		return
	}
	*unresolved = append(*unresolved, unresolvedAt(kind, expressionName(expr), path, line, kind+" is computed"))
}

func appendCSVUsages(usages *[]Usage, unresolved *[]Unresolved, kind string, expr ast.Expr, path string, line int) {
	if value, ok := expr.(*ast.StringExpr); ok {
		for _, name := range splitCSV(value.Value) {
			*usages = append(*usages, Usage{Kind: kind, Name: name, File: path, Line: line})
		}
		return
	}
	*unresolved = append(*unresolved, unresolvedAt(kind, expressionName(expr), path, line, kind+" is computed"))
}

func splitCSV(value string) []string {
	parts := strings.Split(value, ",")
	out := make([]string, 0, len(parts))
	for _, part := range parts {
		if value := strings.TrimSpace(part); value != "" {
			out = append(out, value)
		}
	}
	return out
}

func unresolvedAt(kind, expression, path string, line int, reason string) Unresolved {
	if expression == "" {
		expression = "<dynamic>"
	}
	return Unresolved{Kind: kind, Expression: expression, File: path, Line: line, Column: 0, Reason: reason, Evidence: Evidence{"confidence": "dynamic-unresolved"}}
}

func callName(call *ast.FuncCallExpr) string {
	if call == nil {
		return ""
	}
	if call.Receiver != nil {
		if base := expressionName(call.Receiver); base != "" && call.Method != "" {
			return base + ":" + call.Method
		}
	}
	return expressionName(call.Func)
}

func expressionName(expr ast.Expr) string {
	switch value := expr.(type) {
	case *ast.IdentExpr:
		return value.Value
	case *ast.StringExpr:
		return value.Value
	case *ast.AttrGetExpr:
		base, key := expressionName(value.Object), expressionName(value.Key)
		if base != "" && key != "" {
			return base + "." + key
		}
	}
	return ""
}

func parseErrorLine(err error) int {
	var line int
	_, _ = fmt.Sscanf(err.Error(), "%*[^:]:%d:", &line)
	return line
}

func dedupeUsages(input []Usage) []Usage {
	seen := map[string]bool{}
	out := make([]Usage, 0, len(input))
	for _, usage := range input {
		key := fmt.Sprintf("%s\x00%s\x00%s\x00%d", usage.Kind, usage.Name, usage.File, usage.Line)
		if seen[key] {
			continue
		}
		seen[key] = true
		out = append(out, usage)
	}
	return out
}

func minInt64(a, b int64) int {
	if a < b {
		return int(a)
	}
	return int(b)
}
