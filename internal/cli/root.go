package cli

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"

	"github.com/spf13/cobra"

	"wowdoc/internal/shared/analyze"
	"wowdoc/internal/shared/config"
	"wowdoc/internal/shared/contracts"
	wowmcp "wowdoc/internal/shared/mcp"
	"wowdoc/internal/shared/source"
	"wowdoc/internal/shared/tools"
	"wowdoc/internal/stdio"
)

var (
	cliSourceGit     source.GitRunner      = execGit{}
	cliSourceArchive source.ArchiveFetcher = source.NewHTTPArchiveFetcher(http.DefaultClient)
)

const commonErrorCodes = "client_required client_not_found source_not_found source_invalid ref_not_found git_unavailable_archive_failed capability_unavailable index_unavailable timeout unsupported_ref"

func sourceBackedHelp(summary, required, minimumCall, mcpArguments string) string {
	return summary + `

Required:
  --client retail|classic|classic-ptr|classic-titan|ptr|ptr2|<discovered alias>
` + required + `

Optional:
  --ref REF       branch, tag, or commit. Defaults to the client's latest ref.

Source resolution:
  --source-path wins over --source-root + --client + --ref.
  If no source root is set, wowdoc uses <exe-dir>/sources.

Agent next step:
  If client is unknown, run: wowdoc clients list --include-diagnostics

Minimum valid call:
  ` + minimumCall + `

MCP arguments:
  ` + mcpArguments + `

Common error codes:
  ` + commonErrorCodes
}

func tocHelp() string {
	return `Validate TOC metadata.

Required:
  --toc-content or --toc-path

Optional:
  --client CLIENT   Enables source/version-aware Interface guidance.
  --ref REF         Branch, tag, or commit. Defaults to the client's latest ref.

Source resolution:
  --source-path wins over --source-root + --client + --ref.
  If no source root is set, wowdoc uses <exe-dir>/sources.

Agent next step:
  If client is unknown, run: wowdoc clients list --include-diagnostics

Minimum valid call:
  wowdoc toc validate --toc-path .\MyAddon.toc

MCP arguments:
  {"tocPath":".\\MyAddon.toc"}

Common error codes:
  ` + commonErrorCodes
}

func clientsListHelp() string {
	return `List detected WoW UI source clients.

First diagnostic command:
  Run this before source-backed commands when the client alias or source state is unknown.

Optional:
  --include-diagnostics   Include invalid source diagnostics.
  --include-refs          Include default/requested/resolved refs.

Source resolution:
  If no source root is set, wowdoc uses <exe-dir>/sources.

Agent next step:
  If a source-backed command returns client_not_found, run this command with --include-diagnostics.

Minimum valid call:
  wowdoc clients list --include-diagnostics

MCP arguments:
  {"includeDiagnostics":true}

Common error codes:
  ` + commonErrorCodes
}

func stdioHelp() string {
	return `Run the MCP server over stdin/stdout.

Optional:
  --source-root ROOT   Source cache root. Defaults to <exe-dir>/sources.

Source resolution:
  Stdio uses the shared source manager and default source seeds.
  Source-backed MCP tools still require client and may include optional ref.

Agent next step:
  Start the server, then call list_clients with includeDiagnostics before source-backed tools.

Minimum valid call:
  wowdoc mcp stdio`
}

func NewRootCommand() *cobra.Command {
	root := &cobra.Command{Use: "wowdoc", SilenceUsage: true, SilenceErrors: true}
	root.AddCommand(apiCommand())
	root.AddCommand(clientsCommand())
	root.AddCommand(sourcesCommand())
	root.AddCommand(frameXMLCommand())
	root.AddCommand(widgetCommand())
	root.AddCommand(cvarCommand())
	root.AddCommand(constantsCommand())
	root.AddCommand(mixinCommand())
	root.AddCommand(tocCommand())
	root.AddCommand(mcpCommand())
	return root
}

