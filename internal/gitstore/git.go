package gitstore

import (
	"bufio"
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/follenfang/wowdoc/internal/catalog"
	"github.com/follenfang/wowdoc/internal/home"
	"github.com/follenfang/wowdoc/internal/lock"
	"github.com/follenfang/wowdoc/internal/result"
	"github.com/follenfang/wowdoc/internal/store"
)

type Manager struct {
	Layout   home.Layout
	Progress io.Writer
}
type TreeEntry struct {
	OID, Path string
	Size      int64
}
type Worktree struct {
	manager  Manager
	sourceID string
	path     string
}
type BlobReader struct {
	cmd    *exec.Cmd
	stdin  io.WriteCloser
	stdout *bufio.Reader
	stderr bytes.Buffer
	mu     sync.Mutex
	closed bool
}

func (m Manager) Mirror(sourceID string) string {
	return filepath.Join(m.Layout.Repositories, sourceID+".git")
}

func (m Manager) CreateWorktree(ctx context.Context, sourceID, commit, taskID string) (*Worktree, error) {
	base := filepath.Join(m.Layout.Temp, "worktrees")
	if err := os.MkdirAll(base, 0o755); err != nil {
		return nil, err
	}
	_ = m.CleanupStaleWorktrees(ctx)
	if taskID == "" || strings.ContainsAny(taskID, `/\\`) {
		return nil, result.E("invalid_task_id", "worktree task id is invalid", 2)
	}
	path := filepath.Join(base, taskID)
	if _, err := os.Stat(path); err == nil {
		return nil, result.E("worktree_exists", "task worktree already exists", 5)
	}
	if _, err := run(ctx, "",
		"-c", "core.longpaths=true",
		"-c", "filter.lfs.smudge=",
		"-c", "filter.lfs.process=",
		"-c", "filter.lfs.required=false",
		"--git-dir", m.Mirror(sourceID), "worktree", "add", "--detach", path, commit,
	); err != nil {
		_, _ = run(ctx, "", "-c", "core.longpaths=true", "--git-dir", m.Mirror(sourceID), "worktree", "remove", "--force", path)
		_ = os.RemoveAll(path)
		_, _ = run(ctx, "", "-c", "core.longpaths=true", "--git-dir", m.Mirror(sourceID), "worktree", "prune")
		return nil, classifyGit(err)
	}
	marker, _ := json.Marshal(map[string]any{"sourceId": sourceID, "commit": commit, "taskId": taskID, "leaseUntil": time.Now().UTC().Add(24 * time.Hour).Format(time.RFC3339)})
	if err := os.WriteFile(filepath.Join(path, ".wowdoc-task.json"), marker, 0o600); err != nil {
		_, _ = run(ctx, "", "-c", "core.longpaths=true", "--git-dir", m.Mirror(sourceID), "worktree", "remove", "--force", path)
		return nil, err
	}
	return &Worktree{manager: m, sourceID: sourceID, path: path}, nil
}

func (m Manager) CleanupStaleWorktrees(ctx context.Context) error {
	base := filepath.Join(m.Layout.Temp, "worktrees")
	entries, err := os.ReadDir(base)
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		return err
	}
	type marker struct{ SourceID, LeaseUntil string }
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		path := filepath.Join(base, entry.Name())
		data, readErr := os.ReadFile(filepath.Join(path, ".wowdoc-task.json"))
		if readErr != nil {
			continue
		}
		var item marker
		if json.Unmarshal(data, &item) != nil || item.SourceID == "" {
			continue
		}
		expires, parseErr := time.Parse(time.RFC3339, item.LeaseUntil)
		if parseErr != nil || time.Now().UTC().Before(expires) {
			continue
		}
		target, _ := filepath.Abs(path)
		root, _ := filepath.Abs(base)
		if target == root || !strings.HasPrefix(strings.ToLower(target), strings.ToLower(root+string(os.PathSeparator))) {
			continue
		}
		_, _ = run(ctx, "", "-c", "core.longpaths=true", "--git-dir", m.Mirror(item.SourceID), "worktree", "remove", "--force", target)
		_, _ = run(ctx, "", "-c", "core.longpaths=true", "--git-dir", m.Mirror(item.SourceID), "worktree", "prune")
	}
	return nil
}

