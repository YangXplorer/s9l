# s9l 任务分解（WBS）

> 配套文档：[PLAN.md](./PLAN.md)。本文件把计划细化到工程师可直接领取的 task 粒度。
> 约定：每个 task 有 ID、产出物（Deliverable）、完成标准（DoD）、预估、依赖。预估为粗略人日（理想工时），仅供排期参考。
> 状态标记：`[ ]` 待办 · `[~]` 进行中 · `[x]` 完成 · `[!]` 阻塞

**前置硬阻塞**：技术栈 D1 = Go 待你最终确认。以下 WBS 按 **Go** 编写。

---

## Phase 0 — 脚手架（目标：跑通最小闭环 + CI 绿）

预估合计：~1 人日

- [x] **P0-1 初始化 Go 项目**
  - 产出：`go.mod`（`module github.com/YangXplorer/s9l`，go1.26），`.gitignore`，`.editorconfig`
  - DoD：`go build ./...` 通过 ✅
  - 依赖：D1 确认 · 预估：0.25d

- [x] **P0-2 目录骨架**
  - 产出：`cmd/s9l/main.go` + `internal/` 下：
    - `cli/`（命令解析）、`driver/`（DB 适配）、`repl/`（交互）、`render/`（输出）
    - `config/`（`config.go` + `connection.go`，YAML）
    - `secret/`（`store.go` 接口 + `keychain.go` + `memory.go`）
    - `history/`（`repository.go` + `sqlite.go`）
    - `query/`（`history.go` + `saved.go`）
  - DoD：各包有占位文件，`go vet ./...` 通过 ✅
  - 实现说明：Phase 1 才落地的包（cli/config/secret/history/query/repl）以 `doc.go` 占位；driver/render 为实体
  - 依赖：P0-1 · 预估：0.25d

- [x] **P0-3 `Driver` 接口草案（v0）**
  - 产出：`internal/driver/driver.go`，定义最小接口 + 注册机制
  - 接口含：`Driver.Name()`（兼作 DSN scheme，见 driver.go 取舍说明）+ `Open(ctx, dsn) (Conn, error)`；`Conn.Query/Exec/Close`；`Rows`（流式：`Columns()/Next()/Values()/Err()/Close()`）；`Result.RowsAffected()`
  - 注册：`driver.Register(d)` + `driver.Get(name)` + `driver.Open(ctx, name, dsn)` + `driver.Names()`
  - 取舍：合并 `Name()/Scheme()` 为单一 `Name()`，避免双重事实源（注释已说明）。`Rows` 用 `Values() ([]any, error)` 而非 `Scan(dest...)`，更贴合"不预知列类型"的终端客户端场景
  - DoD：接口编译通过，有 doc 注释说明设计取舍 ✅
  - 依赖：P0-2 · 预估：0.25d · **关键：阻塞所有适配器**

- [x] **P0-4 SQLite 适配器（基准实现）**
  - 产出：`internal/driver/sqlite/sqlite.go`，用纯 Go 驱动 `modernc.org/sqlite`（免 CGO）；`[]byte`→`string` 归一化
  - DoD：能 `Open` 一个 `.db` 文件并执行 `select 1` 返回流式 rows ✅
  - 依赖：P0-3 · 预估：0.25d

- [x] **P0-5 最小 CLI 入口 + `-e` 单次执行**
  - 产出：`s9l <path-or-dsn> -e "SQL"`，最朴素表格打印；`-version`；flag 可在 dsn 前后混排
  - DoD：`go run ./cmd/s9l ./test.db -e "select 1 as n"` 输出结果 ✅
  - 依赖：P0-4 · 预估：0.25d

- [x] **P0-6 CI（GitHub Actions）**
  - 产出：`.github/workflows/ci.yml`：lint(golangci-lint) + test(`go test -short -race ./...`) + build(`go build ./...`) 三 job
  - DoD：PR/push(main,develop) 触发，三步全绿（⚠️ 需一次 GitHub Actions 远端实跑确认）
  - 依赖：P0-1 · 预估：0.25d（可与 P0-3~5 并行）

