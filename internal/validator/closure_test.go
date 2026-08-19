package validator

import (
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"testing"
)

func TestBuildClosureUsesOrderedTOCClosure(t *testing.T) {
	root := filepath.Join(t.TempDir(), "Addon With Spaces")
	if err := os.MkdirAll(root, 0o755); err != nil {
		t.Fatal(err)
	}
	writeClosureFixture(t, root, "Addon.toc", "\ufeff## Interface: 50504\r\n# ignored\r\nXML\\Root.xml\r\nDirect.lua\r\n")
	writeClosureFixture(t, root, "XML/Root.xml", `<x:Ui xmlns:x="urn:test"><x:sCrIpT x:FiLe="../Nested.lua"/><x:Include file="Child.xml"/></x:Ui>`)
	writeClosureFixture(t, root, "XML/Child.xml", `<Ui><Script file="Grandchild.lua"/></Ui>`)
	writeClosureFixture(t, root, "XML/Grandchild.lua", "local grandchild = true")
	writeClosureFixture(t, root, "Nested.lua", "local nested = true")
	writeClosureFixture(t, root, "Direct.lua", "local direct = true")
	writeClosureFixture(t, root, "Outside.lua", "this is deliberately outside the closure")

	closure, err := BuildClosure(root, "addon.TOC")
	if err != nil {
		t.Fatal(err)
	}
	if closure.Interface != "50504" {
		t.Fatalf("Interface = %q", closure.Interface)
	}
	want := []string{"XML/Root.xml", "Nested.lua", "XML/Child.xml", "XML/Grandchild.lua", "Direct.lua"}
	if got := closurePaths(closure); !reflect.DeepEqual(got, want) {
		t.Fatalf("paths = %#v, want %#v", got, want)
	}
	if _, ok := closure.Contents["Outside.lua"]; ok {
		t.Fatal("file outside closure was read")
	}
	if len(closure.Diagnostics) != 0 {
		t.Fatalf("diagnostics = %#v", closure.Diagnostics)
	}
	for i, file := range closure.Files {
		if file.LoadOrder != i {
			t.Fatalf("load order at %d = %d", i, file.LoadOrder)
		}
	}
}

func TestBuildClosureDuplicatePreservesSources(t *testing.T) {
	root := t.TempDir()
	writeClosureFixture(t, root, "Addon.toc", "First.xml\nShared.lua\n")
	writeClosureFixture(t, root, "First.xml", "<Ui>\n  <Script file=\"Shared.lua\"/>\n  <Include file=\"Nested.xml\"/>\n</Ui>")
	writeClosureFixture(t, root, "Nested.xml", "<Ui><Script file=\"Shared.lua\"/></Ui>")
	writeClosureFixture(t, root, "Shared.lua", "return true")

	closure, err := BuildClosure(root, "Addon.toc")
	if err != nil {
		t.Fatal(err)
	}
	shared := findClosureFile(t, closure, "Shared.lua")
	if len(shared.LoadedBy) != 3 {
		t.Fatalf("loadedBy = %#v", shared.LoadedBy)
	}
	if shared.LoadedBy[0] != (LoadSource{File: "First.xml", Line: 2, Kind: "script"}) {
		t.Fatalf("first source = %#v", shared.LoadedBy[0])
	}
	if countDiagnostic(closure, diagnosticDuplicate) != 2 {
		t.Fatalf("diagnostics = %#v", closure.Diagnostics)
	}
}

func TestBuildClosureXMLCycleTerminates(t *testing.T) {
	root := t.TempDir()
	writeClosureFixture(t, root, "Addon.toc", "A.xml\n")
	writeClosureFixture(t, root, "A.xml", `<Ui><Include file="B.xml"/></Ui>`)
	writeClosureFixture(t, root, "B.xml", `<Ui><Include file="A.xml"/></Ui>`)

	closure, err := BuildClosure(root, "Addon.toc")
	if err != nil {
		t.Fatal(err)
	}
	if got := closurePaths(closure); !reflect.DeepEqual(got, []string{"A.xml", "B.xml"}) {
		t.Fatalf("paths = %#v", got)
	}
	if countDiagnostic(closure, diagnosticCycle) != 1 {
		t.Fatalf("diagnostics = %#v", closure.Diagnostics)
	}
	a := findClosureFile(t, closure, "A.xml")
	if len(a.LoadedBy) != 2 {
		t.Fatalf("cycle source not retained: %#v", a.LoadedBy)
	}
}

