package mcp

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	sdkmcp "github.com/modelcontextprotocol/go-sdk/mcp"

	"wowdoc/internal/shared/contracts"
	"wowdoc/internal/shared/tools"
)

func TestNewServerRegistersAllToolSchemas(t *testing.T) {
	server := NewServer(ServerOptions{Name: "wowdoc-test", Version: "v0.0.0-test"})

	schemas := ToolInputSchemas()
	names := server.RegisteredToolNames()
	if len(names) != 15 {
		t.Fatalf("registered tool count = %d, want 15: %#v", len(names), names)
	}
	if len(names) != len(schemas) {
		t.Fatalf("registered tool count = %d, schema count = %d", len(names), len(schemas))
	}
	if server.SDKRegisteredToolCount() != len(schemas) {
		t.Fatalf("sdk registered tool count = %d, schema count = %d", server.SDKRegisteredToolCount(), len(schemas))
	}
	for name := range schemas {
		if !server.HasTool(name) {
			t.Fatalf("server missing schema-backed tool %q", name)
		}
	}
}

func TestServerInspectRemoteRefsToolReturnsConfiguredBranches(t *testing.T) {
	ctx := context.Background()
	session := connectTestClient(t, NewServer(ServerOptions{
		Name: "wowdoc-test",
		SourceRepos: map[string]string{
			"ptr":    "https://example.test/wow-ui-source.git",
			"retail": "https://example.test/wow-ui-source.git",
		},
		DefaultRefs: map[string]string{
			"ptr":    "ptr2",
			"retail": "live",
		},
		Git: remoteRefsMCPGit{
			"live": "9746fb49dbaaf708dbc1110180607154c10b55b7",
			"ptr2": "96443d533fd09a5b6195637f95c896439c616cee",
		},
	}))

	result, err := session.CallTool(ctx, &sdkmcp.CallToolParams{
		Name:      "inspect_remote_refs",
		Arguments: map[string]any{"client": "ptr"},
	})
	if err != nil {
		t.Fatalf("call inspect_remote_refs: %v", err)
	}
	if result.IsError {
		t.Fatalf("inspect_remote_refs returned MCP error: %#v", result)
	}
	var env contracts.Envelope[tools.RemoteRefsData]
	mustDecodeStructured(t, result.StructuredContent, &env)
	if !env.OK || len(env.Data.Clients) != 1 {
		t.Fatalf("inspect_remote_refs envelope wrong: %#v", env)
	}
	if env.Data.Clients[0].Alias != "ptr" || env.Data.Clients[0].ConfiguredRef != "ptr2" || env.Data.Clients[0].Commit != "96443d533fd09a5b6195637f95c896439c616cee" {
		t.Fatalf("ptr remote ref wrong: %#v", env.Data.Clients[0])
	}
}

func TestServerInspectRemoteRefsReportsUnknownClient(t *testing.T) {
	ctx := context.Background()
	session := connectTestClient(t, NewServer(ServerOptions{
		Name:        "wowdoc-test",
		SourceRepos: map[string]string{"retail": "https://example.test/wow-ui-source.git"},
		DefaultRefs: map[string]string{"retail": "live"},
		Git:         remoteRefsMCPGit{"live": "9746fb49dbaaf708dbc1110180607154c10b55b7"},
	}))

	result, err := session.CallTool(ctx, &sdkmcp.CallToolParams{
		Name:      "inspect_remote_refs",
		Arguments: map[string]any{"client": "not-real"},
	})
	if err != nil {
		t.Fatalf("call inspect_remote_refs: %v", err)
	}
	if !result.IsError {
		t.Fatalf("inspect_remote_refs should be an MCP error: %#v", result)
	}
	var env contracts.Envelope[tools.RemoteRefsData]
	mustDecodeStructured(t, result.StructuredContent, &env)
	if env.OK || env.Error == nil || env.Error.Code != contracts.ErrClientNotFound {
		t.Fatalf("unknown client envelope wrong: %#v", env)
	}
}

func TestServerListClientsToolReturnsSourceDiagnostics(t *testing.T) {
	ctx := context.Background()
	session := connectTestClient(t, NewServer(ServerOptions{
		Name:       "wowdoc-test",
		Version:    "v0.0.0-test",
		SourceRoot: filepath.Join("..", "..", "..", "testdata", "sources"),
	}))

	result, err := session.CallTool(ctx, &sdkmcp.CallToolParams{
		Name:      "list_clients",
		Arguments: map[string]any{"includeDiagnostics": true},
	})
	if err != nil {
		t.Fatalf("call list_clients: %v", err)
	}
	if result.IsError {
		t.Fatalf("list_clients returned MCP error: %#v", result)
	}
	var env contracts.Envelope[tools.ListClientsData]
	mustDecodeStructured(t, result.StructuredContent, &env)
	if !env.OK || len(env.Data.Clients) < 2 || len(env.Diagnostics) != 1 {
		t.Fatalf("list_clients envelope wrong: %#v", env)
	}
}

