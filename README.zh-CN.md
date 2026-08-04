<div align="center">

# wowdoc

**可定位、可验证、带版本的魔兽世界源码参考。**

把游戏 Build、插件版本、Tag 或 Commit，变成 Agent 和插件作者可以直接引用的代码证据。

[English](README.md) | [简体中文](README.zh-CN.md)

[![CI](https://img.shields.io/github/actions/workflow/status/Follen/wowdoc/ci.yml?branch=main&style=flat-square&label=CI)](https://github.com/Follen/wowdoc/actions/workflows/ci.yml)
[![Release](https://img.shields.io/github/actions/workflow/status/Follen/wowdoc/release.yml?style=flat-square&label=release)](https://github.com/Follen/wowdoc/actions/workflows/release.yml)
[![npm](https://img.shields.io/npm/v/@follenfang/wowdoc?style=flat-square&logo=npm)](https://www.npmjs.com/package/@follenfang/wowdoc)
[![downloads](https://img.shields.io/npm/dm/@follenfang/wowdoc?style=flat-square&label=downloads)](https://www.npmjs.com/package/@follenfang/wowdoc)
[![GitHub release](https://img.shields.io/github/v/release/Follen/wowdoc?style=flat-square&sort=semver)](https://github.com/Follen/wowdoc/releases/latest)
[![license](https://img.shields.io/github/license/Follen/wowdoc?style=flat-square)](LICENSE)

</div>

---

`wowdoc` 是一个纯本地、纯 CLI 的源码检索工具。它把 `latest`、`retail`、插件版本这类会变化的名字固定到不可变 Git Commit，然后给出每条结果对应的仓库路径、行号、代码片段和 SHA-256。

随 npm 安装的 Agent Skill 负责判断查哪个源码、产品、版本和命令；CLI 负责 Git、解析、索引和证据。初始化完成后，查询过程全部在本地执行。

## 安装

```powershell
npm install -g @follenfang/wowdoc
wowdoc --version
wowdoc init
```

npm 包会安装：

- 一个可执行命令：`wowdoc`
- 一个用户级 Skill：`~/.agents/skills/wowdoc`

`wowdoc init` 会创建 `~/.wowdoc`，下载配置好的 bare Git mirror，并构建可查询的源码快照。初始化支持断点续跑；网络或进程中断后，重新执行同一个命令即可继续。

> 本机没有 Git 时，`wowdoc init` 会识别系统包管理器，显示即将执行的命令，安装 Git，刷新 `PATH`，最后执行 `git --version` 验证。`wowdoc doctor` 只检查状态，不修改系统。

## 它会产出什么

```text
分支 / Tag / 版本 / Commit
              │
              ▼
         不可变 Commit
              │
              ▼
 Lua · XML · TOC · 素材 · 符号 · 调用关系
              │
              ▼
 路径 · 行号 · 代码片段 · SHA-256 · 实际 Commit
```

- **版本准确**：Tag 和分支只解析一次，随后以不可变快照保存。
- **可以复查**：每条代码参考都能回到对应 Git blob 验证。
- **支持并发**：每个解析任务使用独立 detached worktree，查询不依赖 checkout 状态。
- **初始化后纯本地**：查询只读 SQLite 和不可变 Pack，不切分支，也不临时联网。
- **适合 Agent**：命令稳定、职责单一，提供 JSON 输出和明确诊断。

## 支持的源码

| 源码 | 产品 / 通道 |
| --- | --- |
| 暴雪 UI 源码 | Retail、PTR、PTR2、Beta、Classic、Classic PTR/Beta、Classic Era/PTR、Anniversary、Titan |
| [ElvUI](https://github.com/tukui-org/ElvUI) | main、PTR |
| [WeakAuras](https://github.com/WeakAuras/WeakAuras2) | main |
| [NDui](https://github.com/siweia/NDui) | main、Classic、Era、Anniversary、Titan |
| [EllesmereUI](https://github.com/EllesmereGaming/EllesmereUI) | main |

第三方插件的版本依据是 `Tag -> Commit -> snapshot`。如果用户提供的版本没有匹配 Tag，Agent Skill 可以退回该产品的最新快照，但会明确标记这是 latest fallback，并保留实际 Commit。

## 常用查询

### 查一个 API 定义

```powershell
wowdoc query `
  --source wow-ui-source `
  --product retail `
  --ref latest `
  --topic api `
  --text C_AuctionHouse.GetItemSearchResultInfo
```

### 查看指定 ElvUI 版本里的符号

```powershell
wowdoc inspect `
  --source elvui `
  --product main `
  --ref v15.18 `
  --symbol 'lib:RegisterPlugin'
```

### 比较两个 WeakAuras 版本

```powershell
wowdoc diff `
  --source weakauras `
  --product main `
  --from 5.20.7 `
  --to 5.21.9
```

### 按目标版本检查插件

```powershell
wowdoc validate `
  --path D:\AddOns\MyAddon `
  --source wow-ui-source `
  --product retail `
  --ref latest
```

查询成功后会返回 source、product、用户请求的 ref、匹配到的 Tag、实际 Commit、仓库路径、行号、代码片段和内容哈希。

## 命令一览

```text
检索       wowdoc query | explore | inspect | diff | validate
源码       wowdoc source list | check | sync
索引       wowdoc index build | refresh | status
检查       wowdoc doctor
生命周期   wowdoc init | update | clean | uninstall
```

正常更新流程是显式的：

```powershell
wowdoc source check --source elvui --product main
wowdoc source sync --source elvui --product main
wowdoc index refresh --source elvui --product main --ref latest
```

`wowdoc update` 只更新 npm 包和 Skill，不拉取源码，也不重建索引。`wowdoc clean` 默认只预览，加入 `--yes` 才执行清理。

## 本地存储

默认情况下，所有数据都放在 `~/.wowdoc`：

```text
config/         源码目录和本地配置
repositories/   完整 bare Git mirror
objects/packs/  不可变、按内容寻址的 Pack 分段
indexes/        共享内容库和分支独立的 WAL/FTS 数据库
manifests/      不可变快照清单
state/          初始化和任务状态
tmp/worktrees/  解析期间使用的临时 detached worktree
locks/          仓库和发布锁
logs/           本地诊断日志
```

通过 `WOWDOC_HOME` 可以修改数据目录：

```powershell
$env:WOWDOC_HOME = 'D:\WOWData\wowdoc'
wowdoc doctor
wowdoc init
```

内容相同的源码、AST 和素材只在不可变 Pack 中保存一份，在不同 Tag 和分支之间复用。source 级 SQLite 保存共享事实；每个分支保留自己的快照成员关系和 FTS 统计，因此版本过滤和 BM25 排序不会被其他分支影响。

## 性能

以下数据来自 Windows 11、完整本地 mirror、8 个解析线程，以及同一个包含 3,685 个文件的 Retail Commit：

| 方案 | 冷构建 | SQLite | 完整数据目录 |
| --- | ---: | ---: | ---: |
| 基线 `b201d38` | 41.1 秒 | 158.6 MB | 352.2 MB |
| 紧凑内容存储 | 11.4 秒 | 67.7 MB | 170.0 MB |

这次测试中，解析和索引时间减少 72%，SQLite 减少 57%，完整本地数据减少 52%；Lua/XML 搜索覆盖和返回的源码证据保持一致。详细场景、全量 catalog 数据和取舍见[性能记录](docs/performance.md)。

## Agent 怎么使用

安装的 Skill 只保存命令选择规则和 source/product 别名，不复制源码知识。Agent 会从问题中选择最窄、最稳定的标识符，调用 `wowdoc`，然后引用 CLI 返回的证据。

质量回归包含 50 个真实插件开发问题，覆盖不同产品分支和历史 Tag。首条参考必须同时满足正确、相关、上下文完整、版本正确，并且能逐字节回到实际 Git blob，才算通过。

## 开发

需要 Go 1.23+、Node.js 20+ 和 Git。

```powershell
go test ./...
go vet ./...
npm pack --dry-run
go run ./cmd/wowdoc --help
```

发布 Tag 使用 `vMAJOR.MINOR.PATCH`。GitHub Actions 会测试 Windows、Linux 和 macOS，构建五个平台二进制，生成校验和与 GitHub Release，再通过 Trusted Publisher OIDC 发布带 provenance 的 npm 包。

## License

[MIT](LICENSE)