func TestBuildClosureReportsFailuresAndContinues(t *testing.T) {
	root := t.TempDir()
	outside := filepath.Join(filepath.Dir(root), "escape.lua")
	if err := os.WriteFile(outside, []byte("return true"), 0o600); err != nil {
		t.Fatal(err)
	}
	writeClosureFixture(t, root, "Addon.toc", "Missing.lua\n../escape.lua\nNotes.txt\nBroken.xml\nGood.lua\n")
	writeClosureFixture(t, root, "Notes.txt", "not loadable")
	writeClosureFixture(t, root, "Broken.xml", `<Ui><Script file="Good.lua"></Ui>`)
	writeClosureFixture(t, root, "Good.lua", "return true")

	closure, err := BuildClosure(root, "Addon.toc")
	if err != nil {
		t.Fatal(err)
	}
	for _, code := range []string{diagnosticMissingFile, diagnosticPathEscape, diagnosticUnsupportedType, diagnosticXMLParse} {
		if countDiagnostic(closure, code) != 1 {
			t.Errorf("diagnostic %s count = %d; all = %#v", code, countDiagnostic(closure, code), closure.Diagnostics)
		}
	}
	if got := closurePaths(closure); !reflect.DeepEqual(got, []string{"Broken.xml", "Good.lua"}) {
		t.Fatalf("paths = %#v", got)
	}
	for _, diagnostic := range closure.Diagnostics {
		if diagnostic.Severity == "" || diagnostic.File == "" || diagnostic.Message == "" || diagnostic.Evidence == nil {
			t.Fatalf("unstable diagnostic = %#v", diagnostic)
		}
	}
}

func TestBuildClosureRejectsSymlinkEscape(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("creating symlinks commonly requires extra Windows privileges")
	}
	root := t.TempDir()
	outside := t.TempDir()
	writeClosureFixture(t, outside, "escaped.lua", "return true")
	if err := os.Symlink(outside, filepath.Join(root, "linked")); err != nil {
		t.Skipf("symlink unavailable: %v", err)
	}
	writeClosureFixture(t, root, "Addon.toc", "linked/escaped.lua\n")

	closure, err := BuildClosure(root, "Addon.toc")
	if err != nil {
		t.Fatal(err)
	}
	if countDiagnostic(closure, diagnosticPathEscape) != 1 {
		t.Fatalf("diagnostics = %#v", closure.Diagnostics)
	}
	if len(closure.Files) != 0 {
		t.Fatalf("escaped file loaded: %#v", closure.Files)
	}
}

func TestBuildClosureEmptySlicesAreStable(t *testing.T) {
	root := t.TempDir()
	writeClosureFixture(t, root, "Addon.toc", "## Interface: 120100\n")
	closure, err := BuildClosure(root, filepath.Join(root, "Addon.toc"))
	if err != nil {
		t.Fatal(err)
	}
	if closure.Files == nil || closure.Diagnostics == nil || closure.Contents == nil {
		t.Fatalf("nil collection in %#v", closure)
	}
}

func writeClosureFixture(t *testing.T, root, name, contents string) {
	t.Helper()
	file := filepath.Join(root, filepath.FromSlash(name))
	if err := os.MkdirAll(filepath.Dir(file), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(file, []byte(contents), 0o600); err != nil {
		t.Fatal(err)
	}
}

func closurePaths(closure Closure) []string {
	paths := make([]string, len(closure.Files))
	for i, file := range closure.Files {
		paths[i] = file.Path
	}
	return paths
}

func findClosureFile(t *testing.T, closure Closure, name string) LoadFile {
	t.Helper()
	for _, file := range closure.Files {
		if file.Path == name {
			return file
		}
	}
	t.Fatalf("file %s not found in %#v", name, closure.Files)
	return LoadFile{}
}

func countDiagnostic(closure Closure, code string) int {
	count := 0
	for _, diagnostic := range closure.Diagnostics {
		if diagnostic.Code == code {
			count++
		}
	}
	return count
}