func TestServerListClientsIgnoresInternalSourceCacheDirectories(t *testing.T) {
	root := t.TempDir()
	for _, dir := range []string{"repos", "checkouts", "archives"} {
		if err := os.MkdirAll(filepath.Join(root, dir), 0o755); err != nil {
			t.Fatalf("create internal cache dir: %v", err)
		}
	}
	server := NewServer(ServerOptions{Name: "test", SourceRoot: root})
	req := &sdkmcp.CallToolRequest{Params: &sdkmcp.CallToolParamsRaw{Arguments: json.RawMessage(`{"includeDiagnostics":true}`)}}
	result, err := server.callTool(context.Background(), "list_clients", req)
	if err != nil {
		t.Fatalf("list_clients failed: %v", err)
	}
	var env contracts.Envelope[tools.ListClientsData]
	mustDecodeStructured(t, result.StructuredContent, &env)
	if len(env.Diagnostics) != 0 {
		t.Fatalf("internal cache dirs should not be diagnostics: %#v", env.Diagnostics)
	}
}

func TestServerListClientsCanIncludeDefaultRefs(t *testing.T) {
	ctx := context.Background()
	session := connectTestClient(t, NewServer(ServerOptions{
		Name:       "wowdoc-test",
		Version:    "v0.0.0-test",
		SourceRoot: filepath.Join("..", "..", "..", "testdata", "sources"),
		DefaultRefs: map[string]string{
			"valid-retail": "main",
		},
	}))

	result, err := session.CallTool(ctx, &sdkmcp.CallToolParams{
		Name:      "list_clients",
		Arguments: map[string]any{"includeRefs": true},
	})
	if err != nil {
		t.Fatalf("call list_clients: %v", err)
	}
	if result.IsError {
		t.Fatalf("list_clients returned MCP error: %#v", result)
	}
	var env contracts.Envelope[tools.ListClientsData]
	mustDecodeStructured(t, result.StructuredContent, &env)
	for _, client := range env.Data.Clients {
		if client.Alias == "valid-retail" {
			if client.DefaultRef != "main" {
				t.Fatalf("default ref missing from client summary: %#v", client)
			}
			return
		}
	}
	t.Fatalf("valid-retail missing from list_clients: %#v", env.Data.Clients)
}

func TestServerLookupBlizzardAPIToolReturnsEnvelope(t *testing.T) {
	ctx := context.Background()
	session := connectTestClient(t, NewServer(ServerOptions{
		Name:       "wowdoc-test",
		Version:    "v0.0.0-test",
		SourceRoot: filepath.Join("..", "..", "..", "testdata", "sources"),
	}))

	result, err := session.CallTool(ctx, &sdkmcp.CallToolParams{
		Name: "lookup_blizzard_api",
		Arguments: map[string]any{
			"client": "valid-retail",
			"name":   "C_AuctionHouse",
		},
	})
	if err != nil {
		t.Fatalf("call lookup_blizzard_api: %v", err)
	}
	if result.IsError {
		t.Fatalf("lookup_blizzard_api returned MCP error: %#v", result)
	}
	var env contracts.Envelope[tools.APIResult]
	mustDecodeStructured(t, result.StructuredContent, &env)
	if !env.OK || env.Data.Name != "C_AuctionHouse" || env.Source.Client != "valid-retail" {
		t.Fatalf("lookup_blizzard_api envelope wrong: %#v", env)
	}
}

func TestServerToolResultPreservesTextAndStructuredContent(t *testing.T) {
	ctx := context.Background()
	session := connectTestClient(t, NewServer(ServerOptions{
		Name:       "wowdoc-test",
		Version:    "v0.0.0-test",
		SourceRoot: filepath.Join("..", "..", "..", "testdata", "sources"),
	}))

	result, err := session.CallTool(ctx, &sdkmcp.CallToolParams{
		Name:      "lookup_blizzard_api",
		Arguments: map[string]any{"client": "valid-retail", "name": "C_AuctionHouse"},
	})
	if err != nil {
		t.Fatalf("call lookup_blizzard_api: %v", err)
	}
	if len(result.Content) != 1 {
		t.Fatalf("tool result text content missing: %#v", result.Content)
	}
	text, ok := result.Content[0].(*sdkmcp.TextContent)
	if !ok || !strings.Contains(text.Text, `"name":"C_AuctionHouse"`) {
		t.Fatalf("tool result text content wrong: %#v", result.Content)
	}
	var env contracts.Envelope[tools.APIResult]
	mustDecodeStructured(t, result.StructuredContent, &env)
	if !env.OK || env.Data.Name != "C_AuctionHouse" {
		t.Fatalf("tool result structured content wrong: %#v", env)
	}
}

func TestInitializedNotificationDoesNotProduceUserVisibleError(t *testing.T) {
	ctx := context.Background()
	session := connectTestClient(t, NewServer(ServerOptions{
		Name:       "wowdoc-test",
		Version:    "v0.0.0-test",
		SourceRoot: filepath.Join("..", "..", "..", "testdata", "sources"),
	}))

	result, err := session.CallTool(ctx, &sdkmcp.CallToolParams{
		Name:      "list_clients",
		Arguments: map[string]any{"includeDiagnostics": true},
	})
	if err != nil {
		t.Fatalf("call after initialized notification should not fail: %v", err)
	}
	if result.IsError {
		t.Fatalf("call after initialized notification returned user-visible error: %#v", result)
	}
	var env contracts.Envelope[tools.ListClientsData]
	mustDecodeStructured(t, result.StructuredContent, &env)
	if !env.OK {
		t.Fatalf("list_clients envelope after initialized notification should be ok: %#v", env)
	}
}

