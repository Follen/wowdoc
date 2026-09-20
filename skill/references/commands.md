# Command selection

```text
wowdoc query   --source SOURCE --product PRODUCT --ref REF --topic TOPIC --text TERM --limit 10
wowdoc explore --source SOURCE --product PRODUCT --ref REF --topic TOPIC --text TERM --limit 25
wowdoc inspect --source SOURCE --product PRODUCT --ref REF --symbol QUALIFIED_NAME
wowdoc inspect --source SOURCE --product PRODUCT --ref REF --path REPOSITORY_PATH
wowdoc diff    --source SOURCE --product PRODUCT --from REF --to REF
wowdoc validate --path ADDON_DIR --toc TOC_FILE --source SOURCE --product PRODUCT --ref REF
wowdoc validate-matrix --config MATRIX_JSON
wowdoc source list  --source SOURCE --product PRODUCT
wowdoc source check --source SOURCE --product PRODUCT
wowdoc source sync  --source SOURCE --product PRODUCT
wowdoc index build  --source SOURCE --product PRODUCT --ref REF
wowdoc index refresh --source SOURCE --product PRODUCT --ref REF
wowdoc index status --source SOURCE --product PRODUCT
wowdoc init
wowdoc doctor
wowdoc update --dry-run
wowdoc clean
wowdoc clean --yes
wowdoc uninstall
wowdoc uninstall --yes
```

Use exact qualified identifiers when known. For natural-language questions, select a topic and use the narrowest stable identifier, event, template, TOC field, asset path, or API name present in the question. Prefer results marked `exact_symbol`; verify relationship confidence and retain the returned excerpt as evidence.

Use `validate --toc` when a client TOC is known. It limits checks to that TOC's ordered Lua/XML load closure. Use `validate-matrix` when the AddOn declares multiple client TOCs. Keep `unresolved` items unresolved, and describe `valid: true` as no error found by the reported static checks rather than proof of perfect in-game behavior. Omit `--toc` only when the caller intentionally wants the legacy recursive Lua scan.

## Data directory

Without configuration, all source mirrors, objects, indexes, manifests, locks, logs, and temporary worktrees live under `~/.wowdoc`.

Use another writable directory by setting `WOWDOC_HOME` before invoking wowdoc. PowerShell:

```powershell
$env:WOWDOC_HOME = 'D:\WOWData\wowdoc'
wowdoc doctor
wowdoc init
```

Initialization synchronizes up to three source mirrors concurrently. Git progress is written to stderr with a source ID while stdout remains the JSON result. Transient Git failures are retried, and rerunning the command reuses complete objects, ref batches, repositories, snapshots, and indexes.

Bash or zsh:

```bash
export WOWDOC_HOME="$HOME/wowdoc-data"
wowdoc doctor
wowdoc init
```

The override applies to every wowdoc command launched from that environment. `wowdoc doctor` reports the resolved `home`. The npm package remains in the npm global prefix, and the Skill remains in `~/.agents/skills/wowdoc`.

## Search precision and index upgrade (v0.0.14)

`query` and symbol `inspect` return the strongest available tier: exact definitions/facts, then prefixes only if there are no exact hits, then full text only if neither exists. Multiword full-text queries require all words in the indexed document. `explore` includes weaker tiers and matches any word for discovery. `%` and `_` are literal search characters, not SQL wildcards.

`--topic api` filters generated API definitions; `lua`, `xml`, and `toc` filter source file types; `asset` searches indexed asset paths. Filtering and role ranking happen before the result limit. Duplicate evidence for the same definition is folded together; distinct definitions on one line are preserved. Both result and relation lists are individually bounded by `--limit`; query relations use exact names, while explore also finds substring relations. Invalid topics return `invalid_topic`.

TOC validation caches repeated snapshot lookups, preserves every usage location, and returns at most five representative source matches per fact, with `matchCount` and `matchesTruncated` indicating omitted citations. This affects evidence size, not the validation coverage or verdict.

The parser now indexes runtime event `LiteralName` values (for example `PLAYER_LOGIN`) as well as documentation aliases. Existing indexes need one refresh per source/product/ref used with the new parser:

```powershell
wowdoc index refresh --source wow-ui-source --product retail --ref 12.1.0
```

No repository resynchronization is needed when that commit is already local. Other snapshots are refreshed when needed; old parser indexes are not silently reused.
