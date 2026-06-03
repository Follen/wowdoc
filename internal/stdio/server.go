package stdio

import (
	"context"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"

	sdkmcp "github.com/modelcontextprotocol/go-sdk/mcp"

	"wowdoc/internal/shared/config"
	wowmcp "wowdoc/internal/shared/mcp"
	"wowdoc/internal/shared/source"
)

func DefaultSourceRoot() (string, error) {
	exe, err := os.Executable()
	if err != nil {
		return "", err
	}
	return filepath.Join(filepath.Dir(exe), "sources"), nil
}

func DefaultServerOptions(name, sourceRoot string) wowmcp.ServerOptions {
	repos := map[string]string{}
	refs := map[string]string{}
	for _, seed := range config.DefaultSourceSeeds() {
		repos[seed.Alias] = seed.Repo
		refs[seed.Alias] = seed.Ref
	}
	return wowmcp.ServerOptions{
		Name:        name,
		SourceRoot:  sourceRoot,
		SourceRepos: repos,
		DefaultRefs: refs,
		Git:         execGit{},
		Archive:     source.NewHTTPArchiveFetcher(http.DefaultClient),
	}
}

func Run(ctx context.Context, options wowmcp.ServerOptions, transport sdkmcp.Transport) error {
	if transport == nil {
		transport = &sdkmcp.StdioTransport{}
	}
	session, err := wowmcp.NewServer(options).SDKServer().Connect(ctx, transport, nil)
	if err != nil {
		return err
	}
	return session.Wait()
}

type execGit struct{}

func (execGit) Run(args ...string) error {
	return exec.Command("git", args...).Run()
}

func (execGit) Output(args ...string) ([]byte, error) {
	return exec.Command("git", args...).Output()
}