func apiCommand() *cobra.Command {
	api := &cobra.Command{Use: "api"}
	var client, ref, name, sourceRoot, sourcePath string
	var exact, includeSafety bool
	lookup := &cobra.Command{
		Use:   "lookup",
		Short: "Lookup a Blizzard API symbol.",
		Long: `Lookup a Blizzard API symbol.

Required:
  --client retail|classic|classic-ptr|classic-titan|ptr|ptr2|<discovered alias>
  --name API_NAME

Optional:
  --ref REF       branch, tag, or commit. Defaults to the client's latest ref.

Source resolution:
  --source-path wins over --source-root + --client + --ref.
  If no source root is set, wowdoc uses <exe-dir>/sources.

Agent next step:
  If client is unknown, run: wowdoc clients list --include-diagnostics

Minimum valid call:
  wowdoc api lookup --client retail --name C_AuctionHouse.GetItemSearchResultInfo

MCP arguments:
  {"client":"retail","name":"C_AuctionHouse.GetItemSearchResultInfo"}

Common error codes:
  client_required client_not_found source_not_found source_invalid ref_not_found git_unavailable_archive_failed capability_unavailable index_unavailable timeout unsupported_ref`,
		RunE: func(cmd *cobra.Command, args []string) error {
			if client == "" {
				return errors.New("client_required")
			}
			if name == "" {
				return errors.New("name_required")
			}
			repo, idx, err := loadRepoIndex(client, ref, sourceRoot, sourcePath)
			if err != nil {
				return err
			}
			env := tools.LookupBlizzardAPI(repo, idx, tools.LookupAPIOptions{
				Name:          name,
				Exact:         exact,
				IncludeSafety: includeSafety,
			})
			env.Source.Client = client
			if env.Source.RequestedRef == "" {
				env.Source.RequestedRef = ref
			}
			env.Source.Version = repo.Version
			env.Source.Path = repo.Path
			return json.NewEncoder(cmd.OutOrStdout()).Encode(env)
		},
	}
	lookup.Flags().StringVar(&client, "client", "", "source client alias")
	lookup.Flags().StringVar(&ref, "ref", "", "branch, tag, or commit")
	lookup.Flags().StringVar(&name, "name", "", "API name")
	lookup.Flags().BoolVar(&exact, "exact", false, "require exact API name; default false uses fuzzy substring lookup")
	lookup.Flags().BoolVar(&includeSafety, "include-safety", true, "include safety metadata")
	lookup.Flags().StringVar(&sourceRoot, "source-root", "", "source cache root")
	lookup.Flags().StringVar(&sourcePath, "source-path", "", "explicit source checkout path")
	api.AddCommand(lookup)

	var searchClient, searchRef, query, searchType, searchSafety, searchScenario, searchSourceRoot, searchSourcePath string
	var searchLimit int
	var includeUnsafeOnly bool
	search := &cobra.Command{
		Use:   "search",
		Short: "Search Blizzard API symbols.",
		Long: sourceBackedHelp(
			"Search Blizzard API symbols.",
			"  --query QUERY",
			"wowdoc api search --client retail --query Auction",
			`{"client":"retail","query":"Auction"}`,
		),
		RunE: func(cmd *cobra.Command, args []string) error {
			if searchClient == "" {
				return errors.New("client_required")
			}
			if query == "" {
				return errors.New("query_required")
			}
			repo, idx, err := loadRepoIndex(searchClient, searchRef, searchSourceRoot, searchSourcePath)
			if err != nil {
				return err
			}
			env := tools.SearchBlizzardAPI(repo, idx, tools.APISearchOptions{
				Query:             query,
				Type:              searchType,
				Limit:             searchLimit,
				Safety:            searchSafety,
				Scenario:          searchScenario,
				IncludeUnsafeOnly: includeUnsafeOnly,
			})
			return json.NewEncoder(cmd.OutOrStdout()).Encode(env)
		},
	}
	search.Flags().StringVar(&searchClient, "client", "", "source client alias")
	search.Flags().StringVar(&searchRef, "ref", "", "branch, tag, or commit")
	search.Flags().StringVar(&query, "query", "", "search query")
	search.Flags().StringVar(&searchType, "type", "", "API entry type filter")
	search.Flags().IntVar(&searchLimit, "limit", 20, "maximum results")
	search.Flags().StringVar(&searchSafety, "safety", "", "safety classification filter")
	search.Flags().StringVar(&searchScenario, "scenario", "", "usage scenario")
	search.Flags().BoolVar(&includeUnsafeOnly, "include-unsafe-only", false, "only include unsafe API results")
	search.Flags().StringVar(&searchSourceRoot, "source-root", "", "source cache root")
	search.Flags().StringVar(&searchSourcePath, "source-path", "", "explicit source checkout path")
	api.AddCommand(search)

	var eventsClient, eventsRef, event, eventFilter, eventsSourceRoot, eventsSourcePath string
	events := &cobra.Command{
		Use:   "events",
		Short: "Get Blizzard API event payload documentation.",
		Long: sourceBackedHelp(
			"Get Blizzard API event payload documentation.",
			"  --event EVENT|list",
			"wowdoc api events --client retail --event AUCTION_HOUSE_SHOW",
			`{"client":"retail","event":"AUCTION_HOUSE_SHOW"}`,
		),
		RunE: func(cmd *cobra.Command, args []string) error {
			if eventsClient == "" {
				return errors.New("client_required")
			}
			if event == "" {
				return errors.New("event_required")
			}
			repo, idx, err := loadRepoIndex(eventsClient, eventsRef, eventsSourceRoot, eventsSourcePath)
			if err != nil {
				return err
			}
			env := tools.GetAPIEvents(repo, idx, tools.EventOptions{Event: event, Filter: eventFilter})
			return json.NewEncoder(cmd.OutOrStdout()).Encode(env)
		},
	}
	events.Flags().StringVar(&eventsClient, "client", "", "source client alias")
	events.Flags().StringVar(&eventsRef, "ref", "", "branch, tag, or commit")
	events.Flags().StringVar(&event, "event", "", "event name or list")
	events.Flags().StringVar(&eventFilter, "filter", "", "event name/payload filter")
	events.Flags().StringVar(&eventsSourceRoot, "source-root", "", "source cache root")
	events.Flags().StringVar(&eventsSourcePath, "source-path", "", "explicit source checkout path")
	api.AddCommand(events)

	var namespaceClient, namespaceRef, namespace, namespaceSourceRoot, namespaceSourcePath string
	namespaceCmd := &cobra.Command{
		Use:   "namespace",
		Short: "Get Blizzard API namespace documentation.",
		Long: sourceBackedHelp(
			"Get Blizzard API namespace documentation.",
			"  --namespace NAMESPACE|list",
			"wowdoc api namespace --client retail --namespace C_AuctionHouse",
			`{"client":"retail","namespace":"C_AuctionHouse"}`,
		),
		RunE: func(cmd *cobra.Command, args []string) error {
			if namespaceClient == "" {
				return errors.New("client_required")
			}
			if namespace == "" {
				return errors.New("namespace_required")
			}
			repo, idx, err := loadRepoIndex(namespaceClient, namespaceRef, namespaceSourceRoot, namespaceSourcePath)
			if err != nil {
				return err
			}
			env := tools.GetAPINamespace(repo, idx, namespace)
			return json.NewEncoder(cmd.OutOrStdout()).Encode(env)
		},
	}
	namespaceCmd.Flags().StringVar(&namespaceClient, "client", "", "source client alias")
	namespaceCmd.Flags().StringVar(&namespaceRef, "ref", "", "branch, tag, or commit")
	namespaceCmd.Flags().StringVar(&namespace, "namespace", "", "namespace name or list")
	namespaceCmd.Flags().StringVar(&namespaceSourceRoot, "source-root", "", "source cache root")
	namespaceCmd.Flags().StringVar(&namespaceSourcePath, "source-path", "", "explicit source checkout path")
	api.AddCommand(namespaceCmd)

	var deprecationClient, deprecationRef, luaCode, deprecationSourceRoot, deprecationSourcePath string
	deprecation := &cobra.Command{
		Use:   "deprecation",
		Short: "Check Lua code for deprecated API usage.",
		Long: sourceBackedHelp(
			"Check Lua code for deprecated API usage.",
			"  --lua-code LUA_CODE",
			`wowdoc api deprecation --client retail --lua-code "GetContainerItemInfo(0, 1)"`,
			`{"client":"retail","luaCode":"GetContainerItemInfo(0, 1)"}`,
		),
		RunE: func(cmd *cobra.Command, args []string) error {
			if deprecationClient == "" {
				return errors.New("client_required")
			}
			if luaCode == "" {
				return errors.New("lua_code_required")
			}
			repo, idx, err := loadRepoIndex(deprecationClient, deprecationRef, deprecationSourceRoot, deprecationSourcePath)
			if err != nil {
				return err
			}
			env := tools.CheckAPIDeprecationWithIndex(repo, idx, luaCode)
			return json.NewEncoder(cmd.OutOrStdout()).Encode(env)
		},
	}
	deprecation.Flags().StringVar(&deprecationClient, "client", "", "source client alias")
	deprecation.Flags().StringVar(&deprecationRef, "ref", "", "branch, tag, or commit")
	deprecation.Flags().StringVar(&luaCode, "lua-code", "", "Lua code to inspect")
	deprecation.Flags().StringVar(&deprecationSourceRoot, "source-root", "", "source cache root")
	deprecation.Flags().StringVar(&deprecationSourcePath, "source-path", "", "explicit source checkout path")
	api.AddCommand(deprecation)

	var migrationClient, migrationRef, oldFunction, migrationSourceRoot, migrationSourcePath string
	migration := &cobra.Command{
		Use:   "migration",
		Short: "Suggest a migration for an old API function.",
		Long: sourceBackedHelp(
			"Suggest a migration for an old API function.",
			"  --old-function FUNCTION_NAME",
			"wowdoc api migration --client retail --old-function GetContainerItemInfo",
			`{"client":"retail","oldFunction":"GetContainerItemInfo"}`,
		),
		RunE: func(cmd *cobra.Command, args []string) error {
			if migrationClient == "" {
				return errors.New("client_required")
			}
			if oldFunction == "" {
				return errors.New("old_function_required")
			}
			repo, idx, err := loadRepoIndex(migrationClient, migrationRef, migrationSourceRoot, migrationSourcePath)
			if err != nil {
				return err
			}
			env := tools.SuggestAPIMigrationWithIndex(repo, idx, oldFunction)
			return json.NewEncoder(cmd.OutOrStdout()).Encode(env)
		},
	}
	migration.Flags().StringVar(&migrationClient, "client", "", "source client alias")
	migration.Flags().StringVar(&migrationRef, "ref", "", "branch, tag, or commit")
	migration.Flags().StringVar(&oldFunction, "old-function", "", "old API function")
	migration.Flags().StringVar(&migrationSourceRoot, "source-root", "", "source cache root")
	migration.Flags().StringVar(&migrationSourcePath, "source-path", "", "explicit source checkout path")
	api.AddCommand(migration)

	var safetyClient, safetyRef, symbol, scenario, safetySourceRoot, safetySourcePath string
	safety := &cobra.Command{
		Use:   "safety",
		Short: "Explain safety metadata for a Blizzard API symbol.",
		Long: sourceBackedHelp(
			"Explain safety metadata for a Blizzard API symbol.",
			"  --symbol API_SYMBOL",
			"wowdoc api safety --client retail --symbol Button.SetText --scenario combat",
			`{"client":"retail","symbol":"Button.SetText","scenario":"combat"}`,
		),
		RunE: func(cmd *cobra.Command, args []string) error {
			if safetyClient == "" {
				return errors.New("client_required")
			}
			if symbol == "" {
				return errors.New("symbol_required")
			}
			repo, idx, err := loadRepoIndex(safetyClient, safetyRef, safetySourceRoot, safetySourcePath)
			if err != nil {
				return err
			}
			env := tools.ExplainAPISafety(repo, idx, symbol, scenario)
			return json.NewEncoder(cmd.OutOrStdout()).Encode(env)
		},
	}
	safety.Flags().StringVar(&safetyClient, "client", "", "source client alias")
	safety.Flags().StringVar(&safetyRef, "ref", "", "branch, tag, or commit")
	safety.Flags().StringVar(&symbol, "symbol", "", "API symbol")
	safety.Flags().StringVar(&scenario, "scenario", "", "usage scenario")
	safety.Flags().StringVar(&safetySourceRoot, "source-root", "", "source cache root")
	safety.Flags().StringVar(&safetySourcePath, "source-path", "", "explicit source checkout path")
	api.AddCommand(safety)
	return api
}

