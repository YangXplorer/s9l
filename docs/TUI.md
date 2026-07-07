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

---

## TUI 强化（Phase 3，目标 v0.6）

Phase T 已交付可用的全屏 TUI；本阶段在其上做 **lazygit 风格的视觉/布局打磨与三个体验增强**。原则不变：**只改 `internal/tui/`，复用 driver/config/secret/history，核心零改动**；逻辑与渲染解耦，白盒 + SimulationScreen 冒烟 + 手动清单。

### 目标布局（强化后）

```
┌─[1] Connections ─┐┌─[3] Results ─────────────────────────┐
│  pg               ││ id │ name  │ email      │ age       │   ← 仅图标+名称
│  my               ││ 1  │ alice │ a@x.io     │ 30        │
│  demo             ││ 2  │ bob   │ NULL       │ 25        │
├─[2] Schema ───────┤│ /filter: ali ───────── filtered 1/2 │   ← 结果过滤器
│ ▾ app             ││                                      │
│   users           │└──────────────────────────────────────┘
│   orders          │┌─[4] SQL (F5 run) ────────────────────┐
│                   ││ select * from users                  │
│                   ││ where age > 18                       │   ← 编辑器约翻倍
│                   ││ order by created_at desc             │
│                   ││ limit 200;                           │
└───────────────────┘└──────────────────────────────────────┘
 pg · app.users · 200 rows · 12ms                              ← 状态行
 [Tab] panel  [n] new conn  [/] filter  [F5] run  [?] help  [q] quit   ← 键位栏
```

要点（对应用户 5 项需求）：

1. **配色与布局（lazygit 风格）**——集中式主题 `theme.go`：聚焦面板高亮边框（绿/青）、非聚焦淡色、圆角边框（`tview.Borders`）、面板标题带序号 `[1] Connections`（与 `1/2/3/4` 跳转键一致）、底部独立「键位提示栏」（与状态行分离）。尊重 `NO_COLOR`。
2. **Connections 仅名称 + 图标**——主文本 `<icon> <name|id>`，按驱动给数据库图标（Nerd Font 字形 + ASCII 回退 `[pg]/[my]/[sq]`，可关）；host/db 等细节移到淡色副行或去除。
3. **SQL 编辑器面积翻倍**——固定高 6 → 12（小窗口有回退），Results/SQL 纵向比例相应调整。
4. **界面内「新增连接」表单**——Connections 面板 `n` 打开 `tview.Form`（id/name/driver 下拉/host/port/user/database/ssl/password 或 password-ref），提交→`config.Add`+`Save`，有密码则写系统 keychain（配置仅存 ref）→刷新列表；`Esc` 取消。复用 `config`/`secret`，核心零改动。
5. **结果过滤器**——App 持有上次结果集（列+行），`/` 打开过滤框，按子串（大小写不敏感、跨列）客户端实时过滤并重渲染，状态显示 `filtered M/N`，`Esc` 关闭/清空。

### 新增键位

| 键 | 作用 | 适用面板 |
|----|------|----------|
| `n`（或 `Ctrl-N`） | 打开「新增连接」表单 | Connections |
| `/` | 打开结果过滤框 | Results |
| `Esc` | 取消查询 / 关闭浮层·过滤·表单 | 全局 |

### 强化阶段测试策略
- **纯函数优先**：`connIcon(driver)`、`filterRows(cols, rows, term)`、表单值→`ConnectionConfig` 映射等抽成可单测纯函数。
- **持久化校验**：新增连接表单走 `config.Load` 往返断言；密码进 keychain（go-keyring `MockInit`）只验 ref 与解析，不碰真实 OS keychain。
- **SimulationScreen 冒烟**：聚焦切换边框色、`n` 开表单、`/` 过滤路径。
- **手动清单**：真实终端核对配色/圆角/图标/键位栏（视觉项无法完全自动化，明确为手动验证）。

---

## 交互重构（Phase 4，目标 v0.7）

按用户反馈调整层次：**数据库从 Schema 面板上移到 Connections**，Schema 只剩「当前库的表 + 检索」。这取代 Phase 3/B-7 在 Schema 内的库→表树。

### 目标布局（重构后）

