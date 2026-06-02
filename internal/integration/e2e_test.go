package integration

import (
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"wowdoc/internal/shared/tools"
)

func TestBuildTargetsExistAndHelpMentionsAgentFields(t *testing.T) {
	root, err := filepath.Abs(filepath.Join("..", ".."))
	if err != nil {
		t.Fatalf("repo root: %v", err)
	}
	wowdoc := filepath.Join(root, "dist", "wowdoc.exe")
	server := filepath.Join(root, "dist", "wowdoc-server.exe")
	cmd := exec.Command("go", "build", "-o", wowdoc, "./cmd/wowdoc")
	cmd.Dir = root
	if err := cmd.Run(); err != nil {
		t.Fatalf("build wowdoc: %v", err)
	}
	cmd = exec.Command("go", "build", "-o", server, "./cmd/wowdoc-server")
	cmd.Dir = root
	if err := cmd.Run(); err != nil {
		t.Fatalf("build wowdoc-server: %v", err)
	}
	out, err := exec.Command(wowdoc, "api", "lookup", "--help").CombinedOutput()
	if err != nil {
		t.Fatalf("help failed: %v\n%s", err, string(out))
	}
	for _, want := range []string{"Required:", "Source resolution:", "MCP arguments:", "Agent next step:"} {
		if !strings.Contains(string(out), want) {
			t.Fatalf("help missing %q:\n%s", want, string(out))
		}
	}
}

func TestOmittedTypeScriptToolsAreAbsent(t *testing.T) {
	registry := tools.DefaultRegistry()
	if _, ok := registry.Tools["scaffold_addon"]; ok {
		t.Fatalf("omitted TypeScript tools must stay absent")
	}
	if _, ok := registry.Tools["get_blizzard_addon"]; ok {
		t.Fatalf("omitted TypeScript tools must stay absent")
	}
	if _, ok := registry.Tools["lint_addon_lua"]; ok {
		t.Fatalf("omitted TypeScript tools must stay absent")
	}
}

func repoRoot(t *testing.T) string {
	t.Helper()
	root, err := filepath.Abs(filepath.Join("..", ".."))
	if err != nil {
		t.Fatalf("repo root: %v", err)
	}
	return root
}
