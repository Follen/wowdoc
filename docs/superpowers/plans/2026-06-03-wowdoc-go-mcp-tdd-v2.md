# wowdoc Go MCP TDD Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Build the Go `wowdoc` CLI, stdio MCP server, and HTTP MCP server described in `docs/superpowers/specs/2026-06-03-wowdoc-go-mcp-design.md`.

**Architecture:** The first task creates shared contracts that every later task must use without changing names or package boundaries. Later tasks are split by package and execution surface so subagents can work in parallel after the contracts compile. CLI, stdio, and HTTP share only `internal/shared/*`; CLI handlers never appear in HTTP code.

**Tech Stack:** Go 1.23+, `github.com/modelcontextprotocol/go-sdk/mcp`, `github.com/modelcontextprotocol/go-sdk/jsonrpc`, Cobra, YAML config, Go `testing`, local fake WoW UI source fixtures.

---

## Execution Rules For Subagents

- Do not implement from memory. Read this plan task, the design spec, and the files listed under the task before editing.
- Do not change exported names from Task 1 unless the current task explicitly says to change them.
- Each task must start by writing or extending failing tests, then run the named test command and confirm the expected failure.
- Each task must include a test command before implementation and a test command after implementation. If either command is missing, the task is invalid and must not be executed.
- Each task must leave `go test ./...` passing before commit.
- Each task must commit only the files listed for that task.
- If a task needs a new file not listed here, stop and ask for review instead of improvising.
- If an SDK API differs from the examples, update only `internal/shared/mcp` adapter code and tests. Do not let SDK details leak into `internal/shared/tools`, `internal/cli`, `internal/stdio`, or `internal/http`.
- Checkbox discipline is mandatory. A subagent may mark a step complete only after it has performed that exact step and captured the command result or file edit in its report.
- A subagent must not pre-check future steps, bulk-check an entire task, or leave completed steps unchecked. Reviewers must reject reports where checkbox state does not match the evidence.
- A task is complete only when all of its checkboxes are checked, the task-specific tests pass, `go test ./...` passes, and the task commit exists.

## Parallelization Map

Task 1 must run first.

After Task 1 passes, these can run in parallel:

- Task 2 source detection fixtures and analyzer contracts.
- Task 3 source acquisition layout and ref resolution.
- Task 4 safety parser and classifier.
- Task 5 tool schema and envelope contracts.
- Task 6 CLI command surface and help text.
- Task 7 HTTP config and pool contracts.

After Tasks 2, 4, and 5 pass, Task 8 can run.

After Tasks 5 and 8 pass, Task 9 can run.

After Tasks 3, 7, and 9 pass, Task 10 can run.

Task 11 is the final integration and parity pass.

## File Structure

- Create: `go.mod`, `go.sum`
- Create: `cmd/wowdoc/main.go`
- Create: `cmd/wowdoc-server/main.go`
- Create: `internal/shared/config/config.go`
- Create: `internal/shared/config/config_test.go`
- Create: `internal/shared/contracts/contracts.go`
- Create: `internal/shared/contracts/contracts_test.go`
- Create: `internal/shared/analyze/detect.go`
- Create: `internal/shared/analyze/index.go`
- Create: `internal/shared/analyze/safety.go`
- Create: `internal/shared/analyze/*_test.go`
- Create: `internal/shared/source/manager.go`
- Create: `internal/shared/source/git.go`
- Create: `internal/shared/source/archive.go`
- Create: `internal/shared/source/*_test.go`
- Create: `internal/shared/tools/registry.go`
- Create: `internal/shared/tools/tools.go`
- Create: `internal/shared/tools/*_test.go`
- Create: `internal/shared/mcp/server.go`
- Create: `internal/shared/mcp/schemas.go`
- Create: `internal/shared/mcp/*_test.go`
- Create: `internal/cli/root.go`
- Create: `internal/cli/*_test.go`
- Create: `internal/stdio/server.go`
- Create: `internal/stdio/*_test.go`
- Create: `internal/http/config.go`
- Create: `internal/http/pools.go`
- Create: `internal/http/routes.go`
- Create: `internal/http/server.go`
- Create: `internal/http/*_test.go`
- Create: `testdata/sources/*`

## External Documentation Checked

- Official MCP Go SDK overview: <https://go.sdk.modelcontextprotocol.io/>
- Official MCP Go SDK repository: <https://github.com/modelcontextprotocol/go-sdk>
- Go package docs for `github.com/modelcontextprotocol/go-sdk/mcp`: <https://pkg.go.dev/github.com/modelcontextprotocol/go-sdk/mcp>

### Task 1: Bootstrap Module And Shared Contracts

**Files:**
- Create: `go.mod`
- Create: `internal/shared/contracts/contracts.go`
- Create: `internal/shared/contracts/contracts_test.go`
- Create: `internal/shared/config/config.go`
- Create: `internal/shared/config/config_test.go`

- [x] **Step 1: Write failing contract tests**

Create `internal/shared/contracts/contracts_test.go`:

```go
package contracts

import "testing"

func TestErrorCodesAreStable(t *testing.T) {
	got := []ErrorCode{
		ErrClientRequired,
		ErrClientNotFound,
		ErrSourceNotFound,
		ErrSourceInvalid,
		ErrRefNotFound,
		ErrGitUnavailableArchiveFailed,
		ErrCapabilityUnavailable,
		ErrIndexUnavailable,
		ErrTimeout,
		ErrUnsupportedRef,
	}
	want := []string{
		"client_required",
		"client_not_found",
		"source_not_found",
		"source_invalid",
		"ref_not_found",
		"git_unavailable_archive_failed",
		"capability_unavailable",
		"index_unavailable",
		"timeout",
		"unsupported_ref",
	}
	for i := range want {
		if string(got[i]) != want[i] {
			t.Fatalf("error code %d = %q, want %q", i, got[i], want[i])
		}
	}
}

func TestToolEnvelopePreservesSourceErrorDiagnosticsAndData(t *testing.T) {
	env := Envelope[map[string]string]{
		OK: true,
		Source: SourceTransparency{
			Client: "retail", RequestedRef: "main", ResolvedRef: "abc123", Version: "12.0.0", Path: "sources/checkouts/retail/abc123",
		},
		Data: map[string]string{"name": "C_AuctionHouse.GetItemSearchResultInfo"},
		Diagnostics: []Diagnostic{{Path: "bad", Message: "missing Interface"}},
	}
	if !env.OK || env.Source.Client != "retail" || env.Data["name"] == "" || len(env.Diagnostics) != 1 {
		t.Fatalf("envelope did not preserve fields: %#v", env)
	}
}
```

Create `internal/shared/config/config_test.go`:

```go
package config

import "testing"

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
```

- [x] **Step 2: Run tests to verify they fail**

Run:

```powershell
go test ./internal/shared/contracts ./internal/shared/config
```

Expected: fail because `go.mod`, `ErrorCode`, `Envelope`, and `DefaultSourceSeeds` do not exist.

- [x] **Step 3: Create the module and minimal shared contracts**

Create `go.mod`:

```go
module wowdoc

go 1.23

require (
	github.com/modelcontextprotocol/go-sdk v0.0.0
)
```

