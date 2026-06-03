package integration

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	sdkmcp "github.com/modelcontextprotocol/go-sdk/mcp"

	"wowdoc/internal/shared/contracts"
	wowmcp "wowdoc/internal/shared/mcp"
	"wowdoc/internal/shared/tools"
)

func TestBuildTargetsExistAndHelpMentionsAgentFields(t *testing.T) {
	root, err := filepath.Abs(filepath.Join("..", ".."))
	if err != nil {
		t.Fatalf("repo root: %v", err)
	}
	wowdoc := filepath.Join(root, "dist", "wowdoc.exe")
	server := filepath.Join(root, "dist", "wowdoc-server.exe")
	cmd := exec.Command("go", "build", "-o", wowdoc, "./cmd/wowdoc")
	cmd.Dir = root
	if err := cmd.Run(); err != nil {
		t.Fatalf("build wowdoc: %v", err)
	}
	cmd = exec.Command("go", "build", "-o", server, "./cmd/wowdoc-server")
	cmd.Dir = root
	if err := cmd.Run(); err != nil {
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

func TestOmittedTypeScriptToolsAreAbsent(t *testing.T) {
	registry := tools.DefaultRegistry()
	if _, ok := registry.Tools["scaffold_addon"]; ok {
		t.Fatalf("omitted TypeScript tools must stay absent")
	}
	if _, ok := registry.Tools["get_blizzard_addon"]; ok {
		t.Fatalf("omitted TypeScript tools must stay absent")
	}
	if _, ok := registry.Tools["lint_addon_lua"]; ok {
		t.Fatalf("omitted TypeScript tools must stay absent")
	}
}

func TestWowdocStdioCommandServesMCPTools(t *testing.T) {
	root := repoRoot(t)
	wowdoc := filepath.Join(root, "dist", "wowdoc.exe")
	cmd := exec.Command("go", "build", "-o", wowdoc, "./cmd/wowdoc")
	cmd.Dir = root
	if err := cmd.Run(); err != nil {
		t.Fatalf("build wowdoc: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	client := sdkmcp.NewClient(&sdkmcp.Implementation{Name: "integration-client", Version: "v0.0.0-test"}, nil)
	session, err := client.Connect(ctx, &sdkmcp.CommandTransport{
		Command:           exec.Command(wowdoc, "mcp", "stdio"),
		TerminateDuration: time.Second,
	}, nil)
	if err != nil {
		t.Fatalf("connect to wowdoc mcp stdio: %v", err)
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

func TestWowdocStdioCommandUsesSourceRootForToolCalls(t *testing.T) {
	root := repoRoot(t)
	wowdoc := filepath.Join(root, "dist", "wowdoc.exe")
	cmd := exec.Command("go", "build", "-o", wowdoc, "./cmd/wowdoc")
	cmd.Dir = root
	if err := cmd.Run(); err != nil {
		t.Fatalf("build wowdoc: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	client := sdkmcp.NewClient(&sdkmcp.Implementation{Name: "integration-client", Version: "v0.0.0-test"}, nil)
	session, err := client.Connect(ctx, &sdkmcp.CommandTransport{
		Command: exec.Command(
			wowdoc,
			"mcp", "stdio",
			"--source-root", filepath.Join(root, "testdata", "sources"),
		),
		TerminateDuration: time.Second,
	}, nil)
	if err != nil {
		t.Fatalf("connect to wowdoc mcp stdio: %v", err)
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

	lookup, err := session.CallTool(ctx, &sdkmcp.CallToolParams{
		Name: "lookup_blizzard_api",
		Arguments: map[string]any{
			"client":        "valid-retail",
			"name":          "C_AuctionHouse.GetItemSearchResultInfo",
			"includeSafety": false,
		},
	})
	if err != nil {
		t.Fatalf("call lookup_blizzard_api: %v", err)
	}
	if lookup.IsError {
		t.Fatalf("lookup_blizzard_api returned error: %#v", lookup)
	}
	var lookupEnv contracts.Envelope[tools.APIResult]
	mustDecodeStructured(t, lookup.StructuredContent, &lookupEnv)
	if !lookupEnv.OK || lookupEnv.Data.Name != "C_AuctionHouse.GetItemSearchResultInfo" || lookupEnv.Data.Namespace != "C_AuctionHouse" {
		t.Fatalf("lookup_blizzard_api envelope wrong: %#v", lookupEnv)
	}
}

func TestWowdocServerCommandServesStreamableHTTPMCP(t *testing.T) {
	root := repoRoot(t)
	serverPath := filepath.Join(root, "dist", "wowdoc-server.exe")
	cmd := exec.Command("go", "build", "-o", serverPath, "./cmd/wowdoc-server")
	cmd.Dir = root
	if err := cmd.Run(); err != nil {
		t.Fatalf("build wowdoc-server: %v", err)
	}

	port := freeLocalPort(t)
	configPath := filepath.Join(t.TempDir(), "wowdoc.yaml")
	config := fmt.Sprintf(`
server:
  host: 127.0.0.1
  port: %d
sources:
  root: %s
`, port, filepath.ToSlash(filepath.Join(root, "testdata", "sources")))
	if err := os.WriteFile(configPath, []byte(config), 0o600); err != nil {
		t.Fatalf("write config: %v", err)
	}

	serverCmd := exec.Command(serverPath, "mcp", "http")
	serverCmd.Env = append(os.Environ(), "WOWDOC_CONFIG="+configPath)
	if err := serverCmd.Start(); err != nil {
		t.Fatalf("start wowdoc-server: %v", err)
	}
	defer func() {
		_ = serverCmd.Process.Kill()
		_ = serverCmd.Wait()
	}()

	baseURL := fmt.Sprintf("http://127.0.0.1:%d", port)
	waitForHealth(t, baseURL+"/health")
	helpResp, err := http.Get(baseURL + "/help")
	if err != nil {
		t.Fatalf("GET /help: %v", err)
	}
	defer helpResp.Body.Close()
	if helpResp.StatusCode != http.StatusOK {
		t.Fatalf("GET /help status = %d, want %d", helpResp.StatusCode, http.StatusOK)
	}
	var help map[string]string
	if err := json.NewDecoder(helpResp.Body).Decode(&help); err != nil {
		t.Fatalf("decode /help: %v", err)
	}
	if help["mcp"] != "/mcp" || help["health"] != "/health" {
		t.Fatalf("/help body = %#v, want mcp and health routes", help)
	}
	if _, ok := help["cli"]; ok {
		t.Fatalf("/help must not expose CLI routes: %#v", help)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	client := sdkmcp.NewClient(&sdkmcp.Implementation{Name: "http-binary-test-client", Version: "v0.0.0-test"}, nil)
	session, err := client.Connect(ctx, &sdkmcp.StreamableClientTransport{Endpoint: baseURL + "/mcp"}, nil)
	if err != nil {
		t.Fatalf("connect to wowdoc-server /mcp: %v", err)
	}
	defer session.Close()

	listed, err := session.ListTools(ctx, &sdkmcp.ListToolsParams{})
	if err != nil {
		t.Fatalf("list tools over binary http: %v", err)
	}
	if len(listed.Tools) != len(wowmcp.ToolInputSchemas()) {
		t.Fatalf("tool count = %d, want %d", len(listed.Tools), len(wowmcp.ToolInputSchemas()))
	}

	result, err := session.CallTool(ctx, &sdkmcp.CallToolParams{
		Name:      "list_clients",
		Arguments: map[string]any{"includeDiagnostics": true},
	})
	if err != nil {
		t.Fatalf("call list_clients over binary http: %v", err)
	}
	if result.IsError {
		t.Fatalf("binary http list_clients returned error: %#v", result)
	}
	var env contracts.Envelope[tools.ListClientsData]
	mustDecodeStructured(t, result.StructuredContent, &env)
	if !env.OK || len(env.Data.Clients) < 2 || len(env.Diagnostics) != 1 {
		t.Fatalf("binary http list_clients envelope wrong: %#v", env)
	}

	lookup, err := session.CallTool(ctx, &sdkmcp.CallToolParams{
		Name: "lookup_blizzard_api",
		Arguments: map[string]any{
			"client":        "valid-retail",
			"name":          "C_AuctionHouse.GetItemSearchResultInfo",
			"includeSafety": false,
		},
	})
	if err != nil {
		t.Fatalf("call lookup_blizzard_api over binary http: %v", err)
	}
	if lookup.IsError {
		t.Fatalf("binary http lookup_blizzard_api returned error: %#v", lookup)
	}
	var lookupEnv contracts.Envelope[tools.APIResult]
	mustDecodeStructured(t, lookup.StructuredContent, &lookupEnv)
	if !lookupEnv.OK || lookupEnv.Data.Name != "C_AuctionHouse.GetItemSearchResultInfo" || lookupEnv.Data.Namespace != "C_AuctionHouse" {
		t.Fatalf("binary http lookup_blizzard_api envelope wrong: %#v", lookupEnv)
	}
}

func TestWowdocServerBinaryRejectsCLICommands(t *testing.T) {
	root := repoRoot(t)
	serverPath := filepath.Join(root, "dist", "wowdoc-server.exe")
	cmd := exec.Command("go", "build", "-o", serverPath, "./cmd/wowdoc-server")
	cmd.Dir = root
	if err := cmd.Run(); err != nil {
		t.Fatalf("build wowdoc-server: %v", err)
	}

	out, err := exec.Command(serverPath, "clients", "list").CombinedOutput()
	if err == nil {
		t.Fatalf("wowdoc-server should reject CLI command, output: %s", string(out))
	}
	if !strings.Contains(string(out), "unsupported command: clients list") {
		t.Fatalf("unexpected wowdoc-server rejection output: %s", string(out))
	}
}

func repoRoot(t *testing.T) string {
	t.Helper()
	root, err := filepath.Abs(filepath.Join("..", ".."))
	if err != nil {
		t.Fatalf("repo root: %v", err)
	}
	return root
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

func freeLocalPort(t *testing.T) int {
	t.Helper()
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen on free port: %v", err)
	}
	defer listener.Close()
	return listener.Addr().(*net.TCPAddr).Port
}

func waitForHealth(t *testing.T, url string) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		resp, err := http.Get(url)
		if err == nil {
			_ = resp.Body.Close()
			if resp.StatusCode == http.StatusOK {
				return
			}
		}
		time.Sleep(100 * time.Millisecond)
	}
	t.Fatalf("server did not become healthy at %s", url)
}