- [x] **P0-7 完善 CLAUDE.md（填实）**
  - 产出：补充已拍板决策、目标架构、常用命令、测试约定、goal-check 机制、Git 约定
  - DoD：`/init` 复跑无需大改 ✅
  - 依赖：P0-5 · 预估：0.1d

**Phase 0 验收**：clone → `go build` → `s9l ./x.db -e "select 1"` 跑通，CI 绿。

---

## Phase 1 — MVP（目标：能日常替代 psql 做基本查询，支持 SQLite + PostgreSQL）

预估合计：~7–9 人日

### A. 连接与配置
- [x] **P1-A1 配置加载（YAML + XDG）**
  - 产出：`internal/config/config.go` + `connection.go`，读取 `$XDG_CONFIG_HOME/s9l/config.yaml`，回退 `~/.config/s9l/config.yaml`（`gopkg.in/yaml.v3`）
  - 结构：`connections:` 列表，每项 `ConnectionConfig{ID,Name,Driver,Host,Port,User,Database,SSL,Charset,PasswordRef}`
  - DoD：解析多命名连接 ✅；文件不存在视为空配置 ✅；解析错误清晰报错 ✅；YAML 往返一致（go-cmp 测试）✅
  - 依赖：P0-2 · 预估：0.75d

- [x] **P1-A2 连接解析与建立**
  - 产出：`s9l <id>` 查 config → 选 driver → 解析 `password_ref` → `ConnectionConfig.DSN()` → 连接；`s9l <dsn>` 裸 DSN 直连（`resolveTarget`）
  - DoD：SQLite 两种方式均跑通 ✅；postgres DSN 已随 P1-B1 落地 ✅
  - 依赖：P1-A1, P1-A4 · 预估：0.5d · 注：对话式输入密码留作后续，当前支持 `env:` / `keychain://`

- [x] **P1-A3 连接管理命令**
  - 产出：`s9l conn list/add/rm`（`cmd/s9l/conn.go`）
  - DoD：增删查写回 config.yaml ✅；文件权限 0600 ✅；不写明文密码（仅 `password_ref`）✅
  - 依赖：P1-A1 · 预估：0.75d · 注：写回不保留 YAML 注释（yaml.v3 限制）

- [x] **P1-A4 凭据：SecretStore 抽象（v0.1 memory 实现）**
  - 产出：`internal/secret/store.go`（`SecretStore` 接口 + `Resolve`）+ `memory.go`（仅内存）；`password_ref` 支持 `env:NAME` 与 `keychain://s9l/<key>`
  - DoD：不强制明文存密码即可连接 ✅；`Resolve` 框架就位，v0.2 换 keychain 实现不改调用方 ✅
  - 依赖：P0-2 · 预估：0.5d · 注：keychain 真实现见 P2-6；对话式输入后续补

### B. 执行引擎
- [x] **P1-B1 PostgreSQL 适配器**
  - 产出：`internal/driver/postgres/`，用 `jackc/pgx/v5/stdlib`（纯 Go，免 CGO）；`[]byte`→string 归一化；Metadata 用 pg_catalog/information_schema（`$1` 占位）；config 的 postgres DSN（postgres:// URL，sslmode，凭据 url 转义）
  - DoD：连 PG，流式 rows ✅；`RunConformance` 对真实 PG 全 PASS（testcontainers）✅；`\dt`/`\d`/`\l` 正确 ✅
  - 依赖：P0-3 · 预估：1d · **已验证 Driver 抽象对 SQLite/PG 两库够用（R2 风险消解）**

- [x] **P1-B2 流式读取与大结果保护**
  - 产出：`render.Source`/`WriteSource` 流式渲染——csv/tsv/json 逐行写出不缓冲全集（定期 flush）；table 仍缓冲(需算列宽，交互态)；`--max-col-width` 截断表格单元格（仅 table，机器格式不截以保数据完整）
  - DoD：csv/tsv/json 大结果不全量进内存（实测 5w 行流式导出）✅；`--max-col-width` 输出可控 ✅
  - 依赖：P1-B1 · 预估：0.5d · 注：render 单一实现(WriteSource)，Write 退化为 slice 适配