func TestServerLookupBlizzardAPIHonorsExactAndIncludeSafety(t *testing.T) {
	ctx := context.Background()
	session := connectTestClient(t, NewServer(ServerOptions{
		Name:       "wowdoc-test",
		Version:    "v0.0.0-test",
		SourceRoot: filepath.Join("..", "..", "..", "testdata", "sources"),
	}))
	result, err := session.CallTool(ctx, &sdkmcp.CallToolParams{
		Name: "lookup_blizzard_api",
		Arguments: map[string]any{
			"client":        "valid-retail",
			"name":          "GetItemSearchResultInfo",
			"includeSafety": false,
		},
	})
	if err != nil {
		t.Fatalf("call lookup_blizzard_api: %v", err)
	}
	if result.IsError {
		t.Fatalf("lookup_blizzard_api returned MCP error: %#v", result)
	}
	var env contracts.Envelope[tools.APIResult]
	mustDecodeStructured(t, result.StructuredContent, &env)
	if !env.OK || env.Data.Name != "C_AuctionHouse.GetItemSearchResultInfo" {
		t.Fatalf("lookup envelope wrong: %#v", env)
	}
	if env.Data.Safety != nil {
		t.Fatalf("includeSafety=false should omit safety: %#v", env.Data.Safety)
	}
}

func TestServerLookupBlizzardAPIExactTrueRequiresExactName(t *testing.T) {
	ctx := context.Background()
	session := connectTestClient(t, NewServer(ServerOptions{
		Name:       "wowdoc-test",
		Version:    "v0.0.0-test",
		SourceRoot: filepath.Join("..", "..", "..", "testdata", "sources"),
	}))
	result, err := session.CallTool(ctx, &sdkmcp.CallToolParams{
		Name: "lookup_blizzard_api",
		Arguments: map[string]any{
			"client": "valid-retail",
			"name":   "GetItemSearchResultInfo",
			"exact":  true,
		},
	})
	if err != nil {
		t.Fatalf("call lookup_blizzard_api: %v", err)
	}
	if result.IsError {
		t.Fatalf("lookup_blizzard_api returned MCP error: %#v", result)
	}
	var env contracts.Envelope[tools.APIResult]
	mustDecodeStructured(t, result.StructuredContent, &env)
	if !env.OK || env.Data.Name != "GetItemSearchResultInfo" || env.Data.Type != "" {
		t.Fatalf("exact=true should not fuzzy-match partial names: %#v", env)
	}
}

func TestServerSourceBackedToolsRejectMissingPrimaryInputs(t *testing.T) {
	server := NewServer(ServerOptions{
		Name:       "wowdoc-test",
		Version:    "v0.0.0-test",
		SourceRoot: filepath.Join("..", "..", "..", "testdata", "sources"),
	})
	cases := []struct {
		tool string
		args string
	}{
		{tool: "search_blizzard_api", args: `{"client":"valid-retail"}`},
		{tool: "get_api_namespace", args: `{"client":"valid-retail"}`},
		{tool: "get_api_events", args: `{"client":"valid-retail"}`},
		{tool: "search_framexml", args: `{"client":"partial-classic"}`},
		{tool: "find_mixin_template", args: `{"client":"partial-classic"}`},
		{tool: "get_wow_constants", args: `{"client":"valid-retail-constants"}`},
		{tool: "check_api_deprecation", args: `{"client":"valid-retail"}`},
		{tool: "suggest_api_migration", args: `{"client":"valid-retail"}`},
		{tool: "get_widget_api", args: `{"client":"valid-retail"}`},
		{tool: "lookup_cvar", args: `{"client":"valid-retail"}`},
		{tool: "explain_api_safety", args: `{"client":"valid-retail"}`},
	}
	for _, tc := range cases {
		req := &sdkmcp.CallToolRequest{Params: &sdkmcp.CallToolParamsRaw{Arguments: json.RawMessage(tc.args)}}
		result, err := server.callTool(context.Background(), tc.tool, req)
		if err != nil {
			t.Fatalf("%s call failed: %v", tc.tool, err)
		}
		if !result.IsError {
			t.Fatalf("%s should reject missing primary input: %#v", tc.tool, result)
		}
		var env contracts.Envelope[any]
		mustDecodeStructured(t, result.StructuredContent, &env)
		if env.Error == nil || env.Error.Code != contracts.ErrIndexUnavailable {
			t.Fatalf("%s error envelope = %#v, want index_unavailable", tc.tool, env)
		}
	}
}

