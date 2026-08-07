# Outcome

`npm install -g @follenfang/wowdoc` 和后续 `wowdoc init` / `wowdoc source sync` 不再出现无期限、无阶段信息的下载等待。用户能够看到正在处理的对象、已传输数据或 Git 原生进度、耗时、重试次数和下一次重试原因；瞬时网络故障能够自动恢复，永久故障在有界时间内以稳定诊断退出，且不会留下可执行的截断二进制、损坏 mirror 或被误判为 ready 的数据。

# Scope

- npm 安装阶段的当前平台原生二进制交付：发布产物组织、下载/缓存、进度、超时、重试、完整性校验、原子安装、并发安装互斥与陈旧临时文件清理。
- `wowdoc init`、`wowdoc source sync` 以及已有 partial mirror 升级所触发的 Git clone/fetch：分 source 进度、最多三个 source 的全局并发上限、每仓库互斥、瞬时错误重试、取消与超时、已完成工作复用、mirror 校验和原子发布。
- HTTP 与 Git 下载错误分类：DNS、TLS、代理、连接/无进度/总时限、HTTP 401/403/404/408/429/5xx、重定向、长度不符、校验和不符、磁盘写入失败、用户取消和并发占用。
- Windows、Linux、macOS 的安装与初始化回归测试、故障注入测试、发布门禁和中英文文档。

# Non-goals

- npm 安装仍不触发 `wowdoc init`，也不下载任何源码或构建索引。
- 不降低完整 bare Git mirror、默认 10 个热 Tag、源码/AST/素材覆盖或查询证据要求。
- 不把永久性认证失败、仓库不存在、资源不存在或持续完整性失败变成无限重试。
- 不引入常驻下载服务，不要求用户手工管理下载临时文件。

# Acceptance examples

- 冷安装时，标准安装命令能够显示当前阶段；网络正常时安装当前平台二进制并校验发布身份，最终 `wowdoc --version` 与 npm 包版本一致。
- 下载中断、连接重置、408、429 或 5xx 时，客户端保留可复用的已验证内容，按有上限的退避策略重试，并显示 `attempt/limit`、原因和等待时间；达到上限后非零退出且给出可执行的下一步。
- 服务端忽略 Range、返回错误 Content-Range、Content-Length 不符、内容截断或 SHA-256 不符时，不发布目标文件；客户端安全重启或清理该次下载，校验和持续不符时停止。
- 两个安装进程同时处理同一版本和平台时，不会互相覆盖缓存或安装截断文件；等待方显示占用状态，并在锁释放后复用已校验产物。
- `wowdoc init` 同步多个 source 时最多三个 Git 网络任务并发，每条进度都带 source ID；一个 source 瞬时失败会在自己的重试预算内恢复，不取消其他可独立完成的 source。
- Git 进程长时间无网络进展、超过单次尝试时限、收到取消信号或磁盘写入失败时会退出并保留最后一个可验证恢复点；再次执行同一命令复用已完成 repository、ref、snapshot 和索引工作。
- 交互终端提供节流刷新的单行进度，非交互日志提供低频、可逐行保存的阶段事件；机器可读 JSON stdout 不混入进度文本，凭据和带认证信息的 URL 不出现在日志或错误中。

# Constraints and invariants

- 默认行为必须有界：连接、无进度、单次尝试和总重试预算均有明确上限，用户取消立即优先于重试。
- 仅原子发布通过长度与 SHA-256 校验的二进制；仅把通过 Git 完整性检查的 staging mirror 切换为活动 mirror。
- 重试只覆盖可判定为瞬时的错误，并采用指数退避、抖动以及服务端 `Retry-After`（存在时）；认证、权限、404、配置和持续完整性错误快速失败。
- 下载缓存和 staging 路径按版本、平台、架构和资源身份隔离，使用跨进程锁，所有路径保持在受管目录内。
- 现有 `WOWDOC_BINARY_DIR` 离线/CI 注入行为继续可用；注入产物不触发网络。
- 保持现有命令名称、JSON envelope、错误退出语义、目录布局和全局 4-8 worker 预算兼容。

# Decisions

- npm 二进制交付优先采用平台专属 npm optional package，使 npm 自身承担 registry 下载、代理、缓存、重试与 SRI 完整性；`postinstall` 保留 Skill 安装和受控回退职责，不再依赖一个无输出、无超时的裸 `fetch`。
- HTTP 回退下载仍实现显式进度、有界超时、校验和、`.part` 缓存、Range 恢复、重试与原子替换，以覆盖 optional package 缺失但 GitHub Release 可达的受支持场景。
- Git 网络进度写入 stderr，并以 source ID 前缀聚合；最终 JSON 只写 stdout。
- 重试预算是实现配置而非无限循环；永久错误不消耗全部退避时间。
- Git 续跑采用对象/检查点级保证：复用已完整接收的 Git 对象、已完成 ref 批次、repository、snapshot 和索引；单个未完成 pack 的传输可从该次尝试重新开始，不引入自定义字节级 Git 分发协议。

# Open questions

已确认：本 change 将增强 npm 安装和 `wowdoc init`/`source sync` 的下载可靠性：安装使用平台专属 npm 包优先、GitHub 回退具备进度/超时/重试/校验/Range/原子发布；Git 使用最多三个 source 并发、source 级进度、瞬时错误重试，并按已完整 Git 对象、ref 批次、repository、snapshot 和索引检查点续跑，不实现单个未完成 pack 的字节级续传；不改变显式 `wowdoc init` 边界、完整 mirror 和查询语义。

# Verification expectations

- Node 测试使用本地 HTTP fixture 覆盖正常下载、Range 续传、Range 被忽略、重定向、慢流/停流、截断、错误长度、408/429/5xx、`Retry-After`、校验失败、缓存命中、并发锁、取消和原子发布。
- Go 测试使用本地仓库与可控 Git shim 覆盖 clone/fetch 进度解析、错误分类、退避、超时、取消、并发上限、独立 source 失败隔离、已有对象/检查点复用、staging 校验和原子切换。
- 三平台 CI 从打包产物执行真实安装，验证平台包选择、离线注入、Skill 安装、`wowdoc --version` 和无源码自动初始化。
- 运行 `go test ./...`、`go vet ./...`、Node 下载测试、`npm pack --dry-run`，并记录至少一次中断后续跑的端到端验证。
