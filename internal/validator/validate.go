package validator

import "path/filepath"

type Options struct {
	Path           string
	TOC            string
	ID             string
	SourceID       string
	Product        string
	RequestedRef   string
	MatchedTag     string
	ResolvedCommit string
	Evidence       EvidenceLookup
}

func Validate(options Options) (Result, error) {
	closure, err := BuildClosure(options.Path, options.TOC)
	if err != nil {
		return Result{}, err
	}
	usages, unresolved, diagnostics := Analyze(closure)
	diagnostics = append(closure.Diagnostics, diagnostics...)
	facts := make([]CompatibilityFact, 0)
	if options.Evidence != nil {
		resolved, extraUnresolved, extraDiagnostics, lookupErr := options.Evidence.Lookup(usages, closure.Interface)
		if lookupErr != nil {
			return Result{}, lookupErr
		}
		facts = append(facts, resolved...)
		unresolved = append(unresolved, extraUnresolved...)
		diagnostics = append(diagnostics, extraDiagnostics...)
	}
	checkedLua, checkedXML := 0, 0
	for _, file := range closure.Files {
		if file.Type == "lua" {
			checkedLua++
		}
		if file.Type == "xml" {
			checkedXML++
		}
	}
	if diagnostics == nil {
		diagnostics = make([]Diagnostic, 0)
	}
	if unresolved == nil {
		unresolved = make([]Unresolved, 0)
	}
	if facts == nil {
		facts = make([]CompatibilityFact, 0)
	}
	if closure.Files == nil {
		closure.Files = make([]LoadFile, 0)
	}
	checked := len(usages)
	if closure.Interface != "" && options.Evidence != nil {
		checked++
	}
	result := Result{
		ID: options.ID, Valid: !hasErrors(diagnostics), Path: filepath.Clean(options.Path),
		SourceID: options.SourceID, Product: options.Product, Ref: options.RequestedRef, RequestedRef: options.RequestedRef,
		MatchedTag: options.MatchedTag, ResolvedCommit: options.ResolvedCommit,
		TOC: closure.TOC, Interface: closure.Interface, CheckedLua: checkedLua, CheckedXML: checkedXML,
		LoadClosure: closure.Files, Diagnostics: diagnostics, Unresolved: unresolved,
		Coverage: Coverage{Checked: checked, Resolved: len(facts), Unresolved: len(unresolved)}, Facts: facts,
	}
	return result, nil
}

func hasErrors(diagnostics []Diagnostic) bool {
	for _, diagnostic := range diagnostics {
		if diagnostic.Severity == "error" {
			return true
		}
	}
	return false
}