- [x] **P1-B3 错误与上下文处理**
  - 产出：`cmd/s9l/exec.go` `queryContext`(signal.NotifyContext SIGINT + 可选 `--timeout`) + `classifyErr`(Canceled→"query cancelled"、DeadlineExceeded→"query timed out after X")；`driver.Open` 连接错误包装 driver 名(`connect (postgres): ...`)；`-e` 与 REPL 每条查询独立可取消(REPL 取消查询不退出会话)
  - DoD：长查询 Ctrl-C 可中断（实测 `query cancelled`）✅；`--timeout` 可中断（实测 `query timed out after 500ms`）✅；错误含 driver 上下文 ✅
  - 依赖：P1-B1 · 预估：0.5d

### C. 交互与输出
- [x] **P1-C1 输出渲染器（table/json/csv）**
  - 产出：`internal/render/format.go`，`--format table|json|csv|tsv`；`render.Write` 分发；JSON 保列序、NULL→null；CSV/TSV NULL→空字段
  - DoD：四种格式正确输出 ✅，含 NULL 表示、宽表对齐（go-cmp 测试）✅
  - 依赖：P0-3 · 预估：1d

- [x] **P1-C2 管道友好（TTY 检测）**
  - 产出：`cmd/s9l` `outputFormat`/`isTTY`（mattn/go-isatty）：未显式 `--format` 时，TTY→table、非 TTY→tsv
  - DoD：`s9l ... -e "..." | jq`（json）/ 重定向默认 tsv 可解析 ✅
  - 依赖：P1-C1 · 预估：0.25d

- [x] **P1-C3 REPL 模式**
  - 产出：`internal/repl/repl.go`（端末非依存的 Loop：多行输入以 `;` 分割、`\q`/quit/exit 退出、Ctrl-C 丢弃当前缓冲、EOF 退出、exec 错误不中断）+ `cmd/s9l/repl.go`（TTY 用 `chzyer/readline`，非 TTY 用 scanner；连接复用一次、每条记历史）
  - 选库：**chzyer/readline**（纯 Go、成熟、历史/行编辑/Ctrl-C/D）
  - DoD：`s9l <id>`（无 `-e`）进 REPL，连续执行多条 SQL ✅；DDL/DML 无空输出（execute 列零跳过渲染）
  - 依赖：P1-A2, P1-C1 · 预估：1.5d
  - 注：`;` 朴素分割（字符串字面量内的 `;` 暂不特殊处理）；上下键历史由 readline 提供（交互态）

### D. 便捷命令
- [x] **P1-D1 元数据反斜杠命令**
  - 产出：REPL 与 `-e` 均支持 `\l`(库)、`\dt`(表)、`\d [table]`(表结构/列表)、`\?`(帮助)；`driver.Metadata` 可选能力接口（Databases/Tables/Columns），SQLite 用 pragma/sqlite_master 实现；渲染复用 render.Write
  - 实现：`cmd/s9l/meta.go`(runStatement/runMeta)；REPL Loop 对反斜杠命令行级即时分发（无需 `;`）
  - DoD：SQLite 下 `\l`/`\dt`/`\d`/`\?` 正确 ✅；未实现 Metadata 的 driver 报清晰错误 ✅
  - 依赖：P1-C1 · 预估：1d · 注：PG 的 Metadata 实现随 P1-B1 落地（方言差异下沉到各 driver）

### E. 工程化
- [x] **P1-E1 集成测试（testcontainers）**
  - 产出：`internal/driver/postgres/postgres_test.go` 用 `testcontainers-go/modules/postgres` 起容器跑 conformance + metadata；`testing.Short()` 跳过；CI 新增 `integration` job 跑全量 `go test ./...`
  - DoD：本地实测 conformance+metadata 对真实 PG 全 PASS ✅；`-short` 跳过容器测试 ✅；CI integration job 就位
  - 依赖：P1-B1 · 预估：1d

