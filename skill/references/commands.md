# Command selection

```text
wowdoc query   --source SOURCE --product PRODUCT --ref REF --topic TOPIC --text TERM --limit 10
wowdoc explore --source SOURCE --product PRODUCT --ref REF --topic TOPIC --text TERM --limit 25
wowdoc inspect --source SOURCE --product PRODUCT --ref REF --symbol QUALIFIED_NAME
wowdoc inspect --source SOURCE --product PRODUCT --ref REF --path REPOSITORY_PATH
wowdoc diff    --source SOURCE --product PRODUCT --from REF --to REF
wowdoc validate --path ADDON_DIR --source SOURCE --product PRODUCT --ref REF
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

## Data directory

Without configuration, all source mirrors, objects, indexes, manifests, locks, logs, and temporary worktrees live under `~/.wowdoc`.

Use another writable directory by setting `WOWDOC_HOME` before invoking wowdoc. PowerShell:

```powershell
$env:WOWDOC_HOME = 'D:\WOWData\wowdoc'
wowdoc doctor
wowdoc init
```

Bash or zsh:

```bash
export WOWDOC_HOME="$HOME/wowdoc-data"
wowdoc doctor
wowdoc init
```

The override applies to every wowdoc command launched from that environment. `wowdoc doctor` reports the resolved `home`. The npm package remains in the npm global prefix, and the Skill remains in `~/.agents/skills/wowdoc`.