func (w *Worktree) Path() string { return w.path }

func (w *Worktree) Close(ctx context.Context) error {
	if w == nil {
		return nil
	}
	base, _ := filepath.Abs(filepath.Join(w.manager.Layout.Temp, "worktrees"))
	target, err := filepath.Abs(w.path)
	if err != nil || target == base || !strings.HasPrefix(strings.ToLower(target), strings.ToLower(base+string(os.PathSeparator))) {
		return result.E("unsafe_worktree_path", "refusing to remove worktree outside the managed directory", 5)
	}
	_, removeErr := run(ctx, "", "-c", "core.longpaths=true", "--git-dir", w.manager.Mirror(w.sourceID), "worktree", "remove", "--force", target)
	_, pruneErr := run(ctx, "", "-c", "core.longpaths=true", "--git-dir", w.manager.Mirror(w.sourceID), "worktree", "prune")
	if removeErr != nil {
		return classifyGit(removeErr)
	}
	if pruneErr != nil {
		return classifyGit(pruneErr)
	}
	return nil
}

func (m Manager) Sync(ctx context.Context, source catalog.Source) error {
	guard, err := lock.Acquire(filepath.Join(m.Layout.Locks, "repository-"+source.ID+".lock"), "source-sync", 24*time.Hour)
	if err != nil {
		return err
	}
	defer guard.Release()
	if _, err := exec.LookPath("git"); err != nil {
		return result.E("git_not_found", "git is required for source synchronization", 4)
	}
	mirror := m.Mirror(source.ID)
	if _, err := os.Stat(mirror); os.IsNotExist(err) {
		if err := os.MkdirAll(filepath.Dir(mirror), 0o755); err != nil {
			return err
		}
		staging := filepath.Join(filepath.Dir(mirror), "."+source.ID+"-sync.git")
		if err := m.prepareBareStaging(ctx, source, staging); err != nil {
			return classifyGit(err)
		}
		if err := m.syncBareRepository(ctx, source.ID, staging); err != nil {
			return classifyGit(err)
		}
		if _, err := run(ctx, "", "--git-dir", staging, "fsck", "--full", "--no-dangling"); err != nil {
			return result.E("repository_incomplete", "mirror verification found missing or corrupt objects", 5)
		}
		if err := os.Rename(staging, mirror); err != nil {
			return err
		}
	} else if err != nil {
		return err
	} else {
		_ = os.RemoveAll(filepath.Join(filepath.Dir(mirror), "."+source.ID+"-sync.git"))
		partial := m.isPartialMirror(ctx, source.ID)
		if partial {
			return m.replacePartialMirror(ctx, source)
		}
		if _, err := run(ctx, "", "--git-dir", mirror, "remote", "set-url", "origin", source.Repository); err != nil {
			return classifyGit(err)
		}
		if err := m.syncBareRepository(ctx, source.ID, mirror); err != nil {
			return classifyGit(err)
		}
		if _, err := run(ctx, "", "--git-dir", mirror, "fsck", "--full", "--no-dangling"); err != nil {
			return result.E("repository_incomplete", "mirror verification found missing or corrupt objects", 5)
		}
	}
	return nil
}

func (m Manager) prepareBareStaging(ctx context.Context, source catalog.Source, staging string) error {
	if _, err := os.Stat(staging); os.IsNotExist(err) {
		if err := m.initializeBareStaging(ctx, source, staging); err != nil {
			return err
		}
	} else if err != nil {
		return err
	} else {
		bare, bareErr := run(ctx, "", "--git-dir", staging, "rev-parse", "--is-bare-repository")
		if bareErr != nil || strings.TrimSpace(bare) != "true" {
			if removeErr := os.RemoveAll(staging); removeErr != nil {
				return removeErr
			}
			if err := m.initializeBareStaging(ctx, source, staging); err != nil {
				return err
			}
		} else {
			if _, err := run(ctx, "", "--git-dir", staging, "remote", "set-url", "origin", source.Repository); err != nil {
				return err
			}
		}
	}
	_, err := run(ctx, "", "--git-dir", staging, "config", "remote.origin.mirror", "true")
	return err
}