```
┌─[1] Connections ─┐┌─[3] Results ─────────────────────────┐
│ ▾ [my] neohub     ││ id │ name  │ email                   │
│     app           ││ …                                    │
│   ▸ logs          ││                                      │
│ ▸ [pg] dev        │├──────────────────────────────────────┤
├─[2] Schema ───────┤│  (results / filter)                  │
│ /tbl: ord ─────── ││                                      │
│   orders          │└──────────────────────────────────────┘
│   order_items     │┌─[4] SQL (F5 run) ────────────────────┐
│   products        ││ select * from orders                 │
│                   ││ …                                    │
└───────────────────┘└──────────────────────────────────────┘
 my · app · 200 rows · 12ms
 [Tab] panel  [n] new  [/] filter  [^R] history  [?] help  [q] quit
```

- **Connections**：树。连接为根（图标+名称，T3-2），`Enter` 连接并展开其**数据库**（懒加载 `Metadata.Databases()`）；选某数据库设为「当前库」并刷新 Schema。`e`/`d` 在连接节点上编辑/删除（B-6）。
- **Schema**：只列「当前库」的表（`databaseBrowser.TablesIn(currentDB)`，无能力则 `Metadata.Tables()`），支持 `/` 检索（`filterTables` 子串过滤）。选表 `Enter` 预览（`previewQuery`/`qualifyTable` 方言化）。
- **背景**：`tview.Styles.PrimitiveBackgroundColor=ColorDefault` 等，跟随终端（与 lazygit 一致）。

### 新增键位

| 键 | 作用 | 适用 |
|----|------|------|
| `Enter` | 连接节点=连接+展开库 / 数据库节点=设为当前库刷新 Schema | Connections |
| `/` | 检索当前库的表 | Schema |

### 重构测试策略
- 纯函数：`filterTables(names, term)`、Connections 树构建（连接→库）、Schema 表列表过滤。
- fake conn（Metadata + databaseBrowser）驱动 Connections 展开库 / 选库刷新 Schema / 表检索的白盒。
- 手动清单：真实终端核对背景与 lazygit 一致、库展开、表检索手感。

---

## 二轮视觉微调（Phase 5.1，按用户实测反馈）

T5 落地后的可读性/观感二次打磨，仍只改 `internal/tui/`、核心零改动。详见 [TASKS.md](./TASKS.md) Phase 5.1。

1. **选中行 / 输入框背景更浅**——`theme.Selection`、`theme.Field` 进一步调浅，配黑色文字保证浅底深字高对比（兼顾真彩降采样终端），解决「选中行看不清内容」。
2. **去「橙色树形」**——数据库子节点去 `Accent` 着色（不再显橙/绿）；Schema `SetTopLevel(1)` 隐藏库根节点、表列表扁平无缩进；展开/折叠三角 `▾`/`▸` 仅在「上层」连接节点，库/表叶子无三角。
3. **Connections 上下移动**——方向键 + vim `j/k` 选择，当前行经更浅选中样式清晰高亮。

## 三轮微调 + 连接测试（Phase 5.2）

1. **选中行 / 输入框再调浅**——`Selection`/`Field` 进一步近白（0xf0f0f0 / 0xf4f4f4），配黑字高对比。
2. **New connection 表单「Test」按钮**——保存前一键试连：复用 `dial.OpenWithPassword`（用表单内未入库的明文密码试连，空则回退 `secret.Resolve`），带 5s 超时、goroutine 异步、`QueueUpdateDraw` 回推；结果显示在表单标题（`testing…` / `✓ connection OK` / `✗ <error>`），不阻塞 UI。详见 [TASKS.md](./TASKS.md) Phase 5.2。

## 上下文相关 `/` 检索（Phase 5.3）

`/` 的检索对象**随聚焦面板自动切换**，规则一致「检索当前面板的内容」：

- **Connections** → 检索当前连接下的**数据库**（新增；`applyConnFilter` 重建库子节点，状态栏 `databases M/N`）。
- **Schema** → 检索**表**（已支持，`applySchemaFilter`/`filterTables`）。
- **Results** → 过滤**结果行**（已支持，`applyFilter`/`filterRows`）。

实现上 `showFilter`/`hideFilter` 由原布尔 `filterSchema` 改为三态 `filterTarget`（conn/schema/results），按 `focusIdx` 分派 title/initial/onChange。`Enter` 保留、`Esc` 清空。详见 [TASKS.md](./TASKS.md) Phase 5.3。

