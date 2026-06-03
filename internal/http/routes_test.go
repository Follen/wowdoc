package http

import (
	"archive/zip"
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	sdkmcp "github.com/modelcontextprotocol/go-sdk/mcp"

	"wowdoc/internal/shared/contracts"
	wowmcp "wowdoc/internal/shared/mcp"
	"wowdoc/internal/shared/source"
	"wowdoc/internal/shared/tools"
)

func TestHealthReportsPoolsAndDiagnostics(t *testing.T) {
	app := NewApp(DefaultConfig())
	req := httptest.NewRequest("GET", "/health", nil)
	rec := httptest.NewRecorder()
	app.Router().ServeHTTP(rec, req)
	if rec.Code != 200 {
		t.Fatalf("status = %d", rec.Code)
	}
	if !strings.HasPrefix(rec.Header().Get("Content-Type"), "application/json") {
		t.Fatalf("content type = %q", rec.Header().Get("Content-Type"))
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

func TestHealthReportsDiscoveredClientsAndInvalidDirectories(t *testing.T) {
	cfg := DefaultConfig()
	cfg.Sources.Root = filepath.Join("..", "..", "testdata", "sources")
	cfg.Sources.Extra = []SourceEntry{{
		Alias: "local-retail",
		Path:  filepath.Join("..", "..", "testdata", "sources", "valid-retail"),
	}}
	app := NewApp(cfg)
	req := httptest.NewRequest("GET", "/health", nil)
	rec := httptest.NewRecorder()
	app.Router().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d", rec.Code)
	}
	var body struct {
		Clients            []tools.ClientSummary  `json:"clients"`
		InvalidDirectories []contracts.Diagnostic `json:"invalidDirectories"`
		Sources            []map[string]any       `json:"sources"`
		Pools              map[string]int         `json:"pools"`
		RecentErrors       []map[string]any       `json:"recentErrors"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("invalid json: %v\n%s", err, rec.Body.String())
	}
	if len(body.Sources) == 0 || len(body.Pools) == 0 || body.RecentErrors == nil {
		t.Fatalf("health missing operational fields: %#v", body)
	}
	if len(body.InvalidDirectories) != 1 || body.InvalidDirectories[0].Message != "source_invalid" {
		t.Fatalf("invalid directories wrong: %#v", body.InvalidDirectories)
	}
	foundRootClient := false
	foundExtraClient := false
	for _, client := range body.Clients {
		if client.Alias == "valid-retail" {
			foundRootClient = true
		}
		if client.Alias == "local-retail" {
			foundExtraClient = true
		}
	}
	if !foundRootClient || !foundExtraClient {
		t.Fatalf("health clients missing root or extra source: %#v", body.Clients)
	}
}

func TestHealthClientSummariesIncludeDefaultRefs(t *testing.T) {
	cfg := DefaultConfig()
	cfg.Sources.Root = filepath.Join("..", "..", "testdata", "sources")
	cfg.Sources.DefaultRef = "latest"
	app := NewApp(cfg)
	req := httptest.NewRequest("GET", "/health", nil)
	rec := httptest.NewRecorder()
	app.Router().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d", rec.Code)
	}
	var body struct {
		Clients []tools.ClientSummary `json:"clients"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("invalid json: %v\n%s", err, rec.Body.String())
	}
	for _, client := range body.Clients {
		if client.Alias == "valid-retail" {
			if client.DefaultRef != "latest" {
				t.Fatalf("health should include defaultRef for discovered clients: %#v", client)
			}
			return
		}
	}
	t.Fatalf("valid-retail missing from health clients: %#v", body.Clients)
}

func TestHealthReportsConfiguredDefaultSourcesEvenBeforeCheckout(t *testing.T) {
	cfg := DefaultConfig()
	cfg.Sources.Root = t.TempDir()
	app := newAppWithFetchers(cfg, nil, nil)
	req := httptest.NewRequest("GET", "/health", nil)
	rec := httptest.NewRecorder()
	app.Router().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d", rec.Code)
	}
	var body struct {
		Sources []map[string]any `json:"sources"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("invalid json: %v", err)
	}
	found := map[string]bool{}
	for _, source := range body.Sources {
		if alias, ok := source["alias"].(string); ok {
			found[alias] = true
		}
	}
	for _, alias := range []string{"retail", "classic", "classic-ptr", "classic-titan", "ptr2"} {
		if !found[alias] {
			t.Fatalf("configured default source %q missing from health sources: %#v", alias, body.Sources)
		}
	}
}

func TestHealthIgnoresInternalSourceCacheDirectories(t *testing.T) {
	root := t.TempDir()
	for _, dir := range []string{"repos", "checkouts", "archives"} {
		if err := os.MkdirAll(filepath.Join(root, dir), 0o755); err != nil {
			t.Fatalf("create internal cache dir: %v", err)
		}
	}
	cfg := DefaultConfig()
	cfg.Sources.Root = root
	app := newAppWithFetchers(cfg, nil, nil)
	req := httptest.NewRequest("GET", "/health", nil)
	rec := httptest.NewRecorder()
	app.Router().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d", rec.Code)
	}
	var body struct {
		InvalidDirectories []contracts.Diagnostic `json:"invalidDirectories"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("invalid json: %v", err)
	}
	if len(body.InvalidDirectories) != 0 {
		t.Fatalf("internal cache dirs should not be source diagnostics: %#v", body.InvalidDirectories)
	}
}

