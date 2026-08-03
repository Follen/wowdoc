# Outcome

修复 0.0.4 错误引入的第二个独立命令，使本仓库、npm 包和发布产物重新只有一个可执行命令 `wowdoc`。数据初始化、更新、清理和卸载作为 `wowdoc` 的一级子命令提供，并发布补丁版本 0.0.5。

# Scope

- 将 `init`、`update`、`clean`、`uninstall` 挂载到 `wowdoc` 根命令。
- 删除 Go、npm 和发布产物中的错误独立入口。
- 将运行时错误、nextSteps、Git 安装提示统一为 `wowdoc` 命令。
- 更新 README、Skill、当前 canonical spec、测试和 GitHub Actions。
- npm 只导出 `wowdoc`，Release 每个平台只构建一个 `wowdoc` 二进制。
- 将 package.json 与 package-lock.json 版本同步到 0.0.5，提交、打 `v0.0.5` Tag、推送并核验发布。

# Non-goals

- 不改变索引、快照、Git mirror、查询和质量回归的业务逻辑。
- 不引入 MCP、HTTP 服务或第二个 CLI 产品。
- 不处理与本修复无关的独立项目。

# Acceptance examples

- `wowdoc --help` 显示 `init`、`update`、`clean`、`uninstall` 以及已有查询命令。
- `wowdoc init` 执行现有初始化流程；未初始化查询的 nextStep 为 `wowdoc init`。
- `wowdoc update --dry-run` 输出更新 `@follenfang/wowdoc@latest` 的 npm 命令。
- `wowdoc clean` 默认只预览，`wowdoc uninstall` 未确认时提示 `wowdoc uninstall --yes`。
- npm tarball 的 `bin` 只有 `wowdoc`，不包含第二个入口脚本。
- Release workflow 只生成五个平台的 `wowdoc` 产物，数量门禁为 5。
- 整个仓库中不存在错误旧名称的文案、文件、目录或可执行入口。
- 版本 0.0.5 通过 Go 测试、race、vet、CLI help、npm pack 和 diff 检查后发布。

# Constraints and invariants

- `wowdoc` 是本仓库唯一用户可见命令和 npm bin。
- 现有命令参数、JSON 结果结构、错误码、存储布局和 10 个热 Tag 默认值保持不变。
- npm 安装仍只安装 CLI 与 Skill，不自动初始化或下载源码。
- `doctor` 保持只读；Git 缺失时仅 `wowdoc init` 触发现有安装帮助。
- Git 仓库继续使用完整 bare mirror；查询继续只读 SQLite/objects。

# Decisions

- 用户明确：错误旧名称对应完全无关的独立项目，本仓库命令从来只叫 `wowdoc`。
- 用户确认：仓库内不要出现任何错误旧名称相关内容，所有命令和文案统一为 `wowdoc`。
- 生命周期命令采用 `wowdoc init|update|clean|uninstall`。
- 错误发布通过 0.0.5 补丁版本修复。

# Open questions

无。

# Verification expectations

- 运行 `go test ./... -count=1`、`go test -race ./... -count=1`、`go vet ./...`。
- 运行 `go run ./cmd/wowdoc --help` 并检查四个生命周期子命令。
- 运行 `npm pack --dry-run`、`node scripts/check-package.mjs` 和 `git diff --check`。
- 搜索整个仓库中的错误旧命令引用并确认结果为空。
- 推送 Tag 后核验 GitHub Actions、GitHub Release 与 npm 0.0.5。
