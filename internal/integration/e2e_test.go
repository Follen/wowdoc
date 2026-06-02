package integration

import (
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
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
	cmd := exec.Command("go", "test", "./internal/shared/tools", "-run", "TestRegistryContainsExactlySupportedTools", "-v")
	cmd.Dir = repoRoot(t)
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("registry test failed: %v\n%s", err, string(out))
	}
	if strings.Contains(string(out), "scaffold_addon") {
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
