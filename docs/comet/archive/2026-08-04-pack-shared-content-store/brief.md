# Outcome

把源码、AST 和素材从“每个内容一个小文件”改为可校验的不可变 Pack 分段，并把同一 source 各产品分支重复保存的解析事实提升为 source 级共享内容库。查询、跨文件搜索、版本固定、分支内 FTS 排序和代码证据保持业务等价，同时显著减少小文件写入、NTFS 实际占用和跨分支 SQLite 重复数据。

# Scope

- 新增版本化 Pack 格式，按内容 SHA-256 保存压缩后的源码、AST 和素材；索引记录 pack、offset、压缩长度、原始长度、codec 和对象类型。
- 每个构建任务只顺序写自己的 staging Pack；内容与索引校验完成后原子发布不可变分段。异常退出留下的未发布 staging 数据可安全清理，已发布但尚未引用的分段由显式 clean/GC 处理。
- 为每个 source 与 parser/index schema generation 建立共享事实 SQLite，按 `contentHash + language + schema` 唯一保存 contents、strings、symbols、edges、XML、TOC、asset facts、asset refs 和共享 search document 元数据。
- 每个 product 分支继续保留独立 WAL SQLite，保存 snapshot、path/role 到共享 content 的映射，以及分支自己的 contentless FTS 数据和 BM25 语料，避免全局 FTS 改变排序。
- 查询先固定 immutable Commit 和 snapshot，在分支库中完成成员过滤，再关联共享事实库；只为最终结果从 Pack 随机读取少量源码并生成准确 excerpt。
- 兼容读取现有 raw/gzip 单对象；新构建写 Pack。schema generation 完整构建、校验并发布后才切换，不暴露半成品。
- 增加 Pack 校验、并发发布、崩溃恢复、共享事实去重、分支 FTS 等价、旧对象兼容和跨文件查询回归测试。
- 使用现有真实质量问题和可复现基准比较优化前后构建时间、逻辑字节、NTFS 4 KiB 分配字节、文件数量及代码参考质量。

# Non-goals

- 不减少普通 Lua/XML 全文搜索覆盖，不截断文件，不改为向量检索或按需解析 AST。
- 不恢复 partial clone，不减少完整 bare Git mirror、默认 10 Tag 热数据或已支持的 source/product/ref。
- 不增加 MCP、HTTP 服务、常驻进程或第二个命令入口。
- 不要求数据库 row ID、Pack 分段边界、内部 offset、压缩字节或性能统计逐字节相同。
- 本 change 不以删除历史 generation 或自动后台 GC 冒充空间收益；清理继续由显式生命周期动作控制。

# Acceptance examples

- 同一 source 的两个产品分支包含字节相同的 `A.lua`：Pack 中只保存一份源码和对应 AST，共享事实库只保存一份解析事实；两个分支分别保存 snapshot/path 映射和本地 FTS 成员，查询返回各自 Commit 下的真实路径。
- 同一 snapshot 中搜索普通字符串、局部变量、注释、函数、XML 定义和调用关系：优化前后命中覆盖、确定性排序、首条代码参考、path、line、excerpt 与 SHA-256 保持等价。
- 构建包含数千个新内容的冷 snapshot：对象写入表现为少量顺序 staging/Pack 文件，而不是每个内容分别创建源码和 AST 文件；发布后 Pack 中每条记录均可按 hash 随机读取并校验原始字节。
- 构建在 staging 写入、Pack 发布或数据库事务任一阶段中断：旧 snapshot 仍可查询；下次运行忽略或清理未完成数据并续建，不把未引用或校验失败的记录当作 ready。
- 旧安装中只有 raw 或 gzip 单对象：查询仍可透明读取；该内容在后续构建中进入 Pack 后返回相同字节、excerpt 和 hash，不要求用户手动迁移。
- 两个分支并行构建同一新内容：最终共享库与 Pack 只保留一个可寻址内容身份，两个分支均完整发布，写锁和临时分段不互相破坏。
- 50 条真实用户问题继续达到 50/50；正确性、相关性、上下文完整性、版本与 Commit/path/line/SHA-256 可追溯性均不下降。

# Constraints and invariants

- SHA-256 基于解压后的原始字节；Pack、压缩算法、路径或分支不参与内容身份。
- Pack 分段发布后不可原地修改。记录必须有格式版本、边界检查和内容 hash 校验；读取损坏数据返回稳定错误，不返回未经校验的证据。
- 分支 FTS 的文档集合、tokenizer、排序语料和 tie-break 语义保持现有行为；snapshot 成员过滤必须发生在结果截断之前。
- 路径、角色、Tag、Commit 和 snapshot 归属只存在于分支映射，不固化进共享内容事实。
- 相同字节以不同语言解析时可以共享原始对象，但 AST、解析事实和搜索文档必须包含语言与 schema 身份。
- 查询只读已发布的 branch DB、shared content DB、manifest 与 Pack，不依赖 worktree、可变 Git ref 或网络。
- 构建继续遵守 repository fetch 锁、source+commit+schema 构建锁、全局 4-8 解析 worker 预算和分支 SQLite 单写者约束。
- 不以减少搜索能力、Tag、素材、AST 或 Git 历史换取性能和空间。

# Decisions

- 用户确认 Pack 仅替换内部对象存储，Agent 仍通过现有 CLI 和 SQLite 做跨文件搜索，不扫描 Pack。
- 用户确认业务等价是硬门槛；采用每分支本地 FTS，不采用改变 BM25 语料的全局共享 FTS。
- 用户确认共享内容库只提升与版本无关的不可变事实，分支库继续拥有 snapshot/path/role 与本地排序数据。
- 用户明确要求创建 change 后直接实施，无需再次等待确认；该要求是在上述架构与业务等价边界已经共同确认后给出。

# Open questions

- 无。

# Verification expectations

- 运行 `go test ./...`、`go vet ./...`、相关包 race 测试、Linux amd64 与 macOS arm64 的 `CGO_ENABLED=0` 构建、npm 包检查和 `git diff --check`。
- 以固定 fixture 对比现有 generation 与新 generation 的普通全文、精确符号、XML、关系、inspect、diff、路径、行号、excerpt、hash 和排序。
- 注入 staging 写入中断、Pack 截断、hash 不匹配、发布后 DB 事务失败、重复并发发布和旧对象读取场景。
- 运行 50 条真实问题质量回归，必须保持 50/50，并保存逐条质量证据。
- 在隔离 `WOWDOC_HOME` 记录冷 snapshot 与多 Tag 的下载后解析时间、SQLite/Pack/manifest/repository 逻辑字节、4 KiB 分配估算和文件数；估算不得写成实测。
