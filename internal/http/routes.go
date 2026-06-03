package http

import (
	"context"
	"encoding/json"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"time"

	sdkmcp "github.com/modelcontextprotocol/go-sdk/mcp"

	"wowdoc/internal/shared/analyze"
	"wowdoc/internal/shared/contracts"
	wowmcp "wowdoc/internal/shared/mcp"
	"wowdoc/internal/shared/source"
	"wowdoc/internal/shared/tools"
)

type App struct {
	cfg              Config
	pools            *Pools
	mcpHandler       http.Handler
	git              source.GitRunner
	archive          source.ArchiveFetcher
	sourceFetchSlots chan struct{}
	indexBuildSlots  chan struct{}
	recentErrors     []map[string]string
}

func NewApp(cfg Config) *App {
	return newAppWithFetchers(cfg, execGit{}, source.NewHTTPArchiveFetcher(http.DefaultClient))
}

func newAppWithGit(cfg Config, git source.GitRunner) *App {
	return newAppWithFetchers(cfg, git, nil)
}

func newAppWithFetchers(cfg Config, git source.GitRunner, archive source.ArchiveFetcher) *App {
	pools := NewPoolsWithPinned(cfg.Contexts.MaxSourceContexts, cfg.Contexts.MaxIndexContexts, cfg.Contexts.Pinned)
	app := &App{
		cfg:              cfg,
		pools:            pools,
		git:              git,
		archive:          archive,
		sourceFetchSlots: limitSlots(cfg.Limits.MaxConcurrentSourceFetches),
		indexBuildSlots:  limitSlots(cfg.Limits.MaxConcurrentIndexBuilds),
	}
	server := wowmcp.NewServer(wowmcp.ServerOptions{
		Name:          "wowdoc",
		SourceRoot:    cfg.Sources.Root,
		ExtraSources:  localExtraSources(cfg.Sources.Extra),
		SourceRepos:   app.configuredRepos(),
		DefaultRefs:   app.defaultRefs(),
		Git:           git,
		Archive:       archive,
		LoadRepoIndex: app.loadRepoIndex,
	})
	handler := sdkmcp.NewStreamableHTTPHandler(func(r *http.Request) *sdkmcp.Server {
		return server.SDKServer()
	}, &sdkmcp.StreamableHTTPOptions{JSONResponse: true})
	app.mcpHandler = handler
	app.prewarm()
	return app
}

func localExtraSources(entries []SourceEntry) map[string]string {
	out := map[string]string{}
	for _, entry := range entries {
		if entry.Alias != "" && entry.Path != "" {
			out[entry.Alias] = entry.Path
		}
	}
	return out
}

func (a *App) Router() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/health", a.health)
	mux.HandleFunc("/help", a.help)
	mux.HandleFunc("/mcp", a.mcp)
	return mux
}

func (a *App) health(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	repos, err := a.detectSources()
	recentErrors := append([]map[string]string{}, a.recentErrors...)
	if err != nil {
		recentErrors = append(recentErrors, map[string]string{"message": err.Error()})
	}
	clients := tools.ListClients(repos, tools.ListClientsOptions{
		IncludeDiagnostics: true,
		IncludeRefs:        true,
		DefaultRefs:        a.defaultRefs(),
		DefaultRef:         a.cfg.Sources.DefaultRef,
	})
	writeJSON(w, map[string]any{
		"sources":            a.healthSources(repos),
		"clients":            clients.Data.Clients,
		"invalidDirectories": clients.Diagnostics,
		"pools":              a.pools.Stats(),
		"recentErrors":       recentErrors,
	})
}

func (a *App) healthSources(repos []analyze.Repository) []any {
	out := make([]any, 0, len(repos)+len(a.cfg.Sources.Defaults)+len(a.cfg.Sources.Extra))
	seen := map[string]bool{}
	for _, repo := range repos {
		out = append(out, repo)
		seen[repo.Alias] = true
	}
	for _, entry := range append(a.cfg.Sources.Defaults, a.cfg.Sources.Extra...) {
		if entry.Alias == "" || entry.Repo == "" || seen[entry.Alias] {
			continue
		}
		out = append(out, map[string]any{
			"alias":      entry.Alias,
			"repo":       entry.Repo,
			"defaultRef": entry.Ref,
			"status":     "configured",
		})
		seen[entry.Alias] = true
	}
	return out
}

func (a *App) prewarm() {
	if !a.cfg.Prepare.PrewarmOnStart {
		return
	}
	for _, client := range a.cfg.Prepare.PrewarmClients {
		if client == "" {
			continue
		}
		repo, _, err := a.loadRepoIndex(context.Background(), client, "")
		if err != nil {
			a.recentErrors = append(a.recentErrors, map[string]string{"client": client, "message": err.Error()})
			continue
		}
		if !repo.Valid {
			a.recentErrors = append(a.recentErrors, map[string]string{"client": client, "message": "source_invalid"})
		}
	}
}

func (a *App) detectSources() ([]analyze.Repository, error) {
	repos := []analyze.Repository{}
	if a.cfg.Sources.Root != "" {
		entries, err := os.ReadDir(a.cfg.Sources.Root)
		if err != nil {
			if !os.IsNotExist(err) {
				return nil, err
			}
		} else {
			for _, entry := range entries {
				if entry.IsDir() && !isInternalSourceCacheDir(entry.Name()) {
					alias := entry.Name()
					repos = append(repos, analyze.DetectRepository(filepath.Join(a.cfg.Sources.Root, alias), alias))
				}
			}
		}
	}
	for _, entry := range a.cfg.Sources.Extra {
		if entry.Alias != "" && entry.Path != "" {
			repos = append(repos, analyze.DetectRepository(entry.Path, entry.Alias))
		}
	}
	return repos, nil
}

