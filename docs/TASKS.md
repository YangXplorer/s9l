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
- [x] **P2-2 自动补全**：REPL 内 `Tab` 补全 SQL 关键字 / 表名 / 列名。`internal/repl/complete.go` 终端无关核心(`Completer`+`Schema` 接口)：词前缀匹配、`\` 元命令、`table.column` 限定补全、当前语句中引用到的表自动纳入其列；`cmd/s9l/complete.go` `schemaCache`(基于 `driver.Metadata` 懒加载缓存表/列，含 nil 容错) + `readline.AutoCompleter` 适配器，仅 TTY 路径启用。白盒 `complete_test.go`(repl 核心 6 例 + cmd schemaCache 实 SQLite E2E)。核心 driver 层零改动。· 预估：2d
- [x] **P2-3 结果分页/翻页**：TTY 输出经 `$PAGER` 分页(默认 `less -FIRX`，单屏内直接打印)。`cmd/s9l/pager.go`：`pagerArgs`(S9L_PAGER 覆盖 PAGER、空值禁用、默认 less)、`maybePager`(仅 *os.File 终端启用，返回 pipe+finish，否则原样直通)、`isBrokenPipe`(用户提前退出 pager 的 EPIPE 视为正常)。`runStatementPaged` 包裹 `-e`/REPL/`saved run` 渲染；`--no-pager` flag 禁用。非 TTY(管道/脚本/测试)绝不分页。白盒 `pager_test.go`(参数解析 7 例 + 非 TTY 直通 + 禁用 + EPIPE)。核心 driver 层零改动。· 预估：1d
- [x] **P2-4 错误信息与帮助打磨**：`cmd/s9l/help.go` 顶层 `s9l help`/`-h`/`--help` 概览(用法/子命令 conn·history·saved·tui/查询 flags/凭据说明)；`\?` 帮助在 REPL/TUI 既有。错误已带 driver/上下文(既有)。白盒 `TestRunHelp`。· 预估：0.5d
- [x] **P2-5 query_folders 收藏分组**：`query_folders` 表(name UNIQUE) + `saved_queries.folder_id`(幂等 `ALTER TABLE ADD COLUMN`)；Store `CreateFolder`/`ListFolders`/`DeleteFolder`(删文件夹时把内含查询 `folder_id` 置空、不删查询)/`SetSavedFolder`/`ListSavedByFolder`(0=未归档)；CLI `s9l saved folder add|rm`、`saved folders`、`saved add --folder N`、`saved list --folder N`、`saved mv <id> --folder N`。白盒 `TestFolderCRUDAndAssignment`/`TestDeleteFolderUnfilesQueries` + CLI `TestSavedFolders`。核心零改动。· 预估：0.5d
- [x] **P2-6 系统 Keychain（SecretStore.keychain 实现）**
  - 产出：`internal/secret/keychain.go`（`Keychain` 实现 SecretStore，基于 `zalando/go-keyring`；`Default()` 返回 keychain；`KeychainRef`/`ConnPasswordKey` 辅助）；`s9l conn add --password` 写入 keychain 并自动设 `password_ref`，`conn rm` 删除；`resolveTarget` 与 TUI 改用 `secret.Default()`；`keychain://` 解析复用既有 `Resolve`
  - DoD：`conn add --password` 存 keychain、config 仅留 ref、解析回连（白盒 `TestConnAddWithPasswordUsesKeychain`，用 go-keyring `MockInit`）✅；keychain 只在 `keychain://` ref 时触碰（env:/无密码连接无需 keyring 后端）✅；切换 memory→keychain 调用方不变（`SecretStore` 接口）✅
  - 预估：1d · 注：真实 OS keychain 读写为手动验证（CI 用 MockInit，符合 TESTING.md 约定）；对话式密码输入后续
- [x] **P2-7 schema cache**：`internal/schemacache`(SQLite `~/.cache/s9l/schema.db`，遵循 `$XDG_CACHE_HOME`，0700/0600)，按 connection_id 存表/列名(`cached_tables`/`cached_columns`，列保序)。`schemaCache` 接入永续层：**live 优先 + 成功 write-through + live 失败时 disk 回退**(上次会话的 last-known schema)，跨会话/离线仍可补全。**仅名为连接缓存**(bare DSN 可能含密码 → connID="" 不持久化，遵守"不存明文密码")。白盒 schemacache 包测试(round-trip/replace/隔离/列序/迁移幂等) + cmd write-through/fallback 集成测试。核心 driver 层零改动。· 预估：1d
- [x] **P2-8 发布渠道扩展（Homebrew）**：建 `YangXplorer/homebrew-tap`；`.goreleaser.yaml` 加 `homebrew_casks`(非 deprecated, 含 quarantine 清除 hook)；`release.yml` 传 `HOMEBREW_TAP_TOKEN`(已设为 repo secret)；README 加 `brew install YangXplorer/tap/s9l`。`goreleaser check` 干净、snapshot 生成 cask。首个 tag(>= 下次发布)推送 formula 后 brew 可用。· 预估：0.5d · 注：install.sh 留后续

**Phase 2 验收**：新增 MySQL 不触碰核心层；补全/分页可用；密码进系统 Keychain；config.yaml 无明文密码；`brew install` 可用。

> 注：P2-2~P2-8 **暂保留**（用户优先做 Phase T TUI）。

---

## Phase T — 全屏 TUI（lazygit 式，tview）

预估合计：~2–3 人周（做到可用且较打磨）。框架 `rivo/tview`（决策 D10）。设计详见 [TUI.md](./TUI.md)。
原则：**只新增 `internal/tui/` 展示层，不改核心**；逻辑与 tview 渲染解耦以便单测。

### T-0 脚手架（先做）
- [x] **T-0 TUI 骨架 + `s9l tui` 子命令**
  - 产出：`rivo/tview`+`gdamore/tcell/v2` 依赖；`internal/tui/tui.go`（`App`：Flex 占位 body + 状态栏、`q`/`Ctrl-C` 退出、测试接缝 SetScreen/OnReady/SendKey）；`cmd/s9l/tui.go` `runTUI` + `s9l tui [conn]` 子命令 + usage
  - DoD：SimulationScreen 测试启动→送 `q`→干净退出 ✅；真实 pty 冒烟 exit 0 ✅；`go build`/`-short`/lint 不受影响 ✅
  - 依赖：现有 cmd 结构 · 预估：0.5d

### T-1 MVP 垂直切片（连接→树→查询→结果）
- [x] **T-1a 连接列表面板**
  - 产出：lazygit 式多面板 Flex 骨架（Connections/Schema/Results/SQL 占位 + 状态栏）；Connections List 来自 config；`Enter` 经 `secret.Resolve`+`cc.DSN`+`driver.Open` 连接；`s9l tui <conn>` 自动连；status 显示当前连接/错误；退出时关连接；Options 可注入 Config/Store 便于测试
  - DoD：从配置选连接并连上（SQLite 实测；PG/MySQL 同路径）✅；连接失败进 status 不崩溃、conn 保持 nil ✅；白盒测试 connect/auto-connect/错误/列表填充 + 真实 pty 冒烟 exit 0 ✅
  - 依赖：T-0 · 预估：0.75d
- [x] **T-1b schema 树面板**
  - 产出：左下 TreeView，连接成功后 `loadSchema` 经 `driver.Metadata.Tables()` 列出当前库的表；表节点以表名为 reference（供 T-1c 选表查询）；无 Metadata 能力/错误时给提示/进 status 不崩溃；`collectFirstColumn` 读元数据首列
  - DoD：树正确展示当前连接库的表（SQLite 白盒测试；PG/MySQL 走同一 Metadata 路径，已有各自 metadata IT）✅
  - 依赖：T-1a · 预估：0.75d · 注：单连接只见一个库；跨库切换（库→表 多级）留作后续强化
