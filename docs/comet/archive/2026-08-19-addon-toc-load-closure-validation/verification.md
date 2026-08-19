---
generated_from_state_version: 7
---

# Verification

## Current result

- Result: **Passed**
- Assurance: **skill-coordinated**
- Goal cycle: 1
- Iteration: 1
- Verifier attempt: 1
- Completed: 2026-08-19T20:00:59.831Z
- Summary: All 46 acceptance items are covered by isolated closure/query/matrix tests, cross-module CLI fixtures, and the complete release test suite; no blocking defect was found.

## Acceptance

| ID | Result | Source | Criterion | Reason |
| --- | --- | --- | --- | --- |
| A1 | passed | brief.md | 给定包含 209 个 Lua、但 `_Mists.toc` 闭包仅有 25 个 Lua 的目录，带 `--toc` 返回 `checkedLua: 25`，闭包外文件不产生诊断。 | TOC closure and CLI tests prove only loaded Lua is counted; closure-external broken Lua is excluded. |
| A2 | passed | brief.md | XML A include 子目录 XML B，B 的相对 `Script file="../code.lua"` 按 B 所在目录解析；输出闭包顺序和 TOC/XML 来源。 | Closure tests cover nested XML Include/Script and current-XML-relative paths. |
| A3 | passed | brief.md | 同一文件被多个入口引用时仅验证一次，`loadedBy` 保留全部来源，并产生稳定重复引用诊断；循环引用不会递归失控。 | Duplicate and cycle tests prove deduplication, all loadedBy sources, and termination. |
| A4 | passed | brief.md | 缺失文件、无法解析 XML 和逃逸根目录都返回包含 severity/code/file/line/column/message/evidence 的精确诊断。 | Failure tests assert structured missing/XML/escape diagnostics with stable evidence fields. |
| A5 | passed | brief.md | 无诊断、无动态项时 `diagnostics` 与 `unresolved` 均为 `[]`，不为 null 或缺失。 | Closure, query, matrix, and CLI JSON tests assert non-null empty arrays. |
| A6 | passed | brief.md | 精确 `--ref 5.5.4` 返回 `requestedRef: "5.5.4"`、`matchedTag: "5.5.4"` 和 40 位 `resolvedCommit`。 | CLI snapshot fixture asserts requestedRef, exact matchedTag, and 40-character resolvedCommit. |
| A7 | passed | brief.md | 三目标矩阵区分共同事实、签名/参数差异、目标独有文件与问题、Interface 不匹配以及动态无法判定项。 | Three-target unit and CLI tests cover signatures, target-only files/issues, Interface, and unresolved. |
| A8 | passed | brief.md | 不带 `--toc` 的 `validate` 保持递归 Lua 计数、Lua 语法诊断和原有兼容字段。 | Legacy CLI test proves recursive Lua count, syntax diagnostic, ref, and no snapshot requirement. |
| A9 | passed | brief.md | Windows 反斜杠、空格目录和磁盘大小写差异均能稳定解析，输出路径统一为 AddOn 根目录相对的 `/` 路径。 | Closure tests cover Windows separators, spaces, case-insensitive lookup, and normalized slash output. |
| A10 | passed | specs/addon-validation/spec.md | wowdoc 对本地 AddOn 做不执行代码的静态验证。验证可使用整个目录的兼容模式，也可使用单个 TOC 的真实有序加载闭包模式；所有版本兼容性证据来自用户选择的已就绪 wowdoc snapshot，并绑定 source、product、精确 Tag 与不可变 Commit。 | TOC mode selects one ready immutable snapshot before static validation. |
| A11 | passed | specs/addon-validation/spec.md | 静态验证只陈述能由源文件和目标 snapshot 证明的事实。动态构造的 API、事件、模板、Mixin、Frame 类型或路径进入 unresolved，不得被报告为确定兼容或不兼容；验证成功表示“在所覆盖的静态检查中未发现 error”，不表示保证可在游戏内完美运行。 | Dynamic references are unresolved and docs explicitly reject runtime guarantees. |
| A12 | passed | specs/addon-validation/spec.md | `wowdoc validate --path <addon-dir> --source <source> --product <product> --ref <ref>` 保持现有递归 Lua 验证行为。它递归扫描 AddOn 目录内 Lua，返回既有 `path`、`sourceId`、`product`、`ref`、`checkedLua`、`valid` 和 `diagnostics` 字段；新增证据字段不得移除或改变这些字段的含义。 | No-TOC branch retains original WalkDir Lua parser and legacy fields. |
| A13 | passed | specs/addon-validation/spec.md | 提供 `--toc <toc-file>` 时，TOC 路径相对 `--path` 解析；绝对路径仅在最终文件位于 AddOn 根目录内时接受。此模式不得扫描闭包外 Lua/XML。 | BuildClosure resolves relative/contained absolute TOC and scans no external files. |
| A14 | passed | specs/addon-validation/spec.md | TOC 解析器按文本顺序读取非空、非注释文件行，并保留行号。元数据使用 `## Key: Value` 解析，至少提取 `Interface`；普通 `#` 注释不进入闭包。UTF-8 BOM 与 CRLF 必须支持。 | TOC parser tests cover order, Interface, BOM, CRLF, blank lines, and comments. |
| A15 | passed | specs/addon-validation/spec.md | TOC 和 XML 实际加载文件类型为 Lua 与 XML。每个 Lua 进入闭包并做语法与静态兼容性分析；每个 XML 进入闭包、做标准 XML token 解析，并按文档顺序递归处理大小写不敏感的 `Script file="..."` 与 `Include file="..."`。XML 相对路径以当前 XML 所在目录为基准。元素或属性的 namespace 前缀不得改变本地名匹配。 | Standard XML decoder recursively processes case-insensitive namespace-local Script/Include references. |
| A16 | passed | specs/addon-validation/spec.md | 闭包第一次发现文件时确定 loadOrder 并立即按深度优先的客户端加载顺序展开；后续重复引用不再次验证或展开，但把来源追加到同一闭包项的 loadedBy。每个来源包含 file、line 和 kind。路径输出统一为 AddOn 根目录相对、正斜杠分隔的形式。 | Zero-based discovery order and loadedBy arrays are asserted in closure and CLI tests. |
| A17 | passed | specs/addon-validation/spec.md | 解析必须兼容 Windows 反斜杠、带空格目录和大小写不敏感磁盘上的大小写差异。每个候选路径经过清理、绝对化和根目录包含关系检查；`..`、绝对路径、符号链接或挂载点不得使加载目标逃逸 AddOn 根目录。 | Lexical, absolute, case-resolved, and evaluated-symlink root containment checks are implemented and tested where permissions allow. |
| A18 | passed | specs/addon-validation/spec.md | 缺失文件、无法解析 XML、Lua 语法错误、路径逃逸和不支持加载类型产生 error。重复引用与循环引用产生 warning；循环边被记录但不继续递归。错误不导致进程崩溃，闭包解析继续处理可安全处理的其余入口。 | Error/warning severity policy and continued safe traversal are covered by failure fixtures. |
| A19 | passed | specs/addon-validation/spec.md | 成功执行的 envelope 为 `{"ok":true,"data":...}`。TOC 模式 data 至少稳定包含： | RunWowdoc integration tests deserialize successful JSON envelopes. |
| A20 | passed | specs/addon-validation/spec.md | `valid`：不存在 error 级 diagnostic 时为 true； | Validator valid is derived only from error-severity diagnostics. |
| A21 | passed | specs/addon-validation/spec.md | `sourceId`、`product`、`requestedRef`、`matchedTag`、`resolvedCommit`； | Result schema and CLI fixture assert source/product/ref/tag/commit evidence. |
| A22 | passed | specs/addon-validation/spec.md | `path`、`toc`、`checkedLua`、`checkedXml`； | Result schema and tests assert path/toc/Lua/XML counts. |
| A23 | passed | specs/addon-validation/spec.md | `loadClosure`、`diagnostics`、`unresolved`、`coverage`。 | Result always emits loadClosure, diagnostics, unresolved, and coverage. |
| A24 | passed | specs/addon-validation/spec.md | `matchedTag` 在精确 Tag/版本匹配时返回规范 Tag，`resolvedCommit` 为对应的 40 位不可变 Commit。没有匹配 Tag 的合法 Commit 选择可返回空字符串，但字段仍存在。 | Existing exact resolver supplies canonical Tag and immutable Commit without fallback. |
| A25 | passed | specs/addon-validation/spec.md | diagnostics 始终为 JSON 数组，即使为空也输出 `[]`。每条 diagnostic 始终包含 severity、code、file、line、column、message、evidence；未知行列使用 0，evidence 为可序列化对象且不可省略。severity 为 error、warning 或 info。 | Validator Diagnostic has all mandatory non-omitempty fields and tests assert non-null evidence. |
| A26 | passed | specs/addon-validation/spec.md | unresolved 始终为 JSON 数组。每项包含 kind、expression、file、line、column、reason 和 evidence。动态引用不得同时产生“确定不存在”诊断。 | Unresolved schema is stable; dynamic calls/events/templates remain non-diagnostic unresolved items. |
| A27 | passed | specs/addon-validation/spec.md | loadClosure 始终为 JSON 数组。每项包含 path、type、loadedBy 和 loadOrder；type 为 lua 或 xml；loadedBy 始终为数组。checkedLua/checkedXml 分别等于闭包中对应类型的去重项数。 | LoadFile schema and closure tests assert type, loadedBy, zero-based order, and deduplicated counts. |
| A28 | passed | specs/addon-validation/spec.md | coverage 始终为对象，提供静态分析检查数量和确定/未解析数量，但不得暗示运行时覆盖率。 | Coverage reports checked/resolved/unresolved static evidence counts. |
| A29 | passed | specs/addon-validation/spec.md | Lua 使用 AST 提取可静态确定的函数调用、RegisterEvent/RegisterUnitEvent 事件名、CreateFrame Frame 类型与模板、CreateFromMixins Mixin，以及相关字符串字面量。XML 提取元素 Frame 类型、inherits 模板/Mixin 和脚本处理器中的 Lua。动态目标统一进入 unresolved。 | AST/XML analyzer tests cover APIs, events, CreateFrame, templates, Mixins, frame types, and dynamic expressions. |
| A30 | passed | specs/addon-validation/spec.md | 目标 snapshot 的现有共享事实和分支成员索引是唯一兼容性真相来源。验证层通过查询接口读取 API/API 签名、事件/参数、XML 节点、Mixin 与 Frame 类型，不复制索引数据库。索引器应从官方生成文档保存 API 与事件的参数签名，以支持目标间签名比较；索引 schema 变化必须使旧 snapshot 明确要求重建。 | Compatibility lookup joins existing snapshot/content facts and generated signatures; schema bump invalidates old indexes. |
| A31 | passed | specs/addon-validation/spec.md | 可证明某符号在目标 snapshot 中不存在时产生兼容性 error；存在时记录 coverage，不产生“保证运行”的成功声明。存在多个候选或证据不足时进入 unresolved。 | Authoritative absence becomes error while ambiguous globals/Mixins/frame types remain unresolved. |
| A32 | passed | specs/addon-validation/spec.md | TOC Interface 元数据与目标 snapshot 中可证明的目标构建 Interface 比较；不匹配产生稳定诊断，无法取得目标 Interface 证据时进入 unresolved。 | TOC Interface is queried against snapshot TOC evidence and mismatch becomes error; missing authority is unresolved. |
| A33 | passed | specs/addon-validation/spec.md | `wowdoc validate-matrix --config <json>` 读取 UTF-8 JSON。配置顶层包含 path 和非空 targets；每个 target 包含唯一 id、toc、product、ref，可选 source，source 默认 `wow-ui-source`。相对 path 以配置文件所在目录解析；target toc 以 AddOn 根目录解析。 | Matrix config parser validates path/targets/default source and resolves path relative to config. |
| A34 | passed | specs/addon-validation/spec.md | 矩阵按配置顺序运行目标验证，每个目标使用自己的精确 snapshot。任一目标配置或 snapshot 选择失败返回稳定 CLI error；验证诊断本身保留在成功 envelope 中，由 target.valid 和矩阵 valid 表达。 | CLI selects exact snapshot per target and preserves validation diagnostics inside successful envelope. |
| A35 | passed | specs/addon-validation/spec.md | 矩阵 data 始终包含 path、valid、targets 和 summary。targets 保留每端完整验证结果。summary 归并： | Matrix schema tests assert path, valid, ordered targets, and initialized summary. |
| A36 | passed | specs/addon-validation/spec.md | API 存在性与签名的共同项和差异； | Matrix tests assert shared and differing API existence/signatures. |
| A37 | passed | specs/addon-validation/spec.md | 事件存在性与参数签名的共同项和差异； | Matrix tests assert shared and differing event signatures. |
| A38 | passed | specs/addon-validation/spec.md | XML 模板、Mixin、Frame 类型的共同项和差异； | Matrix XML summary merges template, Mixin, and frame-type facts. |
| A39 | passed | specs/addon-validation/spec.md | 每端 TOC Interface 与目标构建匹配状态； | Interface summary retains declared value, facts, and diagnostics per target. |
| A40 | passed | specs/addon-validation/spec.md | 每端 loadClosure 及 sharedFiles、targetOnlyFiles； | Matrix tests assert sharedFiles and targetOnlyFiles across three clients. |
| A41 | passed | specs/addon-validation/spec.md | sharedDiagnostics、targetOnlyDiagnostics； | Matrix tests assert stable shared and target-only diagnostics. |
| A42 | passed | specs/addon-validation/spec.md | 按目标保留的 unresolved。 | Matrix retains sorted unresolved arrays keyed by target. |
| A43 | passed | specs/addon-validation/spec.md | 归并键使用稳定的 kind/name/file/line/code 业务字段，不使用 SQLite row ID、数据库路径或其他内部身份。数组顺序按目标配置顺序、闭包 loadOrder 和字典序确定，以保证同一输入输出稳定。 | Aggregation uses stable business keys and deterministic sort/order; no database identities appear. |
| A44 | passed | specs/addon-validation/spec.md | 未传 `--toc` 的 validate 不要求本地 snapshot 已就绪，不得因为新增闭包或矩阵功能改变其原有 Lua 语法检查结果。传 `--toc` 和 validate-matrix 需要精确 snapshot；未初始化、版本不存在或 snapshot 未就绪继续使用现有稳定错误码和 nextSteps。 | Legacy path bypasses snapshot selection; TOC/matrix use existing stable snapshot errors. |
| A45 | passed | specs/addon-validation/spec.md | 路径、配置或解析错误不得向 stdout 混入非 JSON 文本。所有输出可由 CI 反序列化；空集合使用 `[]` 或 `{}`，不使用 null。 | CLI integration and marshal tests prove one parseable envelope and non-null collections. |
| A46 | passed | specs/addon-validation/spec.md | 此能力随下一个 patch 版本发布。npm 根包、package-lock 根版本、五个平台 optional package、Git Tag、CLI version 和发布公告版本必须一致。发布前必须通过 `go test ./...`、`npm run test:node` 和 `git diff --check`。 | Version is uniformly 0.0.9 and go test, Node tests, vet, diff check, format check, and npm pack all pass. |

## Checks

_No Runtime checks were recorded._

## Blockers

_None._

## Risks and skipped work

- Static validation does not execute AddOn Lua or the WoW runtime and cannot guarantee in-game correctness.
- Dynamic references and unproven AddOn globals remain unresolved by design.
- Windows symlink creation privileges prevent the symlink fixture from running on this host; lexical escape and case behavior run on Windows.
- Generated signatures rely on Blizzard's canonical inline Name/Type/Nilable records.

## Previous iterations

| Goal cycle | Iteration | Attempt | Outcome | Unresolved | Summary | Completed |
| ---: | ---: | ---: | --- | --- | --- | --- |
| 1 | 1 | 1 | pass | — | All 46 acceptance items are covered by isolated closure/query/matrix tests, cross-module CLI fixtures, and the complete release test suite; no blocking defect was found. | 2026-08-19T20:00:59.831Z |

## Conclusion

All 46 acceptance items are covered by isolated closure/query/matrix tests, cross-module CLI fixtures, and the complete release test suite; no blocking defect was found.