- [x] **P1-E2 README + 安装说明**
  - 产出：`README.md`（特性、`go install`、快速上手、config 示例 + `password_ref`、命令速查、开发）；`LICENSE`（MIT）
  - DoD：README 含从 `conn add` 到连 PG 跑查询的完整示例 ✅
  - 依赖：MVP 功能完成 · 预估：0.5d

- [x] **P1-E3 Release 流程（goreleaser + GitHub Actions）**
  - 产出：`.goreleaser.yaml`(v2，CGO_ENABLED=0，linux/darwin/windows×amd64/arm64) + `.github/workflows/release.yml`(tag `v*` 触发)；`cmd/s9l/main.go` 的 `version/commit/date` ldflags 注入；CI `build` job 改为 `goreleaser release --snapshot --clean` 干跑
  - DoD：本地 `goreleaser check` 通过、snapshot 出全平台二进制、`--version` 注入正确 ✅；打 tag 自动发布（待首次 tag 远端验证）
  - 依赖：P1-E2 · 预估：0.75d · 详见 [RELEASE.md](./RELEASE.md)

### F. 历史与收藏（SQLite，v0.1 纳入）
- [x] **P1-F1 history.db 初始化 + 迁移**
  - 产出：`internal/history/{history.go,saved.go}`，在 `~/.config/s9l/history.db`(XDG, 0600) 建库；内嵌迁移建表 `query_history` / `saved_queries`；Store 含两表完整 CRUD（AddHistory/ListHistory + SaveQuery/GetSaved/ListSaved/SearchSaved/DeleteSaved）
  - DDL：
    ```sql
    CREATE TABLE query_history (
      id INTEGER PRIMARY KEY AUTOINCREMENT,
      connection_id TEXT NOT NULL,
      database_name TEXT,
      sql_text TEXT NOT NULL,
      executed_at DATETIME NOT NULL,
      duration_ms INTEGER,
      rows_affected INTEGER,
      success BOOLEAN NOT NULL,
      error_message TEXT
    );
    CREATE TABLE saved_queries (
      id INTEGER PRIMARY KEY AUTOINCREMENT,
      title TEXT NOT NULL,
      description TEXT,
      connection_id TEXT,
      database_name TEXT,
      sql_text TEXT NOT NULL,
      tags TEXT,
      created_at DATETIME NOT NULL,
      updated_at DATETIME NOT NULL
    );
    ```
  - DoD：首次运行自动建库建表 ✅；重复运行迁移幂等 ✅
  - 依赖：P0-2, P0-4（复用 modernc.org/sqlite，直连 database/sql）· 预估：0.5d
  - 注：history 是 s9l 自身存储，直接用 database/sql，不走 Driver 抽象

- [x] **P1-F2 执行后写历史**
  - 产出：`cmd/s9l/history.go` `recordHistory`，`-e` 每次执行（含失败）写入 `query_history`（duration_ms/rows_affected/success/error_message）；`s9l history [--limit N]` 列出
  - DoD：成功与失败查询都落历史 ✅；写历史失败不影响主流程（stderr 降级告警）✅
  - 依赖：P1-F1 · 预估：0.5d
  - 注：database_name 暂留空（后续按命名连接补全）；REPL 接入后每条 REPL 查询亦复用 recordHistory

- [x] **P1-F3 收藏命令（CLI）**
  - 产出：`cmd/s9l/saved.go`：`s9l saved add/list/search/rm/run`；`run` 复用 `runQuery`（解析连接+执行+记历史）；`--format`/位置参数前后混排（`parseFlagsInterspersed`）
  - DoD：保存/列出/按关键字·标签搜索/按 connection 过滤/执行收藏 SQL 全部可用 ✅
  - 依赖：P1-F1 · 预估：0.5d · 注：清理了 `internal/query` 空占位（逻辑落在 internal/history）

**Phase 1 验收**：`s9l mypg` REPL 查询、`\dt`/`\d` 可用、`-e` + 管道导出 CSV 可用、命名连接增删查可用、查询历史自动记录、SQL 收藏可用、README 完整、CI 含集成测试。