- [x] **T-1c 结果表格 + 选表查询**
  - 产出：schema 树 `Enter` 表节点 → `runTableQuery` → `SELECT * FROM <quoteIdent> LIMIT 200` → `runQuery` 填 `tview.Table`（表头固定+加粗、NULL→"NULL"、行可选可滚动）；status 显示行数/耗时；`quoteIdent` 按方言转义(mysql 反引号/其余双引号)；`drainRows`/`cellString` 辅助
  - DoD：选表即见结果（白盒 `TestRunTableQueryFillsResults`：表头+2行+NULL 正确）✅；空结果/NULL 正常 ✅；错误进 status 不崩溃
  - 依赖：T-1b · 预估：1d · 注：当前同步执行（可取消 context 是 T-2b）；查询历史记录随 T-3
- [x] **T-1d 面板切换 + 帮助 + 退出**
  - 产出：`tview.Pages`(main+help)；`onKey`：`Tab`/`Shift-Tab` 循环、`1/2/3` 直达 Connections/Schema/Results、聚焦面板黄色高亮、`?` 帮助浮层(居中, 任意键关闭)、`q`/`Ctrl-C` 退出；面板内导航由 tview 控件(方向键)处理；状态栏更新提示
  - DoD：键盘在三面板间切换并高亮 ✅；`?` 列出键位、任意键关闭 ✅；`q` 退出 ✅（白盒 focus/help/Tab 测试 + 真实 pty `q`/`?→x→q` 均 exit 0）
  - 依赖：T-1c · 预估：0.5d
- **T-1 验收（MVP）✅**：`s9l tui <conn>` → 连接列表/schema 树 → 选表出结果表格 → 键盘切换面板/浏览 → `?` 帮助 → `q` 退出，全程不崩（SQLite 实测；PG/MySQL 同路径）。

### T-2 SQL 编辑器 + 异步执行
- [x] **T-2a SQL 编辑器面板**
  - 产出：`a.editor` 改为 `tview.TextArea`(多行可编辑, placeholder)；加入 navPanels(面板 4, `Tab`/`4` 可达)；**`F5` 运行**编辑器 SQL → `runQuery`(复用 T-1c)→ 结果/错误进 status；`onKey` 作用域化：编辑器聚焦时 `q`/`1-4`/`?` 等作为文本输入透传，仅 `F5`/`Tab`/`Ctrl-C` 全局生效；帮助/状态栏更新(F5 run、1/2/3/4)
  - DoD：编辑器输入 SQL、`F5` 运行见结果（白盒 `TestRunEditorExecutes`）✅；编辑时 `q`/`1`/`?` 不误触发快捷键（白盒 `TestEditorTypingPassesThrough`）✅；空输入提示；真实 pty 输入+`Ctrl-C` exit 0
  - 依赖：T-1c · 预估：1d · 注：运行键用 `F5`(DB 工具习惯, 终端可靠; `Ctrl-Enter` 不可靠);多语句拆分后续
- [x] **T-2b 异步执行 + 取消 + 加载态**
  - 产出：`runQuery` 改异步——查询在 goroutine 跑，结果经 `app.QueueUpdateDraw` 回推填表；执行中 status "running…([Esc] 取消)"、并发再触发提示"已在运行"；`Esc` → `a.cancel()` 取消(空闲时透传)；完成/出错经 `classifyErr`(Canceled→"query cancelled"/DeadlineExceeded→"query timed out")；同步核 `fetch`(query+drain)/`fillResults` 拆出便于单测
  - DoD：查询不阻塞 UI（goroutine+QueueUpdateDraw）✅；`Esc` 取消运行中查询（白盒 `TestEscCancelsRunningQuery`，空闲透传）✅；`fetch` 成功/取消(`TestFetchCancelled`)、`classifyErr` 各分支、空编辑器不启协程 均白盒覆盖 ✅；真实 pty 启动/退出正常
  - 依赖：T-2a · 预估：0.75d · 注：动态 spinner 动画留 T-4 打磨（当前静态运行态提示）；查询历史记录随 T-3

### T-3 历史 / 收藏面板
- [x] **T-3a 历史面板**
  - 产出：`Ctrl-R` 打开历史浮层（`history.ListHistory` 最近 100 条，时间/ok·ERR/SQL 单行）；`Enter` 回填编辑器(不自动执行, 用户审阅后 F5)+聚焦编辑器+关浮层；`Esc`/`Ctrl-R` 关闭；TUI 执行查询亦经 `recordHistory` 写入(含失败/耗时/行数)；history 经 `Options.History` 注入(cmd 开默认库, New 不做 I/O, 缺失则降级禁用)
  - DoD：Ctrl-R 列历史、Enter 回填编辑器（白盒 `TestShowHistoryAndUseSQL`）✅；执行写历史（`TestRecordHistory` 成功+失败）✅；history 禁用时 Ctrl-R/记录 无操作不崩（`TestShowHistoryDisabled`/`...DisabledIsNoop`）✅；真实 pty Ctrl-R→Esc→q exit 0
  - 依赖：T-2a · 预估：0.75d · 注：database_name 暂空；自动执行选项后续
- [x] **T-3b 收藏面板**
  - 产出：`Ctrl-S` 保存编辑器 SQL 为收藏（标题自动取 SQL 首 50 字, connID）；`Ctrl-F` 打开收藏浮层（`history.ListSaved`，title/conn/tags/SQL），`Enter` 运行选中（回填编辑器+`runQuery`），`Esc`/`Ctrl-F` 关闭；空编辑器不保存提示；history 禁用降级；帮助/状态栏更新
  - 键位说明：`Ctrl-S`(tcell raw 模式禁用 XON/XOFF, Ctrl-S 可用) / `Ctrl-F` favorites
  - DoD：保存收藏（白盒 `TestSaveCurrent`，空不存）✅；Ctrl-F 列收藏并可开关（`TestShowSavedOverlay`）✅；禁用 no-op（`TestSaveDisabledIsNoop`）✅；真实 pty 编辑→Ctrl-S→Ctrl-F→Esc→Ctrl-C exit 0 且 `saved list` 确认已存 ✅
  - 依赖：T-2a · 预估：0.75d · 注：标题/标签编辑、搜索框后续（当前列全部）

### T-4 打磨 + 测试 + 文档
- [x] **T-4a 键位/帮助/视觉打磨**：新增 vim 式 `j`/`k`→Down/Up 导航(panels+overlays, 编辑器内仍为文本)；聚焦面板黄框高亮、错误红字进 status(既有切片已具备)；help/状态栏更新。白盒 `TestVimNavTranslatesToArrows`/`TestVimNavLiteralInEditor` + 真实 pty 验证。· 预估：1d · 注：g/G 跳顶底、主题/截图后续(T-4b/c+)
- [x] **T-4b 测试**：逻辑层白盒已覆盖(连接/schema/fetch/fillResults/classifyErr/键路由/历史/收藏)；新增 `TestEndToEndSelectTable`——SimulationScreen 驱动真实事件循环：自动连接→选 schema 树首表→异步 runQuery(goroutine+QueueUpdateDraw)→断言结果表填充(表头+3行)；用 `onResult` 测试钩子同步、停 app 后安全读取。-race 连跑稳定。· 预估：1d
- [x] **T-4c 文档**：README 增 `## Terminal UI` 章节（`s9l tui`、面板、完整键位表）+ 特性条目；TESTING.md 增 TUI 测试策略（白盒/SimulationScreen/手动验证清单）· 预估：0.5d · 注：截图/录屏后续

