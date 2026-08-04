# wowdoc performance notes

This is the reproducible benchmark record for the compact content-addressed pipeline.

## Scenario

- Host: Windows 11, Git 2.53.0
- Source: `wow-ui-source`
- Product: `retail`
- Commit: `c878310d8432a65bac029c7bacc24eeb2e662bbe`
- Files: 3,685
- Parser workers: 8
- Mirror: complete local bare mirror, so Git object availability is not a variable

## Results

| Version | Build duration | SQLite | Full `WOWDOC_HOME` |
| --- | ---: | ---: | ---: |
| baseline `b201d38` | 41,118 ms | 158,629,888 B | 352,202,349 B |
| compact pipeline, cold snapshot | 11,405 ms | 67,743,744 B | 169,972,173 B |
| compact pipeline, 10 Tags | 102,767 ms additional run | 196,460,544 B | 342,740,177 B |

The cold snapshot stage is 3.61x faster, the SQLite database is 57.3% smaller, and the complete home is 51.7% smaller. Compared with the old 10-Tag Retail database already present on the machine (648,822,784 B), the compact 10-Tag database is 69.7% smaller while retaining complete Lua/XML full-text coverage. The old 10-Tag wall-clock rerun was interrupted by a catalog lock and is intentionally not reported as a timing result.

The complete-home number includes the unchanged 87,381,847 B full bare mirror. For this one-source/one-snapshot case that mirror alone is 24.8% of the baseline home, so a 20%-of-baseline complete-home target is mathematically below the required Git data floor. Index payload and complete-home ratios are therefore reported separately.

## What changed

1. Git tree metadata supplies entries directly; the worktree contract remains intact, but the second full directory walk is gone.
2. A file is read once from the immutable Commit blob and that byte slice is used for parsing and object storage.
3. New source objects, AST JSON and assets use sequential Pack segments with transparent legacy raw/gzip reads; manifests remain gzip files.
4. Lua/XML full-text coverage is preserved with one FTS document per file; query resolves the hit back to its exact source line.
5. FTS is contentless and SQLite strings for symbols, calls, and XML nodes are dictionary-backed.
6. Up to three source mirrors synchronize concurrently during `wowdoc init`.

The Pack change targets filesystem write amplification as well as bytes. The quality regression contained 41,855 legacy AST/source files (179 MiB logical, about 286.5 MiB at 4 KiB NTFS allocation). A Pack keeps that payload content-addressed but replaces thousands of create/close/rename operations with one sequential staging write and one atomic publication. The shared source content database removes cross-branch fact duplication while each branch keeps local FTS statistics for ranking equivalence.

## Verification

`go test ./...` passes. Queries for `C_AuctionHouse.GetItemSearchResultInfo` on `latest` and `11.0.5` returned the expected path, line, excerpt, Commit, SHA-256, and call relations. `wowdoc diff` across those Tags also completed successfully.

## Full catalog v11 measurement

The following is a separate end-to-end Windows run from an empty home, using the complete GitHub mirrors and the default 10 reachable Tags per product branch:

| Measure | Result |
| --- | ---: |
| Command | `wowdoc init` |
| Host | Windows 11, Git 2.53.0.windows.1 |
| Parser budget | 8 total workers, 4 branch workers |
| Wall clock (download + parse + publish) | 553,991 ms |
| Ready snapshots | 174 |
| Mirror bytes | 415,176,416 B |
| Pack payload and catalog | 385,701,428 B |
| SQLite indexes (5 shared + 20 branch DBs) | 1,215,102,976 B |
| Gzip manifests | 25,282,617 B |
| Full home logical bytes | 2,041,460,162 B |
| Full home 4 KiB allocation estimate | 2,042,519,552 B |
| Files after cleanup | 465 |

The full-home number includes complete Git history and all default branch/Tag snapshots; it is not comparable to a single-snapshot fixture. Pack records contain 2,380,361,045 logical source/AST/asset bytes and 324,801,900 compressed payload bytes. Concurrent duplicate staging records are filtered during publish, so the catalog retains one physical object identity per `(kind, SHA-256)` while branch-local FTS and all source coverage remain intact.