func TestServerSourceBackedToolsPreferClientRequiredForEmptyArguments(t *testing.T) {
	server := NewServer(ServerOptions{
		Name:       "wowdoc-test",
		Version:    "v0.0.0-test",
		SourceRoot: filepath.Join("..", "..", "..", "testdata", "sources"),
	})
	for _, tool := range []string{
		"search_blizzard_api",
		"get_api_namespace",
		"get_api_events",
		"search_framexml",
		"find_mixin_template",
		"get_wow_constants",
		"check_api_deprecation",
		"suggest_api_migration",
		"get_widget_api",
		"lookup_cvar",
		"explain_api_safety",
	} {
		req := &sdkmcp.CallToolRequest{Params: &sdkmcp.CallToolParamsRaw{Arguments: json.RawMessage(`{}`)}}
		result, err := server.callTool(context.Background(), tool, req)
		if err != nil {
			t.Fatalf("%s call failed: %v", tool, err)
		}
		if !result.IsError {
			t.Fatalf("%s should reject empty arguments: %#v", tool, result)
		}
		var env contracts.Envelope[any]
		mustDecodeStructured(t, result.StructuredContent, &env)
		if env.Error == nil || env.Error.Code != contracts.ErrClientRequired {
			t.Fatalf("%s error envelope = %#v, want client_required", tool, env)
		}
	}
}

func TestServerAcquiresConfiguredRepoRefWithSourceManager(t *testing.T) {
	ctx := context.Background()
	root := t.TempDir()
	git := &fixtureGit{fixture: filepath.Join("..", "..", "..", "testdata", "sources", "valid-retail"), resolvedCommit: "abc123def456"}
	server := NewServer(ServerOptions{
		Name:       "wowdoc-test",
		Version:    "v0.0.0-test",
		SourceRoot: root,
		SourceRepos: map[string]string{
			"retail": "https://example.test/wow-ui-source.git",
		},
		DefaultRefs: map[string]string{
			"retail": "main",
		},
		Git: git,
	})
	session := connectTestClient(t, server)

	result, err := session.CallTool(ctx, &sdkmcp.CallToolParams{
		Name:      "lookup_blizzard_api",
		Arguments: map[string]any{"client": "retail", "name": "C_AuctionHouse"},
	})
	if err != nil {
		t.Fatalf("call lookup_blizzard_api: %v", err)
	}
	if result.IsError {
		t.Fatalf("lookup_blizzard_api returned MCP error: %#v", result)
	}
	var env contracts.Envelope[tools.APIResult]
	mustDecodeStructured(t, result.StructuredContent, &env)
	if !env.OK || env.Source.RequestedRef != "main" || env.Source.ResolvedRef != "abc123def456" || env.Source.Path != filepath.Join(root, "checkouts", "retail", "abc123def456") {
		t.Fatalf("lookup envelope wrong: %#v", env)
	}
}

func TestServerReportsArchiveFallbackDiagnostics(t *testing.T) {
	ctx := context.Background()
	root := t.TempDir()
	server := NewServer(ServerOptions{
		Name:       "wowdoc-test",
		Version:    "v0.0.0-test",
		SourceRoot: root,
		SourceRepos: map[string]string{
			"retail": "https://example.test/wow-ui-source.git",
		},
		DefaultRefs: map[string]string{
			"retail": "main",
		},
		Git:     failingGit{},
		Archive: &fixtureArchive{fixture: filepath.Join("..", "..", "..", "testdata", "sources", "valid-retail")},
	})
	session := connectTestClient(t, server)

	result, err := session.CallTool(ctx, &sdkmcp.CallToolParams{
		Name:      "lookup_blizzard_api",
		Arguments: map[string]any{"client": "retail", "name": "C_AuctionHouse"},
	})
	if err != nil {
		t.Fatalf("call lookup_blizzard_api: %v", err)
	}
	if result.IsError {
		t.Fatalf("lookup_blizzard_api returned MCP error: %#v", result)
	}
	var env contracts.Envelope[tools.APIResult]
	mustDecodeStructured(t, result.StructuredContent, &env)
	if !env.OK || env.Source.Path != filepath.Join(root, "archives", "retail", "main") {
		t.Fatalf("lookup envelope wrong: %#v", env)
	}
	if !containsDiagnostic(env.Diagnostics, "archive fallback is not incremental") ||
		!containsDiagnostic(env.Diagnostics, "branch archives may need periodic redownload") ||
		!containsDiagnostic(env.Diagnostics, "resolved commit may be unknown for archive fallback") {
		t.Fatalf("archive fallback limitations missing from diagnostics: %#v", env.Diagnostics)
	}
}