**Phase T 验收**：`s9l tui` 提供连接/树/结果/编辑器/历史/收藏的键盘驱动全屏体验；核心层零改动；CI 绿；TUI 逻辑有单测 + 冒烟，手动验证清单通过。

---

## Phase 3 — TUI 强化 + SQL Server（目标 v0.6）

预估合计：~2.5–3.5 人周。TUI 五项延续 Phase T 原则：**只改 `internal/tui/`，不动 driver/config/secret/history 核心**；SQL Server 走既有「新增数据库只加一个 driver 包」的扩展模式（同 P2-1 MySQL）。逻辑与渲染解耦，白盒 + SimulationScreen 冒烟 + 手动清单。设计详见 [TUI.md](./TUI.md) 「TUI 强化」节。

### TUI 强化（lazygit 风格打磨）

- [x] **T3-1 主题与 lazygit 式视觉/布局**
  - 产出：`internal/tui/theme.go`——`Theme`(focus/border/title/accent/dim/error/selection)、`newTheme()` 尊重 `NO_COLOR`(全角色塌缩为终端默认、`tag/reset` 返回空)、`useRoundedBorders()` 全局圆角(`tview.Borders` 角 + focus 变体单线由颜色标记)；面板标题带序号 `[1] Connections`…`[4] SQL (F5 run)` + 标题色；聚焦面板绿边框/非聚焦灰(`theme.border`)；选中行高亮(NO_COLOR 时回退 tview 默认)；底部拆为两行——状态行(动态消息/错误经 `theme` 着色) + 静态 lazygit 式键位栏(`keyBar()`)；`editorHeight` 常量化(T3-3 用)
  - DoD：聚焦面板边框高亮、标题带序号；底部键位栏列出上下文键；`NO_COLOR` 下不崩、不输出色标 ✅；白盒 `theme_test.go`(border/NO_COLOR/tag/focusPanel 边框色/keyBar) + 真实 pty 核对(序号标题/键位栏/圆角 ╭╰ 渲染) ✅；核心层零改动
  - 依赖：Phase T · 预估：2d
- [x] **T3-2 Connections 仅名称 + 数据库图标**
  - 产出：`internal/tui/connlist.go`——`connIcon(driver)`（默认 ASCII 标签 `[pg]/[my]/[sq]/[ms]`，**始终渲染对齐**；`S9L_TUI_ICONS=nerd` 用 Nerd Font devicon 字形(`` 等 + 通用 `` 回退)；`=off/none/0` 关闭）；`connDisplayName`(有 name 用 name 否则 id)；`iconMode()`；List 改 `ShowSecondaryText(false)`、主文本 `<icon> <name>`，去掉冗长的 `driver user@host/db` 副行
  - DoD：列表每行 `图标 + 名称`（白盒 `TestConnListShowsIconAndName` 经 `GetItemText` 断言 `[pg] Dev Postgres`）✅；ascii/off/nerd 三模式 + 未知驱动回退 白盒 ✅；默认 ASCII 不依赖字体、对齐稳定；真实 pty 4 面板正常
  - 依赖：T3-1 · 预估：0.5d
- [x] **T3-3 SQL 编辑器面积翻倍**
  - 产出：`editorHeight` 常量 6 → 12（约一倍，含 tview 2 行边框）；Results/SQL 纵向比例随之调整（results 取剩余）
  - DoD：SQL 面板可见行数约翻倍；80x24 下 4 面板不挤爆（真实 pty 确认全部渲染）✅
  - 依赖：T3-1 · 预估：0.25d
- [x] **T3-4 图形化「新增连接」表单**
  - 产出：`internal/tui/connform.go`——Connections 面板 `n` 打开 `tview.Form`：id/name/driver(下拉 sqlite|postgres|mysql|sqlserver)/host/port/user/database/ssl(勾选)/password(掩码)/password-ref；`submitConnForm` 读表单→`saveConnection`：校验(id/driver 必填、sqlite 需 database、port 数字)→`cfg.Add`(唯一 id)→有密码则 `store.Set`(写 keychain，配置仅存 `KeychainRef`)→`cfg.Save`→`populateConnections` 刷新；写失败回滚 Add/secret；`Esc`/Cancel 取消；错误进状态栏。onKey 增 connFormOpen 分支(置于 vim-nav 前，表单输入字面透传)；keybar/help 增 `n`，空列表提示改回 "press n to add"
  - DoD：`saveConnection` 校验三分支 + 持久化(`config.Load` 往返断言) + 重复 id 报错 + 密码进 keychain(go-keyring `MockInit`，仅存 ref、`secret.Resolve` 回解) + 列表刷新 白盒(`connform_test.go`)✅；真实 pty `n`→表单(New connection/Driver/Password)→Esc 退出 exit 0 ✅；核心零改动
  - 依赖：T3-1 · 预估：1.5d · 注：复用 `config`/`secret`；编辑/删除连接见 Backlog B-6
- [x] **T3-5 结果过滤器**
  - 产出：App 保存上次结果集(`lastCols`/`lastData`)，结果经 `setResults` 统一存+渲染并清空过滤；`filterRows(data, term)` 纯函数(子串、大小写不敏感、跨列、NULL→"NULL" 参与匹配、空 term 返回全量)；`/` 打开过滤输入浮层(`tview.InputField`，`SetChangedFunc` 实时 `applyFilter`)，Enter 保留、Esc 清空并关闭(`hideFilter`)；状态栏 `filtered M/N`/清空时 `%d rows`；onKey 在 filterOpen 时优先处理(置于 vim j/k 之前，避免把输入 j/k 转成方向键)；keybar/help 增 `/`
  - DoD：`filterRows` 大小写/跨列/NULL/空 term 白盒 + `applyFilter` 行数断言(header+匹配) + 无结果时不开浮层 `TestShowFilterNoResults` ✅；真实 pty keybar/help 显示 `/ filter` ✅
  - 依赖：T-1c（结果表格）· 预估：1d · 注：客户端内存过滤；服务端 WHERE 注入留后续

### 新数据库：SQL Server

- [x] **P3-DB1 SQL Server 适配器**
  - 产出：`internal/driver/sqlserver/sqlserver.go`，用 `github.com/microsoft/go-mssqldb`（纯 Go，免 CGO，registered as "sqlserver"）；`[]byte`→string 归一化；Metadata（`\l`=`sys.databases`、`\dt`=当前库 `INFORMATION_SCHEMA.TABLES` BASE TABLE、`\d`=`INFORMATION_SCHEMA.COLUMNS`，`@p1` 占位）；config 加 `sqlserverDSN`（`sqlserver://user:pass@host:port?database=db&encrypt=disable|true`，凭据 url 转义）+ DSN 分支 + `cmd/s9l/main.go` 注册
  - DoD：`RunConformance` + Metadata 对真实 SQL Server（testcontainers `mcr.microsoft.com/mssql/server:2022-latest`）由 **CI integration job** 验证全 PASS（本地沙箱无 Docker，同 PG/MySQL 既有方式）；config `TestDSN` 增 sqlserver 用例 ✅；`-short`/lint/build 全绿 ✅；**核心层零改动**（仅新增 driver 包 + config DSN 分支 + 注册）
  - 依赖：P0-3（Driver 接口）· 预估：1.5d
  - 注：一致性套件 SQL 本就可移植（`DROP TABLE IF EXISTS`/多行 VALUES/`ORDER BY id`(INTEGER) 均兼容 T-SQL；TEXT 列只 SELECT 不 ORDER BY）。方言差异（无 `LIMIT`→`TOP`/`OFFSET…FETCH`、`@p1`、`[brackets]`）下沉 driver。镜像 >1GB、启动慢，IT 用 `testing.Short()` 隔离。