func (m Manager) initializeBareStaging(ctx context.Context, source catalog.Source, staging string) error {
	if _, err := run(ctx, "", "init", "--bare", staging); err != nil {
		return err
	}
	if _, err := run(ctx, "", "--git-dir", staging, "remote", "add", "origin", source.Repository); err != nil {
		return err
	}
	return nil
}

func (m Manager) syncBareRepository(ctx context.Context, sourceID, repository string) error {
	m.progress(sourceID, "fetching branch refs")
	if _, err := m.network(ctx, sourceID, "--git-dir", repository, "fetch", "--prune", "--force", "origin", "+refs/heads/*:refs/heads/*"); err != nil {
		return err
	}
	m.progress(sourceID, "fetching tag refs")
	if _, err := m.network(ctx, sourceID, "--git-dir", repository, "fetch", "--prune", "--force", "origin", "+refs/tags/*:refs/tags/*"); err != nil {
		return err
	}
	return nil
}

func (m Manager) isPartialMirror(ctx context.Context, sourceID string) bool {
	out, err := run(ctx, "", "--git-dir", m.Mirror(sourceID), "config", "--get", "remote.origin.promisor")
	return err == nil && strings.EqualFold(strings.TrimSpace(out), "true")
}

func (m Manager) replacePartialMirror(ctx context.Context, source catalog.Source) error {
	mirror := m.Mirror(source.ID)
	worktrees, err := os.ReadDir(filepath.Join(mirror, "worktrees"))
	if err != nil && !os.IsNotExist(err) {
		return err
	}
	if len(worktrees) > 0 {
		return result.E("operation_in_progress", "partial mirror upgrade is waiting for active worktrees", 5)
	}

	parent := filepath.Dir(mirror)
	staging := filepath.Join(parent, "."+source.ID+"-full.git")
	if err := m.prepareBareStaging(ctx, source, staging); err != nil {
		return classifyGit(err)
	}
	if err := m.syncBareRepository(ctx, source.ID, staging); err != nil {
		return classifyGit(err)
	}
	if _, err := run(ctx, "", "--git-dir", staging, "fsck", "--full", "--no-dangling"); err != nil {
		return result.E("repository_incomplete", "full mirror verification found missing or corrupt objects", 5)
	}

	backup, err := os.MkdirTemp(parent, "."+source.ID+"-partial-")
	if err != nil {
		return err
	}
	if err := os.Remove(backup); err != nil {
		return err
	}
	if err := os.Rename(mirror, backup); err != nil {
		return err
	}
	if err := os.Rename(staging, mirror); err != nil {
		_ = os.Rename(backup, mirror)
		return err
	}
	if err := os.RemoveAll(backup); err != nil {
		return err
	}
	return nil
}

func (m Manager) Head(ctx context.Context, sourceID, branch string) (string, error) {
	out, err := run(ctx, "", "--git-dir", m.Mirror(sourceID), "rev-parse", "--verify", "refs/heads/"+branch+"^{commit}")
	if err != nil {
		return "", result.E("ref_not_found", "branch not found: "+branch, 3)
	}
	return strings.TrimSpace(out), nil
}

func (m Manager) Resolve(ctx context.Context, sourceID, ref string) (string, error) {
	out, err := run(ctx, "", "--git-dir", m.Mirror(sourceID), "rev-parse", "--verify", ref+"^{commit}")
	if err != nil {
		return "", result.E("ref_not_found", "ref not found: "+ref, 3)
	}
	return strings.TrimSpace(out), nil
}