func frameXMLCommand() *cobra.Command {
	framexml := &cobra.Command{Use: "framexml"}
	var client, ref, query, filePattern, sourceRoot, sourcePath string
	var limit, contextLines int
	search := &cobra.Command{
		Use:   "search",
		Short: "Search FrameXML Lua/XML sources.",
		Long: sourceBackedHelp(
			"Search FrameXML Lua/XML sources.",
			"  --query QUERY",
			"wowdoc framexml search --client retail --query SecureActionButtonTemplate",
			`{"client":"retail","query":"SecureActionButtonTemplate"}`,
		),
		RunE: func(cmd *cobra.Command, args []string) error {
			if client == "" {
				return errors.New("client_required")
			}
			if query == "" {
				return errors.New("query_required")
			}
			if limit == 0 {
				limit = 15
			}
			if contextLines == 0 {
				contextLines = 3
			}
			repo, idx, err := loadRepoIndex(client, ref, sourceRoot, sourcePath)
			if err != nil {
				return err
			}
			env := tools.SearchFrameXML(repo, idx, tools.FrameXMLSearchOptions{
				Query:        query,
				FilePattern:  filePattern,
				ContextLines: contextLines,
				MaxResults:   limit,
			})
			return json.NewEncoder(cmd.OutOrStdout()).Encode(env)
		},
	}
	search.Flags().StringVar(&client, "client", "", "source client alias")
	search.Flags().StringVar(&ref, "ref", "", "branch, tag, or commit")
	search.Flags().StringVar(&query, "query", "", "search query")
	search.Flags().StringVar(&filePattern, "file-pattern", "", "file path/name filter")
	search.Flags().IntVar(&contextLines, "context-lines", 3, "context lines before and after matches")
	search.Flags().IntVar(&limit, "limit", 15, "maximum results")
	search.Flags().StringVar(&sourceRoot, "source-root", "", "source cache root")
	search.Flags().StringVar(&sourcePath, "source-path", "", "explicit source checkout path")
	framexml.AddCommand(search)
	return framexml
}