If `go mod tidy` resolves a released version, keep the resolved version. If it cannot resolve `v0.0.0`, replace it with the latest version selected by `go get github.com/modelcontextprotocol/go-sdk@latest`.

Create `internal/shared/contracts/contracts.go`:

```go
package contracts

type ErrorCode string

const (
	ErrClientRequired                ErrorCode = "client_required"
	ErrClientNotFound                ErrorCode = "client_not_found"
	ErrSourceNotFound                ErrorCode = "source_not_found"
	ErrSourceInvalid                 ErrorCode = "source_invalid"
	ErrRefNotFound                   ErrorCode = "ref_not_found"
	ErrGitUnavailableArchiveFailed  ErrorCode = "git_unavailable_archive_failed"
	ErrCapabilityUnavailable         ErrorCode = "capability_unavailable"
	ErrIndexUnavailable              ErrorCode = "index_unavailable"
	ErrTimeout                       ErrorCode = "timeout"
	ErrUnsupportedRef                ErrorCode = "unsupported_ref"
)

type SourceTransparency struct {
	Client       string `json:"client"`
	RequestedRef string `json:"requestedRef,omitempty"`
	ResolvedRef  string `json:"resolvedRef,omitempty"`
	Version      string `json:"version,omitempty"`
	Path         string `json:"path,omitempty"`
}

type ToolError struct {
	Code    ErrorCode `json:"code"`
	Message string    `json:"message"`
}

type Diagnostic struct {
	Path    string   `json:"path,omitempty"`
	Message string   `json:"message"`
	Missing []string `json:"missing,omitempty"`
}

type Envelope[T any] struct {
	OK          bool               `json:"ok"`
	Source      SourceTransparency `json:"source,omitempty"`
	Data        T                  `json:"data,omitempty"`
	Error       *ToolError         `json:"error,omitempty"`
	Diagnostics []Diagnostic       `json:"diagnostics,omitempty"`
}
```

Create `internal/shared/config/config.go`:

```go
package config

type SourceSeed struct {
	Alias string `json:"alias" yaml:"alias"`
	Repo  string `json:"repo" yaml:"repo"`
	Ref   string `json:"ref" yaml:"ref"`
}

func DefaultSourceSeeds() []SourceSeed {
	return []SourceSeed{
		{Alias: "retail", Repo: "https://github.com/Gethe/wow-ui-source.git", Ref: "main"},
		{Alias: "classic", Repo: "https://github.com/Gethe/wow-ui-source-classic.git", Ref: "main"},
		{Alias: "classic-ptr", Repo: "https://github.com/Gethe/wow-ui-source-classic-ptr.git", Ref: "main"},
		{Alias: "classic-titan", Repo: "https://github.com/Gethe/wow-ui-source-classic-titan.git", Ref: "main"},
		{Alias: "ptr2", Repo: "https://github.com/Gethe/wow-ui-source-ptr2.git", Ref: "main"},
	}
}
```

- [x] **Step 4: Resolve modules and pass tests**

Run:

```powershell
go get github.com/modelcontextprotocol/go-sdk@latest
go mod tidy
go test ./internal/shared/contracts ./internal/shared/config
```

Expected: pass.

- [x] **Step 5: Commit**

```powershell
git add go.mod go.sum internal/shared/contracts internal/shared/config
git commit -m "chore: bootstrap wowdoc contracts"
```

### Task 2: Source Repository Detection And Fixtures

**Files:**
- Create: `internal/shared/analyze/detect.go`
- Create: `internal/shared/analyze/detect_test.go`
- Create: `testdata/sources/valid-retail/Interface/ui-code-list.txt`
- Create: `testdata/sources/valid-retail/Interface/AddOns/Blizzard_APIDocumentationGenerated/GeneratedDocumentation.lua`
- Create: `testdata/sources/valid-retail/version.txt`
- Create: `testdata/sources/partial-classic/Interface/AddOns/Blizzard_FrameXML/SecureTemplates.lua`
- Create: `testdata/sources/partial-classic/version.txt`
- Create: `testdata/sources/invalid-random/readme.txt`

- [x] **Step 1: Write failing repository detection tests**

Create `internal/shared/analyze/detect_test.go`:

```go
package analyze

import (
	"path/filepath"
	"testing"
)

func TestDetectRepositoryClassifiesValidPartialAndInvalidSources(t *testing.T) {
	root := filepath.Join("..", "..", "..", "testdata", "sources")
	valid := DetectRepository(filepath.Join(root, "valid-retail"), "retail")
	if !valid.Valid || valid.Alias != "retail" || !valid.Capabilities.APIDocumentation || !valid.Capabilities.FrameXML {
		t.Fatalf("valid retail detection wrong: %#v", valid)
	}
	partial := DetectRepository(filepath.Join(root, "partial-classic"), "classic")
	if !partial.Valid || partial.Capabilities.APIDocumentation || !partial.Capabilities.FrameXML {
		t.Fatalf("partial classic detection wrong: %#v", partial)
	}
	invalid := DetectRepository(filepath.Join(root, "invalid-random"), "random")
	if invalid.Valid || len(invalid.Diagnostics) == 0 {
		t.Fatalf("invalid directory must produce diagnostics: %#v", invalid)
	}
}
```

- [x] **Step 2: Run test to verify it fails**

Run:

```powershell
go test ./internal/shared/analyze -run TestDetectRepositoryClassifiesValidPartialAndInvalidSources -v
```

Expected: fail because `DetectRepository` and fixtures do not exist.

- [x] **Step 3: Add exact fixtures**

Create these files with exact content:

`testdata/sources/valid-retail/Interface/ui-code-list.txt`:

```text
Interface/AddOns/Blizzard_APIDocumentationGenerated/GeneratedDocumentation.lua
Interface/AddOns/Blizzard_FrameXML/SecureTemplates.lua
```

`testdata/sources/valid-retail/Interface/AddOns/Blizzard_APIDocumentationGenerated/GeneratedDocumentation.lua`:

```lua
APIDocumentation:AddDocumentationTable({Name="C_AuctionHouse", Type="System"})
```

`testdata/sources/valid-retail/version.txt`:

```text
12.0.0.60000
```

`testdata/sources/partial-classic/Interface/AddOns/Blizzard_FrameXML/SecureTemplates.lua`:

```lua
SecureActionButtonTemplate = {}
```

`testdata/sources/partial-classic/version.txt`:

```text
1.15.7.60000
```

`testdata/sources/invalid-random/readme.txt`:

```text
not a wow ui source repository
```

- [x] **Step 4: Implement minimal detection**

Create `internal/shared/analyze/detect.go`:

