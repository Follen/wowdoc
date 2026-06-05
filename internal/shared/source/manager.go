package source

import (
	"errors"
	"os"
	"path/filepath"
	"strings"

	"wowdoc/internal/shared/contracts"
)

type Options struct {
	Root              string
	AllowArbitraryRef bool
	DefaultRefs       map[string]string
	Repos             map[string]string
	Git               GitRunner
	Archive           ArchiveFetcher
}

type Manager struct {
	root string
	opts Options
}

type ResolvedRef struct {
	Client      string
	Requested   string
	Resolved    string
	CheckoutDir string
	Diagnostics []contracts.Diagnostic
}

type codedError struct {
	code contracts.ErrorCode
	msg  string
}

func (e codedError) Error() string { return e.msg }

func ErrorCode(err error) contracts.ErrorCode {
	var coded codedError
	if errors.As(err, &coded) {
		return coded.code
	}
	return ""
}

func NewManager(opts Options) *Manager {
	return &Manager{root: opts.Root, opts: opts}
}

func (m *Manager) Root() string { return m.root }

func (m *Manager) CheckoutPath(client, resolvedCommit string) string {
	return filepath.Join(m.root, "checkouts", client, resolvedCommit)
}

func (m *Manager) MirrorPath(client string) string {
	return filepath.Join(m.root, "repos", client+".git")
}

func (m *Manager) ArchivePath(client, resolvedCommit string) string {
	return filepath.Join(m.root, "archives", client, resolvedCommit)
}

func (m *Manager) ResolveRef(client, requested string) (ResolvedRef, error) {
	if !safeSegment(client) {
		return ResolvedRef{}, codedError{code: contracts.ErrUnsupportedRef, msg: "unsafe client segment"}
	}
	if requested == "" || requested == "latest" {
		requested = m.opts.DefaultRefs[client]
	}
	if requested == "" {
		return ResolvedRef{}, codedError{code: contracts.ErrRefNotFound, msg: "default ref not found"}
	}
	if !safeRef(requested) {
		return ResolvedRef{}, codedError{code: contracts.ErrUnsupportedRef, msg: "unsafe ref segment"}
	}
	if !m.opts.AllowArbitraryRef && requested != m.opts.DefaultRefs[client] {
		return ResolvedRef{}, codedError{code: contracts.ErrUnsupportedRef, msg: "arbitrary ref disabled"}
	}
	resolved := refStorageKey(requested)
	return ResolvedRef{
		Client:      client,
		Requested:   requested,
		Resolved:    resolved,
		CheckoutDir: m.CheckoutPath(client, resolved),
	}, nil
}

func (m *Manager) ResolveSource(client, requested string) (ResolvedRef, error) {
	resolved, err := m.ResolveRef(client, requested)
	if err != nil {
		return ResolvedRef{}, err
	}
	repoURL, ok := m.opts.Repos[client]
	if !ok {
		return ResolvedRef{}, codedError{code: contracts.ErrClientNotFound, msg: "repo is not configured for client"}
	}
	if m.opts.Git != nil {
		gitResolved, err := m.resolveGitRef(repoURL, resolved)
		if err != nil {
			if archive, archiveErr := m.acquireWithArchive(repoURL, resolved); archiveErr == nil {
				return archive, nil
			} else {
				return ResolvedRef{}, archiveErr
			}
		}
		if info, err := os.Stat(gitResolved.CheckoutDir); err == nil && info.IsDir() {
			return gitResolved, nil
		}
		if err := m.acquireWithGit(gitResolved); err != nil {
			if archive, archiveErr := m.acquireWithArchive(repoURL, resolved); archiveErr == nil {
				return archive, nil
			} else {
				return ResolvedRef{}, archiveErr
			}
		}
		if info, err := os.Stat(gitResolved.CheckoutDir); err == nil && info.IsDir() {
			return gitResolved, nil
		}
	}
	if info, err := os.Stat(resolved.CheckoutDir); err == nil && info.IsDir() {
		return resolved, nil
	}
	if archive, err := m.acquireWithArchive(repoURL, resolved); err == nil {
		return archive, nil
	} else {
		return ResolvedRef{}, err
	}
}