func widgetCommand() *cobra.Command {
	widget := &cobra.Command{Use: "widget"}
	var client, ref, widgetType, sourceRoot, sourcePath string
	api := &cobra.Command{
		Use:   "api",
		Short: "Get widget API documentation.",
		Long: sourceBackedHelp(
			"Get widget API documentation.",
			"  --widget-type WIDGET_TYPE|list",
			"wowdoc widget api --client retail --widget-type Button",
			`{"client":"retail","widgetType":"Button"}`,
		),
		RunE: func(cmd *cobra.Command, args []string) error {
			if client == "" {
				return errors.New("client_required")
			}
			if widgetType == "" {
				return errors.New("widget_type_required")
			}
			repo, idx, err := loadRepoIndex(client, ref, sourceRoot, sourcePath)
			if err != nil {
				return err
			}
			env := tools.GetWidgetAPI(repo, idx, widgetType)
			return json.NewEncoder(cmd.OutOrStdout()).Encode(env)
		},
	}
	api.Flags().StringVar(&client, "client", "", "source client alias")
	api.Flags().StringVar(&ref, "ref", "", "branch, tag, or commit")
	api.Flags().StringVar(&widgetType, "widget-type", "", "widget type or list")
	api.Flags().StringVar(&sourceRoot, "source-root", "", "source cache root")
	api.Flags().StringVar(&sourcePath, "source-path", "", "explicit source checkout path")
	widget.AddCommand(api)
	return widget
}

