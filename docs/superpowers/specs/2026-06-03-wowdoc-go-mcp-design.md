# wowdoc Go MCP Design

## Goal

Build a Go implementation inspired by `netouss/wow-addon-dev-mcp`, rooted at
`D:\Code\MCP\wowdoc`. The new project exposes World of Warcraft addon
development analysis through:

- a local CLI binary,
- a local MCP stdio server,
- a dedicated HTTP MCP server binary.

The TypeScript reference implementation lives at
`D:\Code\Analyze\wow-addon-dev-mcp` and is used only as an oracle/reference.

The project must support multiple WoW UI source clients, multiple source refs
or commits per client, and future source repositories without hard-coding the
client set as the final truth.

## Build Targets

The project has two isolated build targets.

```text
cmd/
  wowdoc/          local binary: CLI + MCP stdio
  wowdoc-server/   server binary: MCP HTTP only
```

Build commands:

```powershell
go build -o dist/wowdoc.exe ./cmd/wowdoc
go build -o dist/wowdoc-server.exe ./cmd/wowdoc-server
```

`wowdoc` must not import server-only packages. `wowdoc-server` must not reuse
CLI handlers. They share only core libraries such as source management,
analysis, tool business logic, and the MCP JSON-RPC server.

## Package Layout

```text
cmd/
  wowdoc/
  wowdoc-server/
internal/
  shared/
    analyze/
    source/
    tools/
    mcp/
    config/
  cli/
  stdio/
  http/
docs/
```

- `internal/shared/analyze`: validates source repositories, identifies clients,
  builds indexes, and answers query-level requests. It has no CLI, stdio, or
  HTTP policy.
- `internal/shared/source`: manages default repo seeds, explicit local paths,
  git clone/fetch, archive fallback, ref resolution, checkout cache, and disk
  layout.
- `internal/shared/tools`: implements tool behavior over Go structs. It does
  not know about CLI, stdio, or HTTP.
- `internal/shared/mcp`: shared MCP registration, schemas, envelopes, and the
  official Go MCP SDK integration used by both stdio and HTTP.
- `internal/shared/config`: shared config structs and defaults that are needed
  by more than one runtime. HTTP-only config lives under `internal/http`.
- `internal/cli`: CLI-only Cobra commands, command help, flag parsing, stdout
  formatting, and CLI runtime policy.
- `internal/stdio`: stdio-only MCP transport startup and stdio runtime policy.
- `internal/http`: HTTP-only config loading, HTTP runtime policy, health/help
  routes, source/index pools, singleflight, concurrency limits, and server MCP
  transport startup.

The boundary rule is strict: if code is used by CLI, stdio, and HTTP, it belongs
under `internal/shared`. If code is private to one surface, it belongs under
`internal/cli`, `internal/stdio`, or `internal/http`.

## MCP SDK Choice

The implementation should use the official Go MCP SDK where practical:

```text
github.com/modelcontextprotocol/go-sdk/mcp
github.com/modelcontextprotocol/go-sdk/jsonrpc
```

The SDK is preferred for protocol correctness, schema handling, transport
compatibility, and future MCP protocol updates. Any custom MCP wrapper must live
under `internal/shared/mcp` and remain a thin compatibility layer around the SDK
unless a documented SDK limitation requires otherwise.

The implementation plan must verify:

- stdio tool serving works with Codex/Claude-style clients,
- Streamable HTTP works for `/mcp`,
- notifications such as initialized notifications do not produce user-visible
  errors,
- structured tool results preserve both text content and structured content,
- tool schemas can express the shared `client` and `ref` arguments cleanly.

## Supported Tools

The MCP service exposes 15 tools:

- `list_clients`
- `lookup_blizzard_api`
- `search_blizzard_api`
- `get_api_namespace`
- `get_api_events`
- `search_framexml`
- `validate_toc`
- `check_api_deprecation`
- `suggest_api_migration`
- `get_wow_constants`
- `get_widget_api`
- `find_mixin_template`
- `lookup_cvar`
- `explain_api_safety`
- `inspect_remote_refs`

The Go implementation intentionally omits these TypeScript tools:

- `get_blizzard_addon`
- `scaffold_addon`
- `lint_addon_lua`

## Default Source Repositories

The default source seeds are:

- `wow-ui-source`
- `wow-ui-source@classic`
- `wow-ui-source@classic_ptr`
- `wow-ui-source@classic_titan`
- `wow-ui-source@ptr2`

These seeds are bootstrap defaults, not the complete universe of supported
clients. Any additional directory under the source root, or any configured
extra source, is passed through repository detection. Valid repositories become
available clients. Invalid directories are not analyzed and appear in
diagnostics.

## Source Repository Detection

`analyze` must not assume that every directory is a WoW UI source repository.
Detection uses structural checks before scanning:

- `Interface/` exists.
- At least one source-list or addon structure exists:
  `Interface/ui-code-list.txt`, `Interface/ui-toc-list.txt`,
  `Interface/ui-gen-addon-list.txt`, or `Interface/AddOns/`.
- `version.txt` exists, or a version can be inferred from repository metadata.
- Capabilities are detected independently:
  `Blizzard_APIDocumentationGenerated`, FrameXML Lua/XML files, widget docs,
  constants/enums, mixin/template definitions, and CVar references.

Invalid directories produce diagnostics with the path and missing structural
signals. Partially valid repositories are registered with only the capabilities
they support.

## Source Acquisition

`source` prefers `git`:

```text
git clone --mirror <repo>
git fetch
git checkout/worktree <ref>
```

If local `git` is unavailable, it falls back to GitHub archive downloads:

```text
download archive for branch/tag/commit
extract into source cache
record repo, requested ref, and resolved metadata when available
```

Fallback limitations are explicit in diagnostics:

- archive fallback is not incremental,
- branch archives may need periodic redownload,
- commit archives are immutable and may be cached permanently,
- resolved commit may be unknown for some non-git archive paths.

## Ref And Build Semantics

Every tool that queries source-backed data accepts:

```json
{
  "client": "retail",
  "ref": "main"
}
```

`client` is required except where the tool is source-independent, such as basic
TOC validation. `ref` is optional. When omitted, the runtime uses the client
default/latest ref.

The runtime resolves requests to a source instance:

```text
sourceInstanceKey = clientAlias + resolvedCommit
```

Responses include source transparency:

```json
{
  "source": {
    "client": "retail",
    "requestedRef": "main",
    "resolvedRef": "a1b2c3d...",
    "version": "12.0.0.x",
    "path": "..."
  }
}
```

Branch, tag, and commit refs are supported. Users can explicitly request old
builds or unusual refs. Default behavior always uses the latest/default ref
unless a ref is explicitly supplied.

## Disk Layout

Source directories are isolated by resolved commit so concurrent users do not
overwrite each other.

```text
sources/
  repos/
    classic.git/
    retail.git/
  checkouts/
    classic/
      1111111/
      2222222/
    retail/
      aaaaaaa/
  archives/
    classic/
      3333333/
```

The system must avoid a mutable global directory such as
`sources/wow-ui-source-classic` that is checked out to different refs per
request.

If user A requests `classic@1111111` and user B requests `classic@2222222`,
both requests bind to independent source instances and indexes.

## Caching And Concurrency

HTTP source and index caching is shared across users.

- `repo + ref` resolution uses singleflight.
- `client + resolvedCommit` checkout/archive acquisition uses singleflight.
- `client + resolvedCommit + indexKind` index construction uses singleflight.
- Concurrent requests for the same ref wait on the same work.
- Concurrent requests for different refs can proceed independently, subject to
  global limits.

HTTP keeps separate pools:

- `SourcePool`: validated source repositories by `client@resolvedCommit`.
- `IndexPool`: parsed indexes by `client@resolvedCommit`.

Pools are LRU-limited. In-flight contexts are not evicted. Pinned/default
contexts are retained preferentially. Disk source caches may outlive in-memory
indexes.

Configurable limits include maximum source contexts, maximum index contexts,
maximum concurrent source fetches, maximum concurrent index builds, request
timeouts, and optional cache pruning.

## CLI Runtime

`wowdoc` exposes CLI commands.

CLI behavior:

- CLI requires explicit `client` for source-backed tools.
- `ref` is optional and defaults to latest/default.
- If no `--source-root` or `--source-path` is specified, the runtime uses
  `<exe-dir>/sources`.
- If default sources are missing, the runtime clones or downloads the default
  seeds.