func TestNewAppPrewarmsConfiguredClients(t *testing.T) {
	cfg := DefaultConfig()
	cfg.Sources.Root = filepath.Join("..", "..", "testdata", "sources")
	cfg.Prepare.PrewarmOnStart = true
	cfg.Prepare.PrewarmClients = []string{"valid-retail"}

	app := NewApp(cfg)
	stats := app.pools.Stats()
	if stats["sources"] != 1 || stats["indexes"] != 1 {
		t.Fatalf("pool stats after prewarm = %#v, want one source and one index", stats)
	}
}

func TestHealthReportsPrewarmErrors(t *testing.T) {
	cfg := DefaultConfig()
	cfg.Sources.Root = filepath.Join("..", "..", "testdata", "sources")
	cfg.Prepare.PrewarmOnStart = true
	cfg.Prepare.PrewarmClients = []string{"missing-client"}
	app := NewApp(cfg)

	req := httptest.NewRequest("GET", "/health", nil)
	rec := httptest.NewRecorder()
	app.Router().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d", rec.Code)
	}
	var body struct {
		RecentErrors []map[string]string `json:"recentErrors"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("invalid json: %v", err)
	}
	if len(body.RecentErrors) == 0 || body.RecentErrors[0]["client"] != "missing-client" {
		t.Fatalf("prewarm error missing from health: %#v", body.RecentErrors)
	}
}

func TestReadOnlyRoutesRejectNonGETMethods(t *testing.T) {
	app := NewApp(DefaultConfig())
	for _, path := range []string{"/health", "/help"} {
		req := httptest.NewRequest(http.MethodPost, path, nil)
		rec := httptest.NewRecorder()
		app.Router().ServeHTTP(rec, req)
		if rec.Code != http.StatusMethodNotAllowed {
			t.Fatalf("%s POST status = %d, want %d", path, rec.Code, http.StatusMethodNotAllowed)
		}
	}
}

func TestHelpReportsOnlyPublicHTTPRoutes(t *testing.T) {
	app := NewApp(DefaultConfig())
	req := httptest.NewRequest(http.MethodGet, "/help", nil)
	rec := httptest.NewRecorder()
	app.Router().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d", rec.Code)
	}
	var body map[string]string
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("invalid json: %v\n%s", err, rec.Body.String())
	}
	if body["mcp"] != "/mcp" || body["health"] != "/health" {
		t.Fatalf("help route map wrong: %#v", body)
	}
	if _, ok := body["cli"]; ok {
		t.Fatalf("HTTP help must not expose CLI routes: %#v", body)
	}
}

func TestUnknownHTTPRouteReturnsNotFound(t *testing.T) {
	app := NewApp(DefaultConfig())
	req := httptest.NewRequest(http.MethodGet, "/clients/list", nil)
	rec := httptest.NewRecorder()
	app.Router().ServeHTTP(rec, req)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("unknown route status = %d, want %d", rec.Code, http.StatusNotFound)
	}
}

func TestMCPRouteServesStreamableHTTPTools(t *testing.T) {
	app := NewApp(DefaultConfig())
	server := httptest.NewServer(app.Router())
	defer server.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	client := sdkmcp.NewClient(&sdkmcp.Implementation{Name: "http-test-client", Version: "v0.0.0-test"}, nil)
	session, err := client.Connect(ctx, &sdkmcp.StreamableClientTransport{Endpoint: server.URL + "/mcp"}, nil)
	if err != nil {
		t.Fatalf("connect streamable http: %v", err)
	}
	defer session.Close()

	tools, err := session.ListTools(ctx, &sdkmcp.ListToolsParams{})
	if err != nil {
		t.Fatalf("list tools: %v", err)
	}
	if len(tools.Tools) != len(wowmcp.ToolInputSchemas()) {
		t.Fatalf("tool count = %d, want %d", len(tools.Tools), len(wowmcp.ToolInputSchemas()))
	}
}

func TestMCPRouteInspectRemoteRefsUsesHTTPGitRunner(t *testing.T) {
	cfg := DefaultConfig()
	cfg.Sources.Defaults = []SourceEntry{{
		Alias: "ptr",
		Repo:  "https://example.test/wow-ui-source.git",
		Ref:   "ptr2",
	}}
	app := newAppWithGit(cfg, remoteRefsHTTPGit{
		"ptr2": "96443d533fd09a5b6195637f95c896439c616cee",
	})
	server := httptest.NewServer(app.Router())
	defer server.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	client := sdkmcp.NewClient(&sdkmcp.Implementation{Name: "http-test-client", Version: "v0.0.0-test"}, nil)
	session, err := client.Connect(ctx, &sdkmcp.StreamableClientTransport{Endpoint: server.URL + "/mcp"}, nil)
	if err != nil {
		t.Fatalf("connect streamable http: %v", err)
	}
	defer session.Close()

	result, err := session.CallTool(ctx, &sdkmcp.CallToolParams{
		Name:      "inspect_remote_refs",
		Arguments: map[string]any{"client": "ptr"},
	})
	if err != nil {
		t.Fatalf("call inspect_remote_refs: %v", err)
	}
	if result.IsError {
		t.Fatalf("inspect_remote_refs returned error: %#v", result)
	}
	var env contracts.Envelope[tools.RemoteRefsData]
	mustDecodeStructured(t, result.StructuredContent, &env)
	if !env.OK || len(env.Data.Clients) != 1 || env.Data.Clients[0].ConfiguredRef != "ptr2" || env.Data.Clients[0].Commit != "96443d533fd09a5b6195637f95c896439c616cee" {
		t.Fatalf("remote refs envelope wrong: %#v", env)
	}
}

func TestMCPRouteUsesConfiguredSourceRootForTools(t *testing.T) {
	cfg := DefaultConfig()
	cfg.Sources.Root = filepath.Join("..", "..", "testdata", "sources")
	app := NewApp(cfg)
	server := httptest.NewServer(app.Router())
	defer server.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	client := sdkmcp.NewClient(&sdkmcp.Implementation{Name: "http-test-client", Version: "v0.0.0-test"}, nil)
	session, err := client.Connect(ctx, &sdkmcp.StreamableClientTransport{Endpoint: server.URL + "/mcp"}, nil)
	if err != nil {
		t.Fatalf("connect streamable http: %v", err)
	}
	defer session.Close()

	result, err := session.CallTool(ctx, &sdkmcp.CallToolParams{
		Name:      "list_clients",
		Arguments: map[string]any{"includeDiagnostics": true},
	})
	if err != nil {
		t.Fatalf("call list_clients: %v", err)
	}
	if result.IsError {
		t.Fatalf("list_clients returned error: %#v", result)
	}
	var env contracts.Envelope[tools.ListClientsData]
	mustDecodeStructured(t, result.StructuredContent, &env)
	if !env.OK || len(env.Data.Clients) < 2 || len(env.Diagnostics) != 1 {
		t.Fatalf("list_clients envelope wrong: %#v", env)
	}
}

func TestMCPRouteIncludesConfiguredExtraLocalSources(t *testing.T) {
	cfg := DefaultConfig()
	cfg.Sources.Extra = []SourceEntry{{
		Alias: "local-retail",
		Path:  filepath.Join("..", "..", "testdata", "sources", "valid-retail"),
	}}
	app := NewApp(cfg)
	server := httptest.NewServer(app.Router())
	defer server.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	client := sdkmcp.NewClient(&sdkmcp.Implementation{Name: "http-test-client", Version: "v0.0.0-test"}, nil)
	session, err := client.Connect(ctx, &sdkmcp.StreamableClientTransport{Endpoint: server.URL + "/mcp"}, nil)
	if err != nil {
		t.Fatalf("connect streamable http: %v", err)
	}
	defer session.Close()

	result, err := session.CallTool(ctx, &sdkmcp.CallToolParams{Name: "list_clients"})
	if err != nil {
		t.Fatalf("call list_clients: %v", err)
	}
	if result.IsError {
		t.Fatalf("list_clients returned error: %#v", result)
	}
	var env contracts.Envelope[tools.ListClientsData]
	mustDecodeStructured(t, result.StructuredContent, &env)
	for _, client := range env.Data.Clients {
		if client.Alias == "local-retail" {
			return
		}
	}
	t.Fatalf("configured extra source alias missing: %#v", env.Data.Clients)
}

func TestMCPRouteListClientsIncludesConfiguredRepoSourcesBeforeCheckout(t *testing.T) {
	cfg := DefaultConfig()
	cfg.Sources.Root = t.TempDir()
	cfg.Sources.Defaults = []SourceEntry{{
		Alias: "configured-retail",
		Repo:  "https://example.test/wow-ui-source.git",
		Ref:   "main",
	}}
	cfg.Sources.Extra = nil
	app := newAppWithFetchers(cfg, nil, nil)
	server := httptest.NewServer(app.Router())
	defer server.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	client := sdkmcp.NewClient(&sdkmcp.Implementation{Name: "http-test-client", Version: "v0.0.0-test"}, nil)
	session, err := client.Connect(ctx, &sdkmcp.StreamableClientTransport{Endpoint: server.URL + "/mcp"}, nil)
	if err != nil {
		t.Fatalf("connect streamable http: %v", err)
	}
	defer session.Close()

	result, err := session.CallTool(ctx, &sdkmcp.CallToolParams{
		Name:      "list_clients",
		Arguments: map[string]any{"includeRefs": true},
	})
	if err != nil {
		t.Fatalf("call list_clients: %v", err)
	}
	if result.IsError {
		t.Fatalf("list_clients returned error: %#v", result)
	}
	var env contracts.Envelope[tools.ListClientsData]
	mustDecodeStructured(t, result.StructuredContent, &env)
	for _, client := range env.Data.Clients {
		if client.Alias == "configured-retail" {
			if client.DefaultRef != "main" {
				t.Fatalf("configured repo default ref missing: %#v", client)
			}
			return
		}
	}
	t.Fatalf("configured repo source alias missing before checkout: %#v", env.Data.Clients)
}

func TestMCPRouteLooksUpConfiguredExtraLocalSource(t *testing.T) {
	cfg := DefaultConfig()
	cfg.Sources.Extra = []SourceEntry{{
		Alias: "local-retail",
		Path:  filepath.Join("..", "..", "testdata", "sources", "valid-retail"),
	}}
	app := NewApp(cfg)
	server := httptest.NewServer(app.Router())
	defer server.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	client := sdkmcp.NewClient(&sdkmcp.Implementation{Name: "http-test-client", Version: "v0.0.0-test"}, nil)
	session, err := client.Connect(ctx, &sdkmcp.StreamableClientTransport{Endpoint: server.URL + "/mcp"}, nil)
	if err != nil {
		t.Fatalf("connect streamable http: %v", err)
	}
	defer session.Close()

	result, err := session.CallTool(ctx, &sdkmcp.CallToolParams{
		Name:      "lookup_blizzard_api",
		Arguments: map[string]any{"client": "local-retail", "name": "C_AuctionHouse"},
	})
	if err != nil {
		t.Fatalf("call lookup_blizzard_api: %v", err)
	}
	if result.IsError {
		t.Fatalf("lookup_blizzard_api returned error: %#v", result)
	}
	var env contracts.Envelope[tools.APIResult]
	mustDecodeStructured(t, result.StructuredContent, &env)
	if !env.OK || env.Source.Client != "local-retail" || env.Data.Name != "C_AuctionHouse" {
		t.Fatalf("lookup envelope wrong: %#v", env)
	}
}

func TestMCPRouteLooksUpConfiguredExtraRepoRefFromExistingCheckout(t *testing.T) {
	root := t.TempDir()
	checkout := filepath.Join(root, "checkouts", "cached-retail", "main")
	copyDir(t, filepath.Join("..", "..", "testdata", "sources", "valid-retail"), checkout)
	cfg := DefaultConfig()
	cfg.Sources.Root = root
	cfg.Sources.Extra = []SourceEntry{{
		Alias: "cached-retail",
		Repo:  "https://example.test/wow-ui-source.git",
		Ref:   "main",
	}}
	app := newAppWithFetchers(cfg, nil, nil)
	server := httptest.NewServer(app.Router())
	defer server.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	client := sdkmcp.NewClient(&sdkmcp.Implementation{Name: "http-test-client", Version: "v0.0.0-test"}, nil)
	session, err := client.Connect(ctx, &sdkmcp.StreamableClientTransport{Endpoint: server.URL + "/mcp"}, nil)
	if err != nil {
		t.Fatalf("connect streamable http: %v", err)
	}
	defer session.Close()

	result, err := session.CallTool(ctx, &sdkmcp.CallToolParams{
		Name:      "lookup_blizzard_api",
		Arguments: map[string]any{"client": "cached-retail", "name": "C_AuctionHouse"},
	})
	if err != nil {
		t.Fatalf("call lookup_blizzard_api: %v", err)
	}
	if result.IsError {
		t.Fatalf("lookup_blizzard_api returned error: %#v", result)
	}
	var env contracts.Envelope[tools.APIResult]
	mustDecodeStructured(t, result.StructuredContent, &env)
	if !env.OK || env.Source.Client != "cached-retail" || env.Source.RequestedRef != "main" || env.Data.Name != "C_AuctionHouse" {
		t.Fatalf("lookup envelope wrong: %#v", env)
	}
}

func TestMCPRouteAcquiresConfiguredExtraRepoRefWithGit(t *testing.T) {
	root := t.TempDir()
	git := &fixtureGit{fixture: filepath.Join("..", "..", "testdata", "sources", "valid-retail"), resolvedCommit: "abc123def456"}
	cfg := DefaultConfig()
	cfg.Sources.Root = root
	cfg.Sources.Extra = []SourceEntry{{
		Alias: "git-retail",
		Repo:  "https://example.test/wow-ui-source.git",
		Ref:   "main",
	}}
	app := newAppWithGit(cfg, git)
	server := httptest.NewServer(app.Router())
	defer server.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	client := sdkmcp.NewClient(&sdkmcp.Implementation{Name: "http-test-client", Version: "v0.0.0-test"}, nil)
	session, err := client.Connect(ctx, &sdkmcp.StreamableClientTransport{Endpoint: server.URL + "/mcp"}, nil)
	if err != nil {
		t.Fatalf("connect streamable http: %v", err)
	}
	defer session.Close()

	result, err := session.CallTool(ctx, &sdkmcp.CallToolParams{
		Name:      "lookup_blizzard_api",
		Arguments: map[string]any{"client": "git-retail", "name": "C_AuctionHouse"},
	})
	if err != nil {
		t.Fatalf("call lookup_blizzard_api: %v", err)
	}
	if result.IsError {
		t.Fatalf("lookup_blizzard_api returned error: %#v", result)
	}
	var env contracts.Envelope[tools.APIResult]
	mustDecodeStructured(t, result.StructuredContent, &env)
	if env.Source.RequestedRef != "main" || env.Source.ResolvedRef != "abc123def456" {
		t.Fatalf("source transparency wrong: %#v", env.Source)
	}
	if _, err := os.Stat(filepath.Join(root, "checkouts", "git-retail", "abc123def456", "Interface")); err != nil {
		t.Fatalf("checkout was not acquired: %v", err)
	}
	want := [][]string{
		{"clone", "--mirror", "https://example.test/wow-ui-source.git", filepath.Join(root, "repos", "git-retail.git")},
		{"--git-dir", filepath.Join(root, "repos", "git-retail.git"), "fetch"},
		{"--git-dir", filepath.Join(root, "repos", "git-retail.git"), "rev-parse", "main^{commit}"},
		{"--git-dir", filepath.Join(root, "repos", "git-retail.git"), "worktree", "add", "--detach", filepath.Join(root, "checkouts", "git-retail", "abc123def456"), "abc123def456"},
	}
	if !equalCommands(git.commands, want) {
		t.Fatalf("git commands = %#v, want %#v", git.commands, want)
	}
}

func TestMCPRouteSupportsSlashBranchRefWhenArbitraryRefsEnabled(t *testing.T) {
	root := t.TempDir()
	git := &fixtureGit{fixture: filepath.Join("..", "..", "testdata", "sources", "valid-retail"), resolvedCommit: "abc123def456"}
	cfg := DefaultConfig()
	cfg.Sources.Root = root
	cfg.Sources.AllowArbitraryRef = true
	cfg.Sources.Extra = []SourceEntry{{
		Alias: "git-retail",
		Repo:  "https://example.test/wow-ui-source.git",
		Ref:   "main",
	}}
	app := newAppWithGit(cfg, git)
	server := httptest.NewServer(app.Router())
	defer server.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	client := sdkmcp.NewClient(&sdkmcp.Implementation{Name: "http-test-client", Version: "v0.0.0-test"}, nil)
	session, err := client.Connect(ctx, &sdkmcp.StreamableClientTransport{Endpoint: server.URL + "/mcp"}, nil)
	if err != nil {
		t.Fatalf("connect streamable http: %v", err)
	}
	defer session.Close()

	result, err := session.CallTool(ctx, &sdkmcp.CallToolParams{
		Name:      "lookup_blizzard_api",
		Arguments: map[string]any{"client": "git-retail", "ref": "feature/auction-fix", "name": "C_AuctionHouse"},
	})
	if err != nil {
		t.Fatalf("call lookup_blizzard_api: %v", err)
	}
	if result.IsError {
		t.Fatalf("lookup_blizzard_api returned error: %#v", result)
	}
	var env contracts.Envelope[tools.APIResult]
	mustDecodeStructured(t, result.StructuredContent, &env)
	if !env.OK || env.Source.RequestedRef != "feature/auction-fix" || env.Source.ResolvedRef != "abc123def456" || env.Source.Path != filepath.Join(root, "checkouts", "git-retail", "abc123def456") {
		t.Fatalf("lookup envelope wrong: %#v", env)
	}
	wantResolve := []string{"--git-dir", filepath.Join(root, "repos", "git-retail.git"), "rev-parse", "feature/auction-fix^{commit}"}
	if !containsCommand(git.commands, wantResolve) {
		t.Fatalf("git commands = %#v, missing rev-parse %#v", git.commands, wantResolve)
	}
}

func TestMCPRouteFallsBackToArchiveWhenGitFails(t *testing.T) {
	root := t.TempDir()
	archive := archiveFixtureZip(t, filepath.Join("..", "..", "testdata", "sources", "valid-retail"))
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/owner/wow-ui-source/archive/main.zip" {
			t.Fatalf("archive request path = %q", r.URL.Path)
		}
		_, _ = w.Write(archive)
	}))
	defer server.Close()
	cfg := DefaultConfig()
	cfg.Sources.Root = root
	cfg.Sources.Extra = []SourceEntry{{
		Alias: "archive-retail",
		Repo:  server.URL + "/owner/wow-ui-source.git",
		Ref:   "main",
	}}
	app := newAppWithFetchers(cfg, failingHTTPGit{}, source.NewHTTPArchiveFetcher(server.Client()))
	mcpServer := httptest.NewServer(app.Router())
	defer mcpServer.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	client := sdkmcp.NewClient(&sdkmcp.Implementation{Name: "http-test-client", Version: "v0.0.0-test"}, nil)
	session, err := client.Connect(ctx, &sdkmcp.StreamableClientTransport{Endpoint: mcpServer.URL + "/mcp"}, nil)
	if err != nil {
		t.Fatalf("connect streamable http: %v", err)
	}
	defer session.Close()

	result, err := session.CallTool(ctx, &sdkmcp.CallToolParams{
		Name:      "lookup_blizzard_api",
		Arguments: map[string]any{"client": "archive-retail", "name": "C_AuctionHouse"},
	})
	if err != nil {
		t.Fatalf("call lookup_blizzard_api: %v", err)
	}
	if result.IsError {
		t.Fatalf("lookup_blizzard_api returned error: %#v", result)
	}
	var env contracts.Envelope[tools.APIResult]
	mustDecodeStructured(t, result.StructuredContent, &env)
	if !containsDiagnostic(env.Diagnostics, "archive fallback is not incremental") ||
		!containsDiagnostic(env.Diagnostics, "branch archives may need periodic redownload") ||
		!containsDiagnostic(env.Diagnostics, "resolved commit may be unknown for archive fallback") {
		t.Fatalf("archive fallback limitations missing from diagnostics: %#v", env.Diagnostics)
	}
	if _, err := os.Stat(filepath.Join(root, "archives", "archive-retail", "main", "Interface")); err != nil {
		t.Fatalf("archive checkout was not extracted: %v", err)
	}
}

func TestMCPRouteReportsStableArchiveFailureCode(t *testing.T) {
	root := t.TempDir()
	cfg := DefaultConfig()
	cfg.Sources.Root = root
	cfg.Sources.Extra = []SourceEntry{{
		Alias: "archive-retail",
		Repo:  "https://example.test/wow-ui-source.git",
		Ref:   "main",
	}}
	app := newAppWithFetchers(cfg, failingHTTPGit{}, failingHTTPArchive{})
	server := httptest.NewServer(app.Router())
	defer server.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	client := sdkmcp.NewClient(&sdkmcp.Implementation{Name: "http-test-client", Version: "v0.0.0-test"}, nil)
	session, err := client.Connect(ctx, &sdkmcp.StreamableClientTransport{Endpoint: server.URL + "/mcp"}, nil)
	if err != nil {
		t.Fatalf("connect streamable http: %v", err)
	}
	defer session.Close()

	result, err := session.CallTool(ctx, &sdkmcp.CallToolParams{
		Name:      "lookup_blizzard_api",
		Arguments: map[string]any{"client": "archive-retail", "name": "C_AuctionHouse"},
	})
	if err != nil {
		t.Fatalf("call lookup_blizzard_api: %v", err)
	}
	if !result.IsError {
		t.Fatalf("expected archive failure to be an MCP tool error: %#v", result)
	}
	var env contracts.Envelope[any]
	mustDecodeStructured(t, result.StructuredContent, &env)
	if env.Error == nil || env.Error.Code != contracts.ErrGitUnavailableArchiveFailed {
		t.Fatalf("error envelope = %#v, want git_unavailable_archive_failed", env)
	}
}

func TestMCPRouteRejectsArbitraryRefWhenDisabled(t *testing.T) {
	root := t.TempDir()
	cfg := DefaultConfig()
	cfg.Sources.Root = root
	cfg.Sources.Extra = []SourceEntry{{
		Alias: "retail",
		Repo:  "https://example.test/wow-ui-source.git",
		Ref:   "main",
	}}
	app := NewApp(cfg)
	server := httptest.NewServer(app.Router())
	defer server.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	client := sdkmcp.NewClient(&sdkmcp.Implementation{Name: "http-test-client", Version: "v0.0.0-test"}, nil)
	session, err := client.Connect(ctx, &sdkmcp.StreamableClientTransport{Endpoint: server.URL + "/mcp"}, nil)
	if err != nil {
		t.Fatalf("connect streamable http: %v", err)
	}
	defer session.Close()

	result, err := session.CallTool(ctx, &sdkmcp.CallToolParams{
		Name:      "lookup_blizzard_api",
		Arguments: map[string]any{"client": "retail", "ref": "feature-build", "name": "C_AuctionHouse"},
	})
	if err != nil {
		t.Fatalf("call lookup_blizzard_api: %v", err)
	}
	if !result.IsError {
		t.Fatalf("expected unsupported ref to be an MCP tool error: %#v", result)
	}
	var env contracts.Envelope[tools.APIResult]
	mustDecodeStructured(t, result.StructuredContent, &env)
	if env.Error == nil || env.Error.Code != contracts.ErrUnsupportedRef {
		t.Fatalf("error envelope = %#v, want unsupported_ref", env)
	}
}

func TestMCPRouteSourceBackedToolCallsUseHTTPPools(t *testing.T) {
	cfg := DefaultConfig()
	cfg.Sources.Root = filepath.Join("..", "..", "testdata", "sources")
	app := NewApp(cfg)
	server := httptest.NewServer(app.Router())
	defer server.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	client := sdkmcp.NewClient(&sdkmcp.Implementation{Name: "http-test-client", Version: "v0.0.0-test"}, nil)
	session, err := client.Connect(ctx, &sdkmcp.StreamableClientTransport{Endpoint: server.URL + "/mcp"}, nil)
	if err != nil {
		t.Fatalf("connect streamable http: %v", err)
	}
	defer session.Close()

	result, err := session.CallTool(ctx, &sdkmcp.CallToolParams{
		Name:      "lookup_blizzard_api",
		Arguments: map[string]any{"client": "valid-retail", "name": "C_AuctionHouse"},
	})
	if err != nil {
		t.Fatalf("call lookup_blizzard_api: %v", err)
	}
	if result.IsError {
		t.Fatalf("lookup_blizzard_api returned error: %#v", result)
	}

	stats := app.pools.Stats()
	if stats["sources"] != 1 || stats["indexes"] != 1 {
		t.Fatalf("pool stats after source-backed MCP call = %#v, want one source and one index", stats)
	}
}

func TestMCPRouteConcurrentSameSourceToolCallsShareHTTPPools(t *testing.T) {
	cfg := DefaultConfig()
	cfg.Sources.Root = filepath.Join("..", "..", "testdata", "sources")
	app := NewApp(cfg)
	server := httptest.NewServer(app.Router())
	defer server.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	var wg sync.WaitGroup
	errs := make(chan error, 2)
	for range 2 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			client := sdkmcp.NewClient(&sdkmcp.Implementation{Name: "http-test-client", Version: "v0.0.0-test"}, nil)
			session, err := client.Connect(ctx, &sdkmcp.StreamableClientTransport{Endpoint: server.URL + "/mcp"}, nil)
			if err != nil {
				errs <- err
				return
			}
			defer session.Close()
			result, err := session.CallTool(ctx, &sdkmcp.CallToolParams{
				Name:      "search_blizzard_api",
				Arguments: map[string]any{"client": "valid-retail", "query": "Auction"},
			})
			if err != nil {
				errs <- err
				return
			}
			if result.IsError {
				errs <- errResult(result)
			}
		}()
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		if err != nil {
			t.Fatalf("concurrent MCP call failed: %v", err)
		}
	}

	stats := app.pools.Stats()
	if stats["sources"] != 1 || stats["indexes"] != 1 {
		t.Fatalf("pool stats after concurrent calls = %#v, want one source and one index", stats)
	}
}

func TestMCPRouteConcurrentDifferentRefsUseSeparateHTTPContexts(t *testing.T) {
	root := t.TempDir()
	git := &blockingWorktreeGit{
		fixture: filepath.Join("..", "..", "testdata", "sources", "valid-retail"),
	}
	cfg := DefaultConfig()
	cfg.Sources.Root = root
	cfg.Sources.AllowArbitraryRef = true
	cfg.Sources.Extra = []SourceEntry{{
		Alias: "git-retail",
		Repo:  "https://example.test/wow-ui-source.git",
		Ref:   "main",
	}}
	app := newAppWithGit(cfg, git)
	server := httptest.NewServer(app.Router())
	defer server.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	type callResult struct {
		ref string
		env contracts.Envelope[tools.APIResult]
		err error
	}
	results := make(chan callResult, 2)
	var wg sync.WaitGroup
	for _, ref := range []string{"a", "b"} {
		ref := ref
		wg.Add(1)
		go func() {
			defer wg.Done()
			client := sdkmcp.NewClient(&sdkmcp.Implementation{Name: "http-test-client", Version: "v0.0.0-test"}, nil)
			session, err := client.Connect(ctx, &sdkmcp.StreamableClientTransport{Endpoint: server.URL + "/mcp"}, nil)
			if err != nil {
				results <- callResult{ref: ref, err: err}
				return
			}
			defer session.Close()
			result, err := session.CallTool(ctx, &sdkmcp.CallToolParams{
				Name:      "lookup_blizzard_api",
				Arguments: map[string]any{"client": "git-retail", "ref": ref, "name": "C_AuctionHouse"},
			})
			if err != nil {
				results <- callResult{ref: ref, err: err}
				return
			}
			if result.IsError {
				results <- callResult{ref: ref, err: errResult(result)}
				return
			}
			var env contracts.Envelope[tools.APIResult]
			mustDecodeStructured(t, result.StructuredContent, &env)
			results <- callResult{ref: ref, env: env}
		}()
	}
	wg.Wait()
	close(results)

	seen := map[string]string{}
	for result := range results {
		if result.err != nil {
			t.Fatalf("concurrent ref %s call failed: %v", result.ref, result.err)
		}
		wantCommit := result.ref + "-commit"
		if !result.env.OK || result.env.Source.RequestedRef != result.ref || result.env.Source.ResolvedRef != wantCommit {
			t.Fatalf("ref %s envelope wrong: %#v", result.ref, result.env)
		}
		seen[result.ref] = result.env.Source.Path
	}
	if seen["a"] == "" || seen["b"] == "" || seen["a"] == seen["b"] {
		t.Fatalf("different refs should use distinct source paths: %#v", seen)
	}
	stats := app.pools.Stats()
	if stats["sources"] != 2 || stats["indexes"] != 2 {
		t.Fatalf("pool stats after different-ref calls = %#v, want two sources and indexes", stats)
	}
}

func TestMCPRouteAppliesConfiguredRequestTimeout(t *testing.T) {
	cfg := DefaultConfig()
	cfg.Limits.RequestTimeoutSeconds = 1
	cfg.Limits.MaxConcurrentSourceFetches = 1
	cfg.Sources.Root = filepath.Join("..", "..", "testdata", "sources")
	app := newAppWithFetchers(cfg, nil, nil)
	app.sourceFetchSlots <- struct{}{}
	defer func() { <-app.sourceFetchSlots }()
	server := httptest.NewServer(app.Router())
	defer server.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	client := sdkmcp.NewClient(&sdkmcp.Implementation{Name: "http-timeout-test-client", Version: "v0.0.0-test"}, nil)
	session, err := client.Connect(ctx, &sdkmcp.StreamableClientTransport{Endpoint: server.URL + "/mcp"}, nil)
	if err != nil {
		t.Fatalf("connect streamable http: %v", err)
	}
	defer session.Close()

	result, err := session.CallTool(ctx, &sdkmcp.CallToolParams{
		Name:      "lookup_blizzard_api",
		Arguments: map[string]any{"client": "valid-retail", "name": "C_AuctionHouse"},
	})
	if err != nil {
		t.Fatalf("call lookup_blizzard_api: %v", err)
	}
	if !result.IsError {
		t.Fatalf("expected timeout tool error: %#v", result)
	}
	var env contracts.Envelope[any]
	mustDecodeStructured(t, result.StructuredContent, &env)
	if env.Error == nil || env.Error.Code != contracts.ErrTimeout {
		t.Fatalf("timeout envelope = %#v, want timeout code", env)
	}
}

func TestMCPRouteTimesOutWaitingForIndexBuildSlot(t *testing.T) {
	cfg := DefaultConfig()
	cfg.Limits.RequestTimeoutSeconds = 1
	cfg.Limits.MaxConcurrentIndexBuilds = 1
	cfg.Sources.Root = filepath.Join("..", "..", "testdata", "sources")
	app := newAppWithFetchers(cfg, nil, nil)
	app.indexBuildSlots <- struct{}{}
	defer func() { <-app.indexBuildSlots }()
	server := httptest.NewServer(app.Router())
	defer server.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	client := sdkmcp.NewClient(&sdkmcp.Implementation{Name: "http-index-timeout-test-client", Version: "v0.0.0-test"}, nil)
	session, err := client.Connect(ctx, &sdkmcp.StreamableClientTransport{Endpoint: server.URL + "/mcp"}, nil)
	if err != nil {
		t.Fatalf("connect streamable http: %v", err)
	}
	defer session.Close()

	result, err := session.CallTool(ctx, &sdkmcp.CallToolParams{
		Name:      "lookup_blizzard_api",
		Arguments: map[string]any{"client": "valid-retail", "name": "C_AuctionHouse"},
	})
	if err != nil {
		t.Fatalf("call lookup_blizzard_api: %v", err)
	}
	if !result.IsError {
		t.Fatalf("expected timeout tool error: %#v", result)
	}
	var env contracts.Envelope[any]
	mustDecodeStructured(t, result.StructuredContent, &env)
	if env.Error == nil || env.Error.Code != contracts.ErrTimeout {
		t.Fatalf("timeout envelope = %#v, want timeout code", env)
	}
}

func TestLoadRepoIndexHonorsMaxConcurrentSourceFetches(t *testing.T) {
	root := t.TempDir()
	git := &blockingWorktreeGit{
		fixture: filepath.Join("..", "..", "testdata", "sources", "valid-retail"),
	}
	cfg := DefaultConfig()
	cfg.Sources.Root = root
	cfg.Sources.Extra = []SourceEntry{
		{Alias: "git-retail-a", Repo: "https://example.test/a.git", Ref: "main"},
		{Alias: "git-retail-b", Repo: "https://example.test/b.git", Ref: "main"},
	}
	cfg.Limits.MaxConcurrentSourceFetches = 1
	app := newAppWithGit(cfg, git)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	var wg sync.WaitGroup
	errs := make(chan error, 2)
	for _, client := range []string{"git-retail-a", "git-retail-b"} {
		client := client
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, _, err := app.loadRepoIndex(ctx, client, "")
			errs <- err
		}()
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		if err != nil {
			t.Fatalf("loadRepoIndex failed: %v", err)
		}
	}
	if git.maxActiveWorktrees > 1 {
		t.Fatalf("source fetches ran concurrently: max active worktrees = %d, want <= 1", git.maxActiveWorktrees)
	}
}

func errResult(result *sdkmcp.CallToolResult) error {
	data, _ := json.Marshal(result.StructuredContent)
	return &toolCallError{message: string(data)}
}

type toolCallError struct {
	message string
}

func (e *toolCallError) Error() string { return e.message }

func copyDir(t *testing.T, src, dst string) {
	t.Helper()
	if err := filepath.WalkDir(src, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(src, path)
		if err != nil {
			return err
		}
		target := filepath.Join(dst, rel)
		if d.IsDir() {
			return os.MkdirAll(target, 0o755)
		}
		b, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		return os.WriteFile(target, b, 0o644)
	}); err != nil {
		t.Fatalf("copy fixture dir: %v", err)
	}
}

type fixtureGit struct {
	fixture        string
	resolvedCommit string
	commands       [][]string
}

type failingHTTPGit struct{}

func (failingHTTPGit) Run(args ...string) error { return os.ErrNotExist }
func (failingHTTPGit) Output(args ...string) ([]byte, error) {
	return nil, os.ErrNotExist
}

type failingHTTPArchive struct{}

func (failingHTTPArchive) FetchArchive(repoURL, ref, destination string) error {
	return os.ErrNotExist
}

type blockingWorktreeGit struct {
	fixture            string
	mu                 sync.Mutex
	activeWorktrees    int
	maxActiveWorktrees int
}

func (g *blockingWorktreeGit) Run(args ...string) error {
	if len(args) == 4 && args[0] == "clone" && args[1] == "--mirror" {
		return os.MkdirAll(args[3], 0o755)
	}
	if len(args) == 7 && args[2] == "worktree" && args[3] == "add" {
		g.mu.Lock()
		g.activeWorktrees++
		if g.activeWorktrees > g.maxActiveWorktrees {
			g.maxActiveWorktrees = g.activeWorktrees
		}
		g.mu.Unlock()
		time.Sleep(50 * time.Millisecond)
		err := copyDirErr(g.fixture, args[5])
		g.mu.Lock()
		g.activeWorktrees--
		g.mu.Unlock()
		return err
	}
	return nil
}

func (g *blockingWorktreeGit) Output(args ...string) ([]byte, error) {
	if len(args) >= 4 && args[len(args)-2] == "rev-parse" {
		return []byte(args[len(args)-1][:1] + "-commit\n"), nil
	}
	return []byte("main\n"), nil
}

func (g *fixtureGit) Run(args ...string) error {
	g.commands = append(g.commands, append([]string(nil), args...))
	if len(args) == 4 && args[0] == "clone" && args[1] == "--mirror" {
		return os.MkdirAll(args[3], 0o755)
	}
	if len(args) == 7 && args[2] == "worktree" && args[3] == "add" {
		return copyDirErr(g.fixture, args[5])
	}
	return nil
}

func (g *fixtureGit) Output(args ...string) ([]byte, error) {
	g.commands = append(g.commands, append([]string(nil), args...))
	if g.resolvedCommit != "" {
		return []byte(g.resolvedCommit + "\n"), nil
	}
	return []byte("main\n"), nil
}

func copyDirErr(src, dst string) error {
	return filepath.WalkDir(src, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(src, path)
		if err != nil {
			return err
		}
		target := filepath.Join(dst, rel)
		if d.IsDir() {
			return os.MkdirAll(target, 0o755)
		}
		b, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		return os.WriteFile(target, b, 0o644)
	})
}

func archiveFixtureZip(t *testing.T, src string) []byte {
	t.Helper()
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	if err := filepath.WalkDir(src, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}
		rel, err := filepath.Rel(src, path)
		if err != nil {
			return err
		}
		w, err := zw.Create(filepath.ToSlash(filepath.Join("wow-ui-source-main", rel)))
		if err != nil {
			return err
		}
		b, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		_, err = w.Write(b)
		return err
	}); err != nil {
		t.Fatalf("build archive fixture: %v", err)
	}
	if err := zw.Close(); err != nil {
		t.Fatalf("close archive fixture: %v", err)
	}
	return buf.Bytes()
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

func mustDecodeStructured(t *testing.T, value any, out any) {
	t.Helper()
	data, err := json.Marshal(value)
	if err != nil {
		t.Fatalf("marshal structured content: %v", err)
	}
	if err := json.Unmarshal(data, out); err != nil {
		t.Fatalf("decode structured content: %v\n%s", err, string(data))
	}
}

func containsDiagnostic(values []contracts.Diagnostic, text string) bool {
	for _, diagnostic := range values {
		if strings.Contains(diagnostic.Message, text) {
			return true
		}
	}
	return false
}

type remoteRefsHTTPGit map[string]string

func (g remoteRefsHTTPGit) Run(args ...string) error { return nil }

func (g remoteRefsHTTPGit) Output(args ...string) ([]byte, error) {
	if len(args) >= 4 && args[0] == "ls-remote" && args[1] == "--heads" {
		ref := args[3]
		commit := g[ref]
		if commit == "" {
			return []byte{}, nil
		}
		return []byte(commit + "\trefs/heads/" + ref + "\n"), nil
	}
	return nil, os.ErrNotExist
}

func TestMCPRouteRejectsUnsupportedMethods(t *testing.T) {
	app := NewApp(DefaultConfig())
	req := httptest.NewRequest(http.MethodDelete, "/mcp", nil)
	rec := httptest.NewRecorder()
	app.Router().ServeHTTP(rec, req)
	if rec.Code != http.StatusMethodNotAllowed {
		t.Fatalf("DELETE /mcp status = %d, want %d", rec.Code, http.StatusMethodNotAllowed)
	}
}