func TestServerSourceBackedToolsReturnRealEnvelopes(t *testing.T) {
	ctx := context.Background()
	session := connectTestClient(t, NewServer(ServerOptions{
		Name:       "wowdoc-test",
		Version:    "v0.0.0-test",
		SourceRoot: filepath.Join("..", "..", "..", "testdata", "sources"),
	}))

	cases := []struct {
		name      string
		arguments map[string]any
		wantName  string
	}{
		{
			name:      "search_blizzard_api",
			arguments: map[string]any{"client": "valid-retail", "query": "Auction"},
			wantName:  "C_AuctionHouse",
		},
		{
			name:      "get_api_namespace",
			arguments: map[string]any{"client": "valid-retail", "namespace": "C_AuctionHouse"},
			wantName:  "C_AuctionHouse",
		},
		{
			name:      "search_framexml",
			arguments: map[string]any{"client": "partial-classic", "query": "SecureActionButtonTemplate", "maxResults": 5},
			wantName:  "SecureActionButtonTemplate",
		},
		{
			name:      "find_mixin_template",
			arguments: map[string]any{"client": "partial-classic", "name": "SecureActionButtonTemplate", "limit": 5},
			wantName:  "SecureActionButtonTemplate",
		},
		{
			name:      "get_wow_constants",
			arguments: map[string]any{"client": "valid-retail-constants", "name": "Enum.ItemQuality"},
			wantName:  "Enum.ItemQuality",
		},
		{
			name:      "explain_api_safety",
			arguments: map[string]any{"client": "valid-retail", "symbol": "C_AuctionHouse", "scenario": "combat"},
			wantName:  "combat",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			result, err := session.CallTool(ctx, &sdkmcp.CallToolParams{Name: tc.name, Arguments: tc.arguments})
			if err != nil {
				t.Fatalf("call %s: %v", tc.name, err)
			}
			if result.IsError {
				t.Fatalf("%s returned MCP error: %#v", tc.name, result)
			}
			data, err := json.Marshal(result.StructuredContent)
			if err != nil {
				t.Fatalf("marshal structured content: %v", err)
			}
			if !containsJSONText(data, tc.wantName) {
				t.Fatalf("%s structured content missing %q:\n%s", tc.name, tc.wantName, string(data))
			}
		})
	}
}

func TestServerSearchBlizzardAPIHonorsFilters(t *testing.T) {
	ctx := context.Background()
	session := connectTestClient(t, NewServer(ServerOptions{
		Name:       "wowdoc-test",
		Version:    "v0.0.0-test",
		SourceRoot: filepath.Join("..", "..", "..", "testdata", "sources"),
	}))
	result, err := session.CallTool(ctx, &sdkmcp.CallToolParams{
		Name: "search_blizzard_api",
		Arguments: map[string]any{
			"client": "valid-retail",
			"query":  "Auction",
			"type":   "Event",
			"limit":  1,
		},
	})
	if err != nil {
		t.Fatalf("call search_blizzard_api: %v", err)
	}
	if result.IsError {
		t.Fatalf("search_blizzard_api returned MCP error: %#v", result)
	}
	var env contracts.Envelope[tools.APISearchData]
	mustDecodeStructured(t, result.StructuredContent, &env)
	if !env.OK || len(env.Data.Results) != 1 || env.Data.Results[0].Type != "Event" {
		t.Fatalf("filtered search envelope wrong: %#v", env)
	}
}

func TestServerSearchFrameXMLHonorsContextOptions(t *testing.T) {
	ctx := context.Background()
	session := connectTestClient(t, NewServer(ServerOptions{
		Name:       "wowdoc-test",
		Version:    "v0.0.0-test",
		SourceRoot: filepath.Join("..", "..", "..", "testdata", "sources"),
	}))
	result, err := session.CallTool(ctx, &sdkmcp.CallToolParams{
		Name: "search_framexml",
		Arguments: map[string]any{
			"client":       "partial-classic",
			"query":        "SecureActionButtonTemplate",
			"filePattern":  "SecureTemplates.lua",
			"contextLines": 1,
			"maxResults":   1,
		},
	})
	if err != nil {
		t.Fatalf("call search_framexml: %v", err)
	}
	if result.IsError {
		t.Fatalf("search_framexml returned MCP error: %#v", result)
	}
	var env contracts.Envelope[tools.FrameXMLSearchData]
	mustDecodeStructured(t, result.StructuredContent, &env)
	if !env.OK || len(env.Data.Results) != 1 || len(env.Data.Results[0].Before) != 1 || len(env.Data.Results[0].After) != 1 {
		t.Fatalf("framexml search envelope wrong: %#v", env)
	}
}

func TestServerGetWowConstantsHonorsKindAndLimit(t *testing.T) {
	ctx := context.Background()
	session := connectTestClient(t, NewServer(ServerOptions{
		Name:       "wowdoc-test",
		Version:    "v0.0.0-test",
		SourceRoot: filepath.Join("..", "..", "..", "testdata", "sources"),
	}))
	result, err := session.CallTool(ctx, &sdkmcp.CallToolParams{
		Name: "get_wow_constants",
		Arguments: map[string]any{
			"client": "valid-retail-constants",
			"name":   "Enum.ItemQuality",
			"kind":   "Enumeration",
			"limit":  1,
		},
	})
	if err != nil {
		t.Fatalf("call get_wow_constants: %v", err)
	}
	if result.IsError {
		t.Fatalf("get_wow_constants returned MCP error: %#v", result)
	}
	var env contracts.Envelope[tools.ConstantsData]
	mustDecodeStructured(t, result.StructuredContent, &env)
	if !env.OK || len(env.Data.Results) != 1 || env.Data.Results[0].Type != "Enumeration" {
		t.Fatalf("constants envelope wrong: %#v", env)
	}
	if len(env.Data.Results[0].Fields) == 0 || len(env.Data.Results[0].Values) == 0 {
		t.Fatalf("constants envelope should include fields/values: %#v", env.Data.Results[0])
	}
}