**MVP 明确不做**：MySQL（→P2）、自动补全、TUI、运行期插件、系统 Keychain（→P2）、schema cache（→P2）、`query_folders`、SSH 隧道/TLS/IAM（→P3）、多结果集。

---

## Phase 2 — 多库扩展 + 体验增强

预估合计：~5–6 人日

- [x] **P2-1 MySQL 适配器**（`go-sql-driver/mysql`，纯 Go）— DoD：仅新增 `internal/driver/mysql/` + config mysql DSN 分支 + 注册，核心零改动 ✅；Metadata 用 information_schema（`?` 占位）✅；testcontainers(mysql:8.4) conformance+metadata 全 PASS ✅ · 预估：1d
- [ ] **P2-2 自动补全**（REPL 内表名/列名补全，基于 Metadata 缓存）· 预估：2d
- [ ] **P2-3 结果分页/翻页**（大结果交互式翻页或 pager 集成）· 预估：1d
- [ ] **P2-4 错误信息与帮助打磨**（`s9l --help`/`\?`）· 预估：0.5d
- [ ] **P2-5 query_folders 收藏分组**（建 `query_folders` 表，`saved_queries.folder_id` 关联）· 预估：0.5d
- [ ] **P2-6 系统 Keychain（SecretStore.keychain 实现）**
  - 产出：`internal/secret/keychain.go`，基于 `zalando/go-keyring`；`password_ref: keychain://s9l/connection.<id>.password` 解析；`s9l conn add` 时写入 keychain
  - DoD：macOS/Windows/Linux 三平台密码可存取；切换 memory→keychain 不改调用方
  - 预估：1d
- [ ] **P2-7 schema cache（可选）**（`~/.cache/s9l/schema.db`，缓存表/列元数据加速补全）· 预估：1d
- [ ] **P2-8 发布渠道扩展**（启用 goreleaser Homebrew tap + `install.sh` + 配 `HOMEBREW_TAP_TOKEN`；建空仓库 `YangXplorer/homebrew-tap`）· 预估：0.5d · 详见 [RELEASE.md](./RELEASE.md)

**Phase 2 验收**：新增 MySQL 不触碰核心层；补全/分页可用；密码进系统 Keychain；config.yaml 无明文密码；`brew install` 可用。

> 注：P2-2~P2-8 **暂保留**（用户优先做 Phase T TUI）。

---

## Phase T — 全屏 TUI（lazygit 式，tview）

预估合计：~2–3 人周（做到可用且较打磨）。框架 `rivo/tview`（决策 D10）。设计详见 [TUI.md](./TUI.md)。
原则：**只新增 `internal/tui/` 展示层，不改核心**；逻辑与 tview 渲染解耦以便单测。

### T-0 脚手架（先做）
- [ ] **T-0 TUI 骨架 + `s9l tui` 子命令**
  - 产出：加 `rivo/tview` 依赖；`internal/tui/` 包；`s9l tui [conn]` 子命令；最小 `App`（启动全屏、`q`/`Ctrl-C` 退出、空 Flex 布局）
  - DoD：`s9l tui` 能进全屏并干净退出；`go build`/`-short` 不受影响；TUI 逻辑可注入测试（SimulationScreen 启停冒烟）
  - 依赖：现有 cmd 结构 · 预估：0.5d

### T-1 MVP 垂直切片（连接→树→查询→结果）
- [ ] **T-1a 连接列表面板**
  - 产出：左上 List/TreeView 列出 config 连接；`Enter` 经 `secret.Resolve`+`driver.Open` 连接；带 `conn` 参数自动连；status 显示当前连接
  - DoD：能从配置选连接并连上 SQLite/PG/MySQL；连接失败错误进 status 不崩溃
  - 依赖：T-0 · 预估：0.75d
- [ ] **T-1b schema 树面板**
  - 产出：左下 TreeView，连接后经 `Metadata.Databases/Tables` 懒加载 库→表
  - DoD：树正确展示当前连接的库与表（三库均可）
  - 依赖：T-1a · 预估：0.75d
