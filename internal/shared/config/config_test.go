package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestDefaultSeedsAreBootstrapDefaults(t *testing.T) {
	seeds := DefaultSourceSeeds()
	wantAliases := []string{"retail", "classic", "classic-ptr", "classic-titan", "ptr2"}
	if len(seeds) != len(wantAliases) {
		t.Fatalf("seed count = %d, want %d", len(seeds), len(wantAliases))
	}
	for i, alias := range wantAliases {
		if seeds[i].Alias != alias {
			t.Fatalf("seed %d alias = %q, want %q", i, seeds[i].Alias, alias)
		}
		if seeds[i].Repo == "" || seeds[i].Ref == "" {
			t.Fatalf("seed %q must include repo and ref: %#v", alias, seeds[i])
		}
	}
}

func TestModuleKeepsPlannedGoBaselineAndMCPSDKDependency(t *testing.T) {
	mod := readModuleFileForTest(t)
	if !containsLine(mod, "go 1.23") && !containsLine(mod, "go 1.23.0") {
		t.Fatalf("go.mod must keep planned Go baseline 1.23.x:\n%s", mod)
	}
	if !containsLine(mod, "\tgithub.com/modelcontextprotocol/go-sdk v") &&
		!containsLine(mod, "github.com/modelcontextprotocol/go-sdk v") {
		t.Fatalf("go.mod must keep the official MCP Go SDK dependency:\n%s", mod)
	}
}

func readModuleFileForTest(t *testing.T) string {
	t.Helper()
	b, err := os.ReadFile(filepath.Join("..", "..", "..", "go.mod"))
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

func containsLine(text, needle string) bool {
	for _, line := range strings.Split(text, "\n") {
		if strings.Contains(line, needle) {
			return true
		}
	}
	return false
}