func (m Manager) ReachableTags(ctx context.Context, sourceID string, product catalog.Product, limit int) ([]store.TagRecord, error) {
	out, err := run(ctx, "", "--git-dir", m.Mirror(sourceID), "tag", "--merged", "refs/heads/"+product.Branch, "--sort=-creatordate")
	if err != nil {
		return nil, classifyGit(err)
	}
	var rules []*regexp.Regexp
	for _, raw := range product.TagRules {
		re, e := regexp.Compile(raw)
		if e != nil {
			return nil, e
		}
		rules = append(rules, re)
	}
	var tags []store.TagRecord
	for _, name := range strings.Fields(out) {
		if len(rules) > 0 {
			matched := false
			for _, rule := range rules {
				if rule.MatchString(name) {
					matched = true
					break
				}
			}
			if !matched {
				continue
			}
		}
		commit, e := m.Resolve(ctx, sourceID, "refs/tags/"+name)
		if e != nil {
			continue
		}
		stampRaw, e := run(ctx, "", "--git-dir", m.Mirror(sourceID), "show", "-s", "--format=%ct", commit)
		if e != nil {
			continue
		}
		stamp, _ := strconv.ParseInt(strings.TrimSpace(stampRaw), 10, 64)
		tags = append(tags, store.TagRecord{Name: name, Commit: commit, CommittedAt: stamp})
		if limit > 0 && len(tags) >= limit {
			break
		}
	}
	sort.SliceStable(tags, func(i, j int) bool { return tags[i].CommittedAt > tags[j].CommittedAt })
	return tags, nil
}

func (m Manager) Tree(ctx context.Context, sourceID, commit string) ([]TreeEntry, error) {
	out, err := runBytes(ctx, "", "--git-dir", m.Mirror(sourceID), "ls-tree", "-r", "-z", "--long", commit)
	if err != nil {
		return nil, classifyGit(err)
	}
	parts := bytes.Split(out, []byte{0})
	entries := make([]TreeEntry, 0, len(parts))
	for _, part := range parts {
		if len(part) == 0 {
			continue
		}
		tab := bytes.IndexByte(part, '\t')
		if tab < 0 {
			continue
		}
		meta := strings.Fields(string(part[:tab]))
		if len(meta) < 4 || meta[1] != "blob" {
			continue
		}
		size, _ := strconv.ParseInt(meta[3], 10, 64)
		entries = append(entries, TreeEntry{OID: meta[2], Size: size, Path: string(part[tab+1:])})
	}
	return entries, nil
}

func (m Manager) Blob(ctx context.Context, sourceID, commit, path string) ([]byte, error) {
	return runBytes(ctx, "", "--git-dir", m.Mirror(sourceID), "cat-file", "blob", commit+":"+path)
}

func (m Manager) OpenBlobReader(ctx context.Context, sourceID string) (*BlobReader, error) {
	cmd := exec.CommandContext(ctx, "git", "--git-dir", m.Mirror(sourceID), "cat-file", "--batch")
	cmd.Env = gitEnvironment(ctx)
	stdin, err := cmd.StdinPipe()
	if err != nil {
		return nil, err
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		_ = stdin.Close()
		return nil, err
	}
	reader := &BlobReader{cmd: cmd, stdin: stdin, stdout: bufio.NewReader(stdout)}
	cmd.Stderr = &reader.stderr
	if err = cmd.Start(); err != nil {
		_ = stdin.Close()
		return nil, classifyGit(err)
	}
	return reader, nil
}

func (r *BlobReader) Read(oid string) ([]byte, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.closed {
		return nil, result.E("git_blob_reader_closed", "Git blob reader is closed", 5)
	}
	if _, err := io.WriteString(r.stdin, oid+"\n"); err != nil {
		return nil, classifyGit(fmt.Errorf("git cat-file: %w: %s", err, strings.TrimSpace(r.stderr.String())))
	}
	header, err := r.stdout.ReadString('\n')
	if err != nil {
		return nil, classifyGit(fmt.Errorf("git cat-file: %w: %s", err, strings.TrimSpace(r.stderr.String())))
	}
	fields := strings.Fields(header)
	if len(fields) == 2 && fields[1] == "missing" {
		return nil, result.E("git_blob_missing", "Git blob is missing: "+oid, 4)
	}
	if len(fields) != 3 || fields[1] != "blob" {
		return nil, result.E("git_blob_invalid", "unexpected Git cat-file response", 5)
	}
	size, err := strconv.ParseInt(fields[2], 10, 64)
	if err != nil || size < 0 {
		return nil, result.E("git_blob_invalid", "invalid Git blob size", 5)
	}
	data := make([]byte, size)
	if _, err = io.ReadFull(r.stdout, data); err != nil {
		return nil, classifyGit(fmt.Errorf("git cat-file: %w: %s", err, strings.TrimSpace(r.stderr.String())))
	}
	if terminator, err := r.stdout.ReadByte(); err != nil || terminator != '\n' {
		return nil, result.E("git_blob_invalid", "Git cat-file response was not terminated", 5)
	}
	return data, nil
}

