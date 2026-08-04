# wowdoc

[![CI](https://github.com/Follen/wowdoc/actions/workflows/ci.yml/badge.svg)](https://github.com/Follen/wowdoc/actions/workflows/ci.yml)
[![npm](https://img.shields.io/npm/v/@follenfang/wowdoc)](https://www.npmjs.com/package/@follenfang/wowdoc)
[![license](https://img.shields.io/github/license/Follen/wowdoc)](LICENSE)

`wowdoc` gives coding agents versioned, auditable code references for World of Warcraft UI source and popular AddOns. It resolves a branch, Tag, version, or Commit to an immutable snapshot and returns the exact repository path, line, excerpt, content hash, and resolved Commit behind every answer.

The product is CLI-only. The included Agent Skill translates a natural-language question into small, stable CLI commands; the CLI handles Git, parsing, indexing, and evidence.

## Install

```powershell
npm install -g @follenfang/wowdoc
wowdoc --version
wowdoc --help
```

The package installs the `wowdoc` CLI and the `wowdoc` Skill in `~/.agents/skills/wowdoc`. The single CLI handles queries, source/index operations, initialization, updates, cleanup, and uninstall.

Run the one-time data initialization before querying source:

```powershell
wowdoc init
```

Initialization creates `~/.wowdoc`, fetches the configured Git mirrors, and builds searchable SQLite snapshots for each product branch and its hot Tags. It can take time and substantial disk space. The command is resumable: rerunning it keeps completed work and continues failed or pending snapshots.

If Git is missing, `wowdoc init` detects the platform package manager, shows the exact package and installer command, installs Git, refreshes `PATH`, and verifies `git --version`. `wowdoc doctor` remains read-only.

## Supported source

| Source | Products |
| --- | --- |
| Blizzard UI source | Retail, PTR, PTR2, Beta, Classic, Classic PTR/Beta, Classic Era/PTR, Anniversary, Titan |
| ElvUI | main, PTR |
| WeakAuras | main |
| NDui | main, Classic, Era, Anniversary, Titan |
| EllesmereUI | main |

Third-party versions use Git truth: `Tag -> Commit -> snapshot`. Release archives and installed AddOn folders can differ because packaging may inject externals or replace placeholders; wowdoc describes Tag source rather than pretending to reconstruct an installed ZIP byte for byte.

## Query source

Find an API definition:

```powershell
wowdoc query `
  --source wow-ui-source `
  --product retail `
  --ref latest `
  --topic api `
  --text C_AuctionHouse.GetItemSearchResultInfo
```

Inspect a known ElvUI function at an exact plugin version:

```powershell
wowdoc inspect `
  --source elvui `
  --product main `
  --ref v15.18 `
  --symbol 'lib:RegisterPlugin'
```

Compare two retained versions:

```powershell
wowdoc diff `
  --source weakauras `
  --product main `
  --from 5.20.7 `
  --to 5.21.9
```

Every successful reference identifies its `sourceId`, product, requested ref, matched Tag when present, resolved Commit, repository path, line, excerpt, and SHA-256 content hash. Queries read only published SQLite snapshots and content-addressed objects; they do not switch a shared checkout or silently access the network.

## Commands

```text
wowdoc query|explore|inspect|diff|validate
wowdoc source list|check|sync
wowdoc index build|refresh|status
wowdoc doctor
wowdoc init|update|clean|uninstall
```

Common lifecycle:

```powershell
# Check whether a branch changed without modifying local state
wowdoc source check --source elvui --product main

# Explicitly fetch new Git metadata and source objects
wowdoc source sync --source elvui --product main

# Build and atomically publish the new snapshot
wowdoc index refresh --source elvui --product main --ref latest

# Preview cleanup; no files are deleted
wowdoc clean
```

Use `wowdoc clean --yes` only after reviewing its candidates. Removing indexed versions requires an explicit version or range. `wowdoc uninstall` requires confirmation and removes the npm package, managed Skill, and `~/.wowdoc` data.

## Storage model

Local state lives under `~/.wowdoc`:

```text
config/         versioned source catalog and local configuration
repositories/   complete bare Git mirrors
objects/        legacy content objects plus immutable Pack storage
objects/packs/  sequential Pack segments and their catalog
ast/            auditable legacy per-file syntax trees (new builds use Pack)
indexes/        one shared content DB plus one WAL SQLite database per product branch
manifests/      immutable snapshot manifests
state/          initialization and task state
tmp/worktrees/  leased detached worktrees used only while parsing
locks/          repository and snapshot build locks
logs/           local diagnostics
```

Each parser task fixes the requested ref to a Commit and creates its own detached worktree. Published queries never depend on that worktree. Identical Git blobs and AST objects are written once to immutable Pack segments and reused across Tags and branches, while snapshot relationships remain isolated by product and Commit.

Source objects, AST and assets are appended to one staging Pack per build and atomically published; the Pack catalog verifies original length and SHA-256 on every read. Legacy raw/gzip objects remain readable. One source-level SQLite keeps immutable facts and search-document metadata per content hash, while each product branch keeps its own WAL snapshot mappings and local FTS corpus so BM25 ordering remains branch-equivalent. A full-text hit is resolved back to the exact Pack source line. Initialization downloads up to three source mirrors in parallel, then parses with the normal 4-8 worker budget.

### Performance baseline

Measured on Windows 11, Git 2.53.0, the same `wow-ui-source` Retail Commit `c878310d8432a65bac029c7bacc24eeb2e662bbe`, 8 parser workers, and a complete local bare mirror:

| Build | Cold build time | Indexed files | SQLite | Home total |
| --- | ---: | ---: | ---: | ---: |
| `b201d38` baseline | 41.1 s | 3,685 | 158,629,888 B | 352,202,349 B |
| compact pipeline | 11.4 s | 3,685 | 67,743,744 B | 169,972,173 B |

The cold parse/index stage is 3.61x faster (about 72% less time), its SQLite is 57% smaller, and its complete home is 52% smaller. A separate 10-Tag run produced 196,460,544 B of branch SQLite and 342,740,177 B total while preserving complete Lua/XML full-text coverage. Exact symbol, plain-text source, relation, Commit, path, line, excerpt, and SHA-256 checks pass after the storage changes. These are local measurements; network clone/fetch time varies by GitHub and TLS conditions.

## Agent integration

The installed Skill contains source/product aliases and command-selection rules, not copied source facts. An Agent chooses the source, product, ref, topic, and narrowest useful identifier, then cites the CLI evidence. A missing exact plugin Tag can fall back to that product branch's latest snapshot only when the Skill labels the result as a latest fallback and preserves the originally requested version.

The quality suite contains 50 realistic AddOn-author questions across all configured product branches and historical Tags. A strict pass requires the first reference to be correct, relevant, context-complete, version-correct, and byte-for-byte traceable to the resolved Git blob. Local scenarios and generated reports live in the Git-ignored `analyze/quality` directory.

## Development

Requirements: Go 1.23+, Node.js 20+, and Git.

```powershell
go test ./...
go vet ./...
npm pack --dry-run
go run ./cmd/wowdoc --help
```

Release tags use `vMAJOR.MINOR.PATCH`. GitHub Actions tests the project, builds `wowdoc` for Windows amd64, Linux amd64/arm64, and macOS amd64/arm64, publishes checksums and a GitHub Release, then publishes the matching npm version through npm Trusted Publisher OIDC with provenance.

## License

[MIT](LICENSE)
