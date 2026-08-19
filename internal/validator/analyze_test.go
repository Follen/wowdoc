package validator

import "testing"

func TestAnalyzeLuaStaticAndDynamicReferences(t *testing.T) {
	closure := Closure{Files: []LoadFile{{Path: "main.lua", Type: "lua"}}, Contents: map[string][]byte{"main.lua": []byte(`
C_Test.StaticCall()
frame:RegisterEvent("PLAYER_LOGIN")
frame:RegisterEvent(eventName)
CreateFrame("Button", "Name", UIParent, "SecureActionButtonTemplate")
CreateFromMixins(ExampleMixin)
`)}}
	usages, unresolved, diagnostics := Analyze(closure)
	if len(diagnostics) != 0 {
		t.Fatalf("diagnostics=%#v", diagnostics)
	}
	want := map[string]bool{"api:C_Test.StaticCall": true, "event:PLAYER_LOGIN": true, "frame-type:Button": true, "template:SecureActionButtonTemplate": true, "mixin:ExampleMixin": true}
	for _, usage := range usages {
		delete(want, usage.Kind+":"+usage.Name)
	}
	if len(want) != 0 {
		t.Fatalf("missing usages: %#v; got=%#v", want, usages)
	}
	if len(unresolved) != 1 || unresolved[0].Kind != "event" {
		t.Fatalf("unresolved=%#v", unresolved)
	}
	foundCandidate := false
	for _, usage := range usages {
		if usage.Kind == "api-candidate" && usage.Name == "CreateFrame" {
			foundCandidate = true
		}
	}
	if !foundCandidate {
		t.Fatalf("global API candidate missing: %#v", usages)
	}
}

func TestAnalyzeLuaSyntaxDiagnosticShape(t *testing.T) {
	closure := Closure{Files: []LoadFile{{Path: "bad.lua", Type: "lua"}}, Contents: map[string][]byte{"bad.lua": []byte("local =")}}
	_, _, diagnostics := Analyze(closure)
	if len(diagnostics) != 1 {
		t.Fatalf("diagnostics=%#v", diagnostics)
	}
	d := diagnostics[0]
	if d.Severity != "error" || d.Code != "lua_parse_failed" || d.File != "bad.lua" || d.Evidence == nil {
		t.Fatalf("diagnostic=%#v", d)
	}
}