**Phase 3 验收**：TUI 具备 lazygit 式配色/圆角/序号面板/底部键位栏；Connections 显示图标+名称；SQL 编辑器约翻倍；可在界面内新增连接并持久化（密码进 keychain）；结果可即时过滤；新增 SQL Server 仅动 driver 层、conformance 全 PASS。核心层零改动；CI 绿；逻辑白盒 + SimulationScreen 冒烟 + 手动清单通过。

---

## Phase 5 — TUI 可读性与操作性微调（目标 v0.8，按用户反馈，优先）

预估合计：~1 人周。延续原则：**只改 `internal/tui/`，核心零改动**；白盒 + SimulationScreen 冒烟 + 手动清单。背景：T4-1 把全局文字/背景设为终端默认后，tview 默认选中样式塌缩成「默认 on 默认」不可读；输入框/模态沿用 tview 默认亮蓝 `ContrastBackgroundColor` 过于刺眼。

- [x] **T5-1 配色可读性：选中行 / 输入框 / 模态背景**
  - 产出：`theme.go` `Theme` 增 `Selection`(浅灰)/`SelectionText`(黑)/`Field`(浅灰)/`FieldText`(黑)/`Contrast`(暗 slate 回退)；`selectionStyle()`(浅底深字，NO_COLOR→反显)；`applyStyles` 设 `ContrastBackgroundColor=Contrast`+`InverseTextColor`。Results `SetSelectedStyle(selectionStyle())`；`treeNode()` 给每个可选节点设 `SetSelectedTextStyle(selectionStyle())`。交互控件显式配色：connform `Form.SetField/Button/Label*`、`/` 过滤与 `Ctrl-E` 导出 `InputField.SetFieldBackground/TextColor`、删除 `Modal.SetBackground/TextColor` 用 `Field`/`FieldText`。修复 T4-1 后选中样式塌缩为「默认 on 默认」不可读的问题。
  - DoD：白盒 `TestSelectionStyleReadable`(fg≠bg、= Selection/SelectionText)、`TestSelectionStyleNoColorReverses` ✅；真实 pty 无崩溃；核心零改动。
  - 依赖：T4-1 · 预估：0.75d
- [x] **T5-2 Connections 去树线 + 展开指示符**
  - 产出：`connTree`/`schema` `SetGraphics(false)`(去 tview 树连线)；`setConnNodeLabel`——连接节点有数据库子节点时按展开态加 `▾ `/`▸ `(否则 2 空格对齐位)，`onConnSelect` 连接节点已连则切换展开并更新三角；数据库/表为叶子无三角。
  - DoD：白盒 `TestConnNodeExpandIndicator`(无子→无三角、展开→▾、折叠→▸) ✅；真实 SQLite pty——树连接符 ├/└ 计数 0、退出 exit 0 ✅；核心零改动。
  - 依赖：T4-2 · 预估：0.5d
- [x] **T5-3 Connections 上下移动选择（确认）**
  - 产出：`connTree` 为 `tview.TreeView`，方向键原生 + 既有 vim `j/k`→Down/Up 在 Connections 生效；`populateConnections` 设首节点为 current；连接展开数据库后可在连接/库间上下移动，当前行经 T5-1 选中样式高亮。
  - DoD：SQLite pty 内 `j` 移动不崩、当前行高亮可见(T5-1)；核心零改动。· 依赖：T5-1、T4-2 · 预估：0.25d
- [x] **T5-4 使用手册 / README 同步**
  - 产出：`docs/MANUAL.md` §11 Connections 说明加「无树线 + 开合三角 `▾`/`▸` + 上下选择」；本轮键位无新增（沿用 Enter/j/k）。
  - DoD：文档与实现一致 ✅。· 依赖：T5-1/T5-2/T5-3 · 预估：0.25d

**Phase 5 验收**：选中行/输入框/模态清晰可读且不刺眼；Connections 无树线、有开合三角、可上下选择；文档同步。核心零改动；CI 绿。

### Phase 5.1 — 二轮视觉微调（按用户截图反馈，优先）

背景：T5 落地后用户实测仍有可读性问题——选中行背景偏深、看不清内容；数据库/表节点的 Accent 着色 + 缩进让 Connections/Schema 看起来仍是「彩色树形」；输入框背景偏深。延续原则：**只改 `internal/tui/`，核心零改动**；白盒 + 真实 pty 冒烟。

- [x] **T5.1-1 选中行 / 输入框背景更浅、文字清晰**（反馈 1·4·5）
  - 产出：`theme.go` 把 `Selection`(0xc0c0c0→更浅，如 0xe4e4e4)、`Field`(0xcfcfcf→更浅，如 0xeaeaea) 调浅；保持 `SelectionText`/`FieldText` 为黑，确保浅底深字高对比（兼顾真彩降采样终端）。
  - DoD：白盒 `TestSelectionStyleReadable`/`TestSelectionStyleNoColorReverses` 仍绿（断言 fg≠bg）；真实 pty 选中行/输入框文字清晰可读；核心零改动。
  - 依赖：T5-1 · 预估：0.25d
- [x] **T5.1-2 Connections/Schema 去「橙色树形」**（反馈 2）
  - 产出：① 数据库子节点去掉 `SetColor(Accent)`（line 361），用终端默认色，消除「橙色」观感；② Schema 面板 `SetTopLevel(1)`，隐藏着色的库根节点、表列表扁平显示（无树缩进）；③ 展开/折叠三角 `▾`/`▸` 仅保留在「上层」连接节点（已有 `setConnNodeLabel`）；叶子（库/表）无三角。
  - DoD：白盒 `TestConnNodeExpandIndicator` 仍绿；新增/调整断言：db 节点无 Accent 色、schema 顶层为表（非库根）；真实 SQLite pty 树连接符计数 0、退出 exit 0；核心零改动。
  - 依赖：T5-2 · 预估：0.5d
- [x] **T5.1-3 Connections 上下移动选择 + 高亮可见（确认/修复）**（反馈 3）
  - 产出：确认方向键 + vim `j/k`→Down/Up 在 Connections 生效；`populateConnections` 首节点为 current；高亮经 T5.1-1 浅底深字清晰可见。若发现移动无响应（如焦点/SetTopLevel 边界），定位并修复。
  - DoD：SQLite pty 内 `j`/`↓` 移动不崩、当前行高亮可见；核心零改动。· 依赖：T5.1-1 · 预估：0.25d
- [x] **T5.1-4 文档同步**
  - 产出：若 Connections/Schema 观感（去色/扁平）有用户可见变化，`docs/MANUAL.md` §11 相应更新；键位无新增。
  - DoD：文档与实现一致。· 依赖：T5.1-1/2/3 · 预估：0.1d

**Phase 5.1 验收**：选中行/输入框浅底深字清晰；Connections/Schema 无彩色树形观感、开合三角仅在上层节点、可上下选择并清晰高亮；文档同步。核心零改动；CI 绿。

### Phase 5.2 — 三轮微调 + 连接测试（按用户实测截图，优先）

背景：5.1 落地后实测，选中行/输入框背景仍偏深，需再调浅；New connection 表单缺少「保存前测试连接」能力，易存错配置。原则不变：**TUI 只改 `internal/tui/`**；`dial` 为 CLI/TUI 共享辅助层（非 driver/config/secret/history 核心），允许向后兼容地新增函数。