- If `git` is missing, archive fallback is used.
- CLI lazily loads the minimum needed index and exits.
- CLI does not read HTTP server YAML, expose HTTP, start health routes, or own
  stdio transport policy.

Representative commands:

```powershell
wowdoc clients list
wowdoc api lookup --client retail --name C_AuctionHouse.GetItemSearchResultInfo
wowdoc api lookup --client retail --ref 4f3a9c1 --name C_AuctionHouse.GetItemSearchResultInfo
wowdoc api safety --client retail --symbol Button.SetText --scenario combat
wowdoc framexml search --client retail --query SecureActionButtonTemplate
wowdoc toc validate --toc-path .\MyAddon.toc
wowdoc mcp stdio
```

## Stdio Runtime

`wowdoc mcp stdio` exposes local MCP over stdio.

Stdio behavior:

- Stdio uses shared MCP tool schemas from `internal/shared/mcp`.
- Stdio requires explicit `client` for source-backed tools.
- `ref` is optional and defaults to latest/default.
- If no source root is configured by flags/env, stdio uses `<exe-dir>/sources`.
- If default sources are missing, stdio uses the shared source manager to clone
  or download the default seeds.
- Stdio is long-lived and reuses loaded source and index contexts across calls.
- Stdio does not read HTTP server YAML, expose HTTP routes, or use HTTP health
  policy.

## HTTP Runtime

`wowdoc-server` exposes only MCP HTTP:

```text
wowdoc-server mcp http
```

HTTP behavior:

- Reads YAML config.
- Uses configured `sources.root`.
- If the directory is missing or default repos are missing, it clones/downloads
  the default seeds.
- Config can add extra repos and local paths.
- Extra directories under the source root are discovered and detected.
- Does not reuse CLI handlers.
- Exposes `/mcp`, `/health`, and `/help`.
- `/health` reports source discovery, clone/download status, client list,
  invalid directory diagnostics, source/index pool status, and recent errors.
- Supports multiple users querying different refs for the same client without
  source or index overwrite.

Example server config:

```yaml
server:
  host: 0.0.0.0
  port: 9789
  base_url: ""

sources:
  root: /srv/wowdoc/sources
  allow_arbitrary_ref: false
  default_ref: latest
  defaults:
    - alias: retail
      repo: https://github.com/Gethe/wow-ui-source.git
      ref: live
    - alias: classic
      repo: https://github.com/Gethe/wow-ui-source.git
      ref: classic
    - alias: classic-ptr
      repo: https://github.com/Gethe/wow-ui-source.git
      ref: classic_ptr
    - alias: classic-titan
      repo: https://github.com/Gethe/wow-ui-source.git
      ref: classic_titan
    - alias: ptr
      repo: https://github.com/Gethe/wow-ui-source.git
      ref: ptr2
    - alias: ptr2
      repo: https://github.com/Gethe/wow-ui-source.git
      ref: ptr2
  extra:
    - alias: old-classic
      repo: https://github.com/Gethe/wow-ui-source-classic.git
      ref: 4f3a9c1
    - alias: local-test
      path: /srv/custom/wow-ui-source

contexts:
  max_source_contexts: 8
  max_index_contexts: 4
  pinned:
    - retail
    - classic

limits:
  max_concurrent_source_fetches: 2
  max_concurrent_index_builds: 2
  request_timeout_seconds: 60

prepare:
  prewarm_on_start: true
  prewarm_clients:
    - retail
    - classic
```

Production HTTP should default to configured/default refs only. Arbitrary ref
fetching is configurable and should be disabled by default to prevent cache
abuse.

## Agent-Friendly CLI And Help

The CLI is primarily for agents, not manual human use. Help output must be
agent-friendly:

- Every command explains required fields, defaults, and source/ref semantics.
- Help includes "minimum valid call" examples.
- Help includes JSON-shaped MCP argument examples for equivalent tools.
- Help documents common error codes and the next action.
- Help avoids vague prose and exposes machine-usable names exactly.
- `clients list` is positioned as the first diagnostic command.

Example help sections:

```text
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

MCP arguments:
  {"client":"retail","name":"C_AuctionHouse.GetItemSearchResultInfo"}
```

Errors should use stable codes:

