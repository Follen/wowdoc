package mcp

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"sort"
	"sync"

	sdkmcp "github.com/modelcontextprotocol/go-sdk/mcp"

	"wowdoc/internal/shared/analyze"
	"wowdoc/internal/shared/contracts"
	"wowdoc/internal/shared/source"
	"wowdoc/internal/shared/tools"
)

type ServerOptions struct {
	Name          string
	Version       string
	SourceRoot    string
	ExtraSources  map[string]string
	SourceRepos   map[string]string
	DefaultRefs   map[string]string
	Git           source.GitRunner
	Archive       source.ArchiveFetcher
	LoadRepoIndex func(ctx context.Context, client, ref string) (analyze.Repository, *analyze.Index, error)
}

type Server struct {
	sdkServer          *sdkmcp.Server
	tools              map[string]JSONSchema
	sdkRegisteredTools int
	sourceRoot         string
	extraSources       map[string]string
	sourceRepos        map[string]string
	defaultRefs        map[string]string
	git                source.GitRunner
	archive            source.ArchiveFetcher
	loadRepoIndexFunc  func(ctx context.Context, client, ref string) (analyze.Repository, *analyze.Index, error)
	indexMu            sync.Mutex
	indexes            map[indexKey]repoIndex
}

type indexKey struct {
	client string
	ref    string
}

type repoIndex struct {
	repo analyze.Repository
	idx  *analyze.Index
}

func NewServer(options ServerOptions) *Server {
	name := options.Name
	if name == "" {
		name = "wowdoc"
	}

	server := &Server{
		sdkServer:         sdkmcp.NewServer(&sdkmcp.Implementation{Name: name, Version: options.Version}, nil),
		tools:             ToolInputSchemas(),
		sourceRoot:        options.SourceRoot,
		extraSources:      options.ExtraSources,
		sourceRepos:       options.SourceRepos,
		defaultRefs:       options.DefaultRefs,
		git:               options.Git,
		archive:           options.Archive,
		loadRepoIndexFunc: options.LoadRepoIndex,
		indexes:           map[indexKey]repoIndex{},
	}
	for toolName, schema := range server.tools {
		toolName := toolName
		server.sdkServer.AddTool(&sdkmcp.Tool{
			Name:        toolName,
			Description: "wowdoc tool " + toolName,
			InputSchema: schema,
		}, func(ctx context.Context, req *sdkmcp.CallToolRequest) (*sdkmcp.CallToolResult, error) {
			return server.callTool(ctx, toolName, req)
		})
		server.sdkRegisteredTools++
	}
	return server
}

