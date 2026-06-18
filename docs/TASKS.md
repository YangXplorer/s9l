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

- [ ] **T3-1 主题与 lazygit 式视觉/布局**
  - 产出：`internal/tui/theme.go` 集中配色（focus/accent/select/border/dim 常量）；圆角边框（`tview.Borders` 配置）；面板标题带序号 `[1] Connections`…`[4] SQL`（与 `1/2/3/4` 跳转键一致）；聚焦面板高亮边框、非聚焦淡色；底部 lazygit 式「键位提示栏」（与状态行分离：状态行 + 键位行两行）；尊重 `NO_COLOR`
  - DoD：聚焦面板边框高亮、标题带序号；底部键位栏列出上下文键；`NO_COLOR` 下不崩、可读；白盒（焦点切换→边框色变化）+ 真实 pty 截图核对
  - 依赖：Phase T · 预估：2d
- [ ] **T3-2 Connections 仅名称 + 数据库图标**
  - 产出：`connIcon(driver)` 图标映射（postgres/mysql/sqlite/sqlserver，Nerd Font 字形 + ASCII 回退 `[pg]/[my]/[sq]/[ms]`，`S9L_TUI_ICONS=0` 可关）；List 主文本改为 `<icon> <name|id>`，host/db 等细节移到淡色副行或去除
  - DoD：列表每行 `图标 + 名称`；无 Nerd Font 时回退不乱码/不错位；白盒 `connIcon` + 列表填充测试
  - 依赖：T3-1 · 预估：0.5d
- [ ] **T3-3 SQL 编辑器面积翻倍**
  - 产出：编辑器固定高 6 → 12（约一倍）；Results/SQL 纵向比例调整；可见行数常量化（便于将来配置）；小终端最小高度回退
  - DoD：SQL 面板可见行数约翻倍；小窗口不挤爆布局；真实 pty 目视确认
  - 依赖：T3-1 · 预估：0.25d
- [ ] **T3-4 图形化「新增连接」表单**
  - 产出：`internal/tui/connform.go`——Connections 面板 `n`（或 `Ctrl-N`）打开 `tview.Form`：id/name/driver(下拉 sqlite|postgres|mysql|sqlserver)/host/port/user/database/ssl(勾选)/password(掩码) 或 password-ref；提交→校验→`config.Add`+`Save`，有密码则 `secret.Default().Set`（写 keychain，配置仅存 ref）→刷新 Connections 列表；`Esc` 取消；错误进表单/状态栏
  - DoD：表单填写→保存→出现在 Connections 且写入 config.yaml（白盒：表单值→保存→`config.Load` 校验、密码进 keychain 用 go-keyring `MockInit` 验 ref 与解析）；重复 id/缺必填 报错不崩；`Esc` 无副作用；真实 pty `n`→填→保存→可见
  - 依赖：T3-1 · 预估：1.5d · 注：复用 `config`/`secret`，核心零改动；编辑/删除连接可后续
- [ ] **T3-5 结果过滤器**
  - 产出：App 保存上次结果集（列+行）；Results 面板 `/` 打开过滤输入框，按子串（大小写不敏感、跨列）实时过滤行并重渲染；`Esc` 关闭/清空、空过滤显示全部；状态栏显示 `filtered M/N`；NULL 单元格按既有文本参与匹配
  - DoD：`filterRows(cols,rows,term)` 纯函数白盒（大小写/跨列/NULL/空 term）+ 应用后行数断言；真实 pty `/`→输入→Esc
  - 依赖：T-1c（结果表格）· 预估：1d · 注：客户端内存过滤（结果已在内存）；服务端 WHERE 注入留后续

### 新数据库：SQL Server

- [ ] **P3-DB1 SQL Server 适配器**
  - 产出：`internal/driver/sqlserver/`，用 `github.com/microsoft/go-mssqldb`（纯 Go，免 CGO）；`[]byte`→string 归一化；Metadata 用 `INFORMATION_SCHEMA` / 系统目录（`\l`=`sys.databases`、`\dt`=当前库 `INFORMATION_SCHEMA.TABLES`、`\d`=`INFORMATION_SCHEMA.COLUMNS`，`@p1` 占位）；config 加 sqlserver DSN 分支（`sqlserver://user:pass@host:port?database=db&encrypt=...`）+ 注册
  - DoD：连 SQL Server，流式 rows；`RunConformance` 对真实 SQL Server（testcontainers `mcr.microsoft.com/mssql/server:2022-latest`）全 PASS；`\l`/`\dt`/`\d` 正确；**核心层零改动**（仅新增 driver 包 + config DSN 分支 + 注册）
  - 依赖：P0-3（Driver 接口）· 预估：1.5d
  - 注：**方言差异需评估**——T-SQL 无 `LIMIT`（用 `TOP n` 或 `OFFSET…FETCH`）、占位符 `@p1`、标识符 `[brackets]` 引用。若 `drivertest` 一致性套件用到 `LIMIT`/自增列等不可移植 SQL，需让套件方言无关或在 driver 内适配（差异下沉到 driver，不污染核心）。镜像较大（>1GB）、容器启动慢，IT 用 `testing.Short()` 隔离并放宽等待超时。

