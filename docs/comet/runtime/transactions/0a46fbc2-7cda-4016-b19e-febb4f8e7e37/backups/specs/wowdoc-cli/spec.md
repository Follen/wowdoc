# wowdoc CLI 规格

## 产品边界

wowdoc 是供 Agent 和插件作者调用的本地源码检索 CLI。npm 包只提供 `wowdoc` 一个可执行入口，并把配套 Skill 安装到用户级 Agent Skill 目录。仓库中的命令、文件名、目录名、代码、文档、测试、工作流和发布产物统一使用 `wowdoc`，不保留其他历史命令名称。产品不包含 MCP stdio、MCP HTTP 或常驻服务。

## 安装与发布

- npm 包名为 `@follenfang/wowdoc`，全局安装后只暴露 `wowdoc`。
- npm 安装只安装当前平台 CLI 和 `~/.agents/skills/wowdoc`，不得自动下载或解析源码。
- Release Tag 使用 `vMAJOR.MINOR.PATCH`；GitHub Actions 为 Windows amd64、Linux amd64/arm64、macOS amd64/arm64 各构建一个 `wowdoc` 产物，并发布校验和。
- npm 版本、Git Tag 和 CLI 发布版本必须一致。

## 命令结构

- 根命令为 `wowdoc`。
- 生命周期命令为 `wowdoc init`、`wowdoc update`、`wowdoc clean`、`wowdoc uninstall`。
- 查询与维护命令继续通过 `wowdoc query|explore|inspect|diff|validate|doctor`、`wowdoc source ...` 和 `wowdoc index ...` 提供。
- 所有帮助、错误消息和 nextSteps 只能引用真实存在的 `wowdoc` 命令。

## 初始化

- `wowdoc init` 创建并验证 `~/.wowdoc` 目录，读取随 Skill 发布的 source catalog，获取官方及第三方产品分支源码，并为每个产品分支当前 head 与最多 10 个匹配 Tag 构建 AST 中间体和 SQLite 索引。
- 初始化使用完整 bare Git mirror。一次 repository 同步后固定本轮 Commit，再以有界并行的 detached worktree 解析各产品分支和 Tag。
- 相同原始内容按 SHA-256 复用 Pack 对象；相同解析单元按 content hash、language 与 parser/index schema 在 source 级共享事实库保存一次。snapshot 只在分支库保存路径、角色和共享内容身份映射。
- 每个构建任务把新源码、AST 和素材顺序写入独立 staging Pack，校验后原子发布不可变分段。未发布 staging 数据不属于 ready snapshot；显式 clean 可以清理过期 staging 和零引用分段。
- 每个产品分支保留独立 WAL SQLite 和本地 contentless FTS。FTS 文档集合、tokenizer、BM25 语料与确定性 tie-break 保持现有分支语义，不提升为改变排序范围的全局 FTS。
- 初始化全部默认基线完成后才原子写入 ready 状态；失败项可续跑，不重复解析 hash 与 schema 均未变化的文件。
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

- `wowdoc update` 通过 npm 更新 `@follenfang/wowdoc` 及随包 Skill，不下载源码、不刷新索引。
- 源码和索引更新分别由 `wowdoc source sync` 与 `wowdoc index refresh` 显式执行。
- `wowdoc clean` 默认只读预览；只有显式确认后才删除临时文件、未发布的过期 staging Pack 和零引用缓存/分段，活动 snapshot 及其 Pack 记录始终受保护。
- `wowdoc uninstall` 先列出 npm 包、Skill 和 `~/.wowdoc` 数据；显式确认后彻底删除，允许通过已有参数保留 npm 包。

## 兼容性与质量门槛

- 未完成首次初始化时，依赖源码或索引的命令快速返回非零状态、稳定错误和 `wowdoc init` 下一步，不自动联网。
- `--help`、`--version`、`wowdoc doctor`、`wowdoc init`、`wowdoc clean` 和 `wowdoc uninstall` 在未初始化状态可运行；doctor 保持只读。
- 现有 CLI 命令、JSON 输出、错误码、版本解析、默认 10 Tag 热范围、并发预算和代码证据格式保持兼容。
- 存储优化不得通过减少搜索覆盖、素材、AST、Tag、Git 历史或证据字段换取性能。50 条真实问题必须继续满足正确性、相关性、上下文完整性及版本/Commit/path/line/SHA-256 可追溯性。
- 内部 SQLite row ID、Pack offset、压缩字节、分段边界、数据库路径 generation 和性能统计不是业务身份，不要求逐字节相同。
