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
- 相同内容按 content hash 复用解析结果和 FTS，snapshot 保存路径到 content hash 的映射。
- 初始化全部默认基线完成后才原子写入 ready 状态；失败项可续跑，不重复解析 hash 与 schema 均未变化的文件。
- 如果 Git 缺失，`wowdoc init` 使用现有平台包管理器安装帮助流程；安装后刷新 PATH 并用 `git --version` 验证。失败时返回稳定错误码和精确 nextSteps。

## 更新、清理与卸载

- `wowdoc update` 通过 npm 更新 `@follenfang/wowdoc` 及随包 Skill，不下载源码、不刷新索引。
- 源码和索引更新分别由 `wowdoc source sync` 与 `wowdoc index refresh` 显式执行。
- `wowdoc clean` 默认只读预览；只有显式确认后才删除临时文件和零引用缓存，活动 snapshot 始终受保护。
- `wowdoc uninstall` 先列出 npm 包、Skill 和 `~/.wowdoc` 数据；显式确认后彻底删除，允许通过已有参数保留 npm 包。

## 查询与版本

- 未完成首次初始化时，依赖源码或索引的命令快速返回非零状态、稳定错误和 `wowdoc init` 下一步，不自动联网。
- `--help`、`--version`、`wowdoc doctor`、`wowdoc init`、`wowdoc clean` 和 `wowdoc uninstall` 在未初始化状态可运行；doctor 保持只读。
- 查询把 branch、Tag 或版本解析为 immutable Commit，只读 SQLite 与 objects，不依赖 worktree 或 Git checkout 状态。
- Tag 是插件版本匹配的主要依据；没有匹配 Tag 时，Skill 引导 Agent 使用最新代码并明确 Commit。
- 结果保留 source、product、requested ref、resolved Commit、路径、行号、excerpt 和内容哈希，确保可追溯。

## 兼容性与非目标

- 现有 JSON 输出、错误码、目录布局、并发预算和索引语义保持兼容。
- 当前默认热数据上限为每个产品分支 10 个匹配 Tag。
- 整个仓库只使用 `wowdoc` 品牌与命令名，不保留错误旧命令的代码、文案、文件或目录。