**Phase 3 验收**：TUI 具备 lazygit 式配色/圆角/序号面板/底部键位栏；Connections 显示图标+名称；SQL 编辑器约翻倍；可在界面内新增连接并持久化（密码进 keychain）；结果可即时过滤；新增 SQL Server 仅动 driver 层、conformance 全 PASS。核心层零改动；CI 绿；逻辑白盒 + SimulationScreen 冒烟 + 手动清单通过。

---

## Backlog（未排期，按需）

> 已细化「需要修改的内容」便于将来直接领取。标注 **架构影响**：✅=核心零改动（新增包/扩展配置即可）；⚠️=需小幅触碰连接编排或 Metadata 可选接口；🔴=需改核心抽象，开工前先做设计 spike。

- [ ] **B-1 SSH Tunnel（连接前建隧道）** · ⚠️ · 预估 2–3d
  - 目标：DB 在堡垒机后时，连接前先建 SSH 本地端口转发再连库。
  - 需要修改：
    - `internal/config/connection.go`：`ConnectionConfig` 增 `ssh:` 块（`ssh_host/ssh_port/ssh_user/ssh_key_path/ssh_key_ref(passphrase)/ssh_password_ref/known_hosts`）。
    - 新增 `internal/tunnel/`：基于 `golang.org/x/crypto/ssh`（纯 Go，免 CGO）拨号堡垒机、开本地 listener 转发到远端 `host:port`，返回本地地址 + `Close()`。
    - 连接编排（`cmd/s9l/main.go:resolveTarget` 与 `internal/tui` connect 路径）：若有 ssh 配置→先起隧道→把 DSN 的 host:port 改写为本地转发地址→`driver.Open`→连接关闭时拆隧道。**driver 接口不变**，仅在"打开连接"这层插入隧道（小幅触碰编排）。
    - `internal/secret`：SSH 密码/私钥 passphrase 复用 `SecretStore`（`ssh_password_ref`/`ssh_key_ref`）。
  - 关键考量/风险：**必须校验 known_hosts**（默认不盲信主机密钥）；支持私钥(含 passphrase)/密码/`ssh-agent` 三种认证；隧道生命周期绑定连接；IT 用容器化 sshd + db。
- [ ] **B-2 TLS 配置（CA/客户端证书、sslmode 细化）** · ✅ · 预估 1.5–2d
  - 目标：比当前布尔 `ssl` 更细——CA 校验、客户端证书(mTLS)、各驱动 sslmode/tls 模式。
  - 需要修改：
    - `internal/config/connection.go`：增 `tls_ca/tls_cert/tls_key/tls_server_name/ssl_mode`（保留 `ssl: true` 向后兼容→等价 `require`）。
    - DSN 构建：postgres 加 `sslmode/sslrootcert/sslcert/sslkey`；mysql 用 `mysql.RegisterTLSConfig(name, *tls.Config)` 后 DSN 带 `tls=<name>`；sqlserver `encrypt`/`trustServerCertificate`。
    - 可新增 `internal/config` 内小助手：由文件路径构建 `*tls.Config`。
  - 关键考量/风险：默认推荐 `verify-full`；mysql 的 RegisterTLSConfig 是全局注册需在 Open 前调用；证书路径错误要清晰报错。
- [ ] **B-3 AWS RDS IAM Auth（临时 token 连接）** · ⚠️ · 预估 2d
  - 目标：用 IAM 生成 ~15 分钟临时 token 作为 RDS/Aurora(pg/mysql) 的密码。
  - 需要修改：
    - 认证模式：`password_ref` 增方案 `aws-rds-iam`（或连接字段 `auth: rds-iam` + `region`）。
    - 新增 `internal/awsauth/`：用 AWS SDK Go v2 `feature/rds/auth.BuildAuthToken(ctx, endpoint, region, user, creds)` 在**连接时**生成 token（时效短，不长缓存）。
    - 连接编排：auth=rds-iam 时即时取 token 当密码，并强制 TLS（RDS IAM 必须）。
  - 关键考量/风险：**引入 AWS SDK 依赖较重**（纯 Go，不破坏 CGO 约束）；凭据链 env/instance-profile/SSO；token 仅握手时需要（长连接不受 TTL 影响）；真实连接需 AWS 环境→**手动验证**，单测只验 token 装配（fake creds）。