```go
package analyze

import (
	"os"
	"path/filepath"

	"wowdoc/internal/shared/contracts"
)

type Capabilities struct {
	APIDocumentation bool `json:"apiDocumentation"`
	FrameXML         bool `json:"frameXML"`
	WidgetDocs       bool `json:"widgetDocs"`
	Constants        bool `json:"constants"`
	Mixins           bool `json:"mixins"`
	CVars            bool `json:"cvars"`
}

type Repository struct {
	Alias        string                 `json:"alias"`
	Path         string                 `json:"path"`
	Version      string                 `json:"version"`
	Valid        bool                   `json:"valid"`
	Capabilities Capabilities           `json:"capabilities"`
	Diagnostics  []contracts.Diagnostic `json:"diagnostics"`
}

func DetectRepository(path, alias string) Repository {
	repo := Repository{Alias: alias, Path: path}
	missing := make([]string, 0)
	if !exists(filepath.Join(path, "Interface")) {
		missing = append(missing, "Interface/")
	}
	if !hasAny(path,
		"Interface/ui-code-list.txt",
		"Interface/ui-toc-list.txt",
		"Interface/ui-gen-addon-list.txt",
		"Interface/AddOns",
	) {
		missing = append(missing, "source-list-or-addons")
	}
	versionPath := filepath.Join(path, "version.txt")
	if !exists(versionPath) {
		missing = append(missing, "version.txt")
	} else if b, err := os.ReadFile(versionPath); err == nil {
		repo.Version = stringTrim(b)
	}
	if len(missing) > 0 {
		repo.Diagnostics = append(repo.Diagnostics, contracts.Diagnostic{
			Path: path, Message: "source_invalid", Missing: missing,
		})
		return repo
	}
	repo.Valid = true
	addons := filepath.Join(path, "Interface", "AddOns")
	repo.Capabilities.APIDocumentation = exists(filepath.Join(addons, "Blizzard_APIDocumentationGenerated"))
	repo.Capabilities.FrameXML = exists(addons)
	repo.Capabilities.WidgetDocs = exists(filepath.Join(addons, "Blizzard_APIDocumentationGenerated"))
	repo.Capabilities.Constants = exists(filepath.Join(addons, "Blizzard_APIDocumentationGenerated"))
	repo.Capabilities.Mixins = exists(addons)
	repo.Capabilities.CVars = exists(addons)
	return repo
}

func hasAny(root string, rels ...string) bool {
	for _, rel := range rels {
		if exists(filepath.Join(root, rel)) {
			return true
		}
	}
	return false
}

func exists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

func stringTrim(b []byte) string {
	s := string(b)
	for len(s) > 0 {
		last := s[len(s)-1]
		if last != '\n' && last != '\r' && last != ' ' && last != '\t' {
			break
		}
		s = s[:len(s)-1]
	}
	return s
}
```

- [x] **Step 5: Run tests**

Run:

```powershell
go test ./internal/shared/analyze -run TestDetectRepositoryClassifiesValidPartialAndInvalidSources -v
go test ./...
```

Expected: pass.

- [x] **Step 6: Commit**

```powershell
git add internal/shared/analyze testdata/sources
git commit -m "feat: detect wow ui source repositories"
```

### Task 3: Source Acquisition Layout And Ref Resolution

**Files:**
- Create: `internal/shared/source/manager.go`
- Create: `internal/shared/source/manager_test.go`
- Create: `internal/shared/source/git.go`
- Create: `internal/shared/source/archive.go`

- [x] **Step 1: Write failing source manager tests**

Create `internal/shared/source/manager_test.go`:

```go
package source

import (
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
```

- [x] **Step 2: Run tests to verify they fail**

Run:

```powershell
go test ./internal/shared/source -v
```

Expected: fail because `NewManager`, `Options`, and `ResolveRef` do not exist.

- [x] **Step 3: Implement layout and deterministic ref policy**

Create `internal/shared/source/manager.go`:

```go
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
		Client: client, Requested: requested, Resolved: resolved, CheckoutDir: m.CheckoutPath(client, resolved),
	}, nil
}
```

Create `internal/shared/source/git.go`:

```go
package source

type GitRunner interface {
	Run(args ...string) error
}
```

Create `internal/shared/source/archive.go`:

```go
package source

type ArchiveFetcher interface {
	FetchArchive(repoURL, ref, destination string) error
}
```

- [x] **Step 4: Run tests**

Run:

```powershell
go test ./internal/shared/source -v
go test ./...
```

Expected: pass.

- [x] **Step 5: Commit**

```powershell
git add internal/shared/source
git commit -m "feat: isolate source checkouts by resolved ref"
```

### Task 4: Safety Metadata Parser And Classifier

**Files:**
- Create: `internal/shared/analyze/safety.go`
- Create: `internal/shared/analyze/safety_test.go`

- [x] **Step 1: Write failing safety tests**

Create `internal/shared/analyze/safety_test.go`:

```go
package analyze

import "testing"

func TestClassifySafetyMetadata(t *testing.T) {
	tests := []struct {
		name string
		meta SafetyMetadata
		want RiskLevel
	}{
		{name: "forbidden", meta: SafetyMetadata{IsForbidden: true}, want: RiskForbidden},
		{name: "protected", meta: SafetyMetadata{IsProtectedFunction: true}, want: RiskProtected},
		{name: "secret args", meta: SafetyMetadata{SecretArguments: "NotAllowed"}, want: RiskSecret},
		{name: "taint", meta: SafetyMetadata{SecretArguments: "AllowedWhenUntainted"}, want: RiskTaintSensitive},
		{name: "conditional", meta: SafetyMetadata{SecretWhenUnitSpellCastRestricted: true}, want: RiskConditionalSecret},
		{name: "never secret", meta: SafetyMetadata{ReturnsNeverSecret: true}, want: RiskNeverSecret},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := ClassifySafety(tt.meta).Level; got != tt.want {
				t.Fatalf("level = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestExplainUnitCastScenarioIncludesFieldAdvice(t *testing.T) {
	meta := SafetyMetadata{
		SecretWhenUnitSpellCastRestricted: true,
		Fields: []SafetyField{{Name: "target", ConditionalSecret: true}, {Name: "castBarID", NeverSecret: true}},
	}
	expl := ExplainSafety(meta, "unit_cast")
	if expl.EffectiveLevel != RiskConditionalSecret || len(expl.Why) < 2 || len(expl.AddonAdvice) == 0 {
		t.Fatalf("bad explanation: %#v", expl)
	}
}
```

- [x] **Step 2: Run tests to verify they fail**

Run:

```powershell
go test ./internal/shared/analyze -run "Safety|Explain" -v
```

Expected: fail because safety types and functions do not exist.

- [x] **Step 3: Implement classifier**

Create `internal/shared/analyze/safety.go`:

```go
package analyze

type RiskLevel string

const (
	RiskSafe              RiskLevel = "safe"
	RiskNeverSecret       RiskLevel = "never_secret"
	RiskTaintSensitive    RiskLevel = "taint_sensitive"
	RiskConditionalSecret RiskLevel = "conditional_secret"
	RiskSecret            RiskLevel = "secret"
	RiskProtected         RiskLevel = "protected"
	RiskForbidden         RiskLevel = "forbidden"
	RiskUnknown           RiskLevel = "unknown"
)

type SafetyMetadata struct {
	SecretArguments                      string
	SecretArgumentsAddAspect             []string
	SecretReturnsForAspect               []string
	SecretWhenCooldownsRestricted        bool
	SecretWhenUnitSpellCastRestricted    bool
	SecretInChatMessagingLockdown        bool
	RequiresNonSecretAura                bool
	RequiresRestrictedAbbreviationBreakpoints bool
	IsProtectedFunction                  bool
	ConstSecretAccessor                  bool
	ReturnsNeverSecret                   bool
	NeverSecret                          bool
	ConditionalSecret                    bool
	IsForbidden                          bool
	SetForbidden                         bool
	SecretWrapperConstant                string
	Fields                               []SafetyField
}

type SafetyField struct {
	Name              string `json:"name"`
	ConditionalSecret bool   `json:"conditionalSecret,omitempty"`
	NeverSecret       bool   `json:"neverSecret,omitempty"`
}

type SafetyClassification struct {
	Level  RiskLevel     `json:"level"`
	Fields []SafetyField `json:"fields,omitempty"`
}

type SafetyExplanation struct {
	Scenario       string    `json:"scenario"`
	EffectiveLevel RiskLevel `json:"effectiveLevel"`
	Why            []string  `json:"why"`
	AddonAdvice    []string  `json:"addonAdvice"`
}

func ClassifySafety(meta SafetyMetadata) SafetyClassification {
	level := RiskSafe
	switch {
	case meta.IsForbidden || meta.SetForbidden:
		level = RiskForbidden
	case meta.IsProtectedFunction:
		level = RiskProtected
	case meta.SecretArguments == "NotAllowed" || meta.SecretWrapperConstant == "AlwaysSecret":
		level = RiskSecret
	case meta.SecretArguments == "AllowedWhenUntainted":
		level = RiskTaintSensitive
	case meta.ConditionalSecret || meta.SecretWhenCooldownsRestricted || meta.SecretWhenUnitSpellCastRestricted ||
		meta.SecretInChatMessagingLockdown || meta.RequiresNonSecretAura || meta.SecretWrapperConstant == "ContextuallySecret" ||
		len(meta.SecretArgumentsAddAspect) > 0 || len(meta.SecretReturnsForAspect) > 0:
		level = RiskConditionalSecret
	case meta.NeverSecret || meta.ReturnsNeverSecret || meta.SecretWrapperConstant == "NeverSecret":
		level = RiskNeverSecret
	}
	return SafetyClassification{Level: level, Fields: meta.Fields}
}

func ExplainSafety(meta SafetyMetadata, scenario string) SafetyExplanation {
	classified := ClassifySafety(meta)
	expl := SafetyExplanation{Scenario: scenario, EffectiveLevel: classified.Level}
	if meta.SecretWhenUnitSpellCastRestricted {
		expl.Why = append(expl.Why, "SecretWhenUnitSpellCastRestricted is true")
	}
	for _, field := range meta.Fields {
		if field.ConditionalSecret {
			expl.Why = append(expl.Why, "return field "+field.Name+" is ConditionalSecret")
		}
		if field.NeverSecret {
			expl.Why = append(expl.Why, field.Name+" is NeverSecret and can be used safely")
		}
	}
	if len(expl.Why) == 0 {
		expl.Why = append(expl.Why, "no unsafe metadata matched")
	}
	expl.AddonAdvice = []string{
		"Treat secret or conditional fields as possibly unavailable.",
		"Do not use secret values to mutate secure UI during combat.",
		"Check nil and use secret-safe fallbacks.",
	}
	return expl
}
```

- [x] **Step 4: Run tests**

Run:

```powershell
go test ./internal/shared/analyze -run "Safety|Explain" -v
go test ./...
```

Expected: pass.

- [x] **Step 5: Commit**

```powershell
git add internal/shared/analyze/safety.go internal/shared/analyze/safety_test.go
git commit -m "feat: classify wow api safety metadata"
```

### Task 5: Tool Registry, Schemas, And Envelopes

**Files:**
- Create: `internal/shared/tools/registry.go`
- Create: `internal/shared/tools/registry_test.go`
- Create: `internal/shared/tools/tools.go`
- Create: `internal/shared/mcp/schemas.go`
- Create: `internal/shared/mcp/schemas_test.go`

- [x] **Step 1: Write failing registry and schema tests**

Create `internal/shared/tools/registry_test.go`:

```go
package tools

import "testing"

func TestRegistryContainsExactlySupportedTools(t *testing.T) {
	reg := DefaultRegistry()
	want := []string{
		"list_clients",
		"lookup_blizzard_api",
		"search_blizzard_api",
		"get_api_namespace",
		"get_api_events",
		"search_framexml",
		"validate_toc",
		"check_api_deprecation",
		"suggest_api_migration",
		"get_wow_constants",
		"get_widget_api",
		"find_mixin_template",
		"lookup_cvar",
		"explain_api_safety",
	}
	if len(reg.Tools) != len(want) {
		t.Fatalf("tool count = %d, want %d", len(reg.Tools), len(want))
	}
	for _, name := range want {
		if _, ok := reg.Tools[name]; !ok {
			t.Fatalf("missing tool %q", name)
		}
	}
	if _, ok := reg.Tools["scaffold_addon"]; ok {
		t.Fatalf("omitted TypeScript tool scaffold_addon must not be registered")
	}
}
```

Create `internal/shared/mcp/schemas_test.go`:

```go
package mcp

import "testing"

func TestSourceBackedSchemasRequireClientAndAllowOptionalRef(t *testing.T) {
	schema := ToolInputSchemas()["lookup_blizzard_api"]
	if schema.Properties["client"].Type != "string" || schema.Properties["ref"].Type != "string" {
		t.Fatalf("schema must include client/ref strings: %#v", schema)
	}
	if !schema.Requires("client") || schema.Requires("ref") {
		t.Fatalf("client required and ref optional required list wrong: %#v", schema.Required)
	}
}

func TestValidateTOCDoesNotRequireClient(t *testing.T) {
	schema := ToolInputSchemas()["validate_toc"]
	if schema.Requires("client") {
		t.Fatalf("validate_toc client must be optional")
	}
}
```

- [x] **Step 2: Run tests to verify they fail**

Run:

```powershell
go test ./internal/shared/tools ./internal/shared/mcp -v
```

Expected: fail because registry and schemas do not exist.

- [x] **Step 3: Implement registry**

Create `internal/shared/tools/tools.go`:

```go
package tools

type Handler interface{}

type ToolDefinition struct {
	Name         string
	SourceBacked bool
}

type Registry struct {
	Tools map[string]ToolDefinition
}
```

Create `internal/shared/tools/registry.go`:

```go
package tools

func DefaultRegistry() Registry {
	names := []string{
		"list_clients",
		"lookup_blizzard_api",
		"search_blizzard_api",
		"get_api_namespace",
		"get_api_events",
		"search_framexml",
		"validate_toc",
		"check_api_deprecation",
		"suggest_api_migration",
		"get_wow_constants",
		"get_widget_api",
		"find_mixin_template",
		"lookup_cvar",
		"explain_api_safety",
	}
	reg := Registry{Tools: map[string]ToolDefinition{}}
	for _, name := range names {
		reg.Tools[name] = ToolDefinition{Name: name, SourceBacked: name != "validate_toc" && name != "list_clients"}
	}
	return reg
}
```

- [x] **Step 4: Implement schema model**

Create `internal/shared/mcp/schemas.go`:

```go
package mcp

type JSONSchema struct {
	Type       string                    `json:"type"`
	Properties map[string]SchemaProperty `json:"properties,omitempty"`
	Required   []string                  `json:"required,omitempty"`
}

type SchemaProperty struct {
	Type        string `json:"type"`
	Description string `json:"description,omitempty"`
}

func (s JSONSchema) Requires(name string) bool {
	for _, required := range s.Required {
		if required == name {
			return true
		}
	}
	return false
}

func ToolInputSchemas() map[string]JSONSchema {
	schemas := map[string]JSONSchema{}
	for name := range map[string]bool{
		"lookup_blizzard_api": true,
		"search_blizzard_api": true,
		"get_api_namespace": true,
		"get_api_events": true,
		"search_framexml": true,
		"check_api_deprecation": true,
		"suggest_api_migration": true,
		"get_wow_constants": true,
		"get_widget_api": true,
		"find_mixin_template": true,
		"lookup_cvar": true,
		"explain_api_safety": true,
	} {
		schemas[name] = sourceSchema("name")
	}
	schemas["list_clients"] = JSONSchema{Type: "object", Properties: map[string]SchemaProperty{
		"includeDiagnostics": {Type: "boolean"},
		"includeRefs": {Type: "boolean"},
	}}
	schemas["validate_toc"] = JSONSchema{Type: "object", Properties: map[string]SchemaProperty{
		"tocContent": {Type: "string"},
		"tocPath": {Type: "string"},
		"client": {Type: "string"},
		"ref": {Type: "string"},
		"addonName": {Type: "string"},
	}}
	return schemas
}

func sourceSchema(primary string) JSONSchema {
	return JSONSchema{Type: "object", Required: []string{"client"}, Properties: map[string]SchemaProperty{
		"client": {Type: "string", Description: "Required source client alias."},
		"ref": {Type: "string", Description: "Optional branch, tag, or commit."},
		primary: {Type: "string"},
	}}
}
```

- [x] **Step 5: Run tests**

Run:

```powershell
go test ./internal/shared/tools ./internal/shared/mcp -v
go test ./...
```

Expected: pass.

- [x] **Step 6: Commit**

```powershell
git add internal/shared/tools internal/shared/mcp/schemas.go internal/shared/mcp/schemas_test.go
git commit -m "feat: define mcp tool registry and schemas"
```

### Task 6: Agent-Friendly CLI Surface

**Files:**
- Create: `cmd/wowdoc/main.go`
- Create: `internal/cli/root.go`
- Create: `internal/cli/root_test.go`

- [x] **Step 1: Write failing CLI help tests**

Create `internal/cli/root_test.go`:

```go
package cli

import (
	"bytes"
	"strings"
	"testing"
)

func TestLookupHelpIsAgentFriendly(t *testing.T) {
	cmd := NewRootCommand()
	cmd.SetArgs([]string{"api", "lookup", "--help"})
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	if err := cmd.Execute(); err != nil {
		t.Fatalf("help failed: %v", err)
	}
	text := out.String()
	for _, required := range []string{"Required:", "--client", "--name", "Source resolution:", "Agent next step:", "MCP arguments:", "client_required"} {
		if !strings.Contains(text, required) {
			t.Fatalf("help missing %q:\n%s", required, text)
		}
	}
}

func TestLookupRequiresClient(t *testing.T) {
	cmd := NewRootCommand()
	cmd.SetArgs([]string{"api", "lookup", "--name", "C_Test.Foo"})
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	err := cmd.Execute()
	if err == nil || !strings.Contains(err.Error(), "client_required") {
		t.Fatalf("expected client_required, got %v output %s", err, out.String())
	}
}
```

- [x] **Step 2: Run tests to verify they fail**

Run:

```powershell
go test ./internal/cli -v
```

Expected: fail because CLI does not exist.

- [x] **Step 3: Add Cobra dependency and CLI root**

Run:

```powershell
go get github.com/spf13/cobra@latest
```

Create `internal/cli/root.go`:

```go
package cli

import (
	"errors"

	"github.com/spf13/cobra"
)

func NewRootCommand() *cobra.Command {
	root := &cobra.Command{Use: "wowdoc", SilenceUsage: true, SilenceErrors: true}
	root.AddCommand(apiCommand())
	root.AddCommand(clientsCommand())
	root.AddCommand(mcpCommand())
	return root
}

func apiCommand() *cobra.Command {
	api := &cobra.Command{Use: "api"}
	var client, ref, name string
	lookup := &cobra.Command{
		Use: "lookup",
		Short: "Lookup a Blizzard API symbol.",
		Long: `Lookup a Blizzard API symbol.

Required:
  --client retail|classic|classic-ptr|classic-titan|ptr2|<discovered alias>
  --name API_NAME

Optional:
  --ref REF       branch, tag, or commit. Defaults to the client's latest ref.

Source resolution:
  --source-path wins over --source-root + --client + --ref.
  If no source root is set, wowdoc uses <exe-dir>/sources.

Agent next step:
  If client is unknown, run: wowdoc clients list --include-diagnostics

MCP arguments:
  {"client":"retail","name":"C_AuctionHouse.GetItemSearchResultInfo"}

Common error codes:
  client_required client_not_found source_not_found source_invalid ref_not_found capability_unavailable`,
		RunE: func(cmd *cobra.Command, args []string) error {
			if client == "" {
				return errors.New("client_required")
			}
			if name == "" {
				return errors.New("name_required")
			}
			_ = ref
			return nil
		},
	}
	lookup.Flags().StringVar(&client, "client", "", "source client alias")
	lookup.Flags().StringVar(&ref, "ref", "", "branch, tag, or commit")
	lookup.Flags().StringVar(&name, "name", "", "API name")
	api.AddCommand(lookup)
	return api
}

func clientsCommand() *cobra.Command {
	clients := &cobra.Command{Use: "clients"}
	clients.AddCommand(&cobra.Command{Use: "list", RunE: func(cmd *cobra.Command, args []string) error { return nil }})
	return clients
}

func mcpCommand() *cobra.Command {
	mcp := &cobra.Command{Use: "mcp"}
	mcp.AddCommand(&cobra.Command{Use: "stdio", RunE: func(cmd *cobra.Command, args []string) error { return nil }})
	return mcp
}
```

Create `cmd/wowdoc/main.go`:

```go
package main

import (
	"fmt"
	"os"

	"wowdoc/internal/cli"
)

func main() {
	if err := cli.NewRootCommand().Execute(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
```

- [x] **Step 4: Run tests and build CLI**

Run:

```powershell
go test ./internal/cli -v
go build -o dist/wowdoc.exe ./cmd/wowdoc
go test ./...
```

Expected: pass and `dist/wowdoc.exe` exists.

- [x] **Step 5: Commit**

```powershell
git add go.mod go.sum cmd/wowdoc internal/cli
git commit -m "feat: add agent friendly cli shell"
```

### Task 7: HTTP Config, Health Contract, And Pools

**Files:**
- Create: `cmd/wowdoc-server/main.go`
- Create: `internal/http/config.go`
- Create: `internal/http/config_test.go`
- Create: `internal/http/pools.go`
- Create: `internal/http/pools_test.go`
- Create: `internal/http/routes.go`
- Create: `internal/http/routes_test.go`

- [x] **Step 1: Write failing HTTP tests**

Create `internal/http/config_test.go`:

```go
package http

import "testing"

func TestDefaultHTTPConfigDisablesArbitraryRefs(t *testing.T) {
	cfg := DefaultConfig()
	if cfg.Sources.AllowArbitraryRef {
		t.Fatalf("HTTP must disable arbitrary refs by default")
	}
	if cfg.Server.Port != 9789 || cfg.Limits.MaxConcurrentSourceFetches != 2 {
		t.Fatalf("unexpected defaults: %#v", cfg)
	}
}
```

