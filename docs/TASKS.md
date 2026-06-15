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
  - DoD：SQLite 两种方式均跑通 ✅；PG 待 P1-B1（DSN 构建对 postgres 暂返回未实现错误）
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
- [ ] **P1-B1 PostgreSQL 适配器**
  - 产出：`internal/driver/postgres/`，用 `jackc/pgx`（stdlib 或原生）
  - DoD：连 PG，执行查询返回流式 rows；正确映射常见类型（int/float/text/bool/timestamp/null/bytea）
  - 依赖：P0-3 · 预估：1d · **验证 Driver 抽象是否够用（与 SQLite 差异最大点）**

- [ ] **P1-B2 流式读取与大结果保护**
  - 产出：rows 逐行消费不全量进内存；超大单元格/超宽行截断（可配宽度）
  - DoD：查 10w 行不 OOM；输出可控
  - 依赖：P1-B1 · 预估：0.5d

- [ ] **P1-B3 错误与上下文处理**
  - 产出：统一错误包装（区分连接错误/SQL 错误/超时）；支持 `Ctrl-C` 取消正在执行的查询（context cancel）
  - DoD：长查询可中断；错误信息含 driver 上下文
  - 依赖：P1-B1 · 预估：0.5d

### C. 交互与输出
- [ ] **P1-C1 输出渲染器（table/json/csv）**
  - 产出：`internal/render/`，`--format table|json|csv`，默认 table
  - DoD：三种格式正确输出，含 NULL 表示、宽表对齐
  - 依赖：P0-3 · 预估：1d

- [ ] **P1-C2 管道友好（TTY 检测）**
  - 产出：非 TTY（被管道/重定向）时默认输出 TSV 或 JSON，不打印边框/颜色
  - DoD：`s9l mypg -e "..." | jq` / `> out.csv` 可解析
  - 依赖：P1-C1 · 预估：0.25d

- [ ] **P1-C3 REPL 模式**
  - 产出：`internal/repl/`，进入交互式：多行输入（以 `;` 结束）、历史（上下键）、`Ctrl-C` 取消当前输入、`Ctrl-D` 退出
  - 选库：`chzyer/readline` 或 `charmbracelet/bubbline`/`go-prompt`（待 tech lead 定）
  - DoD：`s9l mypg` 进 REPL，可连续执行多条 SQL
  - 依赖：P1-A2, P1-C1 · 预估：1.5d

### D. 便捷命令
- [ ] **P1-D1 元数据反斜杠命令**
  - 产出：REPL 与 `-e` 均支持 `\l`(库)、`\dt`(表列表)、`\d <table>`(表结构)
  - 实现：每个 driver 提供 `Metadata` 方法返回结构化结果，渲染复用 C1
  - DoD：SQLite + PG 下三命令都返回正确结构
  - 依赖：P1-B1, P1-C1 · 预估：1d · 注：方言差异点，验证抽象

### E. 工程化
- [ ] **P1-E1 集成测试（testcontainers）**
  - 产出：PG 用 `testcontainers-go` 起容器跑集成测试；SQLite 走内存库单测
  - DoD：CI 上集成测试可跑（或标记 `-short` 跳过）
  - 依赖：P1-B1 · 预估：1d

- [ ] **P1-E2 README + 安装说明**
  - 产出：README（特性、安装 `go install`、快速上手、config 示例、命令速查）
  - DoD：照着 README 能从零连上 PG 跑查询
  - 依赖：MVP 功能完成 · 预估：0.5d

- [ ] **P1-E3 Release 流程（goreleaser + GitHub Actions）**
  - 产出：`.goreleaser.yaml`(最小版) + `.github/workflows/release.yml`；`cmd/s9l/main.go` 预留 `version/commit/date` ldflags 变量；CI 的 `build` job 加 `goreleaser release --snapshot --clean` 干跑
  - DoD：打 tag `v*` 自动出多平台二进制 + checksums + GitHub Release；`s9l --version` 可查；`go install .../cmd/s9l@latest` 可装
  - 依赖：P1-E2 · 预估：0.75d · 详见 [RELEASE.md](./RELEASE.md)

### F. 历史与收藏（SQLite，v0.1 纳入）
- [ ] **P1-F1 history.db 初始化 + 迁移**
  - 产出：`internal/history/sqlite.go`，在 `~/.config/s9l/history.db` 建库；内嵌迁移建表 `query_history` / `saved_queries`
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
  - DoD：首次运行自动建库建表；重复运行迁移幂等
  - 依赖：P0-2, P0-4（复用 sqlite 驱动）· 预估：0.5d

- [ ] **P1-F2 执行后写历史**
  - 产出：`internal/query/history.go`，每次执行（含失败）写入 `query_history`，记录 `duration_ms`/`rows_affected`/`success`/`error_message`
  - DoD：成功与失败查询都落历史；写历史失败不影响主流程（降级告警）
  - 依赖：P1-F1, P1-B2 · 预估：0.5d

- [ ] **P1-F3 收藏命令**
  - 产出：`internal/query/saved.go` + CLI/REPL：`save`/`list`/`search`/`run <id>`，支持按 `connection_id`/`tags` 筛选
  - DoD：能保存、列出、按关键字/标签搜索、执行收藏的 SQL
  - 依赖：P1-F1 · 预估：0.75d

**Phase 1 验收**：`s9l mypg` REPL 查询、`\dt`/`\d` 可用、`-e` + 管道导出 CSV 可用、命名连接增删查可用、查询历史自动记录、SQL 收藏可用、README 完整、CI 含集成测试。

**MVP 明确不做**：MySQL（→P2）、自动补全、TUI、运行期插件、系统 Keychain（→P2）、schema cache（→P2）、`query_folders`、SSH 隧道/TLS/IAM（→P3）、多结果集。

---

## Phase 2 — 多库扩展 + 体验增强

预估合计：~5–6 人日

- [ ] **P2-1 MySQL 适配器**（`go-sql-driver/mysql`）— DoD：仅新增 driver 文件即可，不改核心；元数据命令适配 · 预估：1d
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