- [ ] **B-4 ClickHouse 驱动** · ✅ · 预估 1.5d
  - 目标：新增 ClickHouse（关系型、契合现有接口）。
  - 需要修改：新增 `internal/driver/clickhouse/`（`github.com/ClickHouse/clickhouse-go/v2` 的 database/sql stdlib，纯 Go）；`[]byte`→string 归一化；Metadata 用 `system.tables`/`system.columns`/`system.databases`；config 加 clickhouse DSN 分支 + 注册。**核心零改动**（同 MySQL/SQL Server 模式）。
  - 关键考量/风险：方言差异（`LIMIT` OK；类型多）下沉到 driver；testcontainers `clickhouse/clickhouse-server`。
- [ ] **B-5 MongoDB（评估非关系型对接口的冲击）** · 🔴 · 预估：设计 spike 0.5d，落地大
  - 目标：评估能否纳入文档型数据库。
  - 需要修改/冲击：当前 `Driver.Query(sql)`→`Rows(columns/values)` 假设**表格化 SQL**；Mongo 用 find/aggregate + 文档结果，**不契合现有接口**。需新增能力接口（如 `DocumentStore`）或文档→表格投影层 + REPL/TUI 的另一查询模式。
  - 决策点：先 spike 评估接口冲击与价值；很可能**暂不纳入**（s9l 定位 SQL 客户端），或仅做只读文档浏览。**开工前必须设计评审**。
- [ ] **B-6 TUI 连接编辑/删除** · ✅ · 预估 1d
  - 需要修改：扩展 Phase 3 的 `internal/tui/connform.go`——编辑(预填现有值)；Connections 面板 `d` 删除(确认浮层)→`config.Remove`+`Save`+`secret.Delete`(keychain 密码)；刷新列表。复用 config/secret，核心零改动。
- [ ] **B-7 TUI 跨库浏览（库→表 多级树）** · ⚠️ · 预估 1.5d
  - 目标：Schema 树从「单库表列表」升级为「库→表」多级（解决"未选默认库时看不到表"的痛点）。
  - 需要修改：`internal/tui` Schema 树先 `Metadata.Databases()` 列库，展开某库再列其表；需要"按指定库列表"能力——给 `driver.Metadata` 增可选方法 `TablesIn(ctx, db)`（或树内跑带 schema 过滤的查询）。pg/mysql/sqlserver 各自实现（差异下沉 driver）。**Metadata 可选接口扩展**（向后兼容：未实现则回退当前库）。
  - 关键考量：mysql 需 `USE`/限定库名，pg 走 `table_schema`，sqlserver 走三段名；保持向后兼容。
- [ ] **B-8 结果导出（CLI 已有 / TUI 新增）** · ✅ · 预估 0.75d
  - 现状：CLI `s9l <conn> -e "..." --format csv > f` 已能导出。
  - 需要修改：TUI Results 面板加 `e` 导出当前结果集到文件（CSV/JSON，**复用 `internal/render`**）；选路径/格式的小浮层。核心零改动。
- [ ] **B-9 数据导入（CSV/JSON 批量）** · ✅ · 预估 1.5–2d
  - 目标：把 CSV/JSON 批量导入表。
  - 需要修改：新增 `cmd/s9l/import.go`——`s9l <conn> import --table T --file data.csv [--format csv|json] [--batch N]`；解析文件→列映射→事务内批量 `INSERT`(复用 `driver.Conn.Exec` + 参数绑定)；报告导入行数。
  - 关键考量/风险：大文件流式读取、类型推断、冲突策略(skip/replace)、参数占位符按方言；IT 用 SQLite。
- [ ] **B-10 历史统计 `s9l history stats`** · ✅ · 预估 0.75d
  - 需要修改：`internal/history` 加聚合查询（按 SQL/连接 GROUP BY：高频查询 Top N、平均耗时、成功率、按连接计数）；`cmd/s9l/history.go` 加 `stats` 子命令渲染。只读本地 `history.db`，核心零改动。
- [ ] **B-11 历史/收藏云同步** · 🔴 · 预估：设计 needed，大
  - 目标：把 `history.db`/收藏同步到远端（git 仓库 / S3 / 同步端点）。
  - 决策点：需选后端 + 认证 + **隐私设计**（历史含 SQL，可能含敏感信息）。建议**暂缓**，优先做 B-10 本地统计；如要做，先出设计与隐私评审。
- [ ] **B-12 运行期插件机制（plugin / wasm）** · 🔴 · 预估：spike 2–3d，落地大
  - 目标：运行时加载 driver（Go plugin 或 wasm），而非编译期注册。
  - 决策点：当前**编译期 `Driver` 注册已满足"新增库只加一个 driver 文件"**目标；运行期插件带来 ABI 稳定性、安全沙箱（建议 `wazero` wasm 而非 Go plugin）等大复杂度与安全面。**仅当编译期抽象不够用时再评估**，先 spike。

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
