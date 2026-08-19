# Outcome

wowdoc 能以指定 TOC 在真实客户端中的有序加载闭包作为 AddOn 静态验证边界，并能把多个客户端目标的兼容性证据归并为稳定、可供 CI 解析的矩阵结果。验证结果明确绑定精确 Tag 和不可变 Commit，动态无法证明的引用进入 unresolved，不把静态未发现问题描述为游戏内运行保证。

# Scope

- 为 `wowdoc validate` 增加 `--toc <toc-file>`，解析 TOC 及 XML `Script`、`Include` 的递归、有序、去重加载闭包。
- 对闭包中的 Lua/XML 做语法和静态兼容性分析；保留路径、加载顺序与所有引用来源。
- 拒绝 AddOn 根目录逃逸，报告缺失、XML 解析失败、重复和循环引用。
- 扩展稳定 JSON 证据：精确版本身份、loadClosure、diagnostics、unresolved、coverage、checkedLua、checkedXml。
- 新增 `wowdoc validate-matrix --config <json>`，逐目标验证并归并 API、事件、XML 模板/Mixin/Frame 类型、TOC Interface、实际加载文件和动态项的相同与差异。
- 复用现有 snapshot 选择、索引、AST/XML 解析和诊断模型，并补齐索引中兼容性比较需要的签名证据。
- 更新 CLI/Skill 文档、schema 与自动化测试，并按项目发布约束发布下一个 patch 版本。

# Non-goals

- 不执行 AddOn Lua，不启动或调用 WoW 游戏运行时。
- 不证明 AddOn 在游戏内完美运行，也不把动态构造猜测成确定兼容或不兼容。
- 不改变未传 `--toc` 时递归扫描 Lua 的现有兼容行为。
- 不复制第二套 API 或 snapshot 索引。
- 不实现通用 Lua 数据流、跨过程求值或运行时条件分支模拟。

# Acceptance examples

- 给定包含 209 个 Lua、但 `_Mists.toc` 闭包仅有 25 个 Lua 的目录，带 `--toc` 返回 `checkedLua: 25`，闭包外文件不产生诊断。
- XML A include 子目录 XML B，B 的相对 `Script file="../code.lua"` 按 B 所在目录解析；输出闭包顺序和 TOC/XML 来源。
- 同一文件被多个入口引用时仅验证一次，`loadedBy` 保留全部来源，并产生稳定重复引用诊断；循环引用不会递归失控。
- 缺失文件、无法解析 XML 和逃逸根目录都返回包含 severity/code/file/line/column/message/evidence 的精确诊断。
- 无诊断、无动态项时 `diagnostics` 与 `unresolved` 均为 `[]`，不为 null 或缺失。
- 精确 `--ref 5.5.4` 返回 `requestedRef: "5.5.4"`、`matchedTag: "5.5.4"` 和 40 位 `resolvedCommit`。
- 三目标矩阵区分共同事实、签名/参数差异、目标独有文件与问题、Interface 不匹配以及动态无法判定项。
- 不带 `--toc` 的 `validate` 保持递归 Lua 计数、Lua 语法诊断和原有兼容字段。
- Windows 反斜杠、空格目录和磁盘大小写差异均能稳定解析，输出路径统一为 AddOn 根目录相对的 `/` 路径。

# Constraints and invariants

- stdout 始终是单个稳定 JSON envelope；诊断数组稳定存在。
- `loadClosure.loadOrder` 为零基连续整数；第一次发现决定顺序，后续引用只追加 `loadedBy`。
- 根目录约束基于规范化绝对路径与实际磁盘大小写解析，不依赖字符串前缀。
- error 级诊断使 `valid=false`；warning/info 与 unresolved 不单独使验证失败。
- exact/inferred/dynamic-unresolved 证据置信度语义与现有索引一致。
- npm 根包、lockfile、平台包、Tag、CLI 构建版本保持一致。

# Decisions

- 使用独立 `internal/validator` 领域包承载闭包、静态分析和矩阵归并；Cobra 命令只解析参数和组织 snapshot 选择。
- TOC/XML 可加载代码类型按客户端语义限定为 `.lua` 与 `.xml`；其他 TOC/XML 文件引用返回 `unsupported_load_file`，不会当作代码执行。
- 重复引用与 XML 循环是可恢复 warning；缺失、XML/Lua 解析失败、路径逃逸和不支持加载类型是 error。
- `loadedBy` 是带 `file`、`line`、`kind` 的来源对象数组，以便同一闭包项保留多来源证据。
- `validate-matrix` 配置采用用户给出的 `path + targets[]` 结构；每个 target 默认 source 为 `wow-ui-source`，也允许显式覆盖 source。
- 用户已明确授权“开 change，然后不用我确认直接搞”，本次没有额外用户可见歧义，视为对该共享理解摘要的明确确认。

# Open questions

无。

# Verification expectations

- `go test ./...`
- `npm run test:node`
- `git diff --check`
- CLI fixture 运行验证带/不带 `--toc`、精确 ref 证据和三客户端矩阵 JSON schema。
- 发布前核对最高 Tag 并按 patch 递增，统一 npm/lockfile/平台包/Tag/发布版本。