- [x] **T5.2-1 选中行 / 输入框配色（多轮实测后收敛为暗色系）**（反馈：颜色还要再浅些 → 后续统一暗色）
  - 产出：经多轮实测最终统一为**暗色系**：`Selection`=`Field`=暗灰 0x2a2a2a、`SelectionText`=`FieldText`=白；Connections/Schema/Results 的选中高亮与表单输入框**同色**（暗底白字），与暗色卡片（`Surface` 0x1e1e1e）形成层次而不刺眼。表单输入框宽度设 0（占满卡片、消除右侧暗带）。
  - DoD：`TestSelectionStyleReadable`/`TestSelectionStyleNoColorReverses` 仍绿（fg≠bg）；真实 pty 选中行/输入框暗底白字清晰、与表单同色；核心零改动。
  - 依赖：T5.1-1 · 预估：0.1d
- [x] **T5.2-2 `dial.OpenWithPassword`（用未保存的明文密码试连）**
  - 产出：`internal/dial/dial.go` 抽出 `openResolved(ctx, cc, store, password)` 公共体；`Open` 维持原行为（经 `secret.Resolve` 解析 ref）；新增 `OpenWithPassword(ctx, cc, store, password)`——password 非空时直接用之（表单「保存前测试」场景密码尚未入库），为空时回退 `Open`。
  - DoD：白盒 `TestOpenWithPasswordSQLite`（sqlite 文件库试连成功 + close）、`TestOpenWithPasswordEmptyFallsBack`（空密码走 ref 解析路径）；既有 `dial` 测试不破；CLI 行为不变。
  - 依赖：无 · 预估：0.5d
- [x] **T5.2-3 New connection 表单「Test」按钮**（反馈：需要 test 按钮检查输入是否正确）
  - 产出：`connform.go` 抽出 `formConfig(form) (cc, password, err)`（从 `submitConnForm` 复用读取逻辑，不持久化）；表单加 `Test` 按钮（位于 Save/Cancel 间）：先 `validateConn`，再在 goroutine 内带 5s 超时调用 `dial.OpenWithPassword`，结果经 `QueueUpdateDraw` 回推——成功把表单标题更新为 `✓ connection OK`、失败为 `✗ <error>`（标题在模态内始终可见，状态栏被遮挡）；测试期间标题显示 `testing…`，按钮不阻塞 UI。
  - DoD：白盒 `TestFormConfigReadsFields`（表单→cc/password 映射，含 port 非数字报错）、`TestTestConnFormSuccess`/`TestTestConnFormError`（用 fake/ sqlite 驱动 Test 路径，断言标题文案）；真实 pty `n`→填 sqlite 路径→`Test`→`✓`；核心零改动。
  - 依赖：T5.2-2 · 预估：0.75d
- [x] **T5.2-5 New connection 表单暗色卡片 + 增强反差**（反馈：透明背景透出后方内容、不清晰）
  - 产出：表单背景透明导致后方结果/聊天透出、文字不清晰。`theme.go` 加 `Surface`（不透明暗色卡片 0x121212；NO_COLOR→默认）；`Field` 改为**比卡片略浅的暗灰 0x2a2a2a**、`FieldText` 改为白字（暗底白字，输入框与卡片有层次但不刺眼；该色为表单/过滤/导出/删除模态共享，统一暗色风）；`connform.go` `form.SetBackgroundColor(a.theme.Surface)`——白标签/标题在暗卡片上醒目、后方不再透出；保留聚焦绿框。
  - DoD：白盒 `TestSurfaceOpaque`（Surface≠默认、NO_COLOR→默认）；真实 pty 表单为暗色卡片、输入文字清晰、不透出后方；核心零改动。
  - 依赖：T5.2-1 · 预估：0.25d
- [x] **T5.2-6 TUI 全面板不透明背景（消除终端透明透出）**（反馈：connection 和表同样处理、别透出后方）
  - 产出：用户终端开启透明，桌面/聊天透过各面板（Connections 等）显得杂乱。`theme.go` 增 `Background`(0x14161a 暗底) / `PrimaryText`(0xd0d0d0 亮字)；`applyStyles` 由「`PrimitiveBackgroundColor=ColorDefault` 跟随终端」改为**不透明暗底 + 亮字**（颜色层次：背景 0x14161a < 卡片 0x1e1e1e < 选择/输入框 0x2a2a2a）。**NO_COLOR 仍回退 ColorDefault（透明/混入终端）**，保留 lazygit 式行为给需要者。取代 Phase 4 T4-1「背景跟随终端」的默认（仅彩色模式下）。
  - DoD：白盒 `TestApplyStylesOpaqueWithColor`（彩色时 bg/fg≠默认）、`TestApplyStylesTransparentUnderNoColor`（NO_COLOR 回退默认）✅；真实 pty 各面板不再透出后方；核心零改动。
  - 依赖：T5.2-5 · 预估：0.25d
- [x] **T5.2-4 文档同步**
  - 产出：`docs/MANUAL.md` 新增连接表单说明加「Test 按钮：保存前验证连接」；`docs/TUI.md` 强化节补一行。
  - DoD：文档与实现一致。· 依赖：T5.2-1/2/3/5 · 预估：0.1d

**Phase 5.2 验收**：选中行/输入框近白浅底、文字清晰；New connection 表单可在保存前一键测试连接并清晰反馈成功/失败；文档同步。TUI 核心零改动、`dial` 仅向后兼容新增；CI 绿。

---

## Phase 5.3 — 上下文相关 `/` 检索（按聚焦面板切换检索对象，优先）

背景：`/` 已能在 **Schema 检索表**、在 **Results 过滤结果行**（均已实现）。用户希望 `/` 在 **Connections** 也生效——**检索当前连接下的数据库**；总体规则：**聚焦哪个面板，`/` 的检索对象就是该面板的内容**。原则不变：**只改 `internal/tui/`，核心零改动**；纯函数 + 白盒 + SimulationScreen/pty 冒烟。

现状（`showFilter`/`hideFilter`）已按 `focusIdx` 二分（Schema=1 → 表；其余 → 结果行）。本阶段把分派改为**三态**并补齐 Connections（数据库）这一路。

- [x] **T5.3-1 Connections `/` 检索数据库（核心新功能）**
  - 产出：① App 保留当前连接已加载的数据库**全量列表** `connDatabases []string` + 其所属 `connDBNode *tview.TreeNode`（在 `loadConnDatabases` 填充时存入）；② 复用 `filterTables`（子串、大小写不敏感，对名字通用）做 `filterDatabases`；③ `applyConnFilter(term)`：按 term 重建该连接节点的数据库子节点（保留连接节点本身与展开态、当前选中尽量保持），状态栏 `databases M/N`；④ 仅当聚焦 Connections 且**当前连接已连且为多库引擎（有数据库子节点）**时启用，否则 `SetStatus("no databases to filter")`。
  - DoD：纯函数 `TestFilterDatabases`；白盒 `TestApplyConnFilter`（fake browser conn 连接→加载多库→`/ria`→仅匹配库为子节点、计数正确、清空恢复全量）；真实 pty 在 Connections `/` 可缩小库列表；核心零改动。
  - 依赖：B-7（databaseBrowser/loadConnDatabases）· 预估：1d
- [x] **T5.3-2 `showFilter`/`hideFilter` 三态分派重构**
  - 产出：把布尔 `filterSchema` 改为 `filterTarget`（`filterConn`/`filterSchema`/`filterResults` 三态，按 `focusIdx` 0/1/其余判定）；`showFilter` 据此选 title（`Filter databases`/`Filter tables`/`Filter results`）、initial、onChange；`hideFilter(clear)` 据此清空对应过滤。保持现有 Schema/Results 行为不变。
  - DoD：白盒 `TestShowFilterTargetByPanel`（focusIdx 0/1/2 → 对应 target 与 onChange）；既有 `TestShowFilterNoResults` 适配；核心零改动。
  - 依赖：T5.3-1 · 预估：0.5d
