# wowdoc CLI 规格

## 产品边界

wowdoc 是供 Agent 和插件作者调用的本地源码检索 CLI。npm 包只提供 `wowdoc` 一个可执行入口，并把配套 Skill 安装到用户级 Agent Skill 目录。仓库中的命令、文件名、目录名、代码、文档、测试、工作流和发布产物统一使用 `wowdoc`，不保留其他历史命令名称。产品不包含 MCP stdio、MCP HTTP、常驻服务或后台下载守护进程。

## 安装与发布

- npm 包名为 `@follenfang/wowdoc`，全局安装后只暴露 `wowdoc`。
- npm 安装只安装当前平台 CLI 和 `~/.agents/skills/wowdoc`，不得自动下载或解析源码。
- 当前平台二进制必须通过平台、架构和 npm 版本精确选择。首选平台专属 npm optional package，由 npm registry 下载、缓存、代理、重试和 SRI 完整性机制承载；受控 GitHub Release 回退必须保持相同的版本绑定与 SHA-256 验证。
- 安装过程必须显示阶段和有界下载进度。交互终端使用节流刷新的单行进度；非交互环境输出低频、可逐行留存的阶段、字节、耗时、速度和重试事件。
- GitHub Release 回退使用受管 `.part` 缓存和 HTTP Range。服务端支持有效 Range 时从已验证长度继续；忽略或错误响应 Range 时安全重启，绝不把拼接错误或截断内容发布为 CLI。
- 安装下载具有连接、无进度、单次尝试和总预算上限。408、429、瞬时 5xx、连接重置、DNS/TLS 瞬时错误按指数退避、抖动和 `Retry-After` 有界重试；401/403/404、平台不支持、配置错误和持续校验失败返回稳定诊断。
- 二进制只有在响应长度边界和发布 SHA-256 校验通过后才原子进入目标位置。中断、磁盘不足、校验失败或并发安装不得留下可执行的半成品。
- 同版本、平台、架构的并发安装通过受管跨进程锁协调；等待方报告状态，锁释放后复用已验证缓存。陈旧锁和过期临时文件只能在身份与所有权校验后回收。
- `WOWDOC_BINARY_DIR` 继续提供无网络的本地二进制注入，供 CI 和隔离安装使用。
- Release Tag 使用 `vMAJOR.MINOR.PATCH`；GitHub Actions 为 Windows amd64、Linux amd64/arm64、macOS amd64/arm64 各构建一个 `wowdoc` 产物，发布校验和，并发布与主包版本严格一致的平台 npm 包。
- npm 版本、平台包版本、Git Tag、CLI 发布版本和校验清单必须一致；三平台 runner 必须从实际打包产物完成安装和命令运行验证后才允许发布。

## 命令结构

- 根命令为 `wowdoc`。
- 生命周期命令为 `wowdoc init`、`wowdoc update`、`wowdoc clean`、`wowdoc uninstall`。
- 查询与维护命令继续通过 `wowdoc query|explore|inspect|diff|validate|doctor`、`wowdoc source ...` 和 `wowdoc index ...` 提供。
- 所有帮助、错误消息和 nextSteps 只能引用真实存在的 `wowdoc` 命令。

## 初始化与 Git 下载

- `wowdoc init` 创建并验证 `~/.wowdoc` 目录，读取随 Skill 发布的 source catalog，获取官方及第三方产品分支源码，并为每个产品分支当前 head 与最多 10 个匹配 Tag 构建 AST 中间体和 SQLite 索引。
- 初始化使用完整 bare Git mirror。一次 repository 同步后固定本轮 Commit，再以有界并行的 detached worktree 解析各产品分支和 Tag；不得用 shallow 或 partial history 降低版本覆盖。
- Git clone/fetch 必须强制可观察进度。stderr 中每个事件带 source ID、阶段、已接收对象或 Git 原生百分比、耗时和重试状态；多个并发 source 的输出不得形成无法归属的裸 Git 行。stdout 的 JSON envelope 不得混入进度。
- repository 网络任务全局最多三个并发，每个 repository 保持单写锁；一个 source 的瞬时失败在自己的重试预算内处理，其他可独立 source 继续完成并在最终结果中分别报告。
- Git 网络操作具有连接/低速无进展、单次尝试和命令总预算。DNS、TLS、代理、连接重置、服务端暂时不可用和网络超时有界重试；认证、权限、404、无效 ref、仓库配置和本地完整性错误快速失败并返回不同稳定代码与精确 nextSteps。
- 首次 mirror、partial mirror 替换和现有 mirror 更新都使用受管 staging 或 Git 可验证的活动仓库。重试与重新执行必须复用已完整接收的 Git 对象、已完成 ref 批次、repository、snapshot 和索引检查点；单个未完成 pack 可从该次尝试重新开始，不引入自定义字节级 Git 分发协议。活动 mirror 只有在目标 refs 存在且完整性检查通过后才原子切换。
- 用户取消立即终止 Git 子进程和后续重试，保留最后一个可验证恢复点。停流、超时、进程崩溃或磁盘失败不得把损坏 mirror、半发布 ref 或未完成 snapshot 标记为 ready。
- 相同原始内容按 SHA-256 复用 Pack 对象；相同解析单元按 content hash、language 与 parser/index schema 在 source 级共享事实库保存一次。snapshot 只在分支库保存路径、角色和共享内容身份映射。
- 每个构建任务把新源码、AST 和素材顺序写入独立 staging Pack，校验后原子发布不可变分段。未发布 staging 数据不属于 ready snapshot；显式 clean 可以清理过期 staging 和零引用分段。
- 每个产品分支保留独立 WAL SQLite 和本地 contentless FTS。FTS 文档集合、tokenizer、BM25 语料与确定性 tie-break 保持现有分支语义，不提升为改变排序范围的全局 FTS。
- 初始化全部默认基线完成后才原子写入 ready 状态；重复执行先验证并跳过仍有效的 repository、ref、snapshot 和索引，只继续失败、未完成或校验失效的步骤。
- 如果 Git 缺失，`wowdoc init` 使用现有平台包管理器安装帮助流程；安装后刷新 PATH 并用 `git --version` 验证。失败时返回稳定错误码和精确 nextSteps。