func TestServerRemainingToolsReturnStructuredEnvelopes(t *testing.T) {
	ctx := context.Background()
	session := connectTestClient(t, NewServer(ServerOptions{
		Name:       "wowdoc-test",
		Version:    "v0.0.0-test",
		SourceRoot: filepath.Join("..", "..", "..", "testdata", "sources"),
	}))

	cases := []struct {
		name      string
		arguments map[string]any
		wantText  string
	}{
		{
			name:      "get_api_events",
			arguments: map[string]any{"client": "valid-retail", "event": "list"},
			wantText:  `"ok":true`,
		},
		{
			name:      "check_api_deprecation",
			arguments: map[string]any{"client": "valid-retail", "luaCode": "GetContainerItemInfo()"},
			wantText:  "GetContainerItemInfo",
		},
		{
			name:      "suggest_api_migration",
			arguments: map[string]any{"client": "valid-retail", "oldFunction": "GetContainerItemInfo"},
			wantText:  "C_Container.GetContainerItemInfo",
		},
		{
			name:      "get_widget_api",
			arguments: map[string]any{"client": "valid-retail", "widgetType": "Button"},
			wantText:  "Button",
		},
		{
			name:      "lookup_cvar",
			arguments: map[string]any{"client": "valid-retail", "name": "graphics"},
			wantText:  "graphicsQuality",
		},
		{
			name:      "validate_toc",
			arguments: map[string]any{"tocContent": "## Interface: 120000\n## Title: My Addon\n", "addonName": "MyAddon"},
			wantText:  "MyAddon",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			result, err := session.CallTool(ctx, &sdkmcp.CallToolParams{Name: tc.name, Arguments: tc.arguments})
			if err != nil {
				t.Fatalf("call %s: %v", tc.name, err)
			}
			data, err := json.Marshal(result.StructuredContent)
			if err != nil {
				t.Fatalf("marshal structured content: %v", err)
			}
			if !containsJSONText(data, tc.wantText) {
				t.Fatalf("%s structured content missing %q:\n%s", tc.name, tc.wantText, string(data))
			}
			if strings.Contains(string(data), "tool handler is not implemented") {
				t.Fatalf("%s still returned unimplemented handler:\n%s", tc.name, string(data))
			}
		})
	}
}

func TestServerValidateTOCCanUseSourceVersion(t *testing.T) {
	ctx := context.Background()
	session := connectTestClient(t, NewServer(ServerOptions{
		Name:       "wowdoc-test",
		Version:    "v0.0.0-test",
		SourceRoot: filepath.Join("..", "..", "..", "testdata", "sources"),
	}))

	result, err := session.CallTool(ctx, &sdkmcp.CallToolParams{
		Name: "validate_toc",
		Arguments: map[string]any{
			"client":     "valid-retail",
			"tocContent": "## Interface: 11507\n## Title: My Addon\n",
		},
	})
	if err != nil {
		t.Fatalf("call validate_toc: %v", err)
	}
	if result.IsError {
		t.Fatalf("validate_toc returned error: %#v", result)
	}
	var env contracts.Envelope[tools.TOCValidationData]
	mustDecodeStructured(t, result.StructuredContent, &env)
	if !env.OK || !containsText(env.Data.Warnings, "12.0.0.60000") {
		t.Fatalf("source-aware toc warning missing: %#v", env)
	}
}

func TestServerExplainAPISafetyUsesIndexedMetadata(t *testing.T) {
	ctx := context.Background()
	session := connectTestClient(t, NewServer(ServerOptions{
		Name:       "wowdoc-test",
		Version:    "v0.0.0-test",
		SourceRoot: filepath.Join("..", "..", "..", "testdata", "sources"),
	}))

	result, err := session.CallTool(ctx, &sdkmcp.CallToolParams{
		Name:      "explain_api_safety",
		Arguments: map[string]any{"client": "valid-retail", "symbol": "C_AuctionHouse.GetItemSearchResultInfo", "scenario": "unit_cast"},
	})
	if err != nil {
		t.Fatalf("call explain_api_safety: %v", err)
	}
	if result.IsError {
		t.Fatalf("explain_api_safety returned error: %#v", result)
	}
	data, err := json.Marshal(result.StructuredContent)
	if err != nil {
		t.Fatalf("marshal structured content: %v", err)
	}
	if !containsJSONText(data, `"effectiveLevel":"protected"`) || !containsJSONText(data, "SecretWhenUnitSpellCastRestricted") {
		t.Fatalf("safety structured content missing indexed metadata:\n%s", string(data))
	}
}