- [x] **T5.3-3 确认 Schema/Results 既有 `/` 仍工作**
  - 产出：回归确认 Schema 检索表、Results 过滤行在三态重构后不退化（用户期望"schema 也能检索表"——本就支持，确保不破）。
  - DoD：既有 `filter_test.go` / schema 过滤测试全绿；pty 抽查 Schema `/` 与 Results `/`。· 依赖：T5.3-2 · 预估：0.1d
- [x] **T5.3-4 文档同步**
  - 产出：`docs/MANUAL.md` §11/键位表写明「`/` 按聚焦面板检索：Connections→数据库 / Schema→表 / Results→结果行」；`docs/TUI.md` 强化节补一行。
  - DoD：文档与实现一致。· 依赖：T5.3-1/2/3 · 预估：0.1d

**Phase 5.3 验收**：`/` 在 Connections 检索数据库、Schema 检索表、Results 过滤行，且**随聚焦面板自动切换检索对象**；状态栏显示 `M/N` 计数；`Esc` 清空、`Enter` 保留；文档同步。核心零改动；CI 绿；逻辑白盒 + pty 冒烟。

---

## Phase 6 — 发布 v0.10 + Results 面板增强（目标 v0.11）

按用户最新反馈：先把当前改动发版、清掉未处理 PR；随后增强 Results 面板——列过滤、`/` 全字段模糊检索、单元格左右移动与就地编辑（写回）。TUI 增强延续原则：**只改 `internal/tui/`（写回 UPDATE 复用 `driver.Conn.Exec` + import 的方言辅助），核心 driver 接口零改动**；纯函数 + 白盒 + pty 冒烟。

### 6.0 发布 v0.10 + 清理 open PR（先做）

- [x] **T6.0-1 处理 open PR #62（B-9 import）**
  - 现状：B-9 代码已在 develop 且标记完成；PR #62（`feature/b9-import`）疑似冗余。
  - 产出：核对 #62 内容是否已并入 develop——已并入→关闭 PR 并注明；未并入→评审后合并。
  - DoD：#62 有明确处置（关闭或合并）。· 预估：0.25d
- [x] **T6.0-2 推进 Release v0.10.0（PR #69）并打 tag**
  - 现状：PR #69「Release v0.10.0」(develop→main) open；develop 已含 Phase 5.1–5.3 TUI 改进（`5bbd2af..718bd13`）。
  - 产出：确认 #69 含本轮改动（必要时 rebase/更新发布说明）→ 合并到 main → 打 tag `v0.10.0` 触发 `release.yml`（goreleaser 多平台二进制 + Homebrew cask）。
  - DoD：main 含本轮改动；`v0.10.0` release 产物生成；open PR 清空。· 预估：0.5d
  - 注：合并到 main / 打 tag 为对外动作，执行前与用户确认版本号与范围。

### 6.1 Results 列过滤（按字段过滤）

- [x] **T6.1-1 `filterRowsByColumn` 纯函数 + 列过滤 UI**
  - 目标：在 Results 按**某一列**过滤（区别于 `/` 全字段）。
  - 产出：① 纯函数 `filterRowsByColumn(cols, rows, colIdx, term)`（该列大小写不敏感子串/模糊）；② 选定列（复用 6.3 的 cell 左右选择确定列，或弹列选择）+ 输入框 + 实时重渲染；③ 状态栏 `col <name>: M/N`；键位如 `f`（`/` 保留全字段）。与全局过滤初版**互斥**。
  - DoD：纯函数测试（按列匹配/空 term/越界保护）；白盒（选列→过滤→计数→清空恢复）；核心零改动。· 预估：0.75d

### 6.2 Results `/` 全字段模糊检索

- [x] **T6.2-1 `/` 升级为全字段模糊（子序列）匹配**
  - 现状：`filterRows` 已是**跨所有列、大小写不敏感子串**匹配（即"全字段"）。
  - 产出：新增 `fuzzyMatch(text, term)`（子序列、大小写不敏感），`filterRows` 改用之（仍跨所有列）；空 term 全保留。评估子序列过松的风险，必要时保留子串模式可切换。
  - DoD：`fuzzyMatch` 测试（子序列命中/顺序敏感/大小写）；`filterRows` 跨列模糊测试；文档（`/` = 全字段模糊）同步。· 预估：0.5d

### 6.3 Results 单元格左右移动 + 选中编辑（写回 UPDATE）

- [x] **T6.3-1 单元格导航（左右移动选 cell）**
  - 产出：Results `SetSelectable(true, true)`；`←/→` 或 `h/l` 在列间移动选中 cell；状态栏显示 `行N · 列<name>`。只读、低风险。
  - DoD：白盒（cell 选择状态）；pty 抽查左右移动；核心零改动。· 预估：0.5d
- [x] **T6.3-2 查看完整单元格值**
  - 产出：选中 cell 按键（如 `v`）弹浮层显示完整值（长文本/NULL/二进制友好）。只读。
  - DoD：白盒（取值格式化复用 `render.Cell`）；核心零改动。· 预估：0.25d
- [x] **T6.3-3 单元格就地编辑写回（UPDATE）**
  - 目标：选中 cell → 编辑值 → 生成 `UPDATE <表> SET <列>=? WHERE <主键>=?` 并 Exec → 刷新。
  - 前置约束：**仅当结果来自单表预览**（已知表名，`runTableQuery` 路径）且该表有**主键/唯一键**（经 `driver.Metadata` 检测）时允许；否则只读并提示「不可编辑（非单表/无主键）」。
  - 产出：① 记录当前结果的来源表与列；② PK 检测（Metadata，新增能力或复用 Columns + 约束查询）；③ 编辑输入框预填原值（支持置 NULL）；④ `buildUpdate`（方言 placeholder/quoteIdentifier 复用 import）；⑤ 执行前确认弹窗（防误改），Exec 后刷新该行/重查。
  - 风险：**数据变更**、无事务 API（单条 Exec 自动提交，失败仅提示已/未改）、PK 检测各库差异、类型/编码、并发改动。**开工前出小设计评审**。
  - DoD：纯函数 `buildUpdate`（各方言 SET/WHERE/placeholder）测试；白盒（fake conn：编辑→生成正确 UPDATE→Exec 调用参数）；E2E SQLite（预览表→改一格→count/值校验）；无主键/非单表时禁用并提示；docs 同步。· 预估：2–3d

**Phase 6 验收**：v0.10.0 已发布、open PR 清空；Results 支持列过滤、`/` 全字段模糊检索、单元格左右移动与（单表+主键时）就地编辑写回；非单表/无主键安全降级为只读；核心 driver 接口零改动；CI 绿；逻辑白盒 + E2E + pty 冒烟。

---

## Phase 4 — TUI 交互重构（目标 v0.7）

预估合计：~2–2.5 人周。延续原则：**只改 `internal/tui/`，复用 driver/config/secret/history，核心零改动**；逻辑与渲染解耦，白盒 + SimulationScreen 冒烟 + 手动清单。
> 模型调整：把「数据库」层从 Schema 面板（B-7 的库→表树）上移到 **Connections 面板**（连接→数据库可展开），**Schema 面板只显示所选数据库的表**并支持检索。这取代/调整 B-7 在 Schema 内的库层级，更贴合用户期望的 lazygit 式层次。

- [x] **T4-1 背景色与 lazygit 一致（终端默认背景）**
  - 产出：`theme.go` `applyStyles()`——设 `tview.Styles.PrimitiveBackgroundColor/PrimaryTextColor = ColorDefault`(面板/文字透传终端，像 lazygit)，`BorderColor/TitleColor/GraphicsColor/Secondary/Tertiary` 用主题色；`ContrastBackgroundColor`/`MoreContrast`/`Inverse` 保留 tview 默认以保证输入框/按钮/删除模态在透明背景上可见。`New` 中 buildLayout 前调用一次。
  - DoD：白盒 `TestApplyStylesUsesTerminalBackground`(PrimitiveBackground/PrimaryText==ColorDefault) ✅；真实 pty——4 面板正常渲染、输出**无强制黑底 SGR `40m`**(终端默认背景透传，对比改动前满屏 `40m`) ✅；核心零改动。
  - 依赖：T3-1 · 预估：0.5d
