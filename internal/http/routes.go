package http

import (
	"context"
	"encoding/json"
	"fmt"
	"html"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
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
	mux.HandleFunc("/", a.root)
	mux.HandleFunc("/health", a.health)
	mux.HandleFunc("/help", a.help)
	mux.HandleFunc("/mcp", a.mcp)
	return mux
}

func (a *App) root(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/" {
		http.NotFound(w, r)
		return
	}
	if r.Method != http.MethodGet && r.Method != http.MethodHead {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	http.Redirect(w, r, "/help", http.StatusFound)
}

func (a *App) health(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet && r.Method != http.MethodHead {
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
	payload := map[string]any{
		"sources":            a.healthSources(repos),
		"clients":            clients.Data.Clients,
		"invalidDirectories": clients.Diagnostics,
		"pools":              a.pools.Stats(),
		"recentErrors":       recentErrors,
	}
	if r.Method == http.MethodHead {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		return
	}
	writeJSON(w, payload)
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
	if r.Method != http.MethodGet && r.Method != http.MethodHead {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	page := a.helpHTML()
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Content-Length", fmt.Sprint(len(page)))
	w.WriteHeader(http.StatusOK)
	if r.Method != http.MethodHead {
		_, _ = w.Write([]byte(page))
	}
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

func (a *App) helpHTML() string {
	tools := []struct {
		Name        string
		Description string
	}{
		{"list_clients", "列出可用客户端别名、来源状态、默认 ref 和诊断信息。"},
		{"inspect_remote_refs", "查看已配置远端源码仓库的分支、标签和可选版本信息。"},
		{"lookup_blizzard_api", "按名称查找 Blizzard API，可启用精确匹配和安全信息。"},
		{"search_blizzard_api", "按关键词、类型、安全状态或场景搜索 Blizzard API。"},
		{"get_api_events", "查询事件定义和相关事件列表。"},
		{"get_api_namespace", "查看命名空间下的函数、事件和类型。"},
		{"check_api_deprecation", "分析 Lua 代码中的弃用 API 使用。"},
		{"suggest_api_migration", "为旧 API 给出迁移建议。"},
		{"explain_api_safety", "解释 API、FrameXML 符号或控件方法的安全执行约束。"},
		{"search_framexml", "搜索 FrameXML、XML 模板和 Lua 源码上下文。"},
		{"get_widget_api", "查看控件类型的方法、脚本和继承关系。"},
		{"lookup_cvar", "查询 CVar 定义、默认值和说明。"},
		{"get_wow_constants", "查询枚举、常量和生成文档中的常量组。"},
		{"find_mixin_template", "查找 mixin、template、frame 或相关定义。"},
		{"validate_toc", "校验插件 TOC 内容或文件路径。"},
	}
	var toolItems strings.Builder
	for _, tool := range tools {
		fmt.Fprintf(&toolItems, "<li><code>%s</code> - %s</li>\n", html.EscapeString(tool.Name), html.EscapeString(tool.Description))
	}

	sourceItems := configuredSourceItems(append(a.cfg.Sources.Defaults, a.cfg.Sources.Extra...))
	var sources strings.Builder
	for _, item := range sourceItems {
		fmt.Fprintf(&sources, "<li><code>%s</code> <span class=\"muted\">ref:</span> <code>%s</code></li>\n", html.EscapeString(item.alias), html.EscapeString(item.ref))
	}

	baseURL := strings.TrimRight(a.cfg.Server.BaseURL, "/")
	return fmt.Sprintf(`<!doctype html>
<html lang="zh-CN">
<head>
  <meta charset="utf-8">
  <meta name="viewport" content="width=device-width, initial-scale=1">
  <title>wowdoc MCP 帮助</title>
  <style>
    :root { color-scheme: light dark; --bg: #f7f7f5; --fg: #202124; --muted: #5f6368; --line: #d8d7d2; --panel: #ffffff; --accent: #0b7285; }
    @media (prefers-color-scheme: dark) { :root { --bg: #111315; --fg: #eceff1; --muted: #b0b6bb; --line: #34383d; --panel: #191c20; --accent: #4dd0e1; } }
    * { box-sizing: border-box; }
    body { margin: 0; background: var(--bg); color: var(--fg); font: 15px/1.55 system-ui, -apple-system, Segoe UI, sans-serif; }
    main { max-width: 980px; margin: 0 auto; padding: 32px 20px 56px; }
    header { border-bottom: 1px solid var(--line); padding-bottom: 20px; margin-bottom: 24px; }
    h1 { font-size: 34px; line-height: 1.1; margin: 0 0 10px; letter-spacing: 0; }
    h2 { font-size: 20px; margin: 28px 0 10px; }
    p { margin: 8px 0; max-width: 75ch; }
    .muted { color: var(--muted); }
    .grid { display: grid; grid-template-columns: repeat(auto-fit, minmax(240px, 1fr)); gap: 12px; }
    .endpoint, pre { background: var(--panel); border: 1px solid var(--line); border-radius: 8px; padding: 12px; overflow: auto; }
    code { font-family: ui-monospace, SFMono-Regular, Consolas, monospace; font-size: 0.94em; }
    pre { white-space: pre-wrap; }
    ul { padding-left: 20px; }
    li { margin: 6px 0; }
    a { color: var(--accent); }
  </style>
</head>
<body>
<main>
  <header>
    <h1>wowdoc MCP 帮助</h1>
    <p>wowdoc 是 World of Warcraft 插件开发文档服务。它通过 MCP tools 提供 Blizzard API、FrameXML、Widget、CVar、常量、Mixin、TOC 校验和迁移分析能力。</p>
    <p class="muted">此页面由 Go HTTP server 直接提供，是轻量静态 HTML，不依赖外部资源。</p>
  </header>

  <h2>服务入口</h2>
  <div class="grid">
    <div class="endpoint"><strong>MCP</strong><br><code>%s</code><br><span class="muted">Streamable HTTP JSON-RPC endpoint。</span></div>
    <div class="endpoint"><strong>Health</strong><br><code>%s</code><br><span class="muted">服务状态、源码、客户端和缓存池 JSON。</span></div>
  </div>

  <h2>公开 MCP Tools</h2>
  <ul>
%s  </ul>

  <h2>常用参数</h2>
  <ul>
    <li><code>client</code>：源码客户端别名，例如 <code>retail</code>、<code>classic</code>、<code>classic-titan</code> 或 <code>ptr2</code>。</li>
    <li><code>ref</code>：可选分支、标签或 commit；省略时使用配置的默认 ref。</li>
    <li><code>name</code>：API、CVar、常量、mixin 或模板名。</li>
    <li><code>query</code>：搜索关键词。</li>
    <li><code>scenario</code>：安全分析场景，例如 <code>combat</code>。</li>
    <li><code>tocPath</code> 或 <code>tocContent</code>：TOC 校验输入。</li>
  </ul>

  <h2>当前配置 Sources</h2>
  <ul>
%s  </ul>

  <h2>JSON-RPC 示例</h2>
  <pre>{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2025-06-18","capabilities":{},"clientInfo":{"name":"example","version":"1.0.0"}}}</pre>
  <pre>{"jsonrpc":"2.0","method":"notifications/initialized","params":{}}</pre>
  <pre>{"jsonrpc":"2.0","id":2,"method":"tools/list","params":{}}</pre>
  <pre>{"jsonrpc":"2.0","id":3,"method":"tools/call","params":{"name":"list_clients","arguments":{"includeDiagnostics":true,"includeRefs":true}}}</pre>
  <pre>{"jsonrpc":"2.0","id":4,"method":"tools/call","params":{"name":"lookup_blizzard_api","arguments":{"client":"retail","name":"C_AuctionHouse.GetItemSearchResultInfo","includeSafety":true}}}</pre>
  <pre>{"jsonrpc":"2.0","id":5,"method":"tools/call","params":{"name":"search_framexml","arguments":{"client":"retail","query":"SecureActionButtonTemplate","contextLines":2}}}</pre>

  <h2>服务状态</h2>
  <p>打开 <a href="%s"><code>%s</code></a> 查看当前服务快照。内部维护入口不会在此页面列出。</p>
</main>
</body>
</html>`, publicURL(baseURL, "/mcp"), publicURL(baseURL, "/health"), toolItems.String(), sources.String(), publicURL(baseURL, "/health"), publicURL(baseURL, "/health"))
}

type sourceHelpItem struct {
	alias string
	ref   string
}

func configuredSourceItems(entries []SourceEntry) []sourceHelpItem {
	seen := map[string]bool{}
	items := make([]sourceHelpItem, 0, len(entries))
	for _, entry := range entries {
		if entry.Alias == "" || seen[entry.Alias] {
			continue
		}
		ref := entry.Ref
		if ref == "" {
			ref = "latest"
		}
		items = append(items, sourceHelpItem{alias: entry.Alias, ref: ref})
		seen[entry.Alias] = true
	}
	sort.Slice(items, func(i, j int) bool { return items[i].alias < items[j].alias })
	return items
}

func publicURL(baseURL, path string) string {
	if baseURL == "" {
		return path
	}
	return strings.TrimRight(baseURL, "/") + path
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
