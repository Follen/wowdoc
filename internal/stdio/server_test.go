package stdio

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	sdkmcp "github.com/modelcontextprotocol/go-sdk/mcp"

	wowmcp "wowdoc/internal/shared/mcp"
)

func TestDefaultSourceRootUsesExecutableDirectorySources(t *testing.T) {
	root, err := DefaultSourceRoot()
	if err != nil {
		t.Fatalf("DefaultSourceRoot returned error: %v", err)
	}

	exe, err := os.Executable()
	if err != nil {
		t.Fatalf("os.Executable returned error: %v", err)
	}
	want := filepath.Join(filepath.Dir(exe), "sources")
	if root != want {
		t.Fatalf("DefaultSourceRoot = %q, want %q", root, want)
	}
}

func TestDefaultServerOptionsIncludeDefaultSourceSeeds(t *testing.T) {
	opts := DefaultServerOptions("wowdoc-test", "sources")
	if opts.Name != "wowdoc-test" || opts.SourceRoot != "sources" {
		t.Fatalf("basic options wrong: %#v", opts)
	}
	for _, alias := range []string{"retail", "classic", "classic-ptr", "classic-titan", "ptr2"} {
		if opts.SourceRepos[alias] == "" || opts.DefaultRefs[alias] == "" {
			t.Fatalf("default seed %q missing from stdio options: %#v %#v", alias, opts.SourceRepos, opts.DefaultRefs)
		}
	}
	if opts.Git == nil || opts.Archive == nil {
		t.Fatalf("stdio default options must include git and archive fetchers")
	}
}

func TestRunServesToolsOverMCPTransport(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	serverTransport, clientTransport := sdkmcp.NewInMemoryTransports()
	errc := make(chan error, 1)
	go func() {
		errc <- Run(ctx, wowmcp.ServerOptions{Name: "wowdoc-test", Version: "v0.0.0-test"}, serverTransport)
	}()

	client := sdkmcp.NewClient(&sdkmcp.Implementation{Name: "test-client", Version: "v0.0.0-test"}, nil)
	session, err := client.Connect(ctx, clientTransport, nil)
	if err != nil {
		t.Fatalf("client connect: %v", err)
	}
	defer session.Close()

	tools, err := session.ListTools(ctx, &sdkmcp.ListToolsParams{})
	if err != nil {
		t.Fatalf("list tools: %v", err)
	}
	if len(tools.Tools) != len(wowmcp.ToolInputSchemas()) {
		t.Fatalf("tool count = %d, want %d", len(tools.Tools), len(wowmcp.ToolInputSchemas()))
	}

	if err := session.Close(); err != nil {
		t.Fatalf("close session: %v", err)
	}
	select {
	case err := <-errc:
		if err != nil {
			t.Fatalf("server returned error: %v", err)
		}
	case <-ctx.Done():
		t.Fatalf("server did not stop after client close: %v", ctx.Err())
	}
}
