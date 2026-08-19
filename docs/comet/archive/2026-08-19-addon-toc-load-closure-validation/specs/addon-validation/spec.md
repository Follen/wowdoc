# AddOn 静态验证

## 能力边界

wowdoc 对本地 AddOn 做不执行代码的静态验证。验证可使用整个目录的兼容模式，也可使用单个 TOC 的真实有序加载闭包模式；所有版本兼容性证据来自用户选择的已就绪 wowdoc snapshot，并绑定 source、product、精确 Tag 与不可变 Commit。

静态验证只陈述能由源文件和目标 snapshot 证明的事实。动态构造的 API、事件、模板、Mixin、Frame 类型或路径进入 unresolved，不得被报告为确定兼容或不兼容；验证成功表示“在所覆盖的静态检查中未发现 error”，不表示保证可在游戏内完美运行。

## validate 命令

`wowdoc validate --path <addon-dir> --source <source> --product <product> --ref <ref>` 保持现有递归 Lua 验证行为。它递归扫描 AddOn 目录内 Lua，返回既有 `path`、`sourceId`、`product`、`ref`、`checkedLua`、`valid` 和 `diagnostics` 字段；新增证据字段不得移除或改变这些字段的含义。

提供 `--toc <toc-file>` 时，TOC 路径相对 `--path` 解析；绝对路径仅在最终文件位于 AddOn 根目录内时接受。此模式不得扫描闭包外 Lua/XML。

## TOC 与 XML 加载闭包

TOC 解析器按文本顺序读取非空、非注释文件行，并保留行号。元数据使用 `## Key: Value` 解析，至少提取 `Interface`；普通 `#` 注释不进入闭包。UTF-8 BOM 与 CRLF 必须支持。

TOC 和 XML 实际加载文件类型为 Lua 与 XML。每个 Lua 进入闭包并做语法与静态兼容性分析；每个 XML 进入闭包、做标准 XML token 解析，并按文档顺序递归处理大小写不敏感的 `Script file="..."` 与 `Include file="..."`。XML 相对路径以当前 XML 所在目录为基准。元素或属性的 namespace 前缀不得改变本地名匹配。

闭包第一次发现文件时确定 loadOrder 并立即按深度优先的客户端加载顺序展开；后续重复引用不再次验证或展开，但把来源追加到同一闭包项的 loadedBy。每个来源包含 file、line 和 kind。路径输出统一为 AddOn 根目录相对、正斜杠分隔的形式。

解析必须兼容 Windows 反斜杠、带空格目录和大小写不敏感磁盘上的大小写差异。每个候选路径经过清理、绝对化和根目录包含关系检查；`..`、绝对路径、符号链接或挂载点不得使加载目标逃逸 AddOn 根目录。

缺失文件、无法解析 XML、Lua 语法错误、路径逃逸和不支持加载类型产生 error。重复引用与循环引用产生 warning；循环边被记录但不继续递归。错误不导致进程崩溃，闭包解析继续处理可安全处理的其余入口。

## 稳定结果模型

成功执行的 envelope 为 `{"ok":true,"data":...}`。TOC 模式 data 至少稳定包含：

- `valid`：不存在 error 级 diagnostic 时为 true；
- `sourceId`、`product`、`requestedRef`、`matchedTag`、`resolvedCommit`；
- `path`、`toc`、`checkedLua`、`checkedXml`；
- `loadClosure`、`diagnostics`、`unresolved`、`coverage`。

`matchedTag` 在精确 Tag/版本匹配时返回规范 Tag，`resolvedCommit` 为对应的 40 位不可变 Commit。没有匹配 Tag 的合法 Commit 选择可返回空字符串，但字段仍存在。

diagnostics 始终为 JSON 数组，即使为空也输出 `[]`。每条 diagnostic 始终包含 severity、code、file、line、column、message、evidence；未知行列使用 0，evidence 为可序列化对象且不可省略。severity 为 error、warning 或 info。