func (s *Server) RegisteredToolNames() []string {
	names := make([]string, 0, len(s.tools))
	for name := range s.tools {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

func (s *Server) HasTool(name string) bool {
	_, ok := s.tools[name]
	return ok
}

func (s *Server) SDKRegisteredToolCount() int {
	return s.sdkRegisteredTools
}

func (s *Server) SDKServer() *sdkmcp.Server {
	return s.sdkServer
}

func (s *Server) CachedIndexCount() int {
	s.indexMu.Lock()
	defer s.indexMu.Unlock()
	return len(s.indexes)
}

func (s *Server) remoteRefMaps(client string) (map[string]string, map[string]string) {
	repos := map[string]string{}
	refs := map[string]string{}
	for alias, repo := range s.sourceRepos {
		if client != "" && alias != client {
			continue
		}
		repos[alias] = repo
		refs[alias] = s.defaultRefs[alias]
	}
	return repos, refs
}

func (s *Server) callTool(ctx context.Context, name string, req *sdkmcp.CallToolRequest) (*sdkmcp.CallToolResult, error) {
	_ = ctx
	switch name {
	case "list_clients":
		var args struct {
			IncludeDiagnostics bool `json:"includeDiagnostics"`
			IncludeRefs        bool `json:"includeRefs"`
		}
		if err := decodeArgs(req, &args); err != nil {
			return toolResult(errorEnvelope[any](contracts.ErrUnsupportedRef, err.Error()), true)
		}
		repos, err := s.detectSourceRoot()
		if err != nil {
			return toolResult(errorEnvelopeFor(err, contracts.ErrSourceNotFound), true)
		}
		return toolResult(tools.ListClients(repos, tools.ListClientsOptions{
			IncludeDiagnostics: args.IncludeDiagnostics,
			IncludeRefs:        args.IncludeRefs,
			DefaultRefs:        s.defaultRefs,
			DefaultRef:         "latest",
		}), false)
	case "inspect_remote_refs":
		var args struct {
			Client         string `json:"client"`
			IncludeVersion bool   `json:"includeVersion"`
		}
		if err := decodeArgs(req, &args); err != nil {
			return toolResult(errorEnvelope[any](contracts.ErrUnsupportedRef, err.Error()), true)
		}
		repos, refs := s.remoteRefMaps(args.Client)
		if args.Client != "" && len(repos) == 0 {
			env := errorEnvelope[tools.RemoteRefsData](contracts.ErrClientNotFound, "configured source client not found")
			env.Data.Clients = []tools.RemoteRefInfo{}
			return toolResult(env, true)
		}
		resolver := func(alias string) (string, string, error) {
			path, _, _, _, err := s.sourcePath(alias, "")
			if err != nil {
				return "", "", err
			}
			repo := analyze.DetectRepository(path, alias)
			if args.IncludeVersion {
				return repo.Version, repo.Path, nil
			}
			return "", repo.Path, nil
		}
		env := tools.InspectRemoteRefs(repos, refs, s.git, resolver)
		return toolResult(env, false)
	case "lookup_blizzard_api":
		var args struct {
			Client        string `json:"client"`
			Ref           string `json:"ref"`
			Name          string `json:"name"`
			Exact         *bool  `json:"exact"`
			IncludeSafety *bool  `json:"includeSafety"`
		}
		if err := decodeArgs(req, &args); err != nil {
			return toolResult(errorEnvelope[any](contracts.ErrUnsupportedRef, err.Error()), true)
		}
		if args.Client == "" {
			return toolResult(errorEnvelope[any](contracts.ErrClientRequired, "client is required"), true)
		}
		if args.Name == "" {
			return toolResult(errorEnvelope[any](contracts.ErrIndexUnavailable, "name is required"), true)
		}
		repo, idx, err := s.loadRepoIndex(ctx, args.Client, args.Ref)
		if err != nil {
			return toolResult(errorEnvelopeFor(err, contracts.ErrIndexUnavailable), true)
		}
		env := tools.LookupBlizzardAPI(repo, idx, tools.LookupAPIOptions{
			Name:          args.Name,
			Exact:         boolDefault(args.Exact, false),
			IncludeSafety: boolDefault(args.IncludeSafety, true),
		})
		fillRequestedRef(&env.Source, args.Ref)
		return toolResult(env, !env.OK)
	case "search_blizzard_api":
		var args struct {
			Client            string `json:"client"`
			Ref               string `json:"ref"`
			Query             string `json:"query"`
			Type              string `json:"type"`
			Limit             int    `json:"limit"`
			Safety            string `json:"safety"`
			Scenario          string `json:"scenario"`
			IncludeUnsafeOnly bool   `json:"includeUnsafeOnly"`
		}
		if err := decodeArgs(req, &args); err != nil {
			return toolResult(errorEnvelope[any](contracts.ErrUnsupportedRef, err.Error()), true)
		}
		if args.Client == "" {
			return missingClientInput()
		}
		if args.Query == "" {
			return missingPrimaryInput("query")
		}
		if args.Limit == 0 {
			args.Limit = 20
		}
		repo, idx, err := s.loadRepoIndex(ctx, args.Client, args.Ref)
		if err != nil {
			return toolResult(errorEnvelopeFor(err, contracts.ErrSourceNotFound), true)
		}
		env := tools.SearchBlizzardAPI(repo, idx, tools.APISearchOptions{
			Query:             args.Query,
			Type:              args.Type,
			Limit:             args.Limit,
			Safety:            args.Safety,
			Scenario:          args.Scenario,
			IncludeUnsafeOnly: args.IncludeUnsafeOnly,
		})
		fillRequestedRef(&env.Source, args.Ref)
		return toolResult(env, !env.OK)
	case "get_api_namespace":
		var args struct {
			Client    string `json:"client"`
			Ref       string `json:"ref"`
			Namespace string `json:"namespace"`
		}
		if err := decodeArgs(req, &args); err != nil {
			return toolResult(errorEnvelope[any](contracts.ErrUnsupportedRef, err.Error()), true)
		}
		if args.Client == "" {
			return missingClientInput()
		}
		if args.Namespace == "" {
			return missingPrimaryInput("namespace")
		}
		repo, idx, err := s.loadRepoIndex(ctx, args.Client, args.Ref)
		if err != nil {
			return toolResult(errorEnvelopeFor(err, contracts.ErrSourceNotFound), true)
		}
		env := tools.GetAPINamespace(repo, idx, args.Namespace)
		fillRequestedRef(&env.Source, args.Ref)
		return toolResult(env, !env.OK)
	case "search_framexml":
		var args struct {
			Client       string `json:"client"`
			Ref          string `json:"ref"`
			Query        string `json:"query"`
			FilePattern  string `json:"filePattern"`
			ContextLines int    `json:"contextLines"`
			MaxResults   int    `json:"maxResults"`
		}
		if err := decodeArgs(req, &args); err != nil {
			return toolResult(errorEnvelope[any](contracts.ErrUnsupportedRef, err.Error()), true)
		}
		if args.Client == "" {
			return missingClientInput()
		}
		if args.Query == "" {
			return missingPrimaryInput("query")
		}
		if args.MaxResults == 0 {
			args.MaxResults = 15
		}
		if args.ContextLines == 0 {
			args.ContextLines = 3
		}
		repo, idx, err := s.loadRepoIndex(ctx, args.Client, args.Ref)
		if err != nil {
			return toolResult(errorEnvelopeFor(err, contracts.ErrSourceNotFound), true)
		}
		env := tools.SearchFrameXML(repo, idx, tools.FrameXMLSearchOptions{
			Query:        args.Query,
			FilePattern:  args.FilePattern,
			ContextLines: args.ContextLines,
			MaxResults:   args.MaxResults,
		})
		fillRequestedRef(&env.Source, args.Ref)
		return toolResult(env, !env.OK)
	case "find_mixin_template":
		var args struct {
			Client string `json:"client"`
			Ref    string `json:"ref"`
			Name   string `json:"name"`
			Kind   string `json:"kind"`
			Limit  int    `json:"limit"`
		}
		if err := decodeArgs(req, &args); err != nil {
			return toolResult(errorEnvelope[any](contracts.ErrUnsupportedRef, err.Error()), true)
		}
		if args.Client == "" {
			return missingClientInput()
		}
		if args.Name == "" {
			return missingPrimaryInput("name")
		}
		if args.Limit == 0 {
			args.Limit = 25
		}
		repo, idx, err := s.loadRepoIndex(ctx, args.Client, args.Ref)
		if err != nil {
			return toolResult(errorEnvelopeFor(err, contracts.ErrSourceNotFound), true)
		}
		env := tools.FindMixinTemplate(repo, idx, args.Name, args.Kind, args.Limit)
		fillRequestedRef(&env.Source, args.Ref)
		return toolResult(env, !env.OK)
	case "get_wow_constants":
		var args struct {
			Client string `json:"client"`
			Ref    string `json:"ref"`
			Name   string `json:"name"`
			Filter string `json:"filter"`
			Kind   string `json:"kind"`
			Limit  int    `json:"limit"`
		}
		if err := decodeArgs(req, &args); err != nil {
			return toolResult(errorEnvelope[any](contracts.ErrUnsupportedRef, err.Error()), true)
		}
		if args.Client == "" {
			return missingClientInput()
		}
		if args.Name == "" {
			return missingPrimaryInput("name")
		}
		repo, idx, err := s.loadRepoIndex(ctx, args.Client, args.Ref)
		if err != nil {
			return toolResult(errorEnvelopeFor(err, contracts.ErrSourceNotFound), true)
		}
		env := tools.GetWowConstants(repo, idx, tools.ConstantsOptions{Name: args.Name, Filter: args.Filter, Kind: args.Kind, Limit: args.Limit})
		fillRequestedRef(&env.Source, args.Ref)
		return toolResult(env, !env.OK)
	case "explain_api_safety":
		var args struct {
			Client   string `json:"client"`
			Ref      string `json:"ref"`
			Symbol   string `json:"symbol"`
			Scenario string `json:"scenario"`
		}
		if err := decodeArgs(req, &args); err != nil {
			return toolResult(errorEnvelope[any](contracts.ErrUnsupportedRef, err.Error()), true)
		}
		if args.Client == "" {
			return missingClientInput()
		}
		if args.Symbol == "" {
			return missingPrimaryInput("symbol")
		}
		repo, idx, err := s.loadRepoIndex(ctx, args.Client, args.Ref)
		if err != nil {
			return toolResult(errorEnvelopeFor(err, contracts.ErrSourceNotFound), true)
		}
		env := tools.ExplainAPISafety(repo, idx, args.Symbol, args.Scenario)
		fillRequestedRef(&env.Source, args.Ref)
		return toolResult(env, !env.OK)
	case "get_api_events":
		var args struct {
			Client string `json:"client"`
			Ref    string `json:"ref"`
			Event  string `json:"event"`
			Filter string `json:"filter"`
		}
		if err := decodeArgs(req, &args); err != nil {
			return toolResult(errorEnvelope[any](contracts.ErrUnsupportedRef, err.Error()), true)
		}
		if args.Client == "" {
			return missingClientInput()
		}
		if args.Event == "" {
			return missingPrimaryInput("event")
		}
		repo, idx, err := s.loadRepoIndex(ctx, args.Client, args.Ref)
		if err != nil {
			return toolResult(errorEnvelopeFor(err, contracts.ErrSourceNotFound), true)
		}
		env := tools.GetAPIEvents(repo, idx, tools.EventOptions{Event: args.Event, Filter: args.Filter})
		fillRequestedRef(&env.Source, args.Ref)
		return toolResult(env, !env.OK)
	case "check_api_deprecation":
		var args struct {
			Client  string `json:"client"`
			Ref     string `json:"ref"`
			LuaCode string `json:"luaCode"`
		}
		if err := decodeArgs(req, &args); err != nil {
			return toolResult(errorEnvelope[any](contracts.ErrUnsupportedRef, err.Error()), true)
		}
		if args.Client == "" {
			return missingClientInput()
		}
		if args.LuaCode == "" {
			return missingPrimaryInput("luaCode")
		}
		repo, idx, err := s.loadRepoIndex(ctx, args.Client, args.Ref)
		if err != nil {
			return toolResult(errorEnvelopeFor(err, contracts.ErrSourceNotFound), true)
		}
		env := tools.CheckAPIDeprecationWithIndex(repo, idx, args.LuaCode)
		fillRequestedRef(&env.Source, args.Ref)
		return toolResult(env, !env.OK)
	case "suggest_api_migration":
		var args struct {
			Client      string `json:"client"`
			Ref         string `json:"ref"`
			OldFunction string `json:"oldFunction"`
		}
		if err := decodeArgs(req, &args); err != nil {
			return toolResult(errorEnvelope[any](contracts.ErrUnsupportedRef, err.Error()), true)
		}
		if args.Client == "" {
			return missingClientInput()
		}
		if args.OldFunction == "" {
			return missingPrimaryInput("oldFunction")
		}
		repo, idx, err := s.loadRepoIndex(ctx, args.Client, args.Ref)
		if err != nil {
			return toolResult(errorEnvelopeFor(err, contracts.ErrSourceNotFound), true)
		}
		env := tools.SuggestAPIMigrationWithIndex(repo, idx, args.OldFunction)
		fillRequestedRef(&env.Source, args.Ref)
		return toolResult(env, !env.OK)
	case "get_widget_api":
		var args struct {
			Client     string `json:"client"`
			Ref        string `json:"ref"`
			WidgetType string `json:"widgetType"`
		}
		if err := decodeArgs(req, &args); err != nil {
			return toolResult(errorEnvelope[any](contracts.ErrUnsupportedRef, err.Error()), true)
		}
		if args.Client == "" {
			return missingClientInput()
		}
		if args.WidgetType == "" {
			return missingPrimaryInput("widgetType")
		}
		repo, idx, err := s.loadRepoIndex(ctx, args.Client, args.Ref)
		if err != nil {
			return toolResult(errorEnvelopeFor(err, contracts.ErrSourceNotFound), true)
		}
		env := tools.GetWidgetAPI(repo, idx, args.WidgetType)
		fillRequestedRef(&env.Source, args.Ref)
		return toolResult(env, !env.OK)
	case "lookup_cvar":
		var args struct {
			Client string `json:"client"`
			Ref    string `json:"ref"`
			Name   string `json:"name"`
			Detail bool   `json:"detail"`
		}
		if err := decodeArgs(req, &args); err != nil {
			return toolResult(errorEnvelope[any](contracts.ErrUnsupportedRef, err.Error()), true)
		}
		if args.Client == "" {
			return missingClientInput()
		}
		if args.Name == "" {
			return missingPrimaryInput("name")
		}
		repo, idx, err := s.loadRepoIndex(ctx, args.Client, args.Ref)
		if err != nil {
			return toolResult(errorEnvelopeFor(err, contracts.ErrSourceNotFound), true)
		}
		env := tools.LookupCVar(repo, idx, tools.CVarLookupOptions{Name: args.Name, Detail: args.Detail})
		fillRequestedRef(&env.Source, args.Ref)
		return toolResult(env, !env.OK)
	case "validate_toc":
		var args struct {
			TOCContent string `json:"tocContent"`
			TOCPath    string `json:"tocPath"`
			Client     string `json:"client"`
			Ref        string `json:"ref"`
			AddonName  string `json:"addonName"`
		}
		if err := decodeArgs(req, &args); err != nil {
			return toolResult(errorEnvelope[any](contracts.ErrUnsupportedRef, err.Error()), true)
		}
		env := tools.ValidateTOC(args.TOCContent, args.TOCPath, args.AddonName)
		if args.Client != "" {
			repo, _, err := s.loadRepoIndex(ctx, args.Client, args.Ref)
			if err != nil {
				return toolResult(errorEnvelopeFor(err, contracts.ErrSourceNotFound), true)
			}
			env = tools.ValidateTOCWithOptions(args.TOCContent, args.TOCPath, args.AddonName, tools.TOCValidationOptions{SourceVersion: repo.Version})
			env.Source = contracts.SourceTransparency{
				Client:       repo.Alias,
				RequestedRef: repo.RequestedRef,
				ResolvedRef:  repo.ResolvedRef,
				Version:      repo.Version,
				Path:         repo.Path,
			}
			env.Diagnostics = append(env.Diagnostics, repo.Diagnostics...)
			fillRequestedRef(&env.Source, args.Ref)
		}
		return toolResult(env, !env.OK)
	default:
		return toolResult(errorEnvelope[any](contracts.ErrCapabilityUnavailable, "tool handler is not implemented"), true)
	}
}

func decodeArgs(req *sdkmcp.CallToolRequest, out any) error {
	if req == nil || len(req.Params.Arguments) == 0 {
		return nil
	}
	return json.Unmarshal(req.Params.Arguments, out)
}

func (s *Server) detectSourceRoot() ([]analyze.Repository, error) {
	repos := []analyze.Repository{}
	seen := map[string]bool{}
	if s.optionsSourceRoot() != "" {
		entries, err := os.ReadDir(s.optionsSourceRoot())
		if err != nil {
			if !os.IsNotExist(err) {
				return nil, err
			}
		} else {
			for _, entry := range entries {
				if entry.IsDir() && !isInternalSourceCacheDir(entry.Name()) {
					alias := entry.Name()
					repos = append(repos, analyze.DetectRepository(filepath.Join(s.optionsSourceRoot(), alias), alias))
					seen[alias] = true
				}
			}
		}
	}
	for alias, path := range s.extraSources {
		repos = append(repos, analyze.DetectRepository(path, alias))
		seen[alias] = true
	}
	for alias := range s.sourceRepos {
		if alias == "" || seen[alias] {
			continue
		}
		repos = append(repos, analyze.Repository{
			Alias:        alias,
			Path:         "",
			RequestedRef: s.defaultRefs[alias],
			Valid:        true,
		})
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

func (s *Server) sourcePath(client, ref string) (string, string, string, []contracts.Diagnostic, error) {
	if path, ok := s.extraSources[client]; ok {
		return path, ref, ref, nil, nil
	}
	if _, ok := s.sourceRepos[client]; ok {
		manager := source.NewManager(source.Options{
			Root:              s.optionsSourceRoot(),
			AllowArbitraryRef: true,
			DefaultRefs:       s.defaultRefs,
			Repos:             s.sourceRepos,
			Git:               s.git,
			Archive:           s.archive,
		})
		resolved, err := manager.ResolveSource(client, ref)
		if err != nil {
			return "", "", "", nil, err
		}
		return resolved.CheckoutDir, resolved.Requested, resolved.Resolved, resolved.Diagnostics, nil
	}
	root := s.optionsSourceRoot()
	if root == "" {
		return "", "", "", nil, os.ErrNotExist
	}
	return filepath.Join(root, client), ref, ref, nil, nil
}

func (s *Server) loadRepoIndex(ctx context.Context, client, ref string) (analyze.Repository, *analyze.Index, error) {
	if client == "" {
		return analyze.Repository{}, nil, errCode(contracts.ErrClientRequired, "client is required")
	}
	if s.loadRepoIndexFunc != nil {
		return s.loadRepoIndexFunc(ctx, client, ref)
	}
	key := indexKey{client: client, ref: ref}
	s.indexMu.Lock()
	if cached, ok := s.indexes[key]; ok {
		s.indexMu.Unlock()
		return cached.repo, cached.idx, nil
	}
	s.indexMu.Unlock()

	repoPath, requestedRef, resolvedRef, diagnostics, err := s.sourcePath(client, ref)
	if err != nil {
		return analyze.Repository{}, nil, err
	}
	repo := analyze.DetectRepository(repoPath, client)
	repo.RequestedRef = requestedRef
	repo.ResolvedRef = resolvedRef
	repo.Diagnostics = append(repo.Diagnostics, diagnostics...)
	resolvedKey := indexKey{client: client, ref: resolvedRef}
	if resolvedRef != "" && resolvedKey != key {
		s.indexMu.Lock()
		if cached, ok := s.indexes[resolvedKey]; ok {
			s.indexMu.Unlock()
			return cached.repo, cached.idx, nil
		}
		s.indexMu.Unlock()
	}
	idx, err := analyze.BuildIndex(repo)
	if err != nil {
		return analyze.Repository{}, nil, err
	}
	s.indexMu.Lock()
	s.indexes[cacheKeyForRepo(key, repo)] = repoIndex{repo: repo, idx: idx}
	s.indexMu.Unlock()
	return repo, idx, nil
}

func cacheKeyForRepo(fallback indexKey, repo analyze.Repository) indexKey {
	if repo.ResolvedRef != "" {
		return indexKey{client: repo.Alias, ref: repo.ResolvedRef}
	}
	return fallback
}

type codedError struct {
	code contracts.ErrorCode
	msg  string
}

func (e codedError) Error() string { return e.msg }

func errCode(code contracts.ErrorCode, msg string) error {
	return codedError{code: code, msg: msg}
}

func (s *Server) optionsSourceRoot() string {
	return s.sourceRoot
}

func toolResult(v any, isError bool) (*sdkmcp.CallToolResult, error) {
	data, err := json.Marshal(v)
	if err != nil {
		return nil, err
	}
	return &sdkmcp.CallToolResult{
		Content:           []sdkmcp.Content{&sdkmcp.TextContent{Text: string(data)}},
		StructuredContent: v,
		IsError:           isError,
	}, nil
}

func fillRequestedRef(source *contracts.SourceTransparency, requested string) {
	if source.RequestedRef == "" {
		source.RequestedRef = requested
	}
}

func boolDefault(value *bool, fallback bool) bool {
	if value == nil {
		return fallback
	}
	return *value
}

func missingPrimaryInput(name string) (*sdkmcp.CallToolResult, error) {
	return toolResult(errorEnvelope[any](contracts.ErrIndexUnavailable, name+" is required"), true)
}

func missingClientInput() (*sdkmcp.CallToolResult, error) {
	return toolResult(errorEnvelope[any](contracts.ErrClientRequired, "client is required"), true)
}

func errorEnvelope[T any](code contracts.ErrorCode, message string) contracts.Envelope[T] {
	return contracts.Envelope[T]{
		OK: false,
		Error: &contracts.ToolError{
			Code:    code,
			Message: message,
		},
	}
}

func errorEnvelopeFor(err error, fallback contracts.ErrorCode) contracts.Envelope[any] {
	code := fallback
	if errors.Is(err, context.DeadlineExceeded) || errors.Is(err, context.Canceled) {
		code = contracts.ErrTimeout
	} else if sourceCode := source.ErrorCode(err); sourceCode != "" {
		code = sourceCode
	} else {
		var local codedError
		if errors.As(err, &local) {
			code = local.code
		}
	}
	return errorEnvelope[any](code, err.Error())
}
