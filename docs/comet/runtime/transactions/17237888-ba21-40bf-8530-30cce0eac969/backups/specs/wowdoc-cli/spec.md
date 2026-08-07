# wowdoc CLI 完整目标规格

## 产品边界

wowdoc 是供 Agent 调用的本地 CLI 产品。安装包提供 `wowdoc` 查询入口、`wowdata` 数据与安装生命周期入口，以及安装到用户级 Agent Skill 目录的语义桥接资料。产品不包含 MCP stdio、MCP HTTP 或常驻 HTTP 服务。

## 安装与分发

- npm 包必须支持全局安装，并在受支持平台暴露 `wowdoc` 与 `wowdata`。
- 首版支持 Windows、Linux 和 macOS。每个平台的发布产物必须经过对应 GitHub Actions runner 的 npm tarball 安装和命令运行测试；macOS 不依赖开发者本机验证，必须保留 macOS runner 的真实运行证据。
- npm 包的正式 registry 名称是 `@follenfang/wowdoc`，标准全局安装命令是 `npm install -g @follenfang/wowdoc`；实现、文档、CI 与 Skill 必须统一使用该名称。
- 安装脚本必须把 Skill 安装到 `~/.agents/skills/wowdoc`。Skill 只能包含说明、Reference 和必要的文本资源，通过 PATH 调用 CLI，不得捆绑平台二进制。
- npm 包保存 Skill 文件清单与 hash。升级时，如果现有 Skill 与已安装清单一致，可以原子替换；如果任何受管文件被修改，先把整个现有 Skill 目录原子移动到带时间和内容标识的备份路径，再安装完整新版，并在 JSON 结果中返回 `skillBackupPath`。
- Skill 备份不能被后续更新静默覆盖。`wowdata uninstall` 的删除预览必须同时列出当前 Skill 和由 wowdoc 创建的 Skill 备份，用户确认彻底卸载后一起删除。
- 重复安装或升级必须幂等。用户目录不可写、CLI 产物不支持当前平台或 Skill 安装失败时，安装必须失败并给出明确错误，不留下被当成成功安装的残缺状态。
- GitHub Actions 必须在受支持平台执行测试和产物验证，构建 npm tarball，并仅在 tag、包版本和质量门禁一致时发布。发布使用 npm 受支持的可信发布或最小权限凭据机制，并生成可核验 provenance。
- 跨平台门禁至少实际运行 `wowdoc --version`、`wowdata --help`、`wowdoc doctor` 和不访问外网的 fixture 查询，不能只检查编译成功。

## 命令契约