- [x] **T4-2 Connections 面板：连接 → 数据库（可展开）**
  - 产出：Connections 由 `tview.List` 改为 `tview.TreeView`(`SetTopLevel(1)` 隐藏合成根)；`connNodeRef{cc}`/`dbNodeRef{connID,db}` 节点引用；`onConnSelect`：连接节点→`connect`+`loadConnDatabases`(多库引擎列 `Metadata.Databases` 为子节点、Schema 占位 "select a database"；单库引擎直接 `loadSchema`)+展开；数据库节点→设 `currentDB`+`loadSchema`+状态。`connect` 不再直接 loadSchema(由调用方决定，auto-connect 经 `findConnNode`+`onConnSelect` 复用同一路径)；`loadSchema` 改为列「当前库」表(`databaseBrowser.TablesIn(currentDB)` 或 `Metadata.Tables`)；`selectedConn` 由列表索引改为按当前节点引用(库节点回溯到所属连接)，B-6 的 `e`/`d` 适配；图标(T3-2)在连接节点。移除 B-7 在 Schema 内的 db 节点/`loadTablesInto`(库层已上移)。
  - DoD：白盒 `TestLoadConnDatabases`(连接→库子节点排序、currentDB 待选)、`TestLoadSchemaForCurrentDB`(选库→该库表 `tableRef{db}`)、`TestSelectedConn`(树节点映射)、既有 `TestConnectionsPopulated`/`TestConnListShowsIconAndName`/E2E 适配 ✅；真实 MySQL `neohub-dev` pty——auto-connect 后 Connections 展开出 information_schema/mysql 等库、Schema 显示 "select a database" ✅；核心零改动。
  - 依赖：T4-1、B-6、B-7 · 预估：1d
- [x] **T4-3 Schema 面板：当前数据库的表 + 检索**
  - 产出：`loadSchema` 拆为「取表(`databaseBrowser.TablesIn(currentDB)` 否则 `Metadata.Tables`)→存 `schemaTables`→`renderSchema`」；`filterTables(names, term)` 纯函数(子串、大小写不敏感)；`renderSchema` 按 `schemaFilter` 重建表节点(`tableRef{db:currentDB}`)；`applySchemaFilter`(状态 `tables M/N`)；`showFilter` 改为按聚焦面板分派——Schema(focusIdx==1)过滤表名、否则过滤结果行；`hideFilter` 对应清空。help/keybar `/` 文案更新。
  - DoD：白盒 `TestFilterTables`(空/大小写/无匹配)、`TestApplySchemaFilterRerenders`(过滤后节点数+清空恢复)、`TestLoadSchemaForCurrentDB` ✅；真实 MySQL pty——选库→Schema 列表→`/` 输入→"Filter tables"/"tables 5/18" ✅；核心零改动。
  - 依赖：T4-2 · 预估：1d
- [x] **T4-4 使用手册 / README 更新（随 T4-1~3 落地）**
  - 产出：`docs/MANUAL.md` §11 全屏 TUI 重写——新布局图(Connections 连接→库树、Schema 当前库表+`/tbl` 检索、两行状态/键位栏)、按键表(`n/e/d`、`/` 分派、`Enter` 连接展开库/选库/预览)、§11.3 界面内管理连接、终端背景说明；§环境变量增 `NO_COLOR`/`S9L_TUI_ICONS`；支持库加 SQL Server。README `## Terminal UI` 面板/键位已随 T4-1/T4-2 同步。
  - DoD：手册/README 与 T4-1~3 新交互一致(每个新功能都有用法)，随实现 PR 同步提交 ✅。
  - 依赖：T4-1/T4-2/T4-3 · 预估：0.5d

**Phase 4 验收**：背景与终端/lazygit 一致；Connections 可展开到数据库、选库刷新 Schema；Schema 显示当前库表且可检索；新功能用法已写入使用手册。核心层零改动；CI 绿；逻辑白盒 + SimulationScreen 冒烟 + 手动清单通过。

---

## Backlog（未排期，按需）

> 已细化「需要修改的内容」便于将来直接领取。标注 **架构影响**：✅=核心零改动（新增包/扩展配置即可）；⚠️=需小幅触碰连接编排或 Metadata 可选接口；🔴=需改核心抽象，开工前先做设计 spike。

- [x] **B-1 SSH Tunnel（连接前建隧道）** · 预估 2–3d
  - 产出：① `config` 增 `SSHHost/SSHPort/SSHUser/SSHKey/SSHKeyPassRef/SSHKnownHosts/SSHInsecureHostKey` + `HasSSH()`/`DialHostPort()`(驱动默认端口)；② `internal/tunnel`(`golang.org/x/crypto/ssh`，纯 Go)——拨号堡垒机+本地 listener+`direct-tcpip` 转发，私钥/ssh-agent 认证，**默认 known_hosts 校验主机密钥**(`InsecureHostKey` 显式跳过)；③ `internal/dial`——`Open(ctx, cc, store)` 复用于 CLI/TUI：解析密码→有 SSH 则起隧道并把 host/port 改写为本地端→`DSN`→`driver.Open`，返回 conn+合并 closer(关连接+拆隧道)；④ cmd 用 `openTarget`(替代 resolveTarget) 经 dial、TUI `connect` 经 dial(`connClose` 保存以关隧道)；`conn add` 加 `--ssh-*` flags。**driver 接口零改动**，仅"打开连接"层插入。
  - DoD：白盒——`internal/tunnel` 用**进程内 SSH 服务器**端到端验证转发(`TestForwardsThroughSSH`：known_hosts 校验通过、回声穿隧道；`TestInsecureHostKeySkipsVerification`)，无需 Docker；`dial` 无 SSH 走 sqlite + 密码 ref 错误；config `TestSSHHelpers`(HasSSH/DialHostPort 默认)；`-short`/lint/build 全绿 ✅；docs(README/MANUAL §4) 同步。
  - 注：私钥口令经 `ssh_key_pass_ref`(SecretStore)；密码认证/跳板多级留后续。真实"DB over SSH"端到端属手动验证，隧道转发逻辑已由进程内 SSH 测试确定性覆盖。
- [x] **B-2 TLS 配置（CA/客户端证书、sslmode 细化）** · ✅ · 预估 1.5–2d
  - 产出：`internal/config/connection.go` 增 `SSLMode/TLSCA/TLSCert/TLSKey`（`ssl_mode/tls_ca/tls_cert/tls_key`）；`sslMode(whenOn)` 解析（SSLMode 优先，否则 SSL→whenOn/disable）；`validateTLS` 对不支持驱动清晰报错。postgres DSN 加 `sslmode/sslrootcert/sslcert/sslkey`；mysql `mysqlTLS` 映射 `tls`（内置模式，自定义证书报错改用裸 DSN）；sqlserver `encrypt`+`trustservercertificate`（require=加密不校验）+`certificate`(CA)。`conn add` 加 `--ssl-mode/--tls-ca/--tls-cert/--tls-key`。**`ssl: true` 行为不变**（pg=require、mysql=tls=true、sqlserver=encrypt 并校验）。
  - DoD：白盒 `TestDSNTLS`（pg sslmode+CA+客户端证书精确 DSN；mysql 各模式→tls/disable 省略；mysql 证书报错；sqlserver ssl:true=encrypt 校验、ssl_mode=require=trust+CA；sqlserver 客户端证书报错）+ 既有 `TestDSN` 向后兼容仍 PASS ✅；docs(MANUAL §4、README) 同步。核心层零改动（仅 config + cmd flag）。
  - 注：TLS 需真实证书/服务器做端到端，本环境无法 live；以 DSN 字符串断言固定安全相关映射（同既有 DSN 测试方式），供评审。mysql 自定义证书/客户端证书留裸 DSN。