func (m *Manager) resolveGitRef(repoURL string, resolved ResolvedRef) (ResolvedRef, error) {
	mirror := m.MirrorPath(resolved.Client)
	if err := os.MkdirAll(filepath.Dir(mirror), 0o755); err != nil {
		return ResolvedRef{}, err
	}
	if _, err := os.Stat(mirror); os.IsNotExist(err) {
		if err := m.initBareRepo(mirror, repoURL); err != nil {
			return ResolvedRef{}, err
		}
	} else if err == nil {
		out, err := m.opts.Git.Output("--git-dir", mirror, "config", "--get", "remote.origin.url")
		if err != nil {
			return ResolvedRef{}, err
		}
		if strings.TrimSpace(string(out)) != repoURL {
			if err := os.RemoveAll(mirror); err != nil {
				return ResolvedRef{}, err
			}
			if err := m.initBareRepo(mirror, repoURL); err != nil {
				return ResolvedRef{}, err
			}
		}
	} else {
		return ResolvedRef{}, err
	}
	if err := m.opts.Git.Run("--git-dir", mirror, "fetch", "--depth=1", "origin", resolved.Requested+":refs/heads/"+resolved.Requested); err != nil {
		return ResolvedRef{}, err
	}
	out, err := m.opts.Git.Output("--git-dir", mirror, "rev-parse", resolved.Requested+"^{commit}")
	if err != nil {
		return ResolvedRef{}, err
	}
	commit := strings.TrimSpace(string(out))
	if !safeSegment(commit) {
		return ResolvedRef{}, codedError{code: contracts.ErrRefNotFound, msg: "resolved commit is invalid"}
	}
	resolved.Resolved = commit
	resolved.CheckoutDir = m.CheckoutPath(resolved.Client, commit)
	return resolved, nil
}

func (m *Manager) initBareRepo(mirror, repoURL string) error {
	if err := os.MkdirAll(mirror, 0o755); err != nil {
		return err
	}
	if err := m.opts.Git.Run("--git-dir", mirror, "init", "--bare"); err != nil {
		return err
	}
	return m.opts.Git.Run("--git-dir", mirror, "remote", "add", "origin", repoURL)
}

func (m *Manager) acquireWithGit(resolved ResolvedRef) error {
	mirror := m.MirrorPath(resolved.Client)
	if err := os.MkdirAll(filepath.Dir(mirror), 0o755); err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(resolved.CheckoutDir), 0o755); err != nil {
		return err
	}
	return m.opts.Git.Run("--git-dir", mirror, "worktree", "add", "--detach", resolved.CheckoutDir, resolved.Resolved)
}

func (m *Manager) acquireWithArchive(repoURL string, resolved ResolvedRef) (ResolvedRef, error) {
	if m.opts.Archive == nil {
		return ResolvedRef{}, codedError{code: contracts.ErrGitUnavailableArchiveFailed, msg: "archive fetcher is not configured"}
	}
	archive := resolved
	archive.CheckoutDir = m.ArchivePath(resolved.Client, resolved.Resolved)
	archive.Diagnostics = archiveFallbackDiagnostics(archive.CheckoutDir)
	if info, err := os.Stat(archive.CheckoutDir); err == nil && info.IsDir() {
		return archive, nil
	}
	if err := os.MkdirAll(filepath.Dir(archive.CheckoutDir), 0o755); err != nil {
		return ResolvedRef{}, err
	}
	if err := m.opts.Archive.FetchArchive(repoURL, resolved.Requested, archive.CheckoutDir); err != nil {
		return ResolvedRef{}, codedError{code: contracts.ErrGitUnavailableArchiveFailed, msg: err.Error()}
	}
	if info, err := os.Stat(archive.CheckoutDir); err == nil && info.IsDir() {
		return archive, nil
	}
	return ResolvedRef{}, codedError{code: contracts.ErrGitUnavailableArchiveFailed, msg: "archive checkout not found"}
}

func archiveFallbackDiagnostics(path string) []contracts.Diagnostic {
	return []contracts.Diagnostic{
		{Path: path, Message: "archive fallback is not incremental"},
		{Path: path, Message: "branch archives may need periodic redownload"},
		{Path: path, Message: "commit archives are immutable and may be cached permanently"},
		{Path: path, Message: "resolved commit may be unknown for archive fallback"},
	}
}

func safeSegment(value string) bool {
	if value == "" || value == "." || value == ".." || filepath.IsAbs(value) {
		return false
	}
	if strings.ContainsAny(value, `/\`) {
		return false
	}
	return filepath.Clean(value) == value
}

func safeRef(value string) bool {
	if value == "" || filepath.IsAbs(value) || strings.Contains(value, `\`) {
		return false
	}
	for _, segment := range strings.Split(filepath.ToSlash(value), "/") {
		if segment == "" || segment == "." || segment == ".." {
			return false
		}
	}
	return true
}

func refStorageKey(value string) string {
	return strings.ReplaceAll(filepath.ToSlash(value), "/", "__")
}
