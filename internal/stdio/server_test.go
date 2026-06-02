package stdio

import (
	"os"
	"path/filepath"
	"testing"
)

func TestDefaultSourceRootUsesExecutableDirectorySources(t *testing.T) {
	root, err := DefaultSourceRoot()
	if err != nil {
		t.Fatalf("DefaultSourceRoot returned error: %v", err)
	}

	exe, err := os.Executable()
	if err != nil {
		t.Fatalf("os.Executable returned error: %v", err)
	}
	want := filepath.Join(filepath.Dir(exe), "sources")
	if root != want {
		t.Fatalf("DefaultSourceRoot = %q, want %q", root, want)
	}
}