func TestServerGetAPIEventsReturnsPayloadDetails(t *testing.T) {
	ctx := context.Background()
	session := connectTestClient(t, NewServer(ServerOptions{
		Name:       "wowdoc-test",
		Version:    "v0.0.0-test",
		SourceRoot: filepath.Join("..", "..", "..", "testdata", "sources"),
	}))

	result, err := session.CallTool(ctx, &sdkmcp.CallToolParams{
		Name:      "get_api_events",
		Arguments: map[string]any{"client": "valid-retail", "event": "AUCTION_HOUSE_SHOW"},
	})
	if err != nil {
		t.Fatalf("call get_api_events: %v", err)
	}
	if result.IsError {
		t.Fatalf("get_api_events returned error: %#v", result)
	}
	data, err := json.Marshal(result.StructuredContent)
	if err != nil {
		t.Fatalf("marshal structured content: %v", err)
	}
	if !containsJSONText(data, "AUCTION_HOUSE_SHOW(auctionHouseID, isAethereal)") || !containsJSONText(data, "isAethereal") {
		t.Fatalf("event structured content missing payload details:\n%s", string(data))
	}
}

func TestServerGetWidgetAPIReturnsWidgetMethods(t *testing.T) {
	ctx := context.Background()
	session := connectTestClient(t, NewServer(ServerOptions{
		Name:       "wowdoc-test",
		Version:    "v0.0.0-test",
		SourceRoot: filepath.Join("..", "..", "..", "testdata", "sources"),
	}))

	result, err := session.CallTool(ctx, &sdkmcp.CallToolParams{
		Name:      "get_widget_api",
		Arguments: map[string]any{"client": "valid-retail", "widgetType": "Button"},
	})
	if err != nil {
		t.Fatalf("call get_widget_api: %v", err)
	}
	if result.IsError {
		t.Fatalf("get_widget_api returned error: %#v", result)
	}
	data, err := json.Marshal(result.StructuredContent)
	if err != nil {
		t.Fatalf("marshal structured content: %v", err)
	}
	if !containsJSONText(data, "Button.SetText(text, isFormatted)") || !containsJSONText(data, "isFormatted") {
		t.Fatalf("widget structured content missing method details:\n%s", string(data))
	}
}

func TestServerLookupCVarReturnsDetails(t *testing.T) {
	ctx := context.Background()
	session := connectTestClient(t, NewServer(ServerOptions{
		Name:       "wowdoc-test",
		Version:    "v0.0.0-test",
		SourceRoot: filepath.Join("..", "..", "..", "testdata", "sources"),
	}))

	result, err := session.CallTool(ctx, &sdkmcp.CallToolParams{
		Name:      "lookup_cvar",
		Arguments: map[string]any{"client": "valid-retail", "name": "graphicsQuality", "detail": true},
	})
	if err != nil {
		t.Fatalf("call lookup_cvar: %v", err)
	}
	if result.IsError {
		t.Fatalf("lookup_cvar returned error: %#v", result)
	}
	data, err := json.Marshal(result.StructuredContent)
	if err != nil {
		t.Fatalf("marshal structured content: %v", err)
	}
	if !containsJSONText(data, "graphicsQuality") || !containsJSONText(data, `"defaultValue":"5"`) {
		t.Fatalf("cvar structured content missing details:\n%s", string(data))
	}
}

func TestServerLookupCVarOmitsDetailsByDefault(t *testing.T) {
	ctx := context.Background()
	session := connectTestClient(t, NewServer(ServerOptions{
		Name:       "wowdoc-test",
		Version:    "v0.0.0-test",
		SourceRoot: filepath.Join("..", "..", "..", "testdata", "sources"),
	}))
	result, err := session.CallTool(ctx, &sdkmcp.CallToolParams{
		Name:      "lookup_cvar",
		Arguments: map[string]any{"client": "valid-retail", "name": "graphicsQuality"},
	})
	if err != nil {
		t.Fatalf("call lookup_cvar: %v", err)
	}
	if result.IsError {
		t.Fatalf("lookup_cvar returned error: %#v", result)
	}
	var env contracts.Envelope[tools.CVarData]
	mustDecodeStructured(t, result.StructuredContent, &env)
	if !env.OK || len(env.Data.Results) != 1 {
		t.Fatalf("cvar envelope wrong: %#v", env)
	}
	if env.Data.Results[0].DefaultValue != "5" || env.Data.Results[0].Description != "" || env.Data.Results[0].References == 0 {
		t.Fatalf("summary should include default/reference details but omit description: %#v", env.Data.Results[0])
	}
}

func TestServerReusesCachedIndexAcrossToolCalls(t *testing.T) {
	ctx := context.Background()
	server := NewServer(ServerOptions{
		Name:       "wowdoc-test",
		Version:    "v0.0.0-test",
		SourceRoot: filepath.Join("..", "..", "..", "testdata", "sources"),
	})
	session := connectTestClient(t, server)

	for _, call := range []sdkmcp.CallToolParams{
		{Name: "lookup_blizzard_api", Arguments: map[string]any{"client": "valid-retail", "name": "C_AuctionHouse"}},
		{Name: "search_blizzard_api", Arguments: map[string]any{"client": "valid-retail", "query": "Auction"}},
		{Name: "get_api_namespace", Arguments: map[string]any{"client": "valid-retail", "namespace": "C_AuctionHouse"}},
	} {
		result, err := session.CallTool(ctx, &call)
		if err != nil {
			t.Fatalf("call %s: %v", call.Name, err)
		}
		if result.IsError {
			t.Fatalf("%s returned error: %#v", call.Name, result)
		}
	}
	if count := server.CachedIndexCount(); count != 1 {
		t.Fatalf("cached index count = %d, want 1", count)
	}
}