func (r *BlobReader) Close() error {
	if r == nil {
		return nil
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.closed {
		return nil
	}
	r.closed = true
	_ = r.stdin.Close()
	if err := r.cmd.Wait(); err != nil {
		return classifyGit(fmt.Errorf("git cat-file: %w: %s", err, strings.TrimSpace(r.stderr.String())))
	}
	return nil
}

func (m Manager) Check(ctx context.Context, source catalog.Source, product catalog.Product) (map[string]any, error) {
	local := ""
	if _, err := os.Stat(m.Mirror(source.ID)); err == nil {
		local, _ = m.Head(ctx, source.ID, product.Branch)
	}
	out, err := run(ctx, "", "ls-remote", source.Repository, "refs/heads/"+product.Branch)
	if err != nil {
		return nil, classifyGit(err)
	}
	fields := strings.Fields(out)
	if len(fields) == 0 {
		return nil, result.E("ref_not_found", "remote branch not found: "+product.Branch, 3)
	}
	remote := fields[0]
	return map[string]any{"sourceId": source.ID, "product": product.ID, "branch": product.Branch, "localCommit": local, "remoteCommit": remote, "updateAvailable": local != "" && local != remote, "initialized": local != ""}, nil
}

func run(ctx context.Context, dir string, args ...string) (string, error) {
	b, e := runBytes(ctx, dir, args...)
	return string(b), e
}
func runBytes(ctx context.Context, dir string, args ...string) ([]byte, error) {
	cmd := exec.CommandContext(ctx, "git", args...)
	cmd.Dir = dir
	cmd.Env = gitEnvironment(ctx)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	out, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("git %s: %w: %s", args[0], err, strings.TrimSpace(stderr.String()))
	}
	return out, nil
}

const gitNetworkAttempts = 4

var gitProgressMu sync.Mutex

func (m Manager) network(ctx context.Context, sourceID string, args ...string) (string, error) {
	return m.retryNetwork(ctx, sourceID, func(attemptCtx context.Context) (string, error) {
		return m.networkOnce(attemptCtx, sourceID, args...)
	}, func(attempt int) time.Duration {
		return time.Second * time.Duration(1<<(attempt-1))
	})
}

func (m Manager) retryNetwork(ctx context.Context, sourceID string, runAttempt func(context.Context) (string, error), retryDelay func(int) time.Duration) (string, error) {
	var last error
	started := time.Now()
	for attempt := 1; attempt <= gitNetworkAttempts; attempt++ {
		if err := ctx.Err(); err != nil {
			return "", err
		}
		m.progress(sourceID, fmt.Sprintf("network attempt %d/%d", attempt, gitNetworkAttempts))
		attemptCtx, cancel := context.WithTimeout(ctx, 20*time.Minute)
		out, err := runAttempt(attemptCtx)
		attemptErr := attemptCtx.Err()
		cancel()
		if err == nil {
			m.progress(sourceID, fmt.Sprintf("network synchronization complete in %s", time.Since(started).Round(time.Millisecond)))
			return out, nil
		}
		if errors.Is(ctx.Err(), context.Canceled) || errors.Is(ctx.Err(), context.DeadlineExceeded) {
			return "", ctx.Err()
		}
		if errors.Is(attemptErr, context.DeadlineExceeded) {
			err = fmt.Errorf("network timed out: %w", err)
		}
		last = err
		if attempt == gitNetworkAttempts || !retryableGit(err) {
			break
		}
		wait := retryDelay(attempt)
		m.progress(sourceID, fmt.Sprintf("retrying in %s after %s", wait, summarizeGitError(err)))
		timer := time.NewTimer(wait)
		select {
		case <-ctx.Done():
			timer.Stop()
			return "", ctx.Err()
		case <-timer.C:
		}
	}
	return "", last
}

