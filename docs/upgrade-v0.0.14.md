# wowdoc v0.0.14 升级记录

本记录描述 v0.0.14 的实现与发布前验证。

## 实现梳理与改进

原有链路为：CLI 解析目标版本 → catalog 固定 Git commit → indexer 解析 Lua/XML/TOC → 内容寻址 Pack 保存原文与 AST → 共享 SQLite 保存事实、分支数据库保存快照成员与 FTS → query 返回版本化证据。TOC validator 另行解析 AddOn 加载闭包，再使用同一快照事实验证静态引用。

保留这套存储和快照隔离设计。本次把搜索策略集中到 `internal/query/search.go`，修复查询层与解析层的已证实问题，没有引入新的存储格式或第三方依赖。

| 问题 | 处理 |
| --- | --- |
| `--topic` 被接收但未使用 | 在 SQL 候选限额之前按 API 类型或文件类型过滤；资源从 snapshot_assets 查询 |
| 精确命中后仍补入弱结果 | query/符号 inspect 按 exact → prefix → full text 逐级回退，explore 显式保留宽泛发现 |
| 多词 FTS 使用 OR，任一词就命中 | query 要求所有词，explore 保留任意词匹配 |
| 下划线被 SQL LIKE 当作通配符 | 对 `_`、`%`、反斜线转义；资源路径统一规范化 |
| 角色降权只影响展示分数，不影响取数 | 先按最终分数排序，再限制条数；合并重复定义和原文命中，保留同一行的不同定义 |
| 固定附加最多 50 条子串关系 | query 使用完整名称，explore 可查子串；关系和结果分别受 limit 限制 |
| XML 证据查询按每个快照文件全扫 xml_nodes | 用连接顺序保证先遍历 XML，再走已有 content_snapshots 索引定位快照成员 |
| 每个调用位置重复查询同一事实 | 单次 LookupCompatibility 按规范化 kind/name 缓存，证据仍逐位置构建 |
| API 名称 OR 连接遍历大量符号 | 拆分成使用现有复合索引的两条精确查询，并以 UNION 去重 |
| 校验输出重复展开数万条 XML 出处 | 每事实保留 5 个代表性出处，附 matchCount、matchesTruncated；所有调用位置、覆盖统计和结论保留 |
| 事件文档名与运行时字符串混淆 | 同时索引 LiteralName 和原文档名别名，修复 PLAYER_LOGIN 等事件误报 |
| CI 声明 Go 1.23，go.mod 要求 1.24 | CI/release 构建版本从 go.mod 读取 |

## 复现与实测

环境：Windows，AMD Ryzen 7 9800X3D，本地已索引真实数据。所有时间都是本机观测，不是跨机器性能承诺。

源：`wow-ui-source / retail / 12.1.0`；commit：`4e3cbb8c5609e4bfc332c0aebbfa4d79731fab59`。

```powershell
wowdoc query --source wow-ui-source --product retail --ref 12.1.0 --topic api --text CreateFrame
wowdoc validate --path D:/Code/wow/addons/Lychee/addon/Lychee --toc Lychee_Mainline.toc --source wow-ui-source --product retail --ref 12.1.0
go test ./internal/query -run '^$' -bench BenchmarkCompatibilityFrameReferences -benchtime=3x
```

- v0.0.13 全量 TOC：10 秒、120 秒有界观察均未完成、无输出；此前约 8 分钟的历史扫描仅是报告证据，不是本轮重跑到完成的测量。
- 原始 CPU profile：约 94.7% 累计采样位于 SQLite 查询；104 文件的闭包构建与分析仅约 0.135 秒。
- 初步 SQL 修复后：完整 TOC 3.59 秒，但产生 79,689,199 字节 JSON，并暴露 27 个事件误报。
- 限制重复出处后：同一旧解析索引 2.25 秒、2,868,626 字节，覆盖与结论保持一致。
- 事件解析升级并刷新真实快照后：1.74 秒、2,858,443 字节，checkedLua=104，checkedXml=0，diagnostics=[]，valid=true；coverage checked=5529/resolved=385/unresolved=5179。这里包含大量 AddOn 自定义或动态调用，不能把 unresolved 当成全部运行行为已经验证。
- `CreateFrame --topic api`：旧版 10 条混合结果、50 条关系、17,130 字节；新策略 1 条 API 定义、10 条精确关系、2,776 字节。
- `C_AddOns.GetAddOnMetadata --topic api`：旧版 3 条（含原文命中），新策略 1 条 API 定义。
- `GetItem --topic api`：旧版前十含大量普通函数，新策略前十均为 API 定义。
- 独立 100 XML 文件/100 Frame 引用基准：v0.0.9 约 170.85 ms、8.68 MB 分配；新实现约 2.48 ms、0.43 MB 分配。旧版一次、新版三次，作为方向性对照，不将其称为严谨统计基准。

缺陷可追溯到 `274a548`（v0.0.9 引入 TOC 兼容性查询），并在该版复现。v0.0.12→v0.0.13 无生产代码变更，不支持“上一版本混入异常代码”的判断。

## 升级与验证

npm 主包、lockfile 根版本、五个平台包与依赖版本统一为 0.0.14。发布标签为 v0.0.14。

解析器身份由 v4 更新为 v5。旧快照不会被静默作为新解析结果读取。对用到的目标执行一次：

```powershell
wowdoc index refresh --source wow-ui-source --product retail --ref 12.1.0
```

本机该目标刷新约 16.44 秒，无需重新同步已有 Git 对象。其他版本和产品按需刷新。本地验证使用 `dist/wowdoc-v0.0.14.exe`；正式安装通过 npm 获取对应版本。

验证包含完整 Go 测试、go vet、9 项 Node 测试、npm pack --dry-run、git diff --check；新增覆盖主题拥挤、精确与探索差异、多词匹配、同一行多定义、资源路径、事件别名、快照证据位置和输出大小的回归，并保留原有跨快照隔离测试。仅更改 wowdoc 工作区及其索引，未修改 Lychee 或操作游戏。

## 仍需明确的工具边界

- 不带 --toc 的 validate 是递归 Lua 语法检查，不会调用兼容性证据查询；单文件通过不能替代完整 TOC 校验。
- 静态调用分析无法完整还原动态调用与 AddOn 自定义 API，结果保留 unresolved。
- 本次按真实 Retail 快照验证查询性能；没有声称全部源、全部历史版本均已重建并实测。
