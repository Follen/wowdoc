# Outcome

把 wowdoc 重写为面向 Agent 的 CLI 独占工具。npm 安装后，本机获得 `wowdoc` 与独立的 `wowdata` 命令，并自动安装一份不捆绑可执行文件的 wowdoc Skill 到用户级 `~/.agents`。Skill 负责把 Agent 的自然语言意图桥接为稳定的 CLI 原子命令；CLI 负责源码获取、增量解析、索引和可追溯查询。

# Scope

- 从零重写核心实现，旧 Go 代码只作为已存在行为和测试样例的参考。
- 发布 npm 包 `@follenfang/wowdoc`，提供 `wowdoc` 和 `wowdata` 两个可执行命令。
- `wowdoc` 保持既定原子命令面：`query`、`explore`、`inspect`、`diff`、`validate`、`index build|refresh|status`、`source check|list|sync`，并提供只读的 `doctor` 总检查入口。
- `wowdata` 负责安装后的数据与产品生命周期操作，包含 `init`、`update`、`clean` 和 `uninstall`。
- 创建并管理用户级 `~/.wowdoc`，规划配置、源码快照、按内容寻址的解析中间体、SQLite 查询索引、日志、临时文件和锁。
- 支持按游戏 client、build 与 ref 选择官方及第三方源码；第三方包括 ElvUI、WeakAuras2、NDui 和 EllesmereUI。
- 首次全量解析；后续按文件 SHA-256 只解析变化文件；不同提交中的相同文件复用中间体；解析采用有界并发并按文件释放 AST。
- GitHub Actions 完成测试、跨平台 CLI 构建、npm 打包与 npm 发布。
- 首版支持 Windows、Linux 和 macOS；Windows 可本机验证，macOS 必须使用 GitHub Actions 的 macOS runner 执行安装和真实命令测试。
- npm 安装生命周期负责把 wowdoc Skill 安装到 `~/.agents/skills/wowdoc`，Skill 通过 PATH 调用 npm 安装的 CLI，不再携带 exe。
- 覆盖安装、更新、卸载、存储、GitHub 获取、版本解析、索引损坏和并发占用等常见失败，并提供稳定的机器可读错误。
- 每个源码仓库保留全部分支、Tag 和 Commit 关系，但历史文件内容按选中的版本下载并缓存，避免首次同步把所有历史文件全部拉到本机。
- 第三方版本以 Git Tag 为准，版本链固定为 `Tag -> Commit -> source snapshot`；CLI 不获取或索引插件发布平台的 Release 元数据、附件或安装包。

# Non-goals

- 本次不提供 MCP stdio、MCP HTTP、HTTP 服务端、ticket 或服务端镜像。
- CLI 不负责理解用户自然语言，也不把 client/build/第三方兼容关系硬编码成 Agent 决策逻辑；这些知识放在 Skill Reference。
- 不使用向量数据库作为默认检索主链路。
- 不把第三方源码或完整源码快照打进 npm 包。

# Acceptance examples