func cvarCommand() *cobra.Command {
	cvar := &cobra.Command{Use: "cvar"}
	var client, ref, name, sourceRoot, sourcePath string
	var detail bool
	lookup := &cobra.Command{
		Use:   "lookup",
		Short: "Lookup CVar documentation.",
		Long: sourceBackedHelp(
			"Lookup CVar documentation.",
			"  --name CVAR_NAME|list",
			"wowdoc cvar lookup --client retail --name graphicsQuality",
			`{"client":"retail","name":"graphicsQuality"}`,
		),
		RunE: func(cmd *cobra.Command, args []string) error {
			if client == "" {
				return errors.New("client_required")
			}
			if name == "" {
				return errors.New("name_required")
			}
			repo, idx, err := loadRepoIndex(client, ref, sourceRoot, sourcePath)
			if err != nil {
				return err
			}
			env := tools.LookupCVar(repo, idx, tools.CVarLookupOptions{Name: name, Detail: detail})
			return json.NewEncoder(cmd.OutOrStdout()).Encode(env)
		},
	}
	lookup.Flags().StringVar(&client, "client", "", "source client alias")
	lookup.Flags().StringVar(&ref, "ref", "", "branch, tag, or commit")
	lookup.Flags().StringVar(&name, "name", "", "CVar name or list")
	lookup.Flags().BoolVar(&detail, "detail", false, "include default value and description")
	lookup.Flags().StringVar(&sourceRoot, "source-root", "", "source cache root")
	lookup.Flags().StringVar(&sourcePath, "source-path", "", "explicit source checkout path")
	cvar.AddCommand(lookup)
	return cvar
}

