package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestDefaultSeedsAreBootstrapDefaults(t *testing.T) {
	seeds := DefaultSourceSeeds()
	wantAliases := []string{"retail", "classic", "classic-ptr", "classic-titan", "ptr", "ptr2"}
	if len(seeds) != len(wantAliases) {
		t.Fatalf("seed count = %d, want %d", len(seeds), len(wantAliases))
	}
	wantRefs := map[string]string{
		"retail":        "live",
		"classic":       "classic",
		"classic-ptr":   "classic_ptr",
		"classic-titan": "classic_titan",
		"ptr":           "ptr2",
		"ptr2":          "ptr2",
	}
	for i, alias := range wantAliases {
		if seeds[i].Alias != alias {
			t.Fatalf("seed %d alias = %q, want %q", i, seeds[i].Alias, alias)
		}
		if seeds[i].Repo != "https://github.com/Gethe/wow-ui-source.git" {
			t.Fatalf("seed %q repo = %q, want Gethe/wow-ui-source.git", alias, seeds[i].Repo)
		}
		if seeds[i].Ref != wantRefs[alias] {
			t.Fatalf("seed %q ref = %q, want %q", alias, seeds[i].Ref, wantRefs[alias])
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
