package indexer_test

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/follenfang/wowdoc/internal/gitstore"
	"github.com/follenfang/wowdoc/internal/home"
	"github.com/follenfang/wowdoc/internal/indexer"
	"github.com/follenfang/wowdoc/internal/query"
)

func TestGitBuildUsesAndRemovesDetachedWorktree(t *testing.T) {
	git, err := exec.LookPath("git")
	if err != nil {
		t.Skip("git is not installed")
	}
	root := t.TempDir()
	source := filepath.Join(root, "source")
	runGit(t, git, root, "init", source)
	runGit(t, git, source, "config", "user.email", "test@example.invalid")
	runGit(t, git, source, "config", "user.name", "wowdoc test")
	if err = os.WriteFile(filepath.Join(source, "Core.lua"), []byte("function Addon:Initialize()\n  self:RegisterEvent(\"PLAYER_LOGIN\")\nend\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	runGit(t, git, source, "add", "Core.lua")
	runGit(t, git, source, "commit", "-m", "fixture")
	commit := strings.TrimSpace(runGit(t, git, source, "rev-parse", "HEAD"))
	t.Setenv("WOWDOC_HOME", filepath.Join(root, "home"))
	layout, _ := home.Resolve()
	if err = layout.Ensure(); err != nil {
		t.Fatal(err)
	}
	runGit(t, git, root, "clone", "--mirror", source, filepath.Join(layout.Repositories, "fixture.git"))
	runGit(t, git, root, "--git-dir", filepath.Join(layout.Repositories, "fixture.git"), "config", "core.autocrlf", "true")
	manager := gitstore.Manager{Layout: layout}
	stats, err := indexer.Build(context.Background(), indexer.BuildOptions{Layout: layout, SourceID: "fixture", ProductID: "main", Commit: commit, Input: &indexer.GitInput{Manager: manager, SourceID: "fixture", ProductID: "main", Commit: commit}, Workers: 4})
	if err != nil {
		t.Fatal(err)
	}
	response, err := query.Search(layout, query.Context{SourceID: "fixture", ProductID: "main", Commit: commit, SnapshotID: stats.SnapshotID}, "Addon:Initialize", "lua", 3)
	if err != nil {
		t.Fatal(err)
	}
	if len(response.Results) == 0 || response.Results[0].Path != "Core.lua" {
		t.Fatalf("unexpected response: %#v", response)
	}
	raw := []byte("function Addon:Initialize()\n  self:RegisterEvent(\"PLAYER_LOGIN\")\nend\n")
	sum := sha256.Sum256(raw)
	if response.Results[0].ContentHash != hex.EncodeToString(sum[:]) {
		t.Fatalf("content hash does not identify the committed blob: %s", response.Results[0].ContentHash)
	}
	entries, err := os.ReadDir(filepath.Join(layout.Temp, "worktrees"))
	if err != nil && !os.IsNotExist(err) {
		t.Fatal(err)
	}
	if len(entries) != 0 {
		t.Fatalf("worktree was not removed: %#v", entries)
	}
}

func runGit(t *testing.T, git, dir string, args ...string) string {
	t.Helper()
	cmd := exec.Command(git, args...)
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %v: %v\n%s", args, err, out)
	}
	return string(out)
}