unresolved 始终为 JSON 数组。每项包含 kind、expression、file、line、column、reason 和 evidence。动态引用不得同时产生“确定不存在”诊断。

loadClosure 始终为 JSON 数组。每项包含 path、type、loadedBy 和 loadOrder；type 为 lua 或 xml；loadedBy 始终为数组。checkedLua/checkedXml 分别等于闭包中对应类型的去重项数。

coverage 始终为对象，提供静态分析检查数量和确定/未解析数量，但不得暗示运行时覆盖率。

## 静态兼容性证据

Lua 使用 AST 提取可静态确定的函数调用、RegisterEvent/RegisterUnitEvent 事件名、CreateFrame Frame 类型与模板、CreateFromMixins Mixin，以及相关字符串字面量。XML 提取元素 Frame 类型、inherits 模板/Mixin 和脚本处理器中的 Lua。动态目标统一进入 unresolved。

目标 snapshot 的现有共享事实和分支成员索引是唯一兼容性真相来源。验证层通过查询接口读取 API/API 签名、事件/参数、XML 节点、Mixin 与 Frame 类型，不复制索引数据库。索引器应从官方生成文档保存 API 与事件的参数签名，以支持目标间签名比较；索引 schema 变化必须使旧 snapshot 明确要求重建。

可证明某符号在目标 snapshot 中不存在时产生兼容性 error；存在时记录 coverage，不产生“保证运行”的成功声明。存在多个候选或证据不足时进入 unresolved。

TOC Interface 元数据与目标 snapshot 中可证明的目标构建 Interface 比较；不匹配产生稳定诊断，无法取得目标 Interface 证据时进入 unresolved。

## 多客户端矩阵

`wowdoc validate-matrix --config <json>` 读取 UTF-8 JSON。配置顶层包含 path 和非空 targets；每个 target 包含唯一 id、toc、product、ref，可选 source，source 默认 `wow-ui-source`。相对 path 以配置文件所在目录解析；target toc 以 AddOn 根目录解析。

矩阵按配置顺序运行目标验证，每个目标使用自己的精确 snapshot。任一目标配置或 snapshot 选择失败返回稳定 CLI error；验证诊断本身保留在成功 envelope 中，由 target.valid 和矩阵 valid 表达。

矩阵 data 始终包含 path、valid、targets 和 summary。targets 保留每端完整验证结果。summary 归并：

- API 存在性与签名的共同项和差异；
- 事件存在性与参数签名的共同项和差异；
- XML 模板、Mixin、Frame 类型的共同项和差异；
- 每端 TOC Interface 与目标构建匹配状态；
- 每端 loadClosure 及 sharedFiles、targetOnlyFiles；
- sharedDiagnostics、targetOnlyDiagnostics；
- 按目标保留的 unresolved。

归并键使用稳定的 kind/name/file/line/code 业务字段，不使用 SQLite row ID、数据库路径或其他内部身份。数组顺序按目标配置顺序、闭包 loadOrder 和字典序确定，以保证同一输入输出稳定。

## 兼容性与失败语义

未传 `--toc` 的 validate 不要求本地 snapshot 已就绪，不得因为新增闭包或矩阵功能改变其原有 Lua 语法检查结果。传 `--toc` 和 validate-matrix 需要精确 snapshot；未初始化、版本不存在或 snapshot 未就绪继续使用现有稳定错误码和 nextSteps。

路径、配置或解析错误不得向 stdout 混入非 JSON 文本。所有输出可由 CI 反序列化；空集合使用 `[]` 或 `{}`，不使用 null。

## 发布一致性

此能力随下一个 patch 版本发布。npm 根包、package-lock 根版本、五个平台 optional package、Git Tag、CLI version 和发布公告版本必须一致。发布前必须通过 `go test ./...`、`npm run test:node` 和 `git diff --check`。
