<div align="center">

# wowdoc

**Versioned World of Warcraft source, ready to cite.**

Turn a game build, AddOn version, Tag, or Commit into exact code references for coding agents and AddOn authors.

[English](README.md) | [简体中文](README.zh-CN.md)

[![CI](https://img.shields.io/github/actions/workflow/status/Follen/wowdoc/ci.yml?branch=main&style=flat-square&label=CI)](https://github.com/Follen/wowdoc/actions/workflows/ci.yml)
[![Release](https://img.shields.io/github/actions/workflow/status/Follen/wowdoc/release.yml?style=flat-square&label=release)](https://github.com/Follen/wowdoc/actions/workflows/release.yml)
[![npm](https://img.shields.io/npm/v/@follenfang/wowdoc?style=flat-square&logo=npm)](https://www.npmjs.com/package/@follenfang/wowdoc)
[![downloads](https://img.shields.io/npm/dm/@follenfang/wowdoc?style=flat-square&label=downloads)](https://www.npmjs.com/package/@follenfang/wowdoc)
[![GitHub release](https://img.shields.io/github/v/release/Follen/wowdoc?style=flat-square&sort=semver)](https://github.com/Follen/wowdoc/releases/latest)
[![license](https://img.shields.io/github/license/Follen/wowdoc?style=flat-square)](LICENSE)

</div>

---

`wowdoc` is a local, CLI-only source intelligence tool. It resolves mutable names such as `latest`, `retail`, or an AddOn version to an immutable Git Commit, then returns the repository path, line, excerpt, and SHA-256 behind each result.

The bundled Agent Skill handles the choice of source, product, version, and command. The CLI handles Git, parsing, indexing, and evidence. Queries stay local after initialization.

## Install

```powershell
npm install -g @follenfang/wowdoc
wowdoc --version
wowdoc init
```

The npm package installs:

- one executable: `wowdoc`
- one user-level Skill: `~/.agents/skills/wowdoc`

The native executable comes from an npm platform package first, so proxy handling, caching, retries, and package integrity stay with npm. If that package is unavailable, the installer falls back to the matching GitHub Release, reports byte progress, verifies `SHA256SUMS`, resumes a managed `.part` file with HTTP Range, and stops after bounded timeouts and retries. To expose complete lifecycle logs, use:

```powershell
npm install -g @follenfang/wowdoc --foreground-scripts --verbose
```

`wowdoc init` creates `~/.wowdoc`, downloads the configured bare Git mirrors, and builds the searchable snapshots. Up to three source mirrors synchronize concurrently, every Git progress line carries a source ID, and transient network errors are retried. Reruns reuse complete Git objects, ref batches, repositories, snapshots, and indexes. An unfinished individual Git pack may be transferred again on the next attempt.

> Git missing? `wowdoc init` detects the platform package manager, shows the command it will run, installs Git, refreshes `PATH`, and verifies `git --version`. `wowdoc doctor` only reports state and never changes it.

## What you get

```text
branch / Tag / version / Commit
              │
              ▼
       immutable Commit
              │
              ▼
 Lua · XML · TOC · assets · symbols · relations
              │
              ▼
 path · line · excerpt · SHA-256 · resolved Commit
```

- **Version-accurate**: Tag and branch names are resolved once and stored as immutable snapshots.
- **Traceable**: every code reference carries enough information to verify it against the Git blob.
- **Concurrent**: each parser task gets a detached worktree; queries never depend on checkout state.
- **Local after init**: search reads SQLite and immutable Pack objects, without switching Git branches or reaching the network.
- **Built for agents**: stable, narrow commands with JSON output and explicit diagnostics.

## Supported source

| Source | Products / channels |
| --- | --- |
| Blizzard UI source | Retail, PTR, PTR2, Beta, Classic, Classic PTR/Beta, Classic Era/PTR, Anniversary, Titan |
| [ElvUI](https://github.com/tukui-org/ElvUI) | main, PTR |
| [WeakAuras](https://github.com/WeakAuras/WeakAuras2) | main |
| [NDui](https://github.com/siweia/NDui) | main, Classic, Era, Anniversary, Titan |
| [EllesmereUI](https://github.com/EllesmereGaming/EllesmereUI) | main |

For third-party AddOns, version truth is `Tag -> Commit -> snapshot`. If a requested version has no matching Tag, the Agent Skill may use the latest snapshot, but the result is clearly marked as a latest fallback and still identifies the resolved Commit.

## Use it

### Find an API definition

```powershell
wowdoc query `
  --source wow-ui-source `
  --product retail `
  --ref latest `
  --topic api `
  --text C_AuctionHouse.GetItemSearchResultInfo
```

### Inspect an ElvUI symbol at a released version

```powershell
wowdoc inspect `
  --source elvui `
  --product main `
  --ref v15.18 `
  --symbol 'lib:RegisterPlugin'
```

### Compare two WeakAuras versions

```powershell
wowdoc diff `
  --source weakauras `
  --product main `
  --from 5.20.7 `
  --to 5.21.9
```

### Validate an AddOn against a target snapshot

```powershell
wowdoc validate `
  --path D:\AddOns\MyAddon `
  --source wow-ui-source `
  --product retail `
  --ref latest
```

Every successful reference identifies the source, product, requested ref, matching Tag when available, resolved Commit, repository path, line, excerpt, and content hash.

## Command map

```text
Search      wowdoc query | explore | inspect | diff | validate
Sources     wowdoc source list | check | sync
Indexes     wowdoc index build | refresh | status
Health      wowdoc doctor
Lifecycle   wowdoc init | update | clean | uninstall
```

A normal update flow is explicit:

```powershell
wowdoc source check --source elvui --product main
wowdoc source sync --source elvui --product main
wowdoc index refresh --source elvui --product main --ref latest
```

`wowdoc update` updates the npm package and Skill. It does not fetch repositories or rebuild indexes. `wowdoc clean` is a preview unless `--yes` is supplied.

## Storage

All local data lives under `~/.wowdoc` by default:

```text
config/         source catalog and local configuration
repositories/   complete bare Git mirrors
objects/packs/  immutable, content-addressed Pack segments
indexes/        shared content DBs and branch-local WAL/FTS databases
manifests/      immutable snapshot manifests
state/          initialization and task state
tmp/worktrees/  leased detached worktrees used while parsing
locks/          bounded repository and publish locks
logs/           local diagnostics
```

Set `WOWDOC_HOME` to move the data directory:

```powershell
$env:WOWDOC_HOME = 'D:\WOWData\wowdoc'
wowdoc doctor
wowdoc init
```

Content-identical source, AST, and asset bytes are stored once in immutable Pack segments and reused across Tags and branches. A source-level SQLite database holds shared facts; branch databases keep their own snapshot membership and FTS statistics, preserving version filtering and BM25 ordering.

## Performance

Measured on Windows 11 with a complete local mirror, 8 parser workers, and the same 3,685-file Retail Commit:

| Pipeline | Cold build | SQLite | Complete home |
| --- | ---: | ---: | ---: |
| baseline `b201d38` | 41.1 s | 158.6 MB | 352.2 MB |
| compact content store | 11.4 s | 67.7 MB | 170.0 MB |

That run reduced parse/index time by 72%, SQLite size by 57%, and total local data by 52%, while retaining full Lua/XML search coverage and identical source evidence. See [the reproducible benchmark notes](docs/performance.md) for the scenario, full-catalog numbers, and trade-offs.

## Agent integration

The installed Skill contains command-selection rules and source/product aliases, not copied source facts. An Agent uses the narrowest stable identifier in the question, calls `wowdoc`, and cites the returned evidence.

The quality suite covers 50 realistic AddOn-author questions across product branches and historical Tags. A pass requires the first reference to be correct, relevant, context-complete, version-correct, and byte-for-byte traceable to the resolved Git blob.

## Development

Requirements: Go 1.23+, Node.js 20+, and Git.

```powershell
go test ./...
go vet ./...
npm pack --dry-run
go run ./cmd/wowdoc --help
```

Tags follow `vMAJOR.MINOR.PATCH`. GitHub Actions tests Windows, Linux, and macOS, builds five CLI binaries, creates a GitHub Release with checksums, and publishes the matching npm package through Trusted Publisher OIDC with provenance.

## License

[MIT](LICENSE)
