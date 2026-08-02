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
wowdata --help
```

The package installs:

- `wowdoc`: read-only source queries and explicit source/index operations;
- `wowdata`: local data initialization, update, cleanup, and uninstall;
- the `wowdoc` Skill in `~/.agents/skills/wowdoc`.

Run the one-time data initialization before querying source:

```powershell
wowdata init
```

Initialization creates `~/.wowdoc`, fetches the configured Git mirrors, and builds searchable SQLite snapshots for each product branch and its hot Tags. It can take time and substantial disk space. The command is resumable: rerunning it keeps completed work and continues failed or pending snapshots.

If Git is missing, `wowdata init` detects the platform package manager, shows the exact package and installer command, installs Git, refreshes `PATH`, and verifies `git --version`. `wowdoc doctor` remains read-only.

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

wowdata init|update|clean|uninstall
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
wowdata clean
```

Use `wowdata clean --apply` only after reviewing its candidates. Removing indexed versions requires an explicit version or range. `wowdata uninstall` requires confirmation and removes the npm package, managed Skill, and `~/.wowdoc` data.

## Storage model

Local state lives under `~/.wowdoc`:

```text
config/         versioned source catalog and local configuration
repositories/   bare partial Git mirrors
objects/        content-addressed source and asset bytes
ast/            auditable per-file JSON syntax trees
indexes/        one WAL SQLite database per product branch
manifests/      immutable snapshot manifests
state/          initialization and task state
tmp/worktrees/  leased detached worktrees used only while parsing
locks/          repository and snapshot build locks
logs/           local diagnostics
```

Each parser task fixes the requested ref to a Commit and creates its own detached worktree. Published queries never depend on that worktree. Identical Git blobs and AST objects are reused across Tags and branches, while snapshot relationships remain isolated by product and Commit.

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
go run ./cmd/wowdata --help
```

Release tags use `vMAJOR.MINOR.PATCH`. GitHub Actions tests the project, builds `wowdoc` and `wowdata` for Windows amd64, Linux amd64/arm64, and macOS amd64/arm64, publishes checksums and a GitHub Release, then publishes the matching npm version through npm Trusted Publisher OIDC with provenance.

## License

[MIT](LICENSE)