- `wowdoc` 保持这些原子动作：`query`、`explore`、`inspect`、`diff`、`validate`、`index build`、`index refresh`、`index status`、`source check`、`source list`、`source sync`。
- `wowdoc doctor` 是只读总检查入口。它检查 CLI/Skill 版本、`~/.wowdoc` 目录与权限、可用磁盘空间、Git 和 GitHub 访问、source catalog、源码 snapshot、AST 中间体与 SQLite 索引，并为每项返回 `ok|warning|error`、稳定代码和建议的下一条原子命令。
- `doctor` 不得下载源码、修改配置、重建索引、清理数据或自动执行建议命令；Agent 根据它返回的 `nextSteps` 再调用 `source sync`、`index refresh` 等动作。
- 一个原子命令只执行一个明确动作，不根据自然语言自动串联工作流。需要多步处理时，Skill 读取前一步的 `nextSteps` 后决定下一条命令。
- 普通查询不自动联网或更新。用户意图明确要求“最新”时，Skill 必须先调用 `source check` 检查目标 source/product branch；有变化时调用 `source sync` 获取 refs 和源码对象，再调用 `index refresh` 增量构建并发布 snapshot，最后把返回的 `resolvedCommit` 传给查询命令。
- `source check` 不修改本地源码或索引；`source sync` 不解析或发布索引；`index refresh` 不改变远端 ref。每一步失败都返回可继续的 `nextSteps`，Skill 不跳过失败步骤。
- 普通查询可继续使用最后已发布 snapshot 并返回其准确 Commit。明确要求“最新”但远端检查、同步或索引失败时，命令链返回失败和 `lastKnownSnapshot`，不得把它标记为最新结果。
- `wowdata` 是独立可执行入口，提供 `init`、`update`、`clean` 和 `uninstall`。
- `wowdata init` 创建并验证 `~/.wowdoc` 目录、读取 Skill Reference 随版本发布的默认 source catalog、获取 catalog 声明的官方及第三方产品分支源码，并为每个产品分支当前 head 与最多 20 个 catalog 匹配 Tag 构建 AST 中间体和 SQLite 索引；分支不足 20 个匹配 Tag 时全部构建。命令完成后，本地无需再次联网即可查询这些 snapshot。
- `wowdata init` 若启动时找不到 Git，必须主动帮助安装而不是只返回 `git_not_found`。CLI 先探测平台上已经存在的包管理器并把安装器、包名和将执行的参数输出到 stderr：Windows 优先 winget 的 `Git.Git`，随后只考虑已安装的 scoop/choco；macOS 优先已安装的 Homebrew，否则调用 Xcode Command Line Tools 安装入口并说明该流程需要系统交互；Linux 依次选择已存在的 apt、dnf、yum、pacman、zypper。CLI 不下载或执行来源不明的安装脚本。
- 安装命令完成后，`init` 必须重新发现 PATH、运行 `git --version` 并保存非敏感诊断，然后才能继续源码初始化。需要管理员权限、没有支持的包管理器、用户取消、安装器失败、PATH 未刷新或版本验证失败分别返回稳定错误码和精确 `nextSteps`，不得把 Git 未就绪的状态标为 init 成功。`wowdoc doctor` 仍然只读，只报告 Git 状态和建议，不触发安装。
- 初始化对每个基线完整解析 Lua、XML 和 TOC：可审计的原始 AST/CST 以 JSON 内容对象保存；符号、定义、引用、调用关系、XML 节点/继承、TOC 字段、文件定位和全文检索数据写入 SQLite。全局 ready 只在这些查询数据全部发布后成立。
- “最多 20 个 Tag”按 catalog 中的产品分支分别选择。catalog 使用 Tag 前缀、版本范围、TOC/client 证据和 Git 祖先关系声明归属；按 Git `creatordate` 倒序选择，annotated Tag 使用 tagger 时间，lightweight Tag 使用 Commit 时间，并用 Tag 名作为稳定并列排序键。初始化状态保存实际选中的 Tag 和 resolved Commit，重试期间不因远端新增 Tag 改变本次范围。
- 同一个 Tag/Commit 可以关联多个产品分支或 client，但源码对象、AST 和查询事实只保存一次。某分支没有 20 个匹配 Tag 时不得从其他分支填充；没有匹配 Tag 的分支只构建当前 head。
- 热数据之外的 Tag/Commit 关系仍写入 catalog。首次请求冷版本时，Skill 使用原子 `source sync --ref` 与 `index build --ref` 增量准备该 snapshot；准备完成后与初始化 snapshot 一样长期保留和查询。
- npm 安装过程不得调用 `wowdata init` 或下载源码。初始化只能由用户或 Agent 显式触发，并使用持久状态记录每个仓库的下载、解析和索引进度。
- 初始化以 repository/source snapshot 为独立进度单元。某项失败时，已经完整校验并激活的其他项继续保留；总命令返回非零退出码、完成项、失败项及每个失败项的重试建议。
- 再次执行 `wowdata init` 时先验证已完成项，跳过仍然有效的下载和索引，只继续失败、未完成或校验失效的步骤。重试不得重新解析 hash 与 parser schema 均未改变的文件。
- 首次初始化采用整体就绪门槛。只有 catalog 中全部默认基线都完成下载、校验、解析和索引后，才原子写入全局 `ready` 状态；在此之前，所有数据查询返回 `initialization_incomplete`、完成/失败/待处理计数和 `wowdata init` 下一步。完成项仍保留用于续跑，但不能提前查询。
- `wowdata update` 通过 npm 更新 `@follenfang/wowdoc`，并由安装流程同步更新 Skill；它不得下载或更新源码，也不得刷新索引。源码和索引更新分别由 `wowdoc source sync` 与 `wowdoc index refresh` 显式执行。
- `wowdata uninstall` 是彻底卸载命令。执行前必须明确列出将删除 npm 包、`~/.agents/skills/wowdoc` 和整个 `~/.wowdoc`，并要求用户确认；无人值守调用必须显式传入确认参数。确认后删除配置、源码、对象、AST 中间体、索引、状态、锁、日志和临时文件，不保留缓存。
- 任一步骤失败时必须返回未完成项，不能谎报彻底卸载成功；删除数据后再调用 npm 自卸载时，必须使用可跨平台等待的独立子进程或等价机制，避免删除正在运行的程序导致半完成状态。
- `wowdata clean` 默认是只读预览，返回候选项、每项被哪些 snapshot 引用、预计释放字节数和不会删除的活动数据。只有显式传入执行确认参数后才允许删除。
- 清理使用引用关系判断共享对象和 AST 中间体；仍被任何保留 snapshot 引用的内容不得删除。删除中断后必须可再次运行并得到一致结果。
- `wowdata clean --apply` 未提供 snapshot、版本或时间范围选择时，只删除可安全识别的临时文件和零引用缓存，不删除任何旧 snapshot。删除旧版本必须显式选择；活动 snapshot 始终受保护，不能由 clean 删除。
- 所有业务命令默认输出版本化 JSON envelope，至少包含 `ok`、命令版本、输入选择、resolved source/ref/build、结果或 error、diagnostics 和 `nextSteps`。成功退出码为 0；失败使用非零退出码和稳定错误代码。
- 人类可读日志进入 stderr，stdout 保持为完整、可解析的单一结果。`--help` 和 `--version` 可使用人类可读文本。
- 未完成首次初始化时，所有依赖源码或索引的命令必须快速返回非零退出码、`not_initialized` 错误和 `wowdata init` 下一步，不得自动下载。`--help`、`--version`、`wowdoc doctor`、`wowdata init`、`wowdata clean` 和 `wowdata uninstall` 在未初始化状态仍可运行；`doctor` 应把初始化缺失报告为明确检查项。

