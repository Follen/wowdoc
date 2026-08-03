package gitstore

import (
	"context"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/follenfang/wowdoc/internal/catalog"
	"github.com/follenfang/wowdoc/internal/home"
)

func TestSyncCreatesFullBareMirror(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git is not installed")
	}

	root := t.TempDir()
	repository := filepath.Join(root, "source")
	runGitTest(t, root, "init", "--initial-branch=main", repository)
	runGitTest(t, repository, "config", "user.name", "wowdoc test")
	runGitTest(t, repository, "config", "user.email", "wowdoc@example.invalid")
	if err := os.WriteFile(filepath.Join(repository, "Fixture.lua"), []byte("local value = 42\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	runGitTest(t, repository, "add", "Fixture.lua")
	runGitTest(t, repository, "commit", "-m", "fixture")

	layout := home.Layout{
		Repositories: filepath.Join(root, "home", "repositories"),
		Locks:        filepath.Join(root, "home", "locks"),
	}
	manager := Manager{Layout: layout}
	if err := manager.Sync(context.Background(), catalog.Source{ID: "fixture", Repository: repository}); err != nil {
		t.Fatalf("Sync() error = %v", err)
	}

	mirror := manager.Mirror("fixture")
	if got := strings.TrimSpace(runGitTest(t, root, "--git-dir", mirror, "rev-parse", "--is-bare-repository")); got != "true" {
		t.Fatalf("mirror bare status = %q, want true", got)
	}
	runGitTest(t, root, "--git-dir", mirror, "cat-file", "-e", "refs/heads/main:Fixture.lua")
	assertGitConfigMissing(t, mirror, "remote.origin.promisor")
	assertGitConfigMissing(t, mirror, "remote.origin.partialclonefilter")
}

func TestSyncHydratesExistingPartialMirror(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git is not installed")
	}

	root := t.TempDir()
	repository := filepath.Join(root, "source")
	runGitTest(t, root, "init", "--initial-branch=main", repository)
	runGitTest(t, repository, "config", "user.name", "wowdoc test")
	runGitTest(t, repository, "config", "user.email", "wowdoc@example.invalid")
	runGitTest(t, repository, "config", "uploadpack.allowFilter", "true")
	if err := os.WriteFile(filepath.Join(repository, "Fixture.lua"), []byte("local value = 42\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	runGitTest(t, repository, "add", "Fixture.lua")
	runGitTest(t, repository, "commit", "-m", "fixture")

	layout := home.Layout{
		Repositories: filepath.Join(root, "home", "repositories"),
		Locks:        filepath.Join(root, "home", "locks"),
	}
	if err := os.MkdirAll(layout.Repositories, 0o755); err != nil {
		t.Fatal(err)
	}
	repositoryURL := (&url.URL{Scheme: "file", Path: filepath.ToSlash(repository)}).String()
	mirror := filepath.Join(layout.Repositories, "fixture.git")
	runGitTest(t, root, "clone", "--mirror", "--filter=blob:none", repositoryURL, mirror)
	if got := strings.TrimSpace(runGitTest(t, root, "--git-dir", mirror, "config", "--get", "remote.origin.promisor")); got != "true" {
		t.Fatalf("partial mirror promisor = %q, want true", got)
	}

	manager := Manager{Layout: layout}
	if err := manager.Sync(context.Background(), catalog.Source{ID: "fixture", Repository: repositoryURL}); err != nil {
		t.Fatalf("Sync() error = %v", err)
	}

	runGitTest(t, root, "--git-dir", mirror, "cat-file", "-e", "refs/heads/main:Fixture.lua")
	assertGitConfigMissing(t, mirror, "remote.origin.promisor")
	assertGitConfigMissing(t, mirror, "remote.origin.partialclonefilter")
	missing := runGitTest(t, root, "--git-dir", mirror, "rev-list", "--objects", "--all", "--missing=print")
	for _, line := range strings.Split(missing, "\n") {
		if strings.HasPrefix(line, "?") {
			t.Fatalf("partial mirror still has missing object %s", line)
		}
	}
}

func TestCreateWorktreeKeepsLFSBlobWithoutSmudge(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git is not installed")
	}

	root := t.TempDir()
	repository := filepath.Join(root, "source")
	runGitTest(t, root, "init", "--initial-branch=main", repository)
	runGitTest(t, repository, "config", "user.name", "wowdoc test")
	runGitTest(t, repository, "config", "user.email", "wowdoc@example.invalid")
	attributes := "*.bin filter=lfs diff=lfs merge=lfs -text\n"
	pointer := "version https://git-lfs.github.com/spec/v1\noid sha256:0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef\nsize 42\n"
	if err := os.WriteFile(filepath.Join(repository, ".gitattributes"), []byte(attributes), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(repository, "Asset.bin"), []byte(pointer), 0o644); err != nil {
		t.Fatal(err)
	}
	runGitTest(t, repository, "add", ".gitattributes", "Asset.bin")
	runGitTest(t, repository, "commit", "-m", "lfs pointer fixture")

	layout := home.Layout{
		Repositories: filepath.Join(root, "home", "repositories"),
		Locks:        filepath.Join(root, "home", "locks"),
		Temp:         filepath.Join(root, "home", "tmp"),
	}
	manager := Manager{Layout: layout}
	if err := manager.Sync(context.Background(), catalog.Source{ID: "fixture", Repository: repository}); err != nil {
		t.Fatalf("Sync() error = %v", err)
	}
	commit := strings.TrimSpace(runGitTest(t, root, "--git-dir", manager.Mirror("fixture"), "rev-parse", "refs/heads/main"))
	runGitTest(t, root, "--git-dir", manager.Mirror("fixture"), "config", "filter.lfs.smudge", "false")
	runGitTest(t, root, "--git-dir", manager.Mirror("fixture"), "config", "filter.lfs.required", "true")

	worktree, err := manager.CreateWorktree(context.Background(), "fixture", commit, "lfs-fixture")
	if err != nil {
		t.Fatalf("CreateWorktree() error = %v", err)
	}
	defer worktree.Close(context.Background())
	got, err := os.ReadFile(filepath.Join(worktree.Path(), "Asset.bin"))
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != pointer {
		t.Fatalf("worktree asset = %q, want original LFS pointer", got)
	}
}

func TestCreateWorktreeSupportsLongPaths(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git is not installed")
	}

	root := t.TempDir()
	repository := filepath.Join(root, "source")
	runGitTest(t, root, "init", "--initial-branch=main", repository)
	runGitTest(t, repository, "config", "user.name", "wowdoc test")
	runGitTest(t, repository, "config", "user.email", "wowdoc@example.invalid")
	segment := strings.Repeat("long-directory-", 3)
	relativePath := filepath.Join(segment+"a", segment+"b", segment+"c", segment+"d", "Fixture.lua")
	fullPath := filepath.Join(repository, relativePath)
	if err := os.MkdirAll(filepath.Dir(fullPath), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(fullPath, []byte("local value = 42\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	runGitTest(t, repository, "-c", "core.longpaths=true", "add", "--", relativePath)
	runGitTest(t, repository, "commit", "-m", "long path fixture")

	layout := home.Layout{
		Repositories: filepath.Join(root, "home", "repositories"),
		Locks:        filepath.Join(root, "home", "locks"),
		Temp:         filepath.Join(root, "home", "tmp"),
	}
	manager := Manager{Layout: layout}
	if err := manager.Sync(context.Background(), catalog.Source{ID: "fixture", Repository: repository}); err != nil {
		t.Fatalf("Sync() error = %v", err)
	}
	commit := strings.TrimSpace(runGitTest(t, root, "--git-dir", manager.Mirror("fixture"), "rev-parse", "refs/heads/main"))
	worktree, err := manager.CreateWorktree(context.Background(), "fixture", commit, "long-path-fixture")
	if err != nil {
		t.Fatalf("CreateWorktree() error = %v", err)
	}
	if _, err := os.Stat(filepath.Join(worktree.Path(), relativePath)); err != nil {
		t.Fatalf("long path was not checked out: %v", err)
	}
	if err := worktree.Close(context.Background()); err != nil {
		t.Fatalf("Close() error = %v", err)
	}
}

func assertGitConfigMissing(t *testing.T, mirror, key string) {
	t.Helper()
	cmd := exec.Command("git", "--git-dir", mirror, "config", "--get", key)
	if output, err := cmd.CombinedOutput(); err == nil {
		t.Fatalf("git config %s unexpectedly set to %q", key, strings.TrimSpace(string(output)))
	}
}

func runGitTest(t *testing.T, directory string, args ...string) string {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = directory
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %s: %v\n%s", strings.Join(args, " "), err, output)
	}
	return string(output)
}