## Pack 与共享内容存储

- Pack 中每条记录包含格式版本、对象类型、codec、原始字节长度、压缩字节长度和 SHA-256；索引把 SHA-256 与对象类型映射到不可变 Pack 分段及 offset/length。
- SHA-256 始终根据解压后的原始字节计算。读取必须验证边界、长度和 hash；损坏、截断或类型不符返回稳定诊断，不能输出未经校验的代码证据。
- Pack 分段发布后不可原地追加或重写。并发任务各自生成 staging 分段，发布和共享事实合并使用有界锁及短事务；重复内容只产生一个有效内容身份，未引用重复分段可由显式 clean 回收。
- 共享事实库以 source 与 schema generation 隔离，保存 contents、字符串字典、symbols、edges、XML、TOC、asset facts、asset refs 和共享 search document 元数据；路径、角色、Tag、Commit 和 snapshot 归属不得进入共享事实。
- 相同字节以不同语言解析时共享原始 Pack 对象，但 AST、解析事实和 search document 必须按 language 与 schema 隔离。
- 旧 raw/gzip 单对象继续透明可读。新构建写 Pack；内容迁入 Pack 前后返回的原始字节、excerpt 和 SHA-256 必须一致。
- 新 schema generation 完整构建、验证并发布后才成为查询目标。中断或失败不得破坏旧 generation 和已发布 snapshot。

## 查询与跨文件搜索

- 查询把 branch、Tag 或版本解析为 immutable Commit，只读分支 SQLite、共享事实 SQLite、manifest 与 Pack，不依赖 worktree、Git checkout、网络或可变 ref。
- 分支库先按 snapshot 过滤真实成员和路径，再关联共享事实；FTS 和所有结果截断必须在目标 snapshot 成员约束下执行，不能让其他版本内容挤掉当前版本结果。
- 普通 Lua/XML 全文、注释、局部变量、字符串、精确符号、XML 定义、TOC、关系和素材查询覆盖保持不变。Pack 只用于读取最终命中的少量源码/AST/素材，不作为跨文件扫描入口。
- 同一共享内容在 snapshot 中映射到多个路径时，查询分别返回每个真实路径和角色；内容事实不能把首次遇到的路径固化为所有版本的证据路径。
- 结果保留 source、product、requested ref、resolved Commit、路径、行号、excerpt 和内容哈希。优化前后相同固定输入的命中覆盖、确定性排序和首条代码参考质量必须业务等价。
- Tag 是插件版本匹配的主要依据；没有匹配 Tag 时，Skill 引导 Agent 使用最新代码并明确 Commit。

## 更新、清理与卸载

- `wowdoc update` 通过 npm 更新主包、当前平台包及随包 Skill，不下载源码、不刷新索引，并使用与首次安装相同的进度、重试、完整性和原子安装保证。
- 源码和索引更新分别由 `wowdoc source sync` 与 `wowdoc index refresh` 显式执行。
- `wowdoc clean` 默认只读预览；只有显式确认后才删除临时文件、未发布的过期 staging Pack 和零引用缓存/分段，活动 snapshot 及其 Pack 记录始终受保护。
- `wowdoc uninstall` 先列出 npm 包、Skill 和 `~/.wowdoc` 数据；显式确认后彻底删除，允许通过已有参数保留 npm 包。

## 兼容性与质量门槛

- 未完成首次初始化时，依赖源码或索引的命令快速返回非零状态、稳定错误和 `wowdoc init` 下一步，不自动联网。
- `--help`、`--version`、`wowdoc doctor`、`wowdoc init`、`wowdoc clean` 和 `wowdoc uninstall` 在未初始化状态可运行；doctor 保持只读。
- 现有 CLI 命令、JSON 输出、错误码、版本解析、默认 10 Tag 热范围、全局 4-8 worker 预算和代码证据格式保持兼容；新增网络诊断必须稳定且不得泄露 token、认证 header、代理凭据或带认证信息的 URL。
- 存储与下载优化不得通过减少搜索覆盖、素材、AST、Tag、Git 历史或证据字段换取性能。50 条真实问题必须继续满足正确性、相关性、上下文完整性及版本/Commit/path/line/SHA-256 可追溯性。
- 内部 SQLite row ID、Pack offset、压缩字节、分段边界、数据库路径 generation、缓存临时文件名和性能统计不是业务身份，不要求逐字节相同。