## 输入辅助（Phase 7）

- **补全数据源（T7-1）**：`repl.NewSchemaCache`（从 cmd 提升到 `internal/repl` 共用；live-first + schemacache 写透/失败回退，readline 适配器仍留 cmd）。TUI 侧 `tuiSchema` 适配：表名**优先**用 Schema 面板已加载的 `schemaTables`（跟随所选库，连接级 metadata 只见所连库）；预览表的列直接用 `lastCols`；**查询运行中不发 DB 往返**（R4——补全与运行中查询共用 `a.conn`）。completer 随 `connect()` 重建、`closeConn` 置空；schemacache store 惰性打开、`Run` 退出时关闭。
- **WHERE 字段候补（T7-2）**：InputField `SetAutocompleteFunc`＝`whereCandidates`（光标前标识符前缀；引号内不补（奇偶校验含 `''` 转义）；前缀匹配优先+子串次之、大小写不敏感；唯一候补与已输入全等时不弹，避免采用后立刻重开）。`SetAutocompletedFunc` 用候补替换尾部前缀；下拉配色经 `SetAutocompleteStyles` 与编辑器弹层一致（Surface 底+FieldText 字+accent 选中，tview 默认色在暗主题下不可读）。**Enter 两态**：候补展开（`acOpen`）时 Enter/↑↓ 交给输入框选候补、Esc 只收列表；收起时 Enter 应用 WHERE、Esc 清除（onKey `filterOpen` 分支按 `acOpen` 分流）。
- **SQL 编辑器补全弹层（T7-3）**：触发＝标识符 ≥2 字符自动（编辑器 `SetChangedFunc`）或 `Ctrl-Space` 手动；候补＝`repl.Completer.Complete(text, runePos)`（关键字+表+列+`table.col`；pos 按 byte⇄rune 换算，R3）。**弹层不走 Pages**——Pages 任何增删都会把焦点重定向到最顶层可见页（曾致 hideCompletion↔syncFocus 无限递归），改为 `completionHost` 包装主布局、`Draw` 后直接叠画 `tview.List`（display-only、永不持焦，位置每帧跟随光标 `GetCursor`+`GetOffset`，下方放不下→上方→面板底边）。键路由（onKey `completionOpen` 分支）：`↓/↑` 选、`Tab`/`Enter` 采用（`Replace` 按字节替换当前词、undo 栈保留、`compInserting` 防 changed 重入）、`Esc` 只关弹层、`F5` 关弹层并运行；焦点离开编辑器即关（syncFocus）。`repl.isWordRune` 放宽为 Unicode 字母/数字——CJK 标识符也可补全（REPL 同步受益）。
- **F6 编辑器扩大（T7-4）**：`rightFlex.ResizeItem` 切换 editor 固定 12 行 ⇄ 比例 7:3（Results 压缩），布局不重建、状态跨查询/翻页/过滤保持；编辑器聚焦中也可用。

## Results 面板增强（Phase 6）