- [x] **B-4 ClickHouse 驱动（含一致性套件方言化）** · 预估 1.5d
  - 产出：① `internal/driver/drivertest/conformance.go` 方言化——`Option`(`WithTypes`/`WithTableSuffix`/`SkipRowsAffected`)，默认值与原 SQL **完全一致**（SQLite/PG/MySQL/SQL Server 不变）；② `internal/driver/clickhouse/`（`ClickHouse/clickhouse-go/v2` stdlib，纯 Go；`[]byte`→string；Metadata `system.databases`/`system.tables`(currentDatabase)/`system.columns`，`?` 占位）；③ config `clickhouseDSN`(`clickhouse://user:pass@host:9000/db`，ssl→`secure`/require→`skip_verify`；证书文件报错)+ DSN 分支 + `cmd/s9l/main.go` 注册。**核心 driver 接口零改动**。
  - DoD：ClickHouse IT 用方言选项(`Int32`/`String`/`Nullable(String)`、`ENGINE=Memory`、SkipRowsAffected)跑 `RunConformance`+Metadata（testcontainers `clickhouse/clickhouse-server:24.3-alpine`，CI 验证）；既有 SQLite conformance 默认值不变仍 PASS ✅；config `TestDSN`(clickhouse + secure) ✅；`-short`/lint/build 全绿 ✅；docs 同步。
  - 注：ClickHouse 需 `ENGINE` 子句 + `Nullable(...)` + INSERT 不报 RowsAffected——故先把一致性套件做成方言无关，也让将来非标准 SQL 引擎更易接入。
- [x] **B-6 TUI 连接编辑/删除** · ✅ · 预估 1d
  - 产出：`internal/tui/connform.go`——`showConnForm(edit *ConnectionConfig)` 复用为「新增/编辑」(编辑预填字段、密码留空=保留原 ref、改 id 唯一性校验)；`e` 编辑选中、`d` 删除选中(`tview.Modal` 确认)；`editConnection`(remove+add 替换，失败回滚原值；新密码写 keychain)/`deleteConnection`(`cfg.Remove`+`Save`+`secret.Delete` best-effort)/`selectedConn`(列表索引↔cfg.Connections)；onKey 增 confirmOpen 分支 + `e`/`d`(仅 Connections 面板)；help 增 `n/e/d`。复用 config/secret，核心零改动。
  - DoD：白盒 `TestEditConnection`(改名+持久化+缺失/重名报错+回滚)、`TestEditConnectionUpdatesPassword`(keychain 更新)、`TestDeleteConnection`(移除+config.Load 校验+keychain 删除+重复删报错)、`TestSelectedConn`(索引映射) ✅；真实 pty `e`→Edit 表单、`d`→Delete 确认模态、help 列 n/e/d ✅。
- [x] **B-7 TUI 跨库浏览（库→表 多级树）** · 预估 1.5d
  - 产出：解决"未选默认库时看不到表"。**核心零改动**——用 Go 结构化类型：mysql driver 新增 `TablesIn(ctx, db)`(information_schema 服务端范围，按库参数化)；TUI 定义本地 `databaseBrowser` 接口结构化断言判定能力。有能力(mysql)→Schema 树「库→表」两级，展开库节点经 `loadTablesInto` 懒加载其表(`tableRef{db,name}`)；无能力(pg/sqlite/sqlserver)→维持单级当前库(`tableRef{name}`)。pg 同连接不能跨库且总有默认库故单级即可。
  - 附带修复：`previewQuery` 方言化——SQL Server 用 `SELECT TOP n`（T-SQL 无 `LIMIT`），其余 `LIMIT n`，修正此前 TUI 预览在 SQL Server 上必失败的隐患；`qualifyTable` 跨库时 `库.表` 限定。
  - DoD：白盒 `TestLoadSchemaMultiDB`(fake browser conn→库节点→展开懒加载→`tableRef{app,users}`)、`TestPreviewQuery`(TOP/LIMIT)、`TestQualifyTable`(限定/单段)；既有 sqlite 单级 `TestLoadSchemaShowsTables` 适配 `tableRef` ✅；mysql IT `TestMetadata` 增 `TablesIn(other)` 跨库列表(CI 真实容器) ✅；-short/lint/build 全绿 ✅。
  - 注：sqlserver 多级(三段名+schema)与 pg schema 级浏览留后续；本次聚焦 mysql(用户实际痛点) + 修复 SQL Server 预览方言。
- [x] **B-8 结果导出（CLI 已有 / TUI 新增）** · ✅ · 预估 0.75d
  - 产出：`internal/tui/export.go`——`Ctrl-E` 打开保存路径输入框（默认 `results.csv`），`Enter` 写出当前结果集、`Esc` 取消；`exportResults`(复用 `render.Write(f, fmt, lastCols, lastData)`)、`exportFormat`(按扩展名 .json/.tsv 否则 csv)；onKey 加 exportOpen 透传分支 + `Ctrl-E` 触发；keybar/help 更新。CLI 导出（`--format csv > f`）本就支持。
  - DoD：白盒 `TestExportFormat`(扩展名映射)、`TestExportResultsWritesFile`(CSV 头/行 + JSON 对象，含 NULL)、`TestShowExportNoResults`(无结果不开) ✅；核心零改动；docs 同步。
- [x] **B-9 数据导入（CSV/JSON 批量）** · ✅ · 预估 1.5–2d
  - 产出：`cmd/s9l/import.go` + run() 分派 `import`——`s9l <conn|dsn> import --table T --file f [--format csv|json] [--batch N] [--driver]`；`readCSV`(首行列名、其余字符串)/`readJSON`(对象数组、列=首对象键排序、缺失→nil)；`importRows` 分批多行 INSERT；`insertSQL`/`placeholder`(pg `$n`/sqlserver `@pn`/其余 `?`)/`quoteIdentifier`(mysql 反引号/sqlserver 方括号/其余双引号) 按方言；报告导入行数。
  - DoD：白盒 `TestPlaceholderAndQuote`/`TestInsertSQL`(pg 多行编号/sqlite)/`TestReadCSV`/`TestReadJSON`(缺失键→nil)/`TestImportFormat` + E2E `TestRunImportCSVIntoSQLite`(经 run() 导入并 count 校验) ✅；docs(README/help/MANUAL §3·§13) 同步。
  - 注：表需预先存在；`driver.Conn` 无事务 API，故按 batch 多行 INSERT(每批一次 Exec=一次自动提交)，中途出错报告已成功行数；NULL：JSON null→NULL、CSV 空=空串。
- [x] **B-10 历史统计 `s9l history stats`** · ✅ · 预估 0.75d
  - 产出：`internal/history/stats.go`——`Store.Stats(ctx, topN)` 聚合 query_history（总数/成功/失败/平均耗时；按连接计数；Top-N 高频查询含次数+平均耗时）；`cmd/s9l/history.go` `runHistory` 分派 `stats`→`runHistoryStats`（`--top`，渲染总览/按连接/高频查询）。只读本地 `history.db`，核心零改动。
  - DoD：白盒 `TestStats`（总数/成功率/avg 取整/按连接排序/Top 查询 avg）、`TestStatsEmpty` ✅；CLI 实测输出正确 ✅；docs(README/MANUAL §3·§9) 同步 ✅。

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