- [ ] **T-1c 结果表格 + 选表查询**
  - 产出：主区 `tview.Table`；`Enter` 选表 → `SELECT * FROM t LIMIT N`（N 可配）→ 填表格；NULL/宽列处理；上下/翻页/横向滚动；status 显示行数/耗时
  - DoD：选表即见结果，可滚动；空结果/NULL 正常；查询走可取消 context
  - 依赖：T-1b · 预估：1d
- [ ] **T-1d 面板切换 + 帮助 + 退出**
  - 产出：`Tab`/`1-3` 切面板、面板内导航、`?` 帮助浮层、`q` 退出
  - DoD：键盘可在三面板间流畅操作；`?` 列出键位
  - 依赖：T-1c · 预估：0.5d
- **T-1 验收（MVP）**：`s9l tui pg` → 看到连接/schema 树 → 选表出结果表格 → 键盘浏览 → `q` 退出，全程不崩。

### T-2 SQL 编辑器 + 异步执行
- [ ] **T-2a SQL 编辑器面板**（多行 `TextArea`，`R`/`Ctrl-Enter` 运行 → 结果区；错误进 status）· 依赖 T-1c · 预估：1d
- [ ] **T-2b 异步执行 + 取消 + 加载态**（查询在 goroutine，`QueueUpdateDraw` 回推；`Esc` 取消；spinner/状态）· 依赖 T-2a · 预估：0.75d

### T-3 历史 / 收藏面板
- [ ] **T-3a 历史面板**（`Ctrl-R`：列 `history.ListHistory`，`Enter` 回填编辑器/执行）· 依赖 T-2a · 预估：0.75d
- [ ] **T-3b 收藏面板**（列/搜索 `saved`，`Enter` 运行，`s` 保存当前查询）· 依赖 T-2a · 预估：0.75d

### T-4 打磨 + 测试 + 文档
- [ ] **T-4a 键位/帮助/视觉打磨**（lazygit 式键位全集、聚焦高亮、错误提示）· 预估：1d
- [ ] **T-4b 测试**（逻辑层单测：状态/schema 加载/查询编排；tcell `SimulationScreen` 冒烟启动→选表→出结果）· 预估：1d
- [ ] **T-4c 文档**（README 增 TUI 章节 + 截图/演示；TESTING.md 记 TUI 手动验证项）· 预估：0.5d

**Phase T 验收**：`s9l tui` 提供连接/树/结果/编辑器/历史/收藏的键盘驱动全屏体验；核心层零改动；CI 绿；TUI 逻辑有单测 + 冒烟，手动验证清单通过。

---

## Phase 3 — Backlog（v0.3+，按需，未排期）

- **SSH Tunnel**（连接前建隧道）
- **TLS 配置**（CA/客户端证书、`sslmode` 细化）
- **AWS RDS IAM Auth**（临时 token 连接）
- TUI 全屏模式（结果浏览/编辑）
- 更多数据库：SQL Server / ClickHouse / MongoDB（需评估非关系型对接口的冲击）
- 运行期插件机制（plugin / wasm）— 仅当编译期抽象不够用时再评估
- 数据导入导出（CSV/JSON 批量）
- 历史/收藏的云同步与统计

---

## 关键路径（排期参考）

```
D1确认 → P0-3(Driver接口) → P0-4(SQLite) → P0-5(CLI) ──┐
                                                        ├→ P1-B1(PG) → P1-D1(元数据) → 验收
P1-A1(config) → P1-A2(连接) ───────────────────────────┘
P1-C1(渲染) → P1-C3(REPL) ─────────────────────────────┘
```
最长链在 **Driver 接口 → PG 适配 → 元数据/REPL**。Driver 接口（P0-3）是全局阻塞点，务必先稳定。

## 待确认（开工前）
1. **D1 技术栈 = Go？**（最高优先，唯一硬阻塞）
2. REPL 库选型留给 tech lead（P1-C3）。
3. ~~配置格式~~ → 已定 **YAML**。
4. ~~凭据/历史存储~~ → 已定 **config.yaml + 系统 Keychain + SQLite(history.db)**；v0.1 密码起动时输入不保存。