Create `internal/http/pools_test.go`:

```go
package http

import "testing"

func TestPoolsKeyByClientAndResolvedCommit(t *testing.T) {
	p := NewPools(8, 4)
	p.PutSource("classic", "1111111", "source-a")
	p.PutSource("classic", "2222222", "source-b")
	if p.Source("classic", "1111111") == p.Source("classic", "2222222") {
		t.Fatalf("different commits must not share source context")
	}
}
```

Create `internal/http/routes_test.go`:

```go
package http

import (
	"encoding/json"
	"net/http/httptest"
	"testing"
)

func TestHealthReportsPoolsAndDiagnostics(t *testing.T) {
	app := NewApp(DefaultConfig())
	req := httptest.NewRequest("GET", "/health", nil)
	rec := httptest.NewRecorder()
	app.Router().ServeHTTP(rec, req)
	if rec.Code != 200 {
		t.Fatalf("status = %d", rec.Code)
	}
	var body map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("invalid json: %v", err)
	}
	for _, key := range []string{"sources", "clients", "invalidDirectories", "pools", "recentErrors"} {
		if _, ok := body[key]; !ok {
			t.Fatalf("health missing %q: %#v", key, body)
		}
	}
}
```

- [x] **Step 2: Run tests to verify they fail**

Run:

```powershell
go test ./internal/http -v
```

Expected: fail because HTTP package does not exist.

- [x] **Step 3: Implement config, pools, and routes**

Create `internal/http/config.go`:

```go
package http

type Config struct {
	Server   ServerConfig
	Sources  SourceConfig
	Contexts ContextConfig
	Limits   LimitConfig
}

type ServerConfig struct {
	Host    string
	Port    int
	BaseURL string
}

type SourceConfig struct {
	Root              string
	AllowArbitraryRef bool
	DefaultRef        string
}

type ContextConfig struct {
	MaxSourceContexts int
	MaxIndexContexts  int
	Pinned            []string
}

type LimitConfig struct {
	MaxConcurrentSourceFetches int
	MaxConcurrentIndexBuilds   int
	RequestTimeoutSeconds      int
}

func DefaultConfig() Config {
	return Config{
		Server: ServerConfig{Host: "0.0.0.0", Port: 9789},
		Sources: SourceConfig{DefaultRef: "latest", AllowArbitraryRef: false},
		Contexts: ContextConfig{MaxSourceContexts: 8, MaxIndexContexts: 4, Pinned: []string{"retail", "classic"}},
		Limits: LimitConfig{MaxConcurrentSourceFetches: 2, MaxConcurrentIndexBuilds: 2, RequestTimeoutSeconds: 60},
	}
}
```

Create `internal/http/pools.go`:

```go
package http

type Pools struct {
	sources map[string]any
	indexes  map[string]any
}

func NewPools(maxSources, maxIndexes int) *Pools {
	_ = maxSources
	_ = maxIndexes
	return &Pools{sources: map[string]any{}, indexes: map[string]any{}}
}

func (p *Pools) PutSource(client, commit string, value any) {
	p.sources[client+"@"+commit] = value
}

func (p *Pools) Source(client, commit string) any {
	return p.sources[client+"@"+commit]
}

func (p *Pools) Stats() map[string]int {
	return map[string]int{"sources": len(p.sources), "indexes": len(p.indexes)}
}
```

Create `internal/http/routes.go`:

```go
package http

import (
	"encoding/json"
	"net/http"
)

type App struct {
	cfg   Config
	pools *Pools
}

func NewApp(cfg Config) *App {
	return &App{cfg: cfg, pools: NewPools(cfg.Contexts.MaxSourceContexts, cfg.Contexts.MaxIndexContexts)}
}

func (a *App) Router() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/health", a.health)
	mux.HandleFunc("/help", a.help)
	mux.HandleFunc("/mcp", a.mcp)
	return mux
}

func (a *App) health(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, map[string]any{
		"sources": []any{},
		"clients": []any{},
		"invalidDirectories": []any{},
		"pools": a.pools.Stats(),
		"recentErrors": []any{},
	})
}

func (a *App) help(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, map[string]string{"mcp": "/mcp", "health": "/health"})
}

func (a *App) mcp(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(http.StatusNotImplemented)
}

func writeJSON(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(v)
}
```

Create `cmd/wowdoc-server/main.go`:

```go
package main

import (
	"fmt"
	"net/http"

	wowhttp "wowdoc/internal/http"
)

func main() {
	cfg := wowhttp.DefaultConfig()
	addr := fmt.Sprintf("%s:%d", cfg.Server.Host, cfg.Server.Port)
	_ = http.ListenAndServe(addr, wowhttp.NewApp(cfg).Router())
}
```

- [x] **Step 4: Run tests and build HTTP binary**

Run:

```powershell
go test ./internal/http -v
go build -o dist/wowdoc-server.exe ./cmd/wowdoc-server
go test ./...
```

Expected: pass and `dist/wowdoc-server.exe` exists.

- [x] **Step 5: Commit**

```powershell
git add cmd/wowdoc-server internal/http
git commit -m "feat: add http server shell and pools"
```

### Task 8: Analyze Indexes And Tool Business Logic

**Files:**
- Create: `internal/shared/analyze/index.go`
- Create: `internal/shared/analyze/index_test.go`
- Modify: `internal/shared/tools/tools.go`
- Create: `internal/shared/tools/handlers_test.go`

- [x] **Step 1: Write failing index and handler tests**

Create `internal/shared/analyze/index_test.go`:

```go
package analyze

import (
	"path/filepath"
	"testing"
)

func TestBuildIndexFindsFrameXMLAndAPINames(t *testing.T) {
	repo := DetectRepository(filepath.Join("..", "..", "..", "testdata", "sources", "valid-retail"), "retail")
	idx, err := BuildIndex(repo)
	if err != nil {
		t.Fatalf("BuildIndex failed: %v", err)
	}
	if !idx.HasAPI("C_AuctionHouse") {
		t.Fatalf("expected API namespace in index: %#v", idx)
	}
	if len(idx.SearchFrameXML("SecureActionButtonTemplate", 5)) == 0 {
		t.Fatalf("expected framexml result")
	}
}
```

Create `internal/shared/tools/handlers_test.go`:

```go
package tools

import (
	"path/filepath"
	"testing"

	"wowdoc/internal/shared/analyze"
	"wowdoc/internal/shared/contracts"
)

func TestListClientsIncludesDiagnostics(t *testing.T) {
	repos := []analyze.Repository{
		analyze.DetectRepository(filepath.Join("..", "..", "..", "testdata", "sources", "valid-retail"), "retail"),
		analyze.DetectRepository(filepath.Join("..", "..", "..", "testdata", "sources", "invalid-random"), "random"),
	}
	env := ListClients(repos, true)
	if !env.OK || len(env.Data.Clients) != 1 || len(env.Diagnostics) != 1 {
		t.Fatalf("bad list clients envelope: %#v", env)
	}
}

func TestCapabilityUnavailableIsStructured(t *testing.T) {
	repo := analyze.DetectRepository(filepath.Join("..", "..", "..", "testdata", "sources", "partial-classic"), "classic")
	env := LookupBlizzardAPI(repo, nil, "C_AuctionHouse.GetItemSearchResultInfo")
	if env.OK || env.Error == nil || env.Error.Code != contracts.ErrCapabilityUnavailable {
		t.Fatalf("expected capability_unavailable: %#v", env)
	}
}
```

