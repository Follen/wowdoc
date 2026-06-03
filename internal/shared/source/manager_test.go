package source

import (
	"os"
	"path/filepath"
	"testing"
)

func TestCheckoutPathIsIsolatedByClientAndResolvedCommit(t *testing.T) {
	m := NewManager(Options{Root: t.TempDir()})
	got := m.CheckoutPath("classic", "1111111")
	want := filepath.Join(m.Root(), "checkouts", "classic", "1111111")
	if got != want {
		t.Fatalf("checkout path = %q, want %q", got, want)
	}
	other := m.CheckoutPath("classic", "2222222")
	if got == other {
		t.Fatalf("different commits must not share checkout path")
	}
}

func TestUnsupportedArbitraryRefIsRejectedWhenDisabled(t *testing.T) {
	m := NewManager(Options{Root: t.TempDir(), AllowArbitraryRef: false, DefaultRefs: map[string]string{"retail": "main"}})
	_, err := m.ResolveRef("retail", "feature-branch")
	if err == nil || ErrorCode(err) != "unsupported_ref" {
		t.Fatalf("expected unsupported_ref, got %v", err)
	}
}

func TestResolveRefRejectsUnsafePathSegments(t *testing.T) {
	m := NewManager(Options{Root: t.TempDir(), AllowArbitraryRef: true, DefaultRefs: map[string]string{"retail": "main"}})
	for _, tc := range []struct {
		name   string
		client string
		ref    string
	}{
		{name: "client traversal", client: `..\classic`, ref: "main"},
		{name: "ref traversal", client: "retail", ref: `..\..\outside`},
		{name: "ref dot segment", client: "retail", ref: "feature/../branch"},
		{name: "absolute ref", client: "retail", ref: filepath.Join(t.TempDir(), "commit")},
	} {
		t.Run(tc.name, func(t *testing.T) {
			_, err := m.ResolveRef(tc.client, tc.ref)
			if err == nil || ErrorCode(err) != "unsupported_ref" {
				t.Fatalf("expected unsafe segment to be rejected with unsupported_ref, got %v", err)
			}
		})
	}
}

func TestResolveSourceSupportsSlashBranchRefsWithGit(t *testing.T) {
	root := t.TempDir()
	git := &recordingGit{root: root, resolvedCommit: "abc123def456"}
	m := NewManager(Options{
		Root:              root,
		AllowArbitraryRef: true,
		DefaultRefs:       map[string]string{"retail": "main"},
		Repos:             map[string]string{"retail": "https://example.test/wow-ui-source.git"},
		Git:               git,
	})

	resolved, err := m.ResolveSource("retail", "feature/auction-fix")
	if err != nil {
		t.Fatalf("ResolveSource failed: %v", err)
	}
	if resolved.Requested != "feature/auction-fix" || resolved.Resolved != "abc123def456" {
		t.Fatalf("resolved ref = %#v", resolved)
	}
	if resolved.CheckoutDir != filepath.Join(root, "checkouts", "retail", "abc123def456") {
		t.Fatalf("checkout dir = %q", resolved.CheckoutDir)
	}
	wantResolve := []string{"--git-dir", filepath.Join(root, "repos", "retail.git"), "rev-parse", "feature/auction-fix^{commit}"}
	if !containsCommand(git.commands, wantResolve) {
		t.Fatalf("git commands = %#v, missing rev-parse %#v", git.commands, wantResolve)
	}
}

func TestResolveSourceSupportsTagRefsWithGit(t *testing.T) {
	root := t.TempDir()
	git := &recordingGit{root: root, resolvedCommit: "tag123def456"}
	m := NewManager(Options{
		Root:              root,
		AllowArbitraryRef: true,
		DefaultRefs:       map[string]string{"retail": "main"},
		Repos:             map[string]string{"retail": "https://example.test/wow-ui-source.git"},
		Git:               git,
	})

	resolved, err := m.ResolveSource("retail", "v11.2.0")
	if err != nil {
		t.Fatalf("ResolveSource failed: %v", err)
	}
	if resolved.Requested != "v11.2.0" || resolved.Resolved != "tag123def456" {
		t.Fatalf("resolved ref = %#v", resolved)
	}
	wantResolve := []string{"--git-dir", filepath.Join(root, "repos", "retail.git"), "rev-parse", "v11.2.0^{commit}"}
	if !containsCommand(git.commands, wantResolve) {
		t.Fatalf("git commands = %#v, missing rev-parse %#v", git.commands, wantResolve)
	}
}