- **全字段模糊检索 `/`**：`filterRows` 用 `fuzzyMatch`（大小写不敏感**子序列**）跨所有列匹配。
- **按列过滤 `f`**：`filterRowsByColumn` 仅匹配选中列；与 `/` 共用 `openFilterInput` 浮层，`filterTarget` 增 `filterTgtResultsCol`。
- **单元格导航**：`SetSelectable(true,true)`，`←/→`·`h/l` 在 cell 间移动；`v` 浮层查看完整值。
- **单表预览 WHERE 过滤 `/`（服务端）**：预览时 `/` 打开 WHERE 表达式输入（label `WHERE`），Enter 应用（`applyWhere`→page 归零→`refreshPreview` 重查）、Esc 清除；不逐键查询（`pendingWhere` 暂存）。`previewQuery(driver, qualified, where, limit, offset)` 方言化：sqlserver 首页 `TOP n`、翻页 `ORDER BY (SELECT NULL) OFFSET…FETCH`，其余 `LIMIT [OFFSET]`。非预览结果 `/` 仍为客户端全字段模糊。当前表/WHERE/页码常驻 Results 标题（`setResultsTitle`；任意 SQL 执行时复位）。
- **鼠标聚焦同步**：四面板 `SetFocusFunc`→`syncFocus(i)` 统一维护 `focusIdx`+边框色，鼠标点击与 Tab/数字键聚焦行为一致（否则面板键 `]`/`[`/`v`/`f`/`c`/Enter 在鼠标聚焦后失效）；keybar 常显 `[ ] page`。
- **分页键 IME 容错**：`]`/`[` 之外接受 `>`/`<` 与全角 `］［＞＜`（CJK 输入法开启时物理键发出全角字符，否则翻页看似失灵）；查询进行中按键给状态栏提示而非静默。
- **分页 `]` / `[`**：预览态 `resultPage`，`]` 下一页（仅满页时）/ `[` 上一页；`refreshPreview` 统一重查（编辑写回后的刷新同经路，WHERE/页码保留）；查询进行中翻页/换 WHERE 被拒绝（防状态漂移）。
- **网格线**：Results `SetBorders(true)`，行列之间有线分隔。tview 会把 cell 背景涂到四周边框上（`bh=3/bw+2`），故用 `gridTable` 包装：`Draw` 后把网格线字形重涂为边框色+底色，高亮（行条/选中 cell）严格限制在格内。行条用比 accent 绿浅一档的可见中间色，行条 cell `SetTransparency(false)` 整格填充。
- **WHERE 失败回滚**：预览记录最后成功的 `goodWhere/goodPage`；查询失败（如列名笔误）时回滚 `resultWhere/resultPage` 并复位标题，避免"标题显示了 WHERE 但数据没变"的错觉；错误信息在状态栏。
- **默认分页**：`resultLimit`=100（100 行/页），预览标题常显 `page N`（含第 1 页）。
- **任意 SQL 结果客户端分页**：非预览结果 >100 行时按 `viewPage` 客户端切片渲染，`]`/`[` 复用（预览=服务端重查、任意结果=切片翻页），标题 `page N/M`；`/`、`f` 过滤后重新分页且回第 1 页；`v` 查看值经 `viewRows`（渲染切片）映射行。
- **SQL Server Unicode 字面量**：不带 N 的 `'楊'` 会被库默认排序规则转成 `'?'` 静默 0 行——`sqlserverNLiterals` 在预览 WHERE 组 SQL 时自动给含非 ASCII 的单引号字面量补 `N` 前缀（处理 `''` 转义、跳过已有 N）；仅 sqlserver，cell 编辑为参数化查询不受影响。
- **二进制值显示（乱码修复）**：driver 层把 `[]byte` 归一化为 `string`，二进制列（如 MySQL `binary(16)` UUID）原样打印会成乱码——`render.Cell` 对 `[]byte` 及含非法 UTF-8/控制字符的字符串改显 `0x…` 十六进制；TUI `cellString` 与 REPL 表格共用，csv/tsv/json 机器格式保持原始数据。
- **选中行高亮 + 当前 cell 强调**：光标行**整行**套 `selectionStyle()` 行条（tview 表级选中样式只画当前 cell，整行由 `highlightResultsRow` 在选中变化时手动着色、旧行恢复默认）；**当前 cell** 由表级 `SetSelectedStyle(cellCursorStyle())`（accent 背景+黑字+粗体；NO_COLOR 下 reverse+bold）叠加凸显。`fillResults` 重渲染后显式 `Select` 重挂行高亮（Clear 丢样式且 tview 的选中钳制不触发回调）。
- **就地编辑写回 `c` / Enter（`UPDATE`）**：Results 焦点时 Enter 是 `c` 的别名（电子表格直觉；其他面板 Enter 行为不变）。仅**单表预览**（`runTableQuery` 设 `resultEditable`/`resultTable`；`runQuery` 默认置否）可编辑。`buildUpdate` 生成 `UPDATE 表 SET 列=? WHERE <整行原值>`（NULL→`IS NULL`，方言 placeholder 经 `placeholderTUI`、标识符经 `quoteIdent`），确认弹窗显示 SQL → `conn.Exec` 异步执行 → 刷新预览、报告影响行数。**不做主键检测**（整行 WHERE，driver 零改动）；重复行一起更新（实害小）；设 NULL 暂未支持。核心 driver 接口零改动。