- 用户执行 `npm install -g @follenfang/wowdoc` 后，`wowdoc --help` 和 `wowdata --help` 均可执行，且 `~/.agents/skills/wowdoc/SKILL.md` 已安装；Skill 目录中不存在捆绑的 `wowdoc.exe`。
- `wowdoc doctor` 检查安装版本、目录权限、磁盘空间、Git/GitHub 访问、source catalog、已下载源码和索引健康度；它不修改任何内容，问题结果包含建议的下一条原子命令。
- 新用户运行数据初始化或同步命令后，`~/.wowdoc` 的配置、源码、对象、中间体、索引、日志、临时文件和锁分别落在稳定目录中，命令可重复执行且不会破坏已有有效数据。
- Agent 遇到 WoW 插件源码问题时，Skill 根据 Reference 选择 client/source set/build/ref，再调用一个原子 CLI 命令，并从 JSON 结果得到来源、解析版本、文件位置、诊断和下一步。
- 同一源码版本首次索引会解析全部受支持文件；后续版本只重新解析 SHA-256 改变的文件；内容相同的文件直接复用中间体。
- 同步新版本后，已经下载和建立索引的旧版本继续可用；`diff` 可以直接比较两个已保留版本，不需要重新下载和解析。
- 指定不存在的 build/ref 或第三方项目不支持的 build 时，命令返回非零退出码和稳定错误代码，说明请求值、可用值及建议的下一条原子命令，不伪造兼容结果。
- 用户只知道 ElvUI 等插件显示的版本号时，可以直接传入该版本；CLI 按 catalog 的精确前缀规则匹配 Git Tag，再解析为唯一 Commit。结果同时返回用户输入、Tag 和 Commit。
- 插件版本没有匹配 Tag 时，CLI 原子命令先返回 `version_not_found`；Skill 根据 Reference 明确切换到该产品分支的 latest 流程，依次检查、同步、刷新索引并查询最新 Commit。最终回答必须保留原请求版本并标记 `resolutionMode=latest_fallback`，不能把最新代码说成请求版本。
- 第一次同步仓库元数据后，`source list` 可以离线列出已获取的分支、Tag 和 Commit；插件发布平台的 Release 标题、附件和安装包不属于 wowdoc 数据源。
- GitHub 限流、认证失败、网络中断、仓库或 ref 不存在、归档损坏、磁盘不足、权限不足、锁冲突和 SQLite 损坏都有可区分的错误结果，并且临时下载不会替换最后一份可用数据。
- Windows、Linux、macOS 的 CI 都能安装 npm tarball，并实际运行 `wowdoc --version`、`wowdoc doctor` 和基础 fixture 查询；macOS 验证证据来自 GitHub Actions 的 macOS runner。
- 打 tag 后，GitHub Actions 在测试和打包通过时发布 npm 包；失败时不发布半成品。
- 完成实现后，以插件作者会真实提出的问题建立不少于 50 条端到端检索场景，覆盖 catalog 中每个产品分支和多个新旧 Tag。每条场景都从 Agent/Skill 的选源与命令参数开始，检查 CLI 返回的源码文件、行号、excerpt、符号关系和 resolved Commit，并回读固定 Commit 的原始 blob 核对引用是否准确；只返回非空结果不算通过。
- `wowdata update` 只通过 npm 更新 `@follenfang/wowdoc` 及随包安装的 Skill，不下载源码、不刷新索引；源码和索引仍分别由 `wowdoc source sync` 与 `wowdoc index refresh` 处理。
- 更新 Skill 前先核对安装清单中的文件 hash；未修改时原地更新，发现用户修改时先把旧 Skill 完整备份，再原子安装新版，并在结果中返回备份路径。
- 用户确认后运行 `wowdata uninstall`，会卸载 npm 包和 Skill，并删除整个 `~/.wowdoc`，包括配置、源码、解析中间体、索引、状态、日志和临时文件。
- `wowdata clean` 默认只预览候选旧版本、缓存文件和预计释放空间；明确传入执行确认后才删除，并返回实际释放空间和保留版本。
- `wowdata clean --apply` 没有指定旧版本时，只清理临时文件和没有任何 snapshot 引用的缓存；删除旧版本必须显式指定版本或范围。
- `wowdata init` 创建完整的 `~/.wowdoc`，获取 source catalog 中声明的官方及第三方产品分支当前源码，并为每个产品分支最多 50 个匹配 Tag 完成 AST 中间体与 SQLite 索引构建；不足 50 个时全部构建。
- `wowdata init` 启动时若检测不到 Git，先按平台探测并明确输出将执行的安装器与包：Windows 优先 `winget install Git.Git`，并可依次尝试已存在的 Scoop/Chocolatey；macOS 优先已存在的 Homebrew，否则触发 Xcode Command Line Tools 并明确其交互限制；Linux 按已存在的 apt、dnf、yum、pacman、zypper 选择。安装完成后重新发现 PATH 并运行 `git --version`，验证失败则停止初始化并返回稳定错误码和精确下一步。
- `init` 会把 Lua AST、XML 结构、TOC 字段、符号、引用和全文检索数据写入可查询的 SQLite；完成后的基线查询只读 SQLite 和内容对象，不再访问 Git 或网络。
- 热数据之外仍保存 Tag/Commit 关系；第一次查询更旧版本时按需增量下载和建索引，完成后长期保留。
- 普通查询直接使用当前已发布 snapshot，不自动联网；用户明确询问“最新”时，Skill 先检查对应产品分支，发现变化后依次执行 `source sync`、`index refresh`，再用新 resolved Commit 查询。
- 尚未初始化时，所有依赖源码或索引的命令返回 `not_initialized`，明确提示运行 `wowdata init`；`--help`、`--version`、`wowdoc doctor`、`wowdata init` 和卸载等不依赖数据的命令仍可运行。
- `wowdata init` 按仓库和基线 snapshot 保存进度；某项失败时保留其他已完成项，命令返回失败清单；再次运行时只继续未完成或失败项。
- 在所有初始化项完成前，已完成仓库也不开放查询；数据命令返回 `initialization_incomplete`、当前进度和下一步 `wowdata init`。全部完成后一次性把全局状态切换为 ready。