func (m Manager) networkOnce(ctx context.Context, sourceID string, args ...string) (string, error) {
	gitArgs := append([]string(nil), args...)
	for i, arg := range gitArgs {
		if arg == "fetch" || arg == "clone" {
			gitArgs = append(gitArgs[:i+1], append([]string{"--progress"}, gitArgs[i+1:]...)...)
			break
		}
	}
	cmd := exec.CommandContext(ctx, "git", gitArgs...)
	cmd.Env = gitEnvironment(ctx)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	pipe, err := cmd.StderrPipe()
	if err != nil {
		return "", err
	}
	if err = cmd.Start(); err != nil {
		return "", err
	}
	done := make(chan struct{})
	go func() {
		readGitProgress(pipe, sourceID, m.Progress, &stderr)
		close(done)
	}()
	err = cmd.Wait()
	<-done
	if err != nil {
		return "", fmt.Errorf("git network: %w: %s", err, strings.TrimSpace(stderr.String()))
	}
	return stdout.String(), nil
}

func readGitProgress(reader io.Reader, sourceID string, output io.Writer, raw *bytes.Buffer) {
	buffer := make([]byte, 32*1024)
	line := make([]byte, 0, 512)
	last := time.Time{}
	flush := func(force bool) {
		text := strings.TrimSpace(string(line))
		line = line[:0]
		if text == "" || output == nil {
			return
		}
		now := time.Now()
		if !force && !strings.Contains(text, "100%") && now.Sub(last) < 500*time.Millisecond && strings.Contains(text, "%") {
			return
		}
		last = now
		gitProgressMu.Lock()
		fmt.Fprintf(output, "wowdoc: [%s] %s\n", sourceID, redactGitText(text))
		gitProgressMu.Unlock()
	}
	for {
		n, err := reader.Read(buffer)
		if n > 0 {
			raw.Write(buffer[:n])
			for _, value := range buffer[:n] {
				if value == '\r' || value == '\n' {
					flush(false)
					continue
				}
				if len(line) < 16*1024 {
					line = append(line, value)
				}
			}
		}
		if err != nil {
			flush(true)
			return
		}
	}
}

func (m Manager) progress(sourceID, message string) {
	if m.Progress == nil {
		return
	}
	gitProgressMu.Lock()
	fmt.Fprintf(m.Progress, "wowdoc: [%s] %s\n", sourceID, redactGitText(message))
	gitProgressMu.Unlock()
}

func retryableGit(err error) bool {
	text := strings.ToLower(err.Error())
	if strings.Contains(text, "authentication") || strings.Contains(text, "401") || strings.Contains(text, "403") || strings.Contains(text, "404") || strings.Contains(text, "not found") || strings.Contains(text, "repository_incomplete") {
		return false
	}
	for _, marker := range []string{"timed out", "timeout", "operation too slow", "could not resolve host", "connection reset", "connection was reset", "early eof", "remote end hung up", "connection refused", "rpc failed", "curl 18", "curl 28", "curl 52", "curl 56", "http/2 stream", "network", "http 408", "http 429", "http 500", "http 502", "http 503", "http 504", "tls"} {
		if strings.Contains(text, marker) {
			return true
		}
	}
	return false
}

func summarizeGitError(err error) string {
	text := redactGitText(strings.TrimSpace(err.Error()))
	if len(text) > 240 {
		return text[:240] + "..."
	}
	return text
}

var credentialURL = regexp.MustCompile(`(?i)(https?://)[^/@\s]+@`)

func redactGitText(text string) string {
	return credentialURL.ReplaceAllString(text, `${1}<credentials>@`)
}

