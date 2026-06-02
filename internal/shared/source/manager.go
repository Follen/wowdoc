package source

import (
	"errors"
	"path/filepath"

	"wowdoc/internal/shared/contracts"
)

type Options struct {
	Root              string
	AllowArbitraryRef bool
	DefaultRefs       map[string]string
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
	if requested == "" || requested == "latest" {
		requested = m.opts.DefaultRefs[client]
	}
	if requested == "" {
		return ResolvedRef{}, codedError{code: contracts.ErrRefNotFound, msg: "default ref not found"}
	}
	if !m.opts.AllowArbitraryRef && requested != m.opts.DefaultRefs[client] {
		return ResolvedRef{}, codedError{code: contracts.ErrUnsupportedRef, msg: "arbitrary ref disabled"}
	}
	resolved := requested
	return ResolvedRef{
		Client:      client,
		Requested:   requested,
		Resolved:    resolved,
		CheckoutDir: m.CheckoutPath(client, resolved),
	}, nil
}