## 用户目录

`~/.wowdoc` 必须由 CLI 按需创建，并使用稳定职责目录：

```text
~/.wowdoc/
  config/       用户配置、source catalog 与默认选择
  sources/      按 source/repository 保存的源码 checkout 或只读快照
  objects/      按 SHA-256 保存的去重文件内容
  ast/          按文件内容 hash 与 parser schema 保存的 JSON 中间体
  indexes/      按 source/product branch 保存的 SQLite WAL 查询库
  state/        已安装版本、resolved ref/build、活动 snapshot 与迁移状态
  locks/        下载、解析、索引和升级互斥锁
  logs/         有界、可轮转且不含凭据的诊断日志
  tmp/          下载、解压、迁移和原子切换的临时工作区
```

- 路径必须基于用户 home 解析，并允许通过明确的 CLI 参数或环境变量覆盖，以支持测试和隔离运行。
- CLI 不得依赖当前工作目录保存持久状态。
- 每个可复用产物都必须记录 schema/version；不兼容时执行显式迁移或重建，不能静默读取旧格式。

## 源码与版本模型

- source catalog 同时描述官方 WoW UI 源码及 ElvUI、WeakAuras2、NDui、EllesmereUI 等第三方源码。
- source catalog 是 Skill Reference 中的机器可读、版本化文件，CLI 只执行其中明确声明的 repository、产品分支、Tag 规则和版本别名，不从远程分支名称自行猜测产品含义。`wowdata init` 把本次 catalog 及 hash 复制到 `~/.wowdoc/config`，保证初始化和查询可追溯。
- 初始 catalog 的第三方产品分支为：ElvUI `main`、`ptr`；WeakAuras2 `main`；NDui `master`、`Classic`、`Era`、`Anniversary`、`Titan`；EllesmereUI `main`。WeakAuras2 的历史 `cata`、ElvUI 的功能分支、EllesmereUI 的 `revert-*` 等不进入默认热数据，除非后续 Reference 版本明确把它们声明为产品分支。
- 官方仓库声明 `live`、`ptr`、`ptr2`、`beta` 以及 catalog 当前支持的 Classic/Era/Anniversary/Titan 等产品分支；`automation` 等非产品分支不初始化。准确清单随 Reference 更新，不硬编码进 CLI。
- 每个 Git 仓库维护包含全部远程分支、Tag 和可达 Commit 的本地镜像元数据。实现采用 blob 过滤的部分 clone 或具有同等行为的方式，首次同步不下载所有历史文件内容；选中 snapshot 时才获取该 Commit 所需 blob，并放入按 SHA-256 去重的对象缓存。
- 本地 Git 存储使用 bare mirror，不存在供所有 Agent 共同切换分支的 checkout。每个解析任务在取得 `source + resolvedCommit + parserSchema + indexSchema` 构建锁后，以不可变 Commit 创建独立 detached worktree：`git --git-dir <mirror> worktree add --detach ~/.wowdoc/tmp/worktrees/<task-id> <commit>`。Lua、XML、TOC 和素材枚举与读取只来自该任务 worktree；不同 Commit 的任务可拥有不同 worktree 并行执行，同一 Commit/schema 的后来任务复用既有任务状态而不重复创建。
- snapshot 完整发布或任务明确失败后，所有者删除自己的 worktree并在没有冲突写操作时执行 `git worktree prune`。进程异常退出时，遗留 worktree 与任务租约绑定；后续进程只有在租约到期并验证路径位于 `~/.wowdoc/tmp/worktrees`、不属于活动任务后才能清理。普通查询始终只读 SQLite、snapshot manifest 和 objects，不读取 worktree、不执行 checkout，也不因 worktree 清理受阻而返回不完整结果。
- Git clone 提供 branch、Tag、Commit 关系，并且这些 Git 对象是插件版本解析的唯一真本。CLI 不调用插件发布平台的 Release API，不下载 Release 附件，也不建立安装包视图。
- `.pkgmeta`、externals、目录移动和占位符替换可能让用户安装包与 Tag 源码不同。wowdoc 必须在查询结果和 Skill Reference 中把“结果来自 Tag 源码”作为明确来源，不把它描述成安装包逐字节复现。
- CLI 接收明确的 source/source-set、client、build、ref 或 resolved commit；Skill Reference 负责把 `retail`、`classic`、`classic-era`、`ptr`、`ptr2`、Titan 等用户叫法映射到当前可用选择。
- branch、tag、commit 和游戏 build 是不同字段。CLI 必须保存请求值和最终解析出的 commit，不把 branch 名当作 build，也不根据名字宣称第三方兼容性。
- 未指定版本时，CLI 同步所选 source/client 对应分支的最新提交，包括正式服分支，不优先停留在最新稳定 Tag。
- 指定完整 Commit 时直接解析该 Commit；指定 Tag 时解析 Tag 指向的 Commit；指定插件显示版本时只按 catalog 的精确规则归一化到 Tag，再解析为 Commit。`source list` 和 `source check` 必须能返回 `requestedVersion`、`tag`、`resolvedCommit` 及它们之间的关系。
- 插件界面中显示的版本号必须可直接用作版本输入。catalog 可以声明无歧义的前缀规则，例如输入 ElvUI `15.18` 精确匹配 Tag `v15.18`；规则必须随 source 配置保存并可审计，不能用模糊的最近版本替代精确匹配。
- 如果输入同时精确匹配多个 Tag，命令返回 `ambiguous_version` 和候选项；完全匹配不到时返回 `version_not_found` 和可用版本查询方法。两种情况都不得静默选择其他 Commit。
- CLI 对 `version_not_found` 保持原子和严格，不在同一命令内改选分支 head。Skill Reference 定义上层回退：收到该错误后，固定已经选择的 source/product branch，执行 `source check`；远端有变化时依次执行 `source sync`、`index refresh`，最后以该分支最新 resolved Commit 重试原查询。
- latest 回退的最终结构化结果和 Agent 回答必须包含 `requestedVersion`、`matchedTag=null`、`resolutionMode=latest_fallback`、实际 product branch、`resolvedCommit` 和“结果来自最新代码而非请求版本”的 diagnostic。该结果不能写回成请求版本的映射。
- `ambiguous_version` 不触发 latest 回退，Skill 必须保留候选项供消歧；`unsupported_build`、`ref_not_found`、远端检查失败、同步失败或索引失败也不触发回退，避免跨产品线或用未确认的旧 snapshot 冒充最新。
- 第三方仓库没有某个 build/ref 时，结果必须是稳定的 `unsupported_build` 或 `ref_not_found` 类错误，并附可用版本与建议执行的 `source list`/`source check`，不能回退到未声明版本。
- 默认获取对应分支的最新提交；指定 Tag 或 Commit 时解析并锁定准确 Commit。该规则必须同时体现在 CLI、state 和 Skill Reference。
- 热 Tag 必须先满足目标产品分支的 Git 可达关系，再满足 catalog 的产品线版本规则。共享祖先上的其他客户端 Tag 不得用于补足 20 个；不足 20 个匹配 Tag 时只建立实际匹配项。