- `client_required`
- `client_not_found`
- `source_not_found`
- `source_invalid`
- `ref_not_found`
- `git_unavailable_archive_failed`
- `capability_unavailable`
- `index_unavailable`
- `timeout`
- `unsupported_ref`

## Tool Schemas

All source-backed tools accept `client` and optional `ref`.

`list_clients`:

- Input: `includeDiagnostics?`, `includeRefs?`
- Output: clients, capabilities, default ref, resolved commit, discovered
  source paths, and invalid directory diagnostics.

`lookup_blizzard_api`:

- Input: `client`, `ref?`, `name`, `exact?`, `includeSafety?`
- Output: function signature, arguments, returns, namespace, system, raw safety
  metadata, and classified safety.

`search_blizzard_api`:

- Input: `client`, `ref?`, `query`, `type?`, `limit?`, `safety?`,
  `scenario?`, `includeUnsafeOnly?`
- Output: matched functions/events/tables and optional safety summaries.

`get_api_namespace`:

- Input: `client`, `ref?`, `namespace`
- `namespace=list` lists namespaces.

`get_api_events`:

- Input: `client`, `ref?`, `event`, `filter?`
- `event=list` lists events.

`search_framexml`:

- Input: `client`, `ref?`, `query`, `filePattern?`, `contextLines?`,
  `maxResults?`
- Output: file paths, line numbers, and snippets.

`validate_toc`:

- Input: `tocContent?`, `tocPath?`, `client?`, `ref?`, `addonName?`
- Without a client it performs generic validation. With a client it can use
  source/version-aware Interface guidance.

`check_api_deprecation`:

- Input: `client`, `ref?`, `luaCode`
- Uses built-in migration knowledge and validates replacements against the
  selected client source.

`suggest_api_migration`:

- Input: `client`, `ref?`, `oldFunction`
- Must not recommend Retail-only replacements for Classic clients without a
  warning.

`get_wow_constants`:

- Input: `client`, `ref?`, `name`, `filter?`
- `name=list` lists constants/enums.

`get_widget_api`:

- Input: `client`, `ref?`, `widgetType`
- `widgetType=list` lists widget types.

`find_mixin_template`:

- Input: `client`, `ref?`, `name`, `kind?`, `limit?`

`lookup_cvar`:

- Input: `client`, `ref?`, `name`, `detail?`

`explain_api_safety`:

- Input: `client`, `ref?`, `symbol`, `scenario?`
- Output: raw safety metadata, effective risk level, field-level secret status,
  scenario-specific explanation, and addon advice.

`inspect_remote_refs`:

- Input: `client?`, `includeVersion?`
- Output: configured source aliases, repository URLs, configured branch refs,
  remote commits when `git ls-remote` is available, fallback status when git is
  unavailable and archive/source acquisition is used, and detected source
  versions when requested.

## Safety And Secret Value Analysis

Retail API documentation includes security and secret-value metadata that can
affect addon behavior in combat, tainted execution paths, hidden aura cases,
unit cast state, cooldown restrictions, chat lockdown, and protected UI
mutation. The Go parser must preserve and classify these fields instead of
dropping them.

Known metadata to parse includes:

- `SecretArguments`
- `SecretArgumentsAddAspect`
- `SecretReturnsForAspect`
- `SecretWhenCooldownsRestricted`
- `SecretWhenUnitSpellCastRestricted`
- `SecretInChatMessagingLockdown`
- `RequiresNonSecretAura`
- `RequiresRestrictedAbbreviationBreakpoints`
- `IsProtectedFunction`
- `ConstSecretAccessor`
- `ReturnsNeverSecret`
- `NeverSecret`
- `ConditionalSecret`
- `IsForbidden`
- `SetForbidden`
- `IsPreventingSecretValues`
- `SecretWrapperConstants` values:
  `NeverSecret`, `AlwaysSecret`, `ContextuallySecret`
- restricted argument/return types such as
  `UnitTokenRestrictedForAddOns` and `UnitTokenPvPRestrictedForAddOns`

The safety subsystem has:

```text
SafetyParser
SafetyClassifier
SafetyIndex
ScenarioEvaluator
```

Risk levels:

- `safe`
- `never_secret`
- `taint_sensitive`
- `conditional_secret`
- `secret`
- `protected`
- `forbidden`
- `unknown`

Classification rules:

- `IsForbidden` or forbidden frame/object APIs imply `forbidden`.
- `IsProtectedFunction = true` implies at least `protected`.
- `SecretArguments = NotAllowed` implies `secret`.
- `SecretArguments = AllowedWhenUntainted` implies `taint_sensitive`.
- `ConditionalSecret = true` implies `conditional_secret`.
- Any `SecretWhen* = true` implies `conditional_secret`.
- `RequiresNonSecretAura = true` implies `conditional_secret`.
- Secret argument or return aspects imply `conditional_secret` or
  `taint_sensitive`.
- `AlwaysSecret` implies `secret`.
- `ContextuallySecret` implies `conditional_secret`.
- `NeverSecret`, `ReturnsNeverSecret`, and field-level `NeverSecret` are
  preserved as safe field-level facts, but do not override other function-level
  risks.

Supported scenario names:

- `default`
- `combat`
- `tainted`
- `secure_frame`
- `aura`
- `unit_cast`
- `cooldown`
- `chat_lockdown`
- `pvp`

`lookup_blizzard_api` returns safety by default. If a result is not `safe`, an
agent can call `explain_api_safety` for scenario-specific guidance.

Example safety response:

```json
{
  "level": "conditional_secret",
  "taint": {
    "mode": "allowed_when_untainted",
    "requiresUntaintedCaller": true
  },
  "secret": {
    "secrecyLevel": "ContextuallySecret",
    "conditions": ["unit_spell_cast_restricted"],
    "aspects": []
  },
  "protected": {
    "isProtectedFunction": false,
    "combatMutationRisk": false
  },
  "fields": [
    {"name": "target", "conditionalSecret": true},
    {"name": "castBarID", "neverSecret": true}
  ]
}
```

Example scenario explanation:

```json
{
  "scenario": "unit_cast",
  "effectiveLevel": "conditional_secret",
  "why": [
    "SecretWhenUnitSpellCastRestricted is true",
    "return field target is ConditionalSecret",
    "castBarID is NeverSecret and can be used safely"
  ],
  "addonAdvice": [
    "Treat target as possibly unavailable or secret.",
    "Do not use this value to mutate secure UI during combat.",
    "Check nil and secret-safe fallbacks."
  ]
}
```

## Error Handling

Tool handlers return structured envelopes:

```json
{
  "ok": false,
  "source": {},
  "error": {
    "code": "capability_unavailable",
    "message": "client classic@... does not contain Blizzard_APIDocumentationGenerated"
  },
  "diagnostics": []
}
```

Capability absence is not a crash. A Classic or old source without a modern API
doc tree is a valid client with missing capabilities.

## Performance Targets

- CLI loads only indexes required by the command.
- stdio reuses indexes in the long-lived process.
- HTTP discovers on startup and lazily or eagerly builds indexes according to
  config.
- File scanning uses bounded worker pools.
- Query-time index builds are singleflighted.
- Text search should start with a simple reliable line index or inverted index;
  no database materialization is part of this design.
- HTTP requests for different `client@commit` source instances can run
  concurrently.
- HTTP requests for the same source/index wait on shared work.

## Test Plan

Unit and integration tests cover:

- repository detection does not misclassify arbitrary directories,
- default repo seeds can be configured or overridden,
- git acquisition path,
- archive fallback when git is unavailable,
- branch/tag/commit ref resolution,
- isolated disk layout for `classic@commitA` and `classic@commitB`,
- same-ref concurrent requests clone/index once,
- different-ref concurrent requests do not overwrite each other,
- CLI/stdio require explicit `client`,
- HTTP does not route through CLI handlers,
- `list_clients` reports valid clients and invalid directory diagnostics,
- capability-unavailable responses for old or partial sources,
- API parser parity against the TypeScript reference for lookup, search, events,
  namespaces, constants, widgets, mixins, CVars, and migration checks,
- safety metadata parsing for protected, secret, conditional secret, never
  secret, taint-sensitive, and scenario-specific cases,
- agent-friendly help includes required args, default ref behavior, MCP argument
  examples, error codes, and next-step diagnostics.

## Non-Goals

- No database materialization.
- No DuckDB, SQLite metadata store, or Parquet cache.
- No CLI/stdio/HTTP runtime blending.
- No mutable global checkout per client.
- No addon scaffolding.
- No Lua linter.
- No Blizzard addon structure tool.