func constantsCommand() *cobra.Command {
	constants := &cobra.Command{Use: "constants"}
	var client, ref, name, filter, kind, sourceRoot, sourcePath string
	var limit int
	get := &cobra.Command{
		Use:   "get",
		Short: "Get WoW constants and enums.",
		Long: sourceBackedHelp(
			"Get WoW constants and enums.",
			"  --name CONSTANT_NAME|list",
			"wowdoc constants get --client retail --name list",
			`{"client":"retail","name":"list"}`,
		),
		RunE: func(cmd *cobra.Command, args []string) error {
			if client == "" {
				return errors.New("client_required")
			}
			if name == "" {
				return errors.New("name_required")
			}
			repo, idx, err := loadRepoIndex(client, ref, sourceRoot, sourcePath)
			if err != nil {
				return err
			}
			env := tools.GetWowConstants(repo, idx, tools.ConstantsOptions{Name: name, Filter: filter, Kind: kind, Limit: limit})
			return json.NewEncoder(cmd.OutOrStdout()).Encode(env)
		},
	}
	get.Flags().StringVar(&client, "client", "", "source client alias")
	get.Flags().StringVar(&ref, "ref", "", "branch, tag, or commit")
	get.Flags().StringVar(&name, "name", "", "constant name or list")
	get.Flags().StringVar(&filter, "filter", "", "constant name/type/path filter")
	get.Flags().StringVar(&kind, "kind", "", "constant kind/type filter")
	get.Flags().IntVar(&limit, "limit", 0, "maximum results")
	get.Flags().StringVar(&sourceRoot, "source-root", "", "source cache root")
	get.Flags().StringVar(&sourcePath, "source-path", "", "explicit source checkout path")
	constants.AddCommand(get)
	return constants
}

func mixinCommand() *cobra.Command {
	mixin := &cobra.Command{Use: "mixin"}
	var client, ref, name, kind, sourceRoot, sourcePath string
	var limit int
	find := &cobra.Command{
		Use:   "find",
		Short: "Find mixins and templates in FrameXML.",
		Long: sourceBackedHelp(
			"Find mixins and templates in FrameXML.",
			"  --name MIXIN_OR_TEMPLATE_NAME",
			"wowdoc mixin find --client retail --name SecureActionButtonTemplate",
			`{"client":"retail","name":"SecureActionButtonTemplate"}`,
		),
		RunE: func(cmd *cobra.Command, args []string) error {
			if client == "" {
				return errors.New("client_required")
			}
			if name == "" {
				return errors.New("name_required")
			}
			if limit == 0 {
				limit = 25
			}
			repo, idx, err := loadRepoIndex(client, ref, sourceRoot, sourcePath)
			if err != nil {
				return err
			}
			env := tools.FindMixinTemplate(repo, idx, name, kind, limit)
			return json.NewEncoder(cmd.OutOrStdout()).Encode(env)
		},
	}
	find.Flags().StringVar(&client, "client", "", "source client alias")
	find.Flags().StringVar(&ref, "ref", "", "branch, tag, or commit")
	find.Flags().StringVar(&name, "name", "", "mixin or template name")
	find.Flags().StringVar(&kind, "kind", "", "mixin or template")
	find.Flags().IntVar(&limit, "limit", 25, "maximum results")
	find.Flags().StringVar(&sourceRoot, "source-root", "", "source cache root")
	find.Flags().StringVar(&sourcePath, "source-path", "", "explicit source checkout path")
	mixin.AddCommand(find)
	return mixin
}