## 解析与索引

- 受支持的 Lua/XML/TOC 等源码按语言选择可审计解析器。Lua 结构检索以 AST 为基础；纯文本上下文可作为补充，但不能伪装成语义解析结果。
- 正式产品不调用或捆绑 `ast-grep` CLI。Lua 使用锁定版本、源码可审计且编译进二进制的纯 Go 解析库；当前基线采用已通过五仓库 3659 文件探针的 `github.com/yuin/gopher-lua/parse`。XML 结构使用 Go 标准库，SQLite 驱动不得要求用户安装 CGO 工具链。
- XML 除节点和属性外，还必须提取内嵌 `OnLoad`、`OnEvent`、`OnClick` 等 Lua handler，以及 `function`、`method` 形式的 handler 边。内嵌代码按保留原 XML 行号的 Lua 片段解析；单个 handler 失败只记录局部 diagnostic，不丢弃整个 XML 文件。
- TOC 顺序、XML `Include`/`Script` 递归、根目录隐式 `Bindings.xml` 和 AddOn 依赖共同形成加载图。默认语义索引以所选 client 实际可达文件为主，未加载的仓库工具仍可作为低优先级 repository 文件查询。
- 每个文件保存 `project`、`official-generated-api`、`generated-data`、`locale`、`vendor`、`tool` 等角色。权威 API 和项目实现进入正常结构化召回；超大模型路径表、Locale、vendored externals 与工具文件默认降权，并可按角色显式过滤。
- Lua 调用、继承与资源关系保存 `exact`、`inferred`、`dynamic-unresolved` 等可信度。动态全局、Mixin、Hook、回调、运行时字符串和 `loadstring` 不能解析为确定目标时，保留来源与诊断，不生成伪精确边。
- 首次建立 snapshot 时解析全部适用文件。更新时先计算 SHA-256，只对内容 hash 或 parser schema 改变的文件重新解析。
- 相同文件内容跨 source、branch 或 commit 共享 `objects` 与 `ast` 中间体。中间体是可检查的 JSON，包含输入 hash、语言、解析器及 schema 版本、提取节点和来源定位。
- 解析过程一次只需持有单文件 AST，完成提取后释放；默认并发必须有界，并能根据机器资源配置，目标范围为 4 到 8 个 worker。
- SQLite 采用一个小型 `state/catalog.sqlite` 加多个分支查询库。`catalog.sqlite` 只保存 source、repository、产品分支、Tag/build/Commit 映射、初始化状态、活动 snapshot 指针和任务租约；不保存 Lua/XML/TOC 查询事实。
- 每个 catalog 产品分支使用独立的 `indexes/<source-id>/<branch-id>.sqlite`，保存该分支 head、20 个热 Tag、按需建立的冷版本及其规范化节点、关系、FTS/定位和 snapshot 关联。路径 ID 来自 catalog 的稳定标识，不能直接信任远端分支字符串拼接路径。
- `catalog.sqlite` 和所有分支库启用 WAL、foreign keys 与有界 busy timeout。同一分支只有一个写者，但多个 Agent 可以持续读取；不同分支库可以并行写入。事务失败不得改变该分支最后已发布 snapshot。
- 耗时的下载、AST 解析和事实提取在数据库写事务外完成；单写者以批次写入未发布 snapshot，最终短事务原子标记 ready/active。WAL checkpoint 在无活动写事务时受控执行，不能删除其他进程仍需要的 WAL。
- 跨分支 `diff` 分别打开两个分支库并固定只读事务，在查询层按稳定键合并结果。跨分支比较不能要求把所有索引合并回一个全局查询 DB。
- schema 不兼容时为目标分支构建新的 generation DB，验证后由 catalog 原子切换活动 generation；旧 generation 在现有读事务结束前保留，随后才进入显式清理候选。
- 普通 `query`、`explore`、`inspect`、`diff` 和 `validate` 对已就绪 snapshot 只读取 SQLite、snapshot manifest 和按 hash 保存的源码对象，不读取可变 Git ref、不执行 checkout、不访问网络，也不需要把完整 AST 重新放入内存。
- `index status` 必须能报告 active snapshot、源码状态、对象/AST 复用数量、待解析文件、索引 schema 和损坏/过期诊断。
- 每次成功同步的 resolved Commit 都形成可重复选择的 snapshot。新 snapshot 激活后，旧 snapshot 的源码关系和索引仍保留并可用于查询与 `diff`，不得后台自动删除。
- `objects` 与 `ast` 按 SHA-256 跨所有 snapshot 去重；snapshot 只保存自身文件清单和内容引用，不能为每个版本复制一套相同中间体。
- 彻底卸载会删除所有 snapshot。除此之外，旧版本只能由显式的 `wowdata clean` 版本选择删除，普通同步和索引刷新不能隐式回收。

