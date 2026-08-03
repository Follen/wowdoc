---
name: wowdoc
description: Use wowdoc when an Agent needs versioned World of Warcraft UI or supported AddOn source evidence, API definitions, XML templates, TOC metadata, assets, compatibility checks, or source diffs.
---

# wowdoc

Translate the user's wording into explicit `source`, `product`, `ref`, `topic`, and query terms using the references in this Skill. The CLI is the source reader; do not infer code facts from this document.

1. Run `wowdoc source list --source SOURCE --product PRODUCT` when source, product, or available versions are uncertain.
2. Prefer an exact displayed plugin version. The CLI maps it only to an exact Tag and immutable Commit.
3. Use one read command: `query` for a focused question, `explore` for a subsystem, `inspect` for a known symbol/path, `diff` for two versions, or `validate` for a local AddOn.
4. If the CLI returns `snapshot_not_ready`, run only the listed `source sync` and `index build|refresh` steps, then retry the original read command.
5. If an exact plugin version returns `version_not_found`, keep the same source and product, run `source check`, synchronize and refresh if needed, then query `latest`. Mark the answer with `requestedVersion`, `matchedTag=null`, `resolutionMode=latest_fallback`, product branch, resolved Commit, and state that the evidence is from latest rather than the requested version.
6. Do not apply latest fallback to `ambiguous_version`, `unsupported_build`, `ref_not_found`, or update failures.
7. Cite `sourceId`, product, Tag when present, resolved Commit, path, line, and the returned excerpt. Treat `dynamic-unresolved` edges as unresolved, not exact.

The data directory defaults to `~/.wowdoc`. `WOWDOC_HOME` may override it with another writable path for an isolated workspace, another drive, or a server deployment. This changes only wowdoc's source, index, object, lock, and log storage; it does not change the npm installation directory or the Skill directory. Run `wowdoc doctor` to see the resolved data directory.

Read [source-catalog.md](references/source-catalog.md) for source/product names and [commands.md](references/commands.md) for command selection.
