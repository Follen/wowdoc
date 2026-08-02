# Source catalog

| Source | Product | Branch | Notes |
| --- | --- | --- | --- |
| `wow-ui-source` | `retail` | `live` | Official generated API and FrameXML |
| `wow-ui-source` | `ptr`, `ptr2`, `beta` | matching channel | Channel is separate from build |
| `wow-ui-source` | `classic`, `classic-ptr`, `classic-beta` | matching classic channel | Do not infer compatibility from branch name |
| `wow-ui-source` | `classic-era`, `classic-era-ptr`, `anniversary`, `titan` | matching channel | Titan is its own product |
| `elvui` | `main`, `ptr` | matching branch | Version input such as `15.18` maps exactly to Tag `v15.18` |
| `weakauras` | `main` | `main` | Current source supports its declared TOCs; check the selected snapshot |
| `ndui` | `main`, `classic`, `era`, `anniversary`, `titan` | `master`, `Classic`, `Era`, `Anniversary`, `Titan` | Tags are filtered by product branch reachability and product-line rule |
| `ellesmereui` | `main` | `main` | Retail-oriented suite; version input maps to `v` Tag |

The version truth is `Tag -> Commit -> source snapshot`. Release attachments and packaged externals can differ from Tag source; describe evidence as Tag source, not an installed package reconstruction.