## Skill 语义桥接

- Skill 根据 Agent 问题选择 source set、client、build/ref、topic 和最小原子命令。
- Skill 先尝试把插件显示版本精确归一化到 Tag。只有 CLI 返回 `version_not_found` 时，才按 Reference 切换为同一产品分支的 latest；这一策略属于 Skill 语义编排，不改变 CLI 的 Tag 解析原子契约。
- Skill Reference 维护官方与第三方仓库、分支与产品线叫法、TOC/build 范围、已知缺口和兼容判断规则；这些资料可以独立更新，不改变 CLI 原子命令。
- Skill 收到缺失或过期索引诊断时，优先调用 `index status` 或 `source check`，再根据结构化 `nextSteps` 决定是否同步或构建。
- Skill 的回答必须保留 CLI 返回的 source、resolved commit/build、文件路径、定位、诊断和版本缺口，使结论可追溯。

## 查询与搜索路径

- Agent 自然语言首先由 Skill Reference 归一化为明确参数：`sourceId`、产品分支、Tag/build/Commit、`topic`、查询词和结果上限。Skill 不解析源码，也不自行生成搜索结果。
- CLI 收到请求后，从 `catalog.sqlite` 把输入版本解析为唯一 `resolvedCommit` 与 `snapshotId`，并在命令开始时固定它们。没有明确版本时使用该产品分支当前已发布 snapshot；普通查询不读取会变化的远端 branch ref。
- snapshot 未就绪时，查询返回 `snapshot_not_ready`、准确 source/ref/Commit 以及 `source sync --ref`、`index build --ref` 等原子 `nextSteps`。`query`、`explore`、`inspect` 和 `diff` 本身不下载、不解析、不写索引；Skill 完成准备命令后重试原查询。
- 查询命令不提供隐式准备模式。即使调用方允许等待，`query`、`explore`、`inspect`、`diff` 和 `validate` 也只读取已发布 snapshot；下载、解包、解析和建库必须由返回的独立 `source sync`、`index build|refresh` 原子步骤完成。
- snapshot 已就绪时，CLI 对目标分支 DB 开启只读事务，并把所有候选限制在 `snapshot_id`。一次查询不会跨入其他 snapshot，也不会因为并发更新看见一半新、一半旧的数据。
- 查询规划按 topic 选择结构化表：API/事件/常量/安全元数据查专用 API 表；Lua 符号与调用关系查 symbols/references/edges；XML 模板、节点、继承和脚本查 XML 表；TOC 元数据、Interface、依赖和加载顺序查 TOC 表；不明确 topic 时执行有界的多路召回。
- 召回顺序固定为：完整 qualified symbol 精确匹配、名称/路径前缀匹配、结构化关系匹配、FTS5 标识符/文本匹配。代码标识符保留 `.`、`:`、`_` 等规范化键，并使用专用精确列；FTS5 负责词语、注释、文档和源码片段，不替代符号表。
- 排序是可解释且确定的：精确符号高于前缀，定义高于引用，所选 topic 高于泛文本，项目实现和权威 API 高于生成数据/Locale/vendor/tool，关系距离更近且可信度更高者优先，FTS5 BM25 只在同类候选中贡献分数；最后按 path、line、stable id 打破平分。结果返回 `matchedBy`、文件角色、关系可信度与分数组成，不能只返回不可解释的总分。
- 命中行只保存定位与必要搜索文本。最终 excerpt 从 `objects/<contentHash>` 按行号读取，并验证 snapshot manifest 中的 path/content hash，避免 SQLite 重复保存完整源码，也保证片段对应固定 Commit。
- `query` 返回跨 topic 的短列表和建议下一步；`explore` 返回区域、文件、符号及关系的宽列表；`inspect` 以一个精确符号或路径为中心返回定义、签名、引用、调用方/被调用方和片段；`diff` 在两个固定 snapshot 的只读事务中按稳定符号键、签名和内容 hash 比较新增、删除、改变；`validate` 解析用户插件并用目标 snapshot 检查 API、TOC、依赖和兼容性。
- 同一分支的查询共享一个 WAL DB，但每个进程使用独立只读连接和短事务。跨分支 `diff` 分别读取两个分支 DB，在进程内合并稳定键；查询不通过写全局临时表实现跨库比较。
- 默认检索不使用向量。以后增加语义召回时也只能作为可选候选来源，最终结果仍须绑定 snapshot、符号、path、line 和源码证据。
- 检索质量验收必须使用不少于 50 条插件作者真实问题形态的端到端场景。该验收只评价返回代码参考的质量，不以命令返回率、成功退出或结果非空作为质量通过。场景集覆盖 catalog 的每个产品分支与多个新旧 Tag，记录自然语言问题、Skill 选择的 source/product/ref/topic、实际 CLI 命令、期望代码事实和返回证据。每条结果都必须将 path、line、excerpt 和 content hash 回读到 resolved Commit 的 Git blob 核对，并独立评价正确性、相关性、上下文完整性以及版本/Commit/path/line 可追溯性；最终报告汇总各质量维度、代表性好结果、误导性结果、缺少上下文结果和改进决定。

