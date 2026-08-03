# Outcome

让初始化的磁盘占用和解析时间主要随“唯一文件内容”增长，而不是随 snapshot 数量线性复制；默认每个产品分支初始化当前 head 与最多 10 个匹配 Tag，并在有界资源预算内并行处理多个分支。

# Scope

- 将分支 SQLite 的文件级解析事实、关系和 FTS 文档按 `parserSchema + indexSchema + language + contentHash` 保存一次；snapshot 只保存 `snapshotId + path + contentHash + role` 映射及确实依赖整仓库的关系。
- 查询先固定 snapshot，再通过路径映射关联内容事实；结果仍返回该 snapshot 的准确 source、Commit、Tag、path、line、excerpt 和内容 hash。
- 使用 repository 内 Git blob OID 到 SHA-256 的可验证别名缓存和内容索引，跳过未变化文件的读取、AST 解析、事实提取与 FTS 写入。
- source sync 使用包含所选 refs 全部 blob 的完整 bare mirror；构建 worktree 和冷 snapshot 时不依赖 GitHub 临时补取 blob，避免按文件触发 TLS、认证或限流故障。
- `wowdata init` 默认把每个产品分支的热 Tag 上限从 20 改为 10；显式 `--hot-tags` 继续允许用户覆盖。
- 初始化在 repository fetch 完成并固定所有 resolved Commit 后并行构建不同产品分支。每个分支按 head、由新到旧 Tag 串行复用内容；全局解析 worker 总预算保持 4–8，不为每个 snapshot 重复创建一组 worker。
- 每个解析任务继续使用独立 detached worktree；同分支 SQLite 单写，同一内容构建锁去重，不同分支 DB 可以并行发布。
- 旧 schema 按既有 generation 迁移契约逐分支构建新库、验证、原子切换，并在读者释放后把旧 generation 交给清理流程。

# Non-goals

- 不增加 MCP、HTTP 服务、向量检索或远程数据库。
- 不改变 Tag/branch/build 到 immutable Commit 的解析规则、查询排序语义或返回证据格式。
- 不在 npm 安装阶段下载或初始化源码。
- 不保证不同分支 DB 之间只保存一份 SQLite 查询事实；跨分支仍以独立 WAL DB 保持写入和故障隔离，全局 objects/AST 继续跨分支复用。
- 不承诺改造前后内部 row ID、DB 路径、构建耗时、复用计数或 FTS BM25 原始数值逐字节相同；这些不是代码参考的业务身份。

# Acceptance examples

- 同一分支两个 snapshot 的 `A.lua` 内容相同：数据库只有一份该内容的 AST 引用、symbols、edges、search docs 和 FTS 行，但有两条 snapshot/path 映射；查询任一 snapshot 都返回各自固定 Commit 下的 `A.lua` 路径和正确行号。
- 新 Tag 有 1000 个文件且只有 10 个新内容：构建枚举并发布全部路径映射，只读取并解析 10 个未命中的内容；复用计数报告其余内容，不重复写 FTS。
- 同一内容出现在两个路径：内容事实只保存一次，snapshot 映射保存两个路径；查询结果分别补回路径和路径角色，不把路径固化进内容 AST 或 FTS 事实。
- parser 或 index schema 改变：旧内容结果不能误复用，新 generation 完整构建并验证后才切换。
- 默认运行 `wowdata init` 时每个产品分支最多选择 10 个匹配 Tag 加 head；`--hot-tags 3` 时最多选择 3 个，匹配不足时只构建实际项。
- 两个不同分支初始化时允许同时存在独立 worktree 并并行解析；同一分支 Tag 不并发写同一 SQLite，所有任务合计不超过全局 worker 预算。
- 任一并行任务失败时，其他已完成分支保持可续跑状态；全局 ready 仍遵守全部默认基线完成后的既有门槛，失败结果列出完成项、失败项和重试建议。
- 对同一已预热 snapshot 执行相同 query/explore/inspect/diff：source/product/ref/Commit 解析、错误码、命中代码事实、path、line、excerpt、content hash、关系可信度和确定性排序语义保持等价；并发任务完成顺序不能改变所选 Tag 或查询结果。
- 请求原先第 11–20 个热 Tag：新默认初始化不再提前构建，首次请求按既有冷 snapshot 流程准备后得到相同固定 Commit 的查询能力；这是 10 Tag 决策带来的唯一预期业务范围变化。

# Constraints and invariants

- 内容事实必须与 path、branch、Tag、Commit 和 snapshot 无关；路径、角色与版本归属只来自 snapshot 映射。
- 内容身份使用 SHA-256；Git OID 只作为同一 repository 内避免重复读取的别名，首次遇到的 blob 仍须计算并记录 SHA-256。
- 解析单元必须包含语言身份；相同字节分别作为 Lua、XML、TOC 或其他类型出现时共享原始内容对象，但不能复用错误语言的 AST、事实或 FTS。
- FTS 候选必须与目标 `snapshot_files` 做成员关联，不能返回只存在于其他 snapshot 的内容。
- 查询只读已发布 SQLite、manifest 与 objects，不依赖 worktree 或可变 Git ref。
- repository 同步完成后，本轮所有解析任务只访问本地 mirror；构建阶段不得因缺失 promisor blob 隐式联网。
- 分支库保持 WAL、单写者、短发布事务和 generation 原子切换；并发构建不能暴露半成品。
- 并发度是全局资源预算，必须避免“任务数乘以每任务 worker 数”的超量并发。
- 存储重构不能降低 50 条真实问题验收中的正确性、相关性、上下文完整性或版本/Commit/path/line 可追溯性。BM25 语料去重允许校准内部数值，但不得降低确定性排序与首条代码参考质量。

# Decisions

- 用户确认默认热数据为每产品分支最多 10 个 Tag 加当前 head。
- 用户确认关键优化是解析事实和 FTS 按内容 hash 复用，snapshot 只保存路径到内容的映射。
- 用户要求利用独立 worktree 并行初始化不同分支和 Tag；采用跨分支并行、分支内按新到旧串行的调度，以提高复用并保留 SQLite 单写者模型。
- 沿用已确认的 schema generation 迁移、整体 ready、失败可续跑和显式 clean 契约。
- 用户确认除默认热范围从 20 缩到 10 及非业务性能/统计字段外，存储重构与并发调度保持代码参考业务等价。
- 用户确认当前修订版完整契约，包括完整 bare mirror、10 Tag 默认热范围、跨分支并行与分支内串行调度，以及上述业务等价边界。

# Open questions

- 无。

# Verification expectations

- 运行全部 Go 测试、竞态检查可运行范围、CLI help/default 回归和 `git diff --check`。
- 新增数据库结构测试，直接断言相同内容跨 snapshot 只有一份解析事实与 FTS 文档，并验证不同路径、不同 schema 和删除/重建边界。
- 新增构建计数测试，证明第二个 snapshot 只解析新增内容并正确报告复用数量。
- 新增并发初始化 fixture，证明不同分支任务实际重叠、同分支发布串行、全局 worker 上限不被突破、失败后可续跑。
- 新旧 schema 使用同一 fixture 做业务等价对照，逐项核对版本解析、错误、结果顺序、代码事实、路径、行号、excerpt、content hash、关系与 diff；明确列出允许变化的内部统计字段。
- 用五仓库代表性样本记录改造前后 SQLite/objects/AST/manifest 字节数、构建时间、唯一内容数和 snapshot 映射数；质量验收继续核对代码参考的正确性、相关性、上下文完整性与 Commit/path/line 可追溯性。