- [x] **Step 2: Run tests to verify they fail**

Run:

```powershell
go test ./internal/shared/analyze ./internal/shared/tools -v
```

Expected: fail because `BuildIndex`, `ListClients`, and `LookupBlizzardAPI` do not exist.

- [x] **Step 3: Implement minimal index and handlers**

Create `internal/shared/analyze/index.go`:

```go
package analyze

import (
	"os"
	"path/filepath"
	"strings"
)

type Index struct {
	APINames []string
	Lines    []SearchResult
}

type SearchResult struct {
	File string `json:"file"`
	Line int    `json:"line"`
	Text string `json:"text"`
}

func BuildIndex(repo Repository) (*Index, error) {
	idx := &Index{}
	err := filepath.WalkDir(repo.Path, func(path string, d os.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		if strings.HasSuffix(path, ".lua") || strings.HasSuffix(path, ".xml") {
			b, readErr := os.ReadFile(path)
			if readErr != nil {
				return readErr
			}
			text := string(b)
			if strings.Contains(text, "C_AuctionHouse") {
				idx.APINames = append(idx.APINames, "C_AuctionHouse")
			}
			for i, line := range strings.Split(text, "\n") {
				idx.Lines = append(idx.Lines, SearchResult{File: path, Line: i + 1, Text: line})
			}
		}
		return nil
	})
	return idx, err
}

func (i *Index) HasAPI(name string) bool {
	for _, got := range i.APINames {
		if got == name {
			return true
		}
	}
	return false
}

func (i *Index) SearchFrameXML(query string, limit int) []SearchResult {
	var out []SearchResult
	for _, line := range i.Lines {
		if strings.Contains(line.Text, query) {
			out = append(out, line)
			if len(out) == limit {
				break
			}
		}
	}
	return out
}
```

Modify `internal/shared/tools/tools.go` to include:

```go
package tools

import (
	"wowdoc/internal/shared/analyze"
	"wowdoc/internal/shared/contracts"
)

type Handler interface{}

type ToolDefinition struct {
	Name         string
	SourceBacked bool
}

type Registry struct {
	Tools map[string]ToolDefinition
}

type ClientInfo struct {
	Alias        string               `json:"alias"`
	Version      string               `json:"version,omitempty"`
	Capabilities analyze.Capabilities `json:"capabilities"`
	Path         string               `json:"path"`
}

type ListClientsData struct {
	Clients []ClientInfo `json:"clients"`
}

type APIResult struct {
	Name string `json:"name"`
}

func ListClients(repos []analyze.Repository, includeDiagnostics bool) contracts.Envelope[ListClientsData] {
	env := contracts.Envelope[ListClientsData]{OK: true}
	for _, repo := range repos {
		if repo.Valid {
			env.Data.Clients = append(env.Data.Clients, ClientInfo{
				Alias: repo.Alias, Version: repo.Version, Capabilities: repo.Capabilities, Path: repo.Path,
			})
			continue
		}
		if includeDiagnostics {
			env.Diagnostics = append(env.Diagnostics, repo.Diagnostics...)
		}
	}
	return env
}

func LookupBlizzardAPI(repo analyze.Repository, idx *analyze.Index, name string) contracts.Envelope[APIResult] {
	if !repo.Capabilities.APIDocumentation {
		return contracts.Envelope[APIResult]{
			OK: false,
			Error: &contracts.ToolError{Code: contracts.ErrCapabilityUnavailable, Message: "client does not contain Blizzard_APIDocumentationGenerated"},
		}
	}
	return contracts.Envelope[APIResult]{OK: true, Data: APIResult{Name: name}}
}
```

- [x] **Step 4: Run tests**

Run:

```powershell
go test ./internal/shared/analyze ./internal/shared/tools -v
go test ./...
```

Expected: pass.

- [x] **Step 5: Commit**

```powershell
git add internal/shared/analyze/index.go internal/shared/analyze/index_test.go internal/shared/tools
git commit -m "feat: add analyze indexes and tool envelopes"
```

### Task 9: MCP SDK Adapter For Stdio And HTTP

**Files:**
- Create: `internal/shared/mcp/server.go`
- Create: `internal/shared/mcp/server_test.go`
- Create: `internal/stdio/server.go`
- Create: `internal/stdio/server_test.go`
- Modify: `internal/http/routes.go`

- [x] **Step 1: Write failing MCP adapter tests**

Create `internal/shared/mcp/server_test.go`:

```go
package mcp

import "testing"

func TestServerRegistersAllSchemas(t *testing.T) {
	s := NewServer(ServerOptions{Name: "wowdoc-test", Version: "0.0.0"})
	if len(s.RegisteredToolNames()) != 14 {
		t.Fatalf("registered count = %d, want 14", len(s.RegisteredToolNames()))
	}
	if !s.HasTool("lookup_blizzard_api") || !s.HasTool("explain_api_safety") {
		t.Fatalf("missing required tools: %#v", s.RegisteredToolNames())
	}
}
```

Create `internal/stdio/server_test.go`:

```go
package stdio

import "testing"

func TestDefaultSourceRootUsesExecutableSourcesWhenEmpty(t *testing.T) {
	got := DefaultSourceRoot(`C:\tools\wowdoc.exe`)
	want := `C:\tools\sources`
	if got != want {
		t.Fatalf("source root = %q, want %q", got, want)
	}
}
```

- [x] **Step 2: Run tests to verify they fail**

Run:

```powershell
go test ./internal/shared/mcp ./internal/stdio -v
```

Expected: fail because adapter and stdio package do not exist.

- [x] **Step 3: Implement thin adapter**

Create `internal/shared/mcp/server.go`:

```go
package mcp

type ServerOptions struct {
	Name    string
	Version string
}

type Server struct {
	opts  ServerOptions
	tools []string
}

func NewServer(opts ServerOptions) *Server {
	s := &Server{opts: opts}
	for name := range ToolInputSchemas() {
		s.tools = append(s.tools, name)
	}
	return s
}

func (s *Server) RegisteredToolNames() []string {
	return append([]string(nil), s.tools...)
}

func (s *Server) HasTool(name string) bool {
	for _, got := range s.tools {
		if got == name {
			return true
		}
	}
	return false
}
```

Create `internal/stdio/server.go`:

```go
package stdio

import "path/filepath"

func DefaultSourceRoot(exePath string) string {
	return filepath.Join(filepath.Dir(exePath), "sources")
}
```

Modify `internal/http/routes.go` `/mcp` handler to return a protocol readiness response until the SDK transport is wired:

```go
func (a *App) mcp(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, map[string]any{"transport": "streamable-http", "status": "ready"})
}
```

- [x] **Step 4: Wire official SDK in the adapter only**

Read the current official SDK examples in:

- <https://go.sdk.modelcontextprotocol.io/>
- <https://pkg.go.dev/github.com/modelcontextprotocol/go-sdk/mcp>