func isInternalSourceCacheDir(name string) bool {
	switch name {
	case "repos", "checkouts", "archives":
		return true
	default:
		return false
	}
}

func (a *App) help(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	writeJSON(w, map[string]string{"mcp": "/mcp", "health": "/health"})
}

func (a *App) mcp(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet && r.Method != http.MethodPost {
		w.Header().Set("Allow", "GET, POST")
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	a.mcpHandler.ServeHTTP(w, r)
}

func (a *App) loadRepoIndex(ctx context.Context, client, ref string) (analyze.Repository, *analyze.Index, error) {
	ctx, cancel := a.withRequestTimeout(ctx)
	defer cancel()
	releaseSource, err := acquireSlot(ctx, a.sourceFetchSlots)
	if err != nil {
		return analyze.Repository{}, nil, err
	}
	repoPath, sourceKey, requestedRef, resolvedRef, diagnostics, err := a.resolveSourcePath(client, ref)
	releaseSource()
	if err != nil {
		return analyze.Repository{}, nil, err
	}
	sourceValue, err := a.pools.LoadSource(ctx, client, sourceKey, func(context.Context) (any, error) {
		repo := analyze.DetectRepository(repoPath, client)
		repo.RequestedRef = requestedRef
		repo.ResolvedRef = resolvedRef
		repo.Diagnostics = append(repo.Diagnostics, diagnostics...)
		return repo, nil
	})
	if err != nil {
		return analyze.Repository{}, nil, err
	}
	repo := sourceValue.(analyze.Repository)
	indexValue, err := a.pools.LoadIndex(ctx, client, sourceKey, "repo", func(context.Context) (any, error) {
		releaseIndex, err := acquireSlot(ctx, a.indexBuildSlots)
		if err != nil {
			return nil, err
		}
		defer releaseIndex()
		return analyze.BuildIndex(repo)
	})
	if err != nil {
		return analyze.Repository{}, nil, err
	}
	return repo, indexValue.(*analyze.Index), nil
}

func (a *App) withRequestTimeout(ctx context.Context) (context.Context, context.CancelFunc) {
	if a.cfg.Limits.RequestTimeoutSeconds <= 0 {
		return ctx, func() {}
	}
	return context.WithTimeout(ctx, time.Duration(a.cfg.Limits.RequestTimeoutSeconds)*time.Second)
}

func limitSlots(limit int) chan struct{} {
	if limit <= 0 {
		return nil
	}
	return make(chan struct{}, limit)
}

func acquireSlot(ctx context.Context, slots chan struct{}) (func(), error) {
	if slots == nil {
		return func() {}, nil
	}
	select {
	case slots <- struct{}{}:
		return func() { <-slots }, nil
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}

func (a *App) resolveSourcePath(client, ref string) (path string, key string, requestedRef string, resolvedRef string, diagnostics []contracts.Diagnostic, err error) {
	if a.configuredRepo(client) != "" {
		manager := source.NewManager(source.Options{
			Root:              a.cfg.Sources.Root,
			AllowArbitraryRef: a.cfg.Sources.AllowArbitraryRef,
			DefaultRefs:       a.defaultRefs(),
			Repos:             a.configuredRepos(),
			Git:               a.git,
			Archive:           a.archive,
		})
		resolved, err := manager.ResolveSource(client, ref)
		if err != nil {
			return "", "", "", "", nil, err
		}
		return resolved.CheckoutDir, resolved.Resolved, resolved.Requested, resolved.Resolved, resolved.Diagnostics, nil
	}
	key = ref
	if key == "" {
		key = "latest"
	}
	return a.sourcePath(client), key, ref, ref, nil, nil
}

func (a *App) sourcePath(client string) string {
	for _, entry := range a.cfg.Sources.Extra {
		if entry.Alias == client && entry.Path != "" {
			return entry.Path
		}
	}
	return filepath.Join(a.cfg.Sources.Root, client)
}

func (a *App) configuredRepo(client string) string {
	for _, entry := range append(a.cfg.Sources.Defaults, a.cfg.Sources.Extra...) {
		if entry.Alias == client && entry.Repo != "" {
			return entry.Repo
		}
	}
	return ""
}

func (a *App) configuredRepos() map[string]string {
	repos := map[string]string{}
	for _, entry := range append(a.cfg.Sources.Defaults, a.cfg.Sources.Extra...) {
		if entry.Alias != "" && entry.Repo != "" {
			repos[entry.Alias] = entry.Repo
		}
	}
	return repos
}

func (a *App) defaultRefs() map[string]string {
	refs := map[string]string{}
	for _, entry := range append(a.cfg.Sources.Defaults, a.cfg.Sources.Extra...) {
		if entry.Alias != "" && entry.Ref != "" {
			refs[entry.Alias] = entry.Ref
		}
	}
	return refs
}

type execGit struct{}

func (execGit) Run(args ...string) error {
	return exec.Command("git", args...).Run()
}

func (execGit) Output(args ...string) ([]byte, error) {
	return exec.Command("git", args...).Output()
}

func writeJSON(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(v)
}
