# Source catalog

| Source | Product | Branch | Notes |
| --- | --- | --- | --- |
| `wow-ui-source` | `retail` | `live` | Official generated API and FrameXML |
| `wow-ui-source` | `ptr`, `ptr2`, `beta` | matching channel | Channel is separate from build |
| `wow-ui-source` | `classic`, `classic-ptr`, `classic-beta` | matching classic channel | Do not infer compatibility from branch name |
| `wow-ui-source` | `classic-era`, `classic-era-ptr`, `anniversary`, `titan` | matching channel | Titan is its own product |
| `wow-ui-source` | `forever` | `forever` | 1.60 line (e.g. `1.60.1`); do not infer compatibility from branch name |
| `elvui` | `main`, `ptr` | matching branch | Version input such as `15.18` maps exactly to Tag `v15.18` |
| `weakauras` | `main` | `main` | Current source supports its declared TOCs; check the selected snapshot |
| `ndui` | `main`, `classic`, `era`, `anniversary`, `titan` | `master`, `Classic`, `Era`, `Anniversary`, `Titan` | Tags are filtered by product branch reachability and product-line rule |
| `ellesmereui` | `main` | `main` | Retail-oriented suite; version input maps to `v` Tag |

A `--product` value may be the product id, its Git branch, or a declared client alias, and every command resolves all three identically. Only `classic-era` declares an extra short alias (`era`); for every other product use the product id or the branch shown above.

The version truth is `Tag -> Commit -> source snapshot`. Release attachments and packaged externals can differ from Tag source; describe evidence as Tag source, not an installed package reconstruction.

Interface evidence for `validate` comes from the game source's build `version.txt` (e.g. `1.60.1` → `16001`) plus indexed TOC entries. AddOn sources ship their own release versions, which are never read as game build evidence, so their Interface facts rest on TOC entries alone. A `toc_interface_mismatch` against a rebuilt snapshot means the declared Interface is genuinely absent from that build.