# Constraints and invariants

- 技术栈和依赖必须源码可审计，并锁定依赖版本与产物校验信息。
- CLI 是唯一运行时产品边界；代码中不得保留 MCP 或 HTTP server 入口。
- CLI 原子命令的名称和职责保持稳定；语义编排只能存在于 Skill 与 Reference。
- 默认输出面向 Agent，采用版本化 JSON envelope；诊断写入结构化字段，正常 stdout 不混入人类日志。
- 源码、解析中间体和索引均与 source、client、build/ref、resolved commit 及解析器 schema 版本可追溯关联。
- 正式 CLI 不依赖用户安装 `ast-grep`、Tree-sitter CLI 或 Lua 解释器；Lua 解析器作为锁定版本的纯 Go 库编译进二进制，XML/TOC 使用可审计的 Go 实现。
- XML 的内嵌 Lua handler、`function`/`method` handler、隐式 `Bindings.xml` 和 TOC/XML 加载顺序都属于源码图，不能只扫描独立 `.lua` 文件。
- 静态关系必须标注可信度；Lua 的动态全局、Mixin、Hook、回调和 `loadstring` 无法静态确定的边不得伪装成精确调用图。
- 下载和索引更新使用临时目录、校验、原子切换和锁；失败时保留最后一份可用状态。
- 密钥与 npm/GitHub token 不写入仓库、日志、缓存元数据或命令输出。
- Skill 安装发生在用户目录，不把 Skill 源文件复制进本仓库的 `.agents/skills`。

# Decisions