func TestServerCachesIndexesSeparatelyByClientAndRef(t *testing.T) {
	ctx := context.Background()
	server := NewServer(ServerOptions{
		Name:       "wowdoc-test",
		Version:    "v0.0.0-test",
		SourceRoot: filepath.Join("..", "..", "..", "testdata", "sources"),
	})
	session := connectTestClient(t, server)

	for _, call := range []sdkmcp.CallToolParams{
		{Name: "search_blizzard_api", Arguments: map[string]any{"client": "valid-retail", "query": "Auction"}},
		{Name: "search_blizzard_api", Arguments: map[string]any{"client": "valid-retail-constants", "query": "Auction"}},
		{Name: "search_blizzard_api", Arguments: map[string]any{"client": "valid-retail", "ref": "old-build", "query": "Auction"}},
	} {
		_, err := session.CallTool(ctx, &call)
		if err != nil {
			t.Fatalf("call %s: %v", call.Name, err)
		}
	}
	if count := server.CachedIndexCount(); count != 3 {
		t.Fatalf("cached index count = %d, want 3", count)
	}
}

func TestServerCachesIndexesByResolvedCommit(t *testing.T) {
	ctx := context.Background()
	server := NewServer(ServerOptions{
		Name:       "wowdoc-test",
		Version:    "v0.0.0-test",
		SourceRoot: t.TempDir(),
		SourceRepos: map[string]string{
			"retail": "https://example.test/wow-ui-source.git",
		},
		DefaultRefs: map[string]string{
			"retail": "main",
		},
		Git: &fixtureGit{fixture: filepath.Join("..", "..", "..", "testdata", "sources", "valid-retail"), resolvedCommit: "abc123def456"},
	})
	session := connectTestClient(t, server)

	for _, call := range []sdkmcp.CallToolParams{
		{Name: "lookup_blizzard_api", Arguments: map[string]any{"client": "retail", "name": "C_AuctionHouse"}},
		{Name: "lookup_blizzard_api", Arguments: map[string]any{"client": "retail", "ref": "main", "name": "C_AuctionHouse"}},
	} {
		result, err := session.CallTool(ctx, &call)
		if err != nil {
			t.Fatalf("call %s: %v", call.Name, err)
		}
		if result.IsError {
			t.Fatalf("%s returned error: %#v", call.Name, result)
		}
	}
	if count := server.CachedIndexCount(); count != 1 {
		t.Fatalf("cached index count = %d, want 1 for same resolved commit", count)
	}
}

func connectTestClient(t *testing.T, server *Server) *sdkmcp.ClientSession {
	t.Helper()
	ctx := context.Background()
	serverTransport, clientTransport := sdkmcp.NewInMemoryTransports()
	serverSession, err := server.SDKServer().Connect(ctx, serverTransport, nil)
	if err != nil {
		t.Fatalf("connect server: %v", err)
	}
	t.Cleanup(func() { _ = serverSession.Close() })

	client := sdkmcp.NewClient(&sdkmcp.Implementation{Name: "test-client", Version: "v0.0.0-test"}, nil)
	clientSession, err := client.Connect(ctx, clientTransport, nil)
	if err != nil {
		t.Fatalf("connect client: %v", err)
	}
	t.Cleanup(func() { _ = clientSession.Close() })
	return clientSession
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

func containsJSONText(data []byte, text string) bool {
	return len(text) == 0 || json.Valid(data) && strings.Contains(string(data), text)
}

func containsText(values []string, needle string) bool {
	for _, value := range values {
		if strings.Contains(value, needle) {
			return true
		}
	}
	return false
}

func containsDiagnostic(values []contracts.Diagnostic, needle string) bool {
	for _, value := range values {
		if strings.Contains(value.Message, needle) {
			return true
		}
	}
	return false
}

type fixtureGit struct {
	fixture        string
	resolvedCommit string
}

type failingGit struct{}

func (failingGit) Run(args ...string) error { return os.ErrNotExist }
func (failingGit) Output(args ...string) ([]byte, error) {
	return nil, os.ErrNotExist
}

type fixtureArchive struct {
	fixture string
}

func (a *fixtureArchive) FetchArchive(repoURL, ref, destination string) error {
	return copyDirErr(a.fixture, destination)
}

func (g *fixtureGit) Run(args ...string) error {
	if len(args) == 4 && args[0] == "clone" && args[1] == "--mirror" {
		return os.MkdirAll(args[3], 0o755)
	}
	if len(args) == 7 && args[2] == "worktree" && args[3] == "add" {
		return copyDirErr(g.fixture, args[5])
	}
	return nil
}

func (g *fixtureGit) Output(args ...string) ([]byte, error) {
	if g.resolvedCommit != "" {
		return []byte(g.resolvedCommit + "\n"), nil
	}
	return []byte("main\n"), nil
}

type remoteRefsMCPGit map[string]string

func (g remoteRefsMCPGit) Run(args ...string) error { return nil }

func (g remoteRefsMCPGit) Output(args ...string) ([]byte, error) {
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