### 分支 DB 核心数据

- `snapshots`：snapshot ID、Commit、ref/build、parser/index schema、ready/active 状态。
- `files` 与 `snapshot_files`：content hash、AST hash、语言、文件大小，以及 snapshot 中 path 到内容的映射。
- `symbols`、`references`、`edges`：Lua/API 定义、引用、调用与继承关系；文件级事实按 `parserSchema + contentHash` 复用，snapshot 级解析关系按 snapshot ID 保存。
- `api_entries`、`api_params`、`api_events`、`api_values`：官方生成文档和第三方可提取 API 数据。
- `xml_nodes`、`xml_attributes`、`xml_edges`：Frame、Template、Mixin、inherits、include 与 script handler。
- `toc_entries`、`toc_files`：Interface、依赖、SavedVariables、加载文件和客户端适用范围。
- `search_docs` 与 FTS5 虚表：符号文档、注释和有界源码 chunk；FTS 行引用 content hash 与行号，不成为源码真本。

## 素材与二进制资源

- Lua、XML、TOC 之外的图片、材质、字体、声音、模型和其他二进制文件不生成 AST，也不作为 blob 写入 SQLite。实际内容按 SHA-256 保存在共享 `objects/assets/<prefix>/<hash>`，同一内容跨 repository、分支、Tag 和 snapshot 只保存一次。
- 每个热 snapshot 必须枚举资产路径并物化其唯一内容对象，使初始化完成后可以离线检查当前 head 与 20 个热 Tag 的素材。冷版本仍按通用冷 snapshot 流程首次获取并长期缓存。
- 分支 DB 使用 `assets` 与 `snapshot_assets` 保存 content hash、Git blob OID、原始 path、规范化 WoW path、扩展名、检测 MIME、字节数、图片宽高/格式以及 snapshot 中的存在关系；不保存 base64、完整二进制或无界 EXIF/metadata。
- Lua AST 提取 `SetTexture`、`SetAtlas`、`CreateTexture`、字体/声音路径、FileDataID 和可静态确定的资源字符串；XML 提取 Texture、Font、Include、Script 等文件属性；TOC 文件列表也形成引用。它们统一写入 `asset_refs`，包含来源 content hash、path、line、引用类型、原始值和解析后的目标。
- WoW 路径解析保留原始大小写和分隔符，同时生成用于匹配的规范化键；校验可以发现缺失文件、大小写不一致、反斜杠、路径逃逸、未被引用资产和重复内容，但不能因两个不同路径 hash 相同就擅自改写插件源码。
- BLP、TGA、PNG、JPEG、DDS 等图片只用有界、源码可审计的 header/decoder 读取格式和尺寸。字体、音频和模型默认只嗅探格式与大小，不执行、不播放、不加载不受信任的字体。损坏或超限素材记录诊断，不阻塞其他源码建立索引。
- 预览图不在 `init` 中批量生成。`inspect` 确实需要视觉检查时才为受支持图片生成有尺寸上限的 PNG 预览，并按原素材 hash 缓存；命令返回原素材对象路径与预览路径，不把图像字节塞进 JSON。
- `query --topic asset` 按文件名、路径、扩展名、Atlas/FileDataID 和引用方查找；`inspect --path` 返回素材元数据、所在 snapshot、引用方和本地对象路径；`diff` 按规范化 path 与 content hash 报告新增、删除、内容或尺寸变化；`validate` 检查插件中的缺失、大小写和打包引用错误。
- 素材路径和引用说明进入 FTS5，但二进制内容不做 OCR、向量嵌入或视觉特征索引。需要判断“图片画了什么”时，由 Agent 根据 `inspect` 返回的本地素材/预览文件进行视觉检查。