- 采用重写，不在现有 CLI/MCP/HTTP 混合代码上继续拆改。
- 产品只保留 CLI，不构建或发布 MCP stdio、MCP HTTP 和服务端。
- npm 默认安装 CLI 与用户级 Skill；Skill 不捆绑二进制文件。
- AST 解析结果按文件 SHA-256 复用，SQLite 承担查询索引，JSON 文件承担可审计的内容寻址中间体。
- 官方和第三方源码的 client/build/branch/TOC/兼容知识由 Skill Reference 维护，CLI 只接受明确参数并执行原子操作。
- npm 正式包名使用本机 npm 账号 scope：`@follenfang/wowdoc`；标准安装命令为 `npm install -g @follenfang/wowdoc`。
- `wowdata update` 只更新 npm 包和 Skill，不自动同步源码或刷新索引。
- `wowdata uninstall` 采用彻底卸载：经用户明确确认后删除工具、Skill 和整个 `~/.wowdoc` 数据目录。
- 未指定版本时，所有源码默认同步对应分支的最新提交；指定版本时支持 Commit 和 Tag，并维护 `Tag -> Commit` 的可追溯关系。
- 对插件显示版本允许明确的常见前缀归一化，例如 ElvUI `15.18` 可匹配 Tag `v15.18`；CLI 不选择“最接近”的 Tag。多个精确候选时返回歧义；没有匹配时由 Skill 明确改走所选产品分支的 latest 流程。
- 下载和索引过的版本默认长期保留，不因同步新版本而自动删除；相同文件内容、AST 中间体按 SHA-256 跨版本复用。
- 保留 `wowdoc doctor` 作为只读总检查命令；下载、更新、重建和修复仍由对应原子命令显式执行。
- npm 首版支持 Windows、Linux、macOS；本机缺少 macOS 环境时，macOS 的安装与运行验收由 GitHub Actions 的真实 macOS runner 完成。
- Skill 更新不得覆盖用户修改：检测到修改时自动备份旧 Skill，再安装新版；备份位置必须在命令结果中明确返回。
- 提供 `wowdata clean`，默认是只读预览，只有明确确认后才执行清理。
- `wowdata clean --apply` 的默认执行范围不包含任何旧版本；旧版本只能通过显式选择删除。
- GitHub 访问顺序为：显式环境变量凭据、本机 `gh` 已登录凭据、匿名访问。凭据只在请求期间使用，不写入 wowdoc 配置、日志或命令结果。
- 源码仓库使用包含全部分支、Tag 和 Commit 关系的部分 clone；选中某个版本时才获取该 Commit 需要的文件内容，并写入去重对象缓存。
- `npm install` 不下载源码；用户显式运行 `wowdata init` 后，一次准备全部默认基线源码和索引。
- `init` 的默认热数据范围是每个产品分支最多 50 个 catalog 匹配 Tag，加上该分支当前 head；后续新 Tag 由 `source sync` 发现、`index refresh` 增量建立，已建旧版本不自动删除。
- 分支只包含 catalog 明确声明的产品分支，不包含功能、机器人或临时回滚分支。初始第三方范围为 ElvUI `main/ptr`、WeakAuras2 `main`、NDui `master/Classic/Era/Anniversary/Titan`、EllesmereUI `main`。
- 产品分支清单来自 Skill Reference 中随版本发布的机器可读 source catalog，不硬编码在 CLI 逻辑里。
- Tag 由 catalog 的产品线规则归属到分支；同一个 Tag/Commit 服务多个产品线时只解析和存储一次。没有匹配 Tag 的 PTR 等分支只预建当前 head，不拿其他分支的 Tag 凑数。
- 热 Tag 必须同时满足“从产品分支可达”和“匹配 catalog 的产品版本规则”；共享历史中的其他客户端 Tag 不得用于凑满 50 个。
- `ast-grep` 不进入产品运行时依赖。当前五仓库 3659 个 Lua 文件已由 `gopher-lua/parse` 在 BOM/shebang 归一化后全部成功生成 AST；XML 使用 Go 标准库解析结构，XML 内嵌 Lua 片段再交给同一 Lua 解析器。
- 索引对 AddOn 自有实现、官方生成 API、生成数据、本地化、vendored/externals 和仓库工具保存文件角色。默认排序优先项目实现与权威 API，避免超大 ModelPaths、Locale 或重复第三方库污染结果。
- 插件版本只认 Git Tag/Commit。发布平台安装包与 Tag 源码可能因 `.pkgmeta`、externals 或占位符替换而不同；该差异作为 Reference 中的已知限制返回，不建立 package view，也不宣称查询的是用户安装包逐字节内容。
- 查询命令保持只读：Skill 负责把自然语言归一化为 source、产品分支、ref 和 topic；CLI 固定不可变 snapshot，依次执行精确符号、结构化关系和 FTS5 召回，再从内容对象补齐证据。snapshot 未准备时只返回原子准备步骤，不在查询命令内联网、解析或写库。
- Skill 的版本回退只响应 CLI 的 `version_not_found`。`ambiguous_version`、`unsupported_build`、`ref_not_found` 和更新失败保持原错误；回退到 latest 后的结果必须携带请求版本、实际产品分支、resolved Commit 和显式警告。
- `source check` 只检查远端 ref 是否变化；`source sync` 只获取 Git 元数据和源码对象；`index refresh` 只解析变化文件并原子发布新 snapshot，三者职责不混合。
- 更新失败时保留旧 snapshot。普通查询仍可使用并明确显示其 Commit；明确要求“最新”的查询若无法确认或完成更新，则返回更新错误和最后已知版本，不能把旧版本冒充最新。
- 未初始化时，数据命令统一返回 `not_initialized` 和下一步 `wowdata init`，不隐式开始耗时下载。
- 初始化支持断点续跑，成功项不会因其他仓库失败而回滚或重复下载、重复解析。
- 初始化采用整体就绪门槛：全部默认基线完成前不开放任何源码查询。
- Git 仓库使用 bare mirror，不存在共享 checkout。每个解析任务先把 branch/Tag/version 固定为 immutable Commit，再在 `~/.wowdoc/tmp/worktrees/<task-id>` 创建独立 `git worktree add --detach <commit>`，只从该任务 worktree 解析 Lua/XML/TOC/素材；snapshot 完整发布后删除 worktree 并执行受控 prune。异常退出留下的 worktree 只能在任务租约到期后清理。查询始终只读 SQLite、manifest 和 objects，不依赖 worktree。
- 不同 Agent 可以同时读取不同 Commit 的 snapshot；fetch 只更新镜像中的 refs，不改变已固定查询的 Commit。索引按 snapshot 分开构建并原子发布。
- Git 版本信息不写入单文件 AST。snapshot manifest 记录 `source + Commit + path -> content hash -> AST hash`，SQLite 用 `snapshot_id` 关联可复用的文件级语义数据和 snapshot 级关系。
- JSON AST 保留为可审计、按内容复用的原始中间体；SQLite 保存 Agent 查询需要的规范化结果。普通查询不需要重新加载整棵 AST。
- 图片、材质、字体、声音等二进制素材不写入 AST 或 SQLite blob。热 snapshot 的素材内容按 SHA-256 跨分支和 Tag 去重保存在共享 objects，分支 DB 只保存资产元数据、snapshot/path 映射和 Lua/XML 引用关系。
- 同一个 snapshot 同时只构建一次。后来请求者默认立即获得 `operation_in_progress`、任务状态和建议重试时间，也可显式设置最长等待时间；它不会重复下载或重复解析。
- SQLite 不使用单一查询库：每个 catalog 产品分支拥有独立索引 DB，保存该分支 head、50 个热 Tag 和按需建立的旧 snapshot；全部分支库开启 WAL。小型 `catalog.sqlite` 只保存 source/ref/Tag/Commit 映射、初始化状态和任务租约。
- 同一分支内 WAL 允许多 Agent 读和单写者增量更新；不同分支 DB 可以并行写入。跨分支 `diff` 分别固定两个 DB 的只读事务后由查询层合并，不切换或复制工作目录。
- 用户已确认上述完整目标和分库方案，可以进入重写实现。
- 用户已确认更新后的完整契约：插件版本以 Tag/Commit 为准，Tag 匹配失败由 Skill 显式回退到同一产品分支最新 Commit；插件 Release 安装包不进入产品数据模型；查询链路保持只读并采用结构化索引加 FTS5。
- 用户已确认本轮补充契约：Git 采用 bare mirror，并为每个固定 Commit 的解析任务创建带租约的独立 detached worktree；`wowdata init` 缺少 Git 时按平台主动调用可审计的包管理器安装并验证；50 条真实问题只按返回代码参考的正确性、相关性、上下文完整性和版本/Commit/path/line 可追溯性验收，不以退出成功、非空结果或返回率代替质量。