func TestResolveSourceSupportsCommitRefsWithGit(t *testing.T) {
	root := t.TempDir()
	commit := "abc123def456"
	git := &recordingGit{root: root, resolvedCommit: commit}
	m := NewManager(Options{
		Root:              root,
		AllowArbitraryRef: true,
		DefaultRefs:       map[string]string{"retail": "main"},
		Repos:             map[string]string{"retail": "https://example.test/wow-ui-source.git"},
		Git:               git,
	})

	resolved, err := m.ResolveSource("retail", commit)
	if err != nil {
		t.Fatalf("ResolveSource failed: %v", err)
	}
	if resolved.Requested != commit || resolved.Resolved != commit {
		t.Fatalf("resolved ref = %#v", resolved)
	}
	if resolved.CheckoutDir != filepath.Join(root, "checkouts", "retail", commit) {
		t.Fatalf("checkout dir = %q", resolved.CheckoutDir)
	}
	wantResolve := []string{"--git-dir", filepath.Join(root, "repos", "retail.git"), "rev-parse", commit + "^{commit}"}
	if !containsCommand(git.commands, wantResolve) {
		t.Fatalf("git commands = %#v, missing rev-parse %#v", git.commands, wantResolve)
	}
}

func TestResolveSourceUsesExistingIsolatedCheckoutForConfiguredRepoRef(t *testing.T) {
	root := t.TempDir()
	checkout := filepath.Join(root, "checkouts", "retail", "main")
	if err := os.MkdirAll(checkout, 0o755); err != nil {
		t.Fatalf("create checkout: %v", err)
	}
	m := NewManager(Options{
		Root:              root,
		AllowArbitraryRef: false,
		DefaultRefs:       map[string]string{"retail": "main"},
		Repos:             map[string]string{"retail": "https://example.test/wow-ui-source.git"},
	})

	resolved, err := m.ResolveSource("retail", "")
	if err != nil {
		t.Fatalf("ResolveSource failed: %v", err)
	}
	if resolved.Requested != "main" || resolved.Resolved != "main" || resolved.CheckoutDir != checkout {
		t.Fatalf("resolved source = %#v, want existing isolated checkout %q", resolved, checkout)
	}
}

func TestResolveSourceAcquiresMissingCheckoutWithGit(t *testing.T) {
	root := t.TempDir()
	git := &recordingGit{root: root}
	m := NewManager(Options{
		Root:              root,
		AllowArbitraryRef: false,
		DefaultRefs:       map[string]string{"retail": "main"},
		Repos:             map[string]string{"retail": "https://example.test/wow-ui-source.git"},
		Git:               git,
	})

	resolved, err := m.ResolveSource("retail", "")
	if err != nil {
		t.Fatalf("ResolveSource failed: %v", err)
	}
	if resolved.CheckoutDir != filepath.Join(root, "checkouts", "retail", "main") {
		t.Fatalf("checkout dir = %q", resolved.CheckoutDir)
	}
	want := [][]string{
		{"clone", "--mirror", "https://example.test/wow-ui-source.git", filepath.Join(root, "repos", "retail.git")},
		{"--git-dir", filepath.Join(root, "repos", "retail.git"), "fetch"},
		{"--git-dir", filepath.Join(root, "repos", "retail.git"), "rev-parse", "main^{commit}"},
		{"--git-dir", filepath.Join(root, "repos", "retail.git"), "worktree", "add", "--detach", filepath.Join(root, "checkouts", "retail", "main"), "main"},
	}
	if !equalCommands(git.commands, want) {
		t.Fatalf("git commands = %#v, want %#v", git.commands, want)
	}
}

func TestResolveSourceUsesResolvedGitCommitForCheckoutIsolation(t *testing.T) {
	root := t.TempDir()
	git := &recordingGit{root: root, resolvedCommit: "abc123def456"}
	m := NewManager(Options{
		Root:              root,
		AllowArbitraryRef: false,
		DefaultRefs:       map[string]string{"retail": "main"},
		Repos:             map[string]string{"retail": "https://example.test/wow-ui-source.git"},
		Git:               git,
	})

	resolved, err := m.ResolveSource("retail", "")
	if err != nil {
		t.Fatalf("ResolveSource failed: %v", err)
	}
	if resolved.Requested != "main" || resolved.Resolved != "abc123def456" {
		t.Fatalf("resolved ref = %#v", resolved)
	}
	if resolved.CheckoutDir != filepath.Join(root, "checkouts", "retail", "abc123def456") {
		t.Fatalf("checkout dir = %q", resolved.CheckoutDir)
	}
	wantResolve := []string{"--git-dir", filepath.Join(root, "repos", "retail.git"), "rev-parse", "main^{commit}"}
	if !containsCommand(git.commands, wantResolve) {
		t.Fatalf("git commands = %#v, missing rev-parse %#v", git.commands, wantResolve)
	}
}