## 更新、并发与故障

- 每条命令开始时把 branch 或 Tag 解析为不可变 `resolvedCommit`，之后整条命令只访问该 Commit 的 snapshot。并发 fetch 可以移动 branch ref，但不能改变已经开始的查询结果。
- snapshot 清单、对象和 AST 以 Commit/content hash 标识；SQLite 查询按产品分支 DB 隔离、在库内按 snapshot 隔离。新 snapshot 只在完整写入后原子发布，因此 Agent 1 查询最新 Commit 与 Agent 2 查询旧 Commit 不会互相切换源码或覆盖索引。
- fetch 写锁按 repository 设置，snapshot 构建锁按 source + Commit + schema 设置；不同已就绪 snapshot 可以并发读取，正在 fetch 或构建新 snapshot 时也不能阻塞已有 snapshot 的只读查询。
- Git 元数据、AST 和 snapshot 关系必须分层保存：Git mirror 保存 refs 与 Commit 图；单文件 AST 仅保存由文件内容决定的结构，不写 branch、Tag、Commit 或 checkout 状态；不可变 snapshot manifest 保存 `sourceId`、`resolvedCommit` 以及每个 path 对应的 Git blob/content hash/AST hash；SQLite catalog 保存 Tag/build 到 snapshot 的关系与发布状态。
- 文件级查询事实按 `parserSchema + contentHash` 去重，snapshot 查询通过 `snapshot_id -> path -> contentHash` 关联这些事实。调用关系等依赖整份源码的结果可以按 `snapshot_id` 保存。这样相同文件跨 Commit 只解析一次，不同 Agent 只是在同一只读索引中选择不同 snapshot。
- SQLite 使用 WAL 和单写者事务发布完整 snapshot；耗时的下载与解析在事务外完成。发布事务一次写入或切换 manifest、关系和 ready 状态，查询只能看到发布前的旧状态或发布后的完整新状态。
- 构建任务以 `sourceId + resolvedCommit + parserSchema + indexSchema` 为唯一键。第一个进程取得带租约的构建所有权；后来进程不得重复工作，默认返回 `operation_in_progress`、稳定任务 ID、当前阶段、进度和 `retryAfterMs`。调用方可传入有上限的等待时间，在期限内等待同一任务完成；超时仍返回同一任务状态。
- 构建进程异常退出后，租约到期可由后续调用接管；接管继续复用已经校验的对象和 AST，不删除已发布 snapshot，也不把未完成 snapshot 暴露给查询。
- 下载、解压、解析、索引和升级均在 `tmp` 中准备，经校验后原子切换；进程中断或校验失败时清理或隔离临时状态，保留最后一份可用数据。
- GitHub 限流、401/403、404、网络超时、DNS/TLS、代理、归档校验失败、git 缺失、ref 不存在必须使用不同稳定错误代码，并给出不泄露凭据的诊断与下一步。
- GitHub 身份使用顺序为 `GH_TOKEN`/`GITHUB_TOKEN` 等显式环境凭据、当前本机 `gh` 登录凭据、匿名访问。wowdoc 可以自动读取 `gh` 的当前登录凭据来避免匿名限流，但不得把凭据写入磁盘、日志、SQLite、子进程参数或 JSON 输出。
- 权限不足、磁盘空间不足、路径过长或非法、文件被占用、锁冲突、SQLite busy/corrupt、schema 不兼容必须可诊断。只读查询不能在索引已知损坏时返回看似成功的旧结果。
- 同一资源的写操作必须互斥；读操作只能看到完整的旧 snapshot 或完整的新 snapshot，不能看到构建一半的数据。
- 对失败操作，CLI 必须保证退出码、JSON error code、stderr 摘要和 state 状态一致。

## 明确排除

- 不构建或发布 MCP stdio、MCP HTTP、ticket 鉴权、HTTP server、Docker 服务镜像。
- 不默认构建向量嵌入或向量数据库。
- npm tarball 不包含第三方源码、完整 WoW 源码、用户索引、缓存或平台无关的旧服务端代码。
