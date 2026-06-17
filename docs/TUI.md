# s9l TUI 设计（Phase T）

> 目标：把 s9l 做成 **lazygit 式的全屏终端数据库客户端**——多面板、键盘驱动、即时浏览。
> 配套：[PLAN.md](./PLAN.md)（决策 D2/D10）· [TASKS.md](./TASKS.md)（Phase T WBS）。

## 目标与定位

- 形态：`s9l tui [conn]` 进入全屏 TUI（不替代 CLI/REPL，三者并存）。
- 体验对标 **lazygit**：左侧导航面板 + 主区内容 + 键盘驱动 + `?` 帮助 + 上下文操作。
- 内容核心：**连接 → schema 树 → 结果表格 → SQL 编辑**，外加历史/收藏面板。

## 框架决策：`rivo/tview`（已拍板 D10）

| 选项 | 取舍 |
|------|------|
| **tview ✅** | 内置 `Table`/`TreeView`/`Flex`/`Pages`/`Form`，DB 客户端"结果表格 + schema 树"开箱即用；纯 Go、成熟（k9s 同款）。落地最快。 |
| bubbletea | 现代、生态活跃、好看；但 Table/Tree 需自拼，工期更长。 |
| gocui | lazygit 本体用；纯面板、控件少，表格要自己画。 |

理由：s9l 最重的就是表格与树，tview 直接给到，能最快做出可用的 lazygit 式体验。后续若需高度定制视觉，可再评估迁移（属 Backlog）。

## 复用映射（关键：TUI 只是新展示层）

| 已有能力（直接复用） | TUI 用途 |
|---|---|
| `config.Load` / `ConnectionConfig` | 连接列表面板 |
| `secret.Resolve` | 连接时解析密码（env/keychain） |
| `driver.Open` / `Conn.Query` / `Conn.Exec` | 建连、执行、结果 |
| `driver.Metadata`(Databases/Tables/Columns) | schema 树（库→表）、表结构 |
| `history.Store`(AddHistory/ListHistory/Saved*) | 历史面板、收藏面板、执行记录 |
| `render` 的取值格式化（NULL/[]byte→string） | 结果填入表格单元格（抽出 `render.Cell(v) string` 共享） |
| `queryContext`/取消 思路 | TUI 内查询可取消（不阻塞 UI） |

**新增的只有**：`internal/tui/` 这一层（事件循环、布局、面板、键位、异步执行编排）。核心层不改动——延续"可拓展性"原则。

## 布局（tview Flex）

```
┌─ Connections ─┐┌─ Results ────────────────────────┐
│ > pg          ││ id │ name  │ email      │ age     │
│   my          ││ 1  │ alice │ a@x.io     │ 30      │
│   demo        ││ 2  │ bob   │ NULL       │ 25      │
├─ Schema ──────┤│ …(tview.Table, 可滚动/横向滚动)   │
│ ▾ app         ││                                   │
│   ▾ public    ││                                   │
│     users     │└───────────────────────────────────┘
│     orders    │┌─ SQL ─────────────────────────────┐
│   (TreeView)  ││ select * from users limit 200;    │
└───────────────┘└───────────────────────────────────┘
 status: pg · app.public.users · 200 rows · 12ms   [? help]
```

面板：
1. **Connections**（List/TreeView）：来自 config；`Enter` 连接。
2. **Schema**（TreeView）：库 → 表，懒加载（连接后才查 Metadata）；`Enter` 表 → `SELECT * LIMIT N` 进结果区。
3. **Results**（Table）：查询结果；上下/翻页/横向滚动；表头固定。
4. **SQL**（多行编辑，tview `TextArea`）：编辑 + 运行；错误进 status。
5. **Status/help bar**：当前连接/库/表、行数/耗时、错误、`?` 帮助提示。

## 键位（lazygit 式，初版）

- 面板切换：`Tab`/`Shift-Tab`，或 `1/2/3/4` 直达；面板内 `j/k/h/l` + 方向键。
- `Enter`：上下文动作（连接 / 加载表 / 运行）。
- `R` 或 `Ctrl-Enter`：运行 SQL 编辑器内容。
- `Ctrl-R`：历史面板；`s`：收藏当前查询；`p`/`P`：结果翻页。
- `Esc`：取消正在执行的查询 / 关闭浮层。
- `?`：帮助浮层；`q` / `Ctrl-C`：退出。
- 键位表集中在一处定义，便于改键与帮助生成（后续可做成可配置）。

## 状态与并发模型

- `App` 持有 `*tview.Application`、`Pages`、当前 `driver.Conn`、`config`、`history.Store`、选中库/表、当前结果集。
- **tview 单线程**：查询在独立 goroutine 执行，结果经 `app.QueueUpdateDraw(...)` 回推刷新，避免阻塞 UI。
- 查询用可取消 context（复用 B3 的 `queryContext` 思路）；`Esc` 取消当前查询。
- 大结果：默认 `LIMIT N`（可配）+ 表格虚拟滚动；不一次性全量进内存（延续 B2 流式精神，TUI 侧按需取或分页）。

## MVP 垂直切片（Phase T 第一刀）

能跑起来的最小闭环，验证框架与复用：
1. `s9l tui [conn]` 启动全屏。
2. Connections 面板（来自 config）；`Enter` 连接；带 `conn` 参数则自动连。
3. Schema 树（库→表，经 Metadata）。
4. `Enter` 选表 → `SELECT * FROM <t> LIMIT 200` → Results 表格。
5. Results 可滚动；`Tab` 切面板；`?` 帮助；`q` 退出。

**不含**（后续 T 任务）：SQL 编辑器、历史/收藏面板、收藏保存、横向滚动打磨、键位全集。

## 测试策略

- **逻辑与 UI 解耦**：状态转换、查询编排、schema 加载等放进**可单测的纯函数/方法**（不依赖 tview 渲染）。
- **交互冒烟**：用 tcell `SimulationScreen` 驱动基本按键路径（启动→连接→选表→出结果）做有限自动化。
- **手动冒烟**：真实终端跑一遍（连接/树/表格/键位）——TUI 视觉与手感无法完全自动化，明确记录为手动验证项（同 Keychain 的处理）。
- conformance/driver/CLI 既有测试不受影响（TUI 是叠加层）。

## 风险

| 风险 | 缓解 |
|------|------|
| TUI 工期被低估（lazygit 打磨度高） | 严格走 MVP 切片，先可用再打磨；按 T-0~T-8 分解、逐切片 goalkeeper |
| tview 单线程与异步查询竞态 | 统一经 `QueueUpdateDraw` 回推；查询 goroutine 只产数据不碰 UI |
| 大结果卡 UI | 默认 LIMIT + 分页/虚拟滚动，查询可 `Esc` 取消 |
| 自动化测试覆盖弱 | 逻辑层单测 + SimulationScreen 冒烟 + 手动验证清单 |
| 与 CLI/REPL 行为漂移 | 复用同一 driver/config/secret/history，避免逻辑分叉 |