# Open questions

- 无。

# Verification expectations

- 在 Windows、Linux、macOS 验证 npm 全局安装、升级与卸载，以及 `wowdoc`、`wowdata` 的 PATH 可用性。
- 用固定 fixture 验证原子命令 JSON schema、退出码、错误码和 stdout/stderr 边界。
- 用两次仅改变少量文件的源码快照验证首次全量、后续增量和跨 commit 中间体复用。
- 用实际五仓库样本记录原始源码、JSON AST、SQLite、snapshot manifest 的字节数和构建耗时，校准初始化前的磁盘空间预估；空间不足时必须在下载前给出所需空间和当前可用空间。
- 注入网络、GitHub HTTP、权限、磁盘、锁和索引损坏故障，验证原子切换和最后可用状态不被破坏。
- 在 Windows、macOS 和各 Linux 包管理器 fixture 中验证 Git 缺失时的探测、执行前提示、安装、PATH 重新发现、`git --version` 验证，以及管理员权限不足、包管理器缺失、用户取消和安装失败的稳定错误码；`doctor` 仍只读，不触发安装。
- 并发启动同仓库不同 Commit 和同 Commit 的解析任务，验证每个任务使用独立 detached worktree、同 Commit 构建锁去重、完成后删除/prune、租约未到期不清理其他任务 worktree，且查询过程从不访问 worktree。
- 验证 npm tarball 不包含源码缓存、第三方仓库、MCP/HTTP server 或捆绑 exe，并验证 Skill 安装内容与版本。
- 验证 GitHub Actions 的测试、构建、npm provenance/发布门禁和 tag/version 一致性。
- 在五个真实仓库上运行不少于 50 条用户问题模拟，覆盖所有 catalog 产品分支并抽取多个 Tag；逐条评估 top result 是否回答问题、代码 excerpt 是否与固定 Commit 原文一致、版本/分支是否选对、证据是否足够让 Agent 作答，并汇总准确率、无结果、错分支、错版本、定位错误和低质量召回。