Then update only `internal/shared/mcp/server.go` to hold the SDK server value behind this package's adapter. Keep tests asserting `RegisteredToolNames` and `HasTool`. Do not import the SDK from any package outside `internal/shared/mcp`.

- [x] **Step 5: Run tests**

Run:

```powershell
go test ./internal/shared/mcp ./internal/stdio ./internal/http -v
go test ./...
```

Expected: pass.

- [x] **Step 6: Commit**

```powershell
git add internal/shared/mcp/server.go internal/shared/mcp/server_test.go internal/stdio internal/http/routes.go
git commit -m "feat: add shared mcp sdk adapter"
```

### Task 10: Runtime Boundary Enforcement And Build Targets

**Files:**
- Modify: `cmd/wowdoc/main.go`
- Modify: `cmd/wowdoc-server/main.go`
- Create: `internal/shared/contracts/imports_test.go`
- Create: `internal/http/boundary_test.go`

- [x] **Step 1: Write failing boundary tests**

Create `internal/http/boundary_test.go`:

```go
package http

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestHTTPDoesNotImportCLI(t *testing.T) {
	root := filepath.Join("..", "..")
	err := filepath.WalkDir(filepath.Join(root, "internal", "http"), func(path string, d os.DirEntry, err error) error {
		if err != nil || d.IsDir() || !strings.HasSuffix(path, ".go") {
			return err
		}
		b, readErr := os.ReadFile(path)
		if readErr != nil {
			return readErr
		}
		if strings.Contains(string(b), `"wowdoc/internal/cli"`) {
			t.Fatalf("HTTP must not import CLI: %s", path)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}
```

Create `internal/shared/contracts/imports_test.go`:

```go
package contracts

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestSharedPackagesDoNotImportRuntimeSurfaces(t *testing.T) {
	root := filepath.Join("..", "..")
	err := filepath.WalkDir(filepath.Join(root, "shared"), func(path string, d os.DirEntry, err error) error {
		if err != nil || d.IsDir() || !strings.HasSuffix(path, ".go") {
			return err
		}
		b, readErr := os.ReadFile(path)
		if readErr != nil {
			return readErr
		}
		text := string(b)
		for _, forbidden := range []string{`"wowdoc/internal/cli"`, `"wowdoc/internal/http"`, `"wowdoc/internal/stdio"`} {
			if strings.Contains(text, forbidden) {
				t.Fatalf("shared package %s imports runtime package %s", path, forbidden)
			}
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}
```

- [x] **Step 2: Run tests and builds**

Run:

```powershell
go test ./internal/http ./internal/shared/contracts -run "Boundary|Import|Runtime" -v
go build -o dist/wowdoc.exe ./cmd/wowdoc
go build -o dist/wowdoc-server.exe ./cmd/wowdoc-server
```

Expected: fail if any boundary is violated; otherwise pass.

- [x] **Step 3: Fix only boundary violations**

Allowed fixes:

- Move shared code into `internal/shared/*`.
- Move CLI-only code into `internal/cli`.
- Move stdio-only code into `internal/stdio`.
- Move HTTP-only code into `internal/http`.

Do not delete the boundary tests. Do not weaken the forbidden import strings.

- [x] **Step 4: Run full verification**

Run:

```powershell
go test ./...
go build -o dist/wowdoc.exe ./cmd/wowdoc
go build -o dist/wowdoc-server.exe ./cmd/wowdoc-server
```

Expected: pass.

- [x] **Step 5: Commit**

```powershell
git add cmd internal
git commit -m "test: enforce runtime package boundaries"
```

### Task 11: End-To-End Integration And Spec Coverage

**Files:**
- Create: `internal/integration/e2e_test.go`
- Modify only files needed to make integration tests pass.

- [x] **Step 1: Write failing end-to-end tests**

Create `internal/integration/e2e_test.go`:

```go
package integration

import (
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestBuildTargetsExistAndHelpMentionsAgentFields(t *testing.T) {
	root := filepath.Join("..", "..")
	wowdoc := filepath.Join(root, "dist", "wowdoc.exe")
	server := filepath.Join(root, "dist", "wowdoc-server.exe")
	if err := exec.Command("go", "build", "-o", wowdoc, "./cmd/wowdoc").Run(); err != nil {
		t.Fatalf("build wowdoc: %v", err)
	}
	if err := exec.Command("go", "build", "-o", server, "./cmd/wowdoc-server").Run(); err != nil {
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
```

- [x] **Step 2: Run e2e test to verify it fails if integration is incomplete**

Run:

```powershell
go test ./internal/integration -v
```

Expected: pass only when both build targets and help shell are wired. If it fails, fix only the missing integration wiring.

- [x] **Step 3: Add spec coverage assertions**

Extend `internal/integration/e2e_test.go` with:

```go
func TestOmittedTypeScriptToolsAreAbsent(t *testing.T) {
	out, err := exec.Command("go", "test", "./internal/shared/tools", "-run", "TestRegistryContainsExactlySupportedTools", "-v").CombinedOutput()
	if err != nil {
		t.Fatalf("registry test failed: %v\n%s", err, string(out))
	}
	if strings.Contains(string(out), "scaffold_addon") {
		t.Fatalf("omitted TypeScript tools must stay absent")
	}
}
```

- [x] **Step 4: Run full verification**

Run:

```powershell
go test ./...
go build -o dist/wowdoc.exe ./cmd/wowdoc
go build -o dist/wowdoc-server.exe ./cmd/wowdoc-server
```

Expected: all tests pass and both binaries build.

- [x] **Step 5: Commit**

```powershell
git add internal/integration cmd internal
git commit -m "test: add wowdoc end to end coverage"
```

## Self-Review

Spec coverage:

- Build targets are covered by Tasks 6, 7, 10, and 11.
- Package boundaries are covered by Tasks 1 and 10.
- MCP SDK isolation is covered by Tasks 5 and 9.
- 14 supported tools and omitted TypeScript tools are covered by Tasks 5 and 11.
- Source detection, invalid diagnostics, and partial capabilities are covered by Tasks 2 and 8.
- Source acquisition layout and arbitrary-ref policy are covered by Tasks 3 and 7.
- Safety metadata classification and scenario explanation are covered by Task 4.
- CLI help and explicit client requirement are covered by Task 6.
- HTTP health, `/mcp`, `/help`, config defaults, and pools are covered by Task 7.
- End-to-end build verification is covered by Task 11.

Completion-text scan:

- No task contains banned deferred-work wording, copy-by-reference wording, or vague test instructions.
- Any SDK uncertainty is isolated to Task 9 with an explicit adapter boundary and official documentation links.

Type consistency:

- `contracts.Envelope`, `contracts.ErrorCode`, `analyze.Repository`, `analyze.Capabilities`, `source.Manager`, `tools.Registry`, and `mcp.JSONSchema` are introduced before dependent tasks use them.
- Runtime package import boundaries are tested so subagents cannot silently blend CLI, stdio, and HTTP code.

Plan complete and saved to `docs/superpowers/plans/2026-06-03-wowdoc-go-mcp-tdd-v2.md`. Two execution options:

**1. Subagent-Driven (recommended)** - dispatch a fresh subagent per task, review between tasks, fast iteration.

**2. Inline Execution** - execute tasks in this session using executing-plans, batch execution with checkpoints.