var ghTokenOnce sync.Once
var cachedGHToken string

func gitEnvironment(ctx context.Context) []string {
	env := os.Environ()
	token := strings.TrimSpace(os.Getenv("GH_TOKEN"))
	if token == "" {
		token = strings.TrimSpace(os.Getenv("GITHUB_TOKEN"))
	}
	if token == "" {
		ghTokenOnce.Do(func() {
			if _, err := exec.LookPath("gh"); err != nil {
				return
			}
			out, err := exec.CommandContext(ctx, "gh", "auth", "token").Output()
			if err == nil {
				cachedGHToken = strings.TrimSpace(string(out))
			}
		})
		token = cachedGHToken
	}
	env = append(env,
		"GIT_TERMINAL_PROMPT=0",
		"GCM_INTERACTIVE=Never",
		"GIT_HTTP_LOW_SPEED_LIMIT=1024",
		"GIT_HTTP_LOW_SPEED_TIME=45",
	)
	if token == "" {
		return env
	}
	encoded := base64.StdEncoding.EncodeToString([]byte("x-access-token:" + token))
	return append(env,
		"GIT_CONFIG_COUNT=1",
		"GIT_CONFIG_KEY_0=http.https://github.com/.extraheader",
		"GIT_CONFIG_VALUE_0=AUTHORIZATION: basic "+encoded,
	)
}
func classifyGit(err error) error {
	message := redactGitText(err.Error())
	s := strings.ToLower(message)
	makeError := func(code string, steps ...string) error {
		e := result.E(code, message, 4)
		e.NextSteps = steps
		return e
	}
	switch {
	case errors.Is(err, context.Canceled) || strings.Contains(s, "context canceled"):
		return makeError("operation_cancelled", "rerun the same wowdoc command to continue from verified checkpoints")
	case strings.Contains(s, "authentication") || strings.Contains(s, "401"):
		return makeError("github_auth_failed", "verify GH_TOKEN, GITHUB_TOKEN, or gh auth status", "rerun the same wowdoc command")
	case strings.Contains(s, "403") || strings.Contains(s, "rate limit"):
		return makeError("github_rate_limited", "wait for the GitHub rate limit to reset or authenticate with gh", "rerun the same wowdoc command")
	case strings.Contains(s, "not found") || strings.Contains(s, "404"):
		return makeError("repository_not_found", "verify the source repository URL and access")
	case strings.Contains(s, "could not resolve host"):
		return makeError("dns_failed", "check DNS and proxy settings", "rerun the same wowdoc command")
	case strings.Contains(s, "proxy"):
		return makeError("proxy_failed", "check HTTPS_PROXY, HTTP_PROXY, NO_PROXY, and Git proxy configuration", "rerun the same wowdoc command")
	case strings.Contains(s, "certificate") || strings.Contains(s, "ssl") || strings.Contains(s, "tls"):
		return makeError("tls_failed", "check the system clock, certificate store, and TLS-inspecting proxy", "rerun the same wowdoc command")
	case strings.Contains(s, "timed out") || strings.Contains(s, "timeout") || strings.Contains(s, "operation too slow"):
		return makeError("network_timeout", "check network stability and proxy settings", "rerun the same wowdoc command to continue from verified checkpoints")
	case strings.Contains(s, "connection reset") || strings.Contains(s, "connection refused") || strings.Contains(s, "early eof") || strings.Contains(s, "remote end hung up") || strings.Contains(s, "failed to connect"):
		return makeError("network_unavailable", "check network connectivity and proxy settings", "rerun the same wowdoc command to continue from verified checkpoints")
	case strings.Contains(s, "no space left") || strings.Contains(s, "disk full"):
		return makeError("disk_full", "free disk space in WOWDOC_HOME", "rerun the same wowdoc command")
	default:
		return makeError("git_failed", "inspect the prefixed Git progress above", "rerun the same wowdoc command")
	}
}

func Context() (context.Context, context.CancelFunc) {
	return context.WithTimeout(context.Background(), 30*time.Minute)
}