func tocCommand() *cobra.Command {
	toc := &cobra.Command{Use: "toc"}
	var tocContent, tocPath, addonName, client, ref, sourceRoot, sourcePath string
	validate := &cobra.Command{
		Use:   "validate",
		Short: "Validate TOC metadata.",
		Long:  tocHelp(),
		RunE: func(cmd *cobra.Command, args []string) error {
			env := tools.ValidateTOC(tocContent, tocPath, addonName)
			if client != "" {
				repo, _, err := loadRepoIndex(client, ref, sourceRoot, sourcePath)
				if err != nil {
					return err
				}
				env = tools.ValidateTOCWithOptions(tocContent, tocPath, addonName, tools.TOCValidationOptions{SourceVersion: repo.Version})
				env.Source = contracts.SourceTransparency{
					Client:       repo.Alias,
					RequestedRef: repo.RequestedRef,
					ResolvedRef:  repo.ResolvedRef,
					Version:      repo.Version,
					Path:         repo.Path,
				}
				env.Diagnostics = append(env.Diagnostics, repo.Diagnostics...)
			}
			return json.NewEncoder(cmd.OutOrStdout()).Encode(env)
		},
	}
	validate.Flags().StringVar(&tocContent, "toc-content", "", "TOC file content")
	validate.Flags().StringVar(&tocPath, "toc-path", "", "TOC file path")
	validate.Flags().StringVar(&addonName, "addon-name", "", "addon name")
	validate.Flags().StringVar(&client, "client", "", "source client alias")
	validate.Flags().StringVar(&ref, "ref", "", "branch, tag, or commit")
	validate.Flags().StringVar(&sourceRoot, "source-root", "", "source cache root")
	validate.Flags().StringVar(&sourcePath, "source-path", "", "explicit source checkout path")
	toc.AddCommand(validate)
	return toc
}

func clientsCommand() *cobra.Command {
	clients := &cobra.Command{Use: "clients"}
	var includeDiagnostics bool
	var includeRefs bool
	var sourceRoot string
	list := &cobra.Command{Use: "list", Long: clientsListHelp(), RunE: func(cmd *cobra.Command, args []string) error {
		root := sourceRoot
		if root == "" {
			defaultRoot, err := stdio.DefaultSourceRoot()
			if err != nil {
				return err
			}
			root = defaultRoot
		}
		repos, err := detectSourceRoot(root)
		if err != nil {
			return err
		}
		_, refs := defaultSourceMaps()
		return json.NewEncoder(cmd.OutOrStdout()).Encode(tools.ListClients(repos, tools.ListClientsOptions{
			IncludeDiagnostics: includeDiagnostics,
			IncludeRefs:        includeRefs,
			DefaultRefs:        refs,
			DefaultRef:         "latest",
		}))
	}}
	list.Flags().BoolVar(&includeDiagnostics, "include-diagnostics", false, "include invalid source diagnostics")
	list.Flags().BoolVar(&includeRefs, "include-refs", false, "include default/requested/resolved refs")
	list.Flags().StringVar(&sourceRoot, "source-root", "", "source cache root")
	clients.AddCommand(list)
	return clients
}

func sourcesCommand() *cobra.Command {
	sources := &cobra.Command{Use: "sources"}
	var client string
	var includeVersion bool
	var sourceRoot string
	refsCmd := &cobra.Command{
		Use:   "refs",
		Short: "Inspect configured remote source refs.",
		Long: `Inspect configured remote source refs.

Optional:
  --client CLIENT          Limit output to one configured client alias.
  --include-version        Resolve the configured source and include detected version.
  --source-root ROOT       Source cache root. Defaults to <exe-dir>/sources.

Minimum valid call:
  wowdoc sources refs --client retail

MCP arguments:
  {"client":"retail","includeVersion":true}`,
		RunE: func(cmd *cobra.Command, args []string) error {
			repos, refs := defaultSourceMaps()
			if client != "" {
				if repo := repos[client]; repo != "" {
					repos = map[string]string{client: repo}
					refs = map[string]string{client: refs[client]}
				} else {
					return json.NewEncoder(cmd.OutOrStdout()).Encode(contracts.Envelope[tools.RemoteRefsData]{
						OK:   false,
						Data: tools.RemoteRefsData{Clients: []tools.RemoteRefInfo{}},
						Error: &contracts.ToolError{
							Code:    contracts.ErrClientNotFound,
							Message: "configured source client not found",
						},
					})
				}
			}
			resolver := func(alias string) (string, string, error) {
				repoPath, _, _, _, err := resolveSourcePath(alias, "", sourceRoot, "")
				if err != nil {
					return "", "", err
				}
				repo := analyze.DetectRepository(repoPath, alias)
				if includeVersion {
					return repo.Version, repo.Path, nil
				}
				return "", repo.Path, nil
			}
			return json.NewEncoder(cmd.OutOrStdout()).Encode(tools.InspectRemoteRefs(repos, refs, cliSourceGit, resolver))
		},
	}
	refsCmd.Flags().StringVar(&client, "client", "", "configured source client alias")
	refsCmd.Flags().BoolVar(&includeVersion, "include-version", false, "resolve source and include detected version")
	refsCmd.Flags().StringVar(&sourceRoot, "source-root", "", "source cache root")
	sources.AddCommand(refsCmd)
	return sources
}