func TestResolveSourceFallsBackToArchiveWhenGitFails(t *testing.T) {
	root := t.TempDir()
	archive := &recordingArchive{}
	m := NewManager(Options{
		Root:              root,
		AllowArbitraryRef: false,
		DefaultRefs:       map[string]string{"retail": "main"},
		Repos:             map[string]string{"retail": "https://example.test/wow-ui-source.git"},
		Git:               failingGit{},
		Archive:           archive,
	})

	resolved, err := m.ResolveSource("retail", "")
	if err != nil {
		t.Fatalf("ResolveSource failed: %v", err)
	}
	if resolved.CheckoutDir != filepath.Join(root, "archives", "retail", "main") {
		t.Fatalf("checkout dir = %q, want archive path", resolved.CheckoutDir)
	}
	if archive.repoURL != "https://example.test/wow-ui-source.git" || archive.ref != "main" || archive.destination != resolved.CheckoutDir {
		t.Fatalf("archive fetch = %#v", archive)
	}
}

func TestResolveSourceFallsBackToArchiveForSlashBranchRef(t *testing.T) {
	root := t.TempDir()
	archive := &recordingArchive{}
	m := NewManager(Options{
		Root:              root,
		AllowArbitraryRef: true,
		DefaultRefs:       map[string]string{"retail": "main"},
		Repos:             map[string]string{"retail": "https://example.test/wow-ui-source.git"},
		Git:               failingGit{},
		Archive:           archive,
	})

	resolved, err := m.ResolveSource("retail", "feature/auction-fix")
	if err != nil {
		t.Fatalf("ResolveSource failed: %v", err)
	}
	if resolved.Requested != "feature/auction-fix" || resolved.Resolved != "feature__auction-fix" {
		t.Fatalf("resolved ref = %#v", resolved)
	}
	if resolved.CheckoutDir != filepath.Join(root, "archives", "retail", "feature__auction-fix") {
		t.Fatalf("checkout dir = %q", resolved.CheckoutDir)
	}
	if archive.ref != "feature/auction-fix" || archive.destination != resolved.CheckoutDir {
		t.Fatalf("archive fetch = %#v", archive)
	}
}

func TestResolveSourceReturnsStableArchiveFailureCodeWhenGitAndArchiveFail(t *testing.T) {
	m := NewManager(Options{
		Root:              t.TempDir(),
		AllowArbitraryRef: false,
		DefaultRefs:       map[string]string{"retail": "main"},
		Repos:             map[string]string{"retail": "https://example.test/wow-ui-source.git"},
		Git:               failingGit{},
		Archive:           failingArchive{},
	})

	_, err := m.ResolveSource("retail", "")
	if err == nil || ErrorCode(err) != "git_unavailable_archive_failed" {
		t.Fatalf("expected git_unavailable_archive_failed, got %v", err)
	}
}

func TestResolveSourceReturnsStableArchiveFailureCodeWithoutGit(t *testing.T) {
	m := NewManager(Options{
		Root:              t.TempDir(),
		AllowArbitraryRef: false,
		DefaultRefs:       map[string]string{"retail": "main"},
		Repos:             map[string]string{"retail": "https://example.test/wow-ui-source.git"},
		Archive:           failingArchive{},
	})

	_, err := m.ResolveSource("retail", "")
	if err == nil || ErrorCode(err) != "git_unavailable_archive_failed" {
		t.Fatalf("expected git_unavailable_archive_failed, got %v", err)
	}
}

type recordingGit struct {
	root           string
	resolvedCommit string
	commands       [][]string
}

type failingGit struct{}

func (failingGit) Run(args ...string) error { return os.ErrNotExist }
func (failingGit) Output(args ...string) ([]byte, error) {
	return nil, os.ErrNotExist
}

type failingArchive struct{}

func (failingArchive) FetchArchive(repoURL, ref, destination string) error {
	return os.ErrNotExist
}

type recordingArchive struct {
	repoURL     string
	ref         string
	destination string
}

func (a *recordingArchive) FetchArchive(repoURL, ref, destination string) error {
	a.repoURL = repoURL
	a.ref = ref
	a.destination = destination
	return os.MkdirAll(destination, 0o755)
}

func (g *recordingGit) Run(args ...string) error {
	g.commands = append(g.commands, append([]string(nil), args...))
	if len(args) == 4 && args[0] == "clone" && args[1] == "--mirror" {
		return os.MkdirAll(args[3], 0o755)
	}
	if len(args) == 7 && args[2] == "worktree" && args[3] == "add" {
		return os.MkdirAll(args[5], 0o755)
	}
	return nil
}

func (g *recordingGit) Output(args ...string) ([]byte, error) {
	g.commands = append(g.commands, append([]string(nil), args...))
	if g.resolvedCommit != "" && len(args) == 4 && args[2] == "rev-parse" {
		return []byte(g.resolvedCommit + "\n"), nil
	}
	return []byte("main\n"), nil
}

func equalCommands(got, want [][]string) bool {
	if len(got) != len(want) {
		return false
	}
	for i := range got {
		if len(got[i]) != len(want[i]) {
			return false
		}
		for j := range got[i] {
			if got[i][j] != want[i][j] {
				return false
			}
		}
	}
	return true
}

func containsCommand(commands [][]string, want []string) bool {
	for _, command := range commands {
		if equalCommands([][]string{command}, [][]string{want}) {
			return true
		}
	}
	return false
}