func mcpCommand() *cobra.Command {
	mcp := &cobra.Command{Use: "mcp"}
	var sourceRoot string
	stdioCmd := &cobra.Command{
		Use:   "stdio",
		Short: "Run the MCP server over stdin/stdout.",
		Long:  stdioHelp(),
		RunE: func(cmd *cobra.Command, args []string) error {
			if sourceRoot == "" {
				defaultRoot, err := stdio.DefaultSourceRoot()
				if err != nil {
					return err
				}
				sourceRoot = defaultRoot
			}
			return stdio.Run(context.Background(), defaultStdioOptions(sourceRoot), nil)
		},
	}
	stdioCmd.Flags().StringVar(&sourceRoot, "source-root", "", "source cache root")
	mcp.AddCommand(stdioCmd)
	return mcp
}

func defaultStdioOptions(sourceRoot string) wowmcp.ServerOptions {
	return stdio.DefaultServerOptions("wowdoc", sourceRoot)
}

func resolveSourcePath(client, ref, sourceRoot, sourcePath string) (string, string, string, []contracts.Diagnostic, error) {
	if sourcePath != "" {
		return sourcePath, ref, ref, nil, nil
	}
	repos, refs := defaultSourceMaps()
	if sourceRoot != "" {
		if _, ok := repos[client]; ok {
			return resolveConfiguredSource(client, ref, sourceRoot, repos, refs)
		}
		return filepath.Join(sourceRoot, client), ref, ref, nil, nil
	}
	root, err := stdio.DefaultSourceRoot()
	if err != nil {
		return "", "", "", nil, err
	}
	if _, ok := repos[client]; ok {
		return resolveConfiguredSource(client, ref, root, repos, refs)
	}
	return filepath.Join(root, client), ref, ref, nil, nil
}

func resolveConfiguredSource(client, ref, root string, repos, refs map[string]string) (string, string, string, []contracts.Diagnostic, error) {
	manager := source.NewManager(source.Options{
		Root:              root,
		AllowArbitraryRef: true,
		DefaultRefs:       refs,
		Repos:             repos,
		Git:               cliSourceGit,
		Archive:           cliSourceArchive,
	})
	resolved, err := manager.ResolveSource(client, ref)
	if err != nil {
		return "", "", "", nil, err
	}
	return resolved.CheckoutDir, resolved.Requested, resolved.Resolved, resolved.Diagnostics, nil
}

func loadRepoIndex(client, ref, sourceRoot, sourcePath string) (analyze.Repository, *analyze.Index, error) {
	repoPath, requestedRef, resolvedRef, diagnostics, err := resolveSourcePath(client, ref, sourceRoot, sourcePath)
	if err != nil {
		return analyze.Repository{}, nil, err
	}
	repo := analyze.DetectRepository(repoPath, client)
	repo.RequestedRef = requestedRef
	repo.ResolvedRef = resolvedRef
	repo.Diagnostics = append(repo.Diagnostics, diagnostics...)
	idx, err := analyze.BuildIndex(repo)
	if err != nil {
		return analyze.Repository{}, nil, err
	}
	return repo, idx, nil
}

func defaultSourceMaps() (map[string]string, map[string]string) {
	repos := map[string]string{}
	refs := map[string]string{}
	for _, seed := range config.DefaultSourceSeeds() {
		repos[seed.Alias] = seed.Repo
		refs[seed.Alias] = seed.Ref
	}
	return repos, refs
}

type execGit struct{}

func (execGit) Run(args ...string) error {
	return exec.Command("git", args...).Run()
}

func (execGit) Output(args ...string) ([]byte, error) {
	return exec.Command("git", args...).Output()
}

func detectSourceRoot(root string) ([]analyze.Repository, error) {
	entries, err := os.ReadDir(root)
	if err != nil {
		if os.IsNotExist(err) {
			return []analyze.Repository{}, nil
		}
		return nil, err
	}
	repos := make([]analyze.Repository, 0, len(entries))
	for _, entry := range entries {
		if !entry.IsDir() || isInternalSourceCacheDir(entry.Name()) {
			continue
		}
		alias := entry.Name()
		repos = append(repos, analyze.DetectRepository(filepath.Join(root, alias), alias))
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
