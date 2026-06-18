# s9l 项目计划

> 一个在终端里快速连接数据库并进行数据操作的工具。目标：操作简单、功能便捷、适配多数常用数据库、可拓展性高、查询效率高。

---

## 背景

- 仓库：`github.com/YangXplorer/s9l`（public OSS），本地 `/Users/yangxianglong/dev/oss/s9l`。
- 当前状态：空仓库，只有 `CLAUDE.md` 骨架，未选语言、未写代码。
- 同类参考：`usql`（Go，号称"universal"，支持几十种 DB，命令式 + REPL）、`mycli`/`pgcli`（Python，带补全的交互式）、`harlequin`（Python，TUI）、`dblab`/`gobang`（Go TUI）。
- 结论：这是一个"造轮子但有明确差异化空间"的项目。市面已有 usql 这种成熟方案，s9l 要立住，差异点应押在 **操作简单 + 命令便捷 + 可拓展架构** 上，而不是"支持的 DB 数量"。

## 当前目标

用最小可行版本（MVP）验证核心体验：**在终端里用一条极简命令连上一个数据库、跑查询、把结果舒服地看清楚**。先把"一个数据库跑通且好用"做到位，再谈"多数据库"。

## 关键决策点（需要你拍板，已给推荐默认）

> 这些直接决定后续架构，建议先定下来再开工。我已给推荐项，你只需确认或改。

| # | 决策 | 选项 | 推荐默认 | 理由 |
|---|------|------|---------|------|
| D1 | **技术栈** | Go / TypeScript(Node) | **Go** ✅ | 单二进制分发、跨平台、`database/sql` 统一驱动接口天然契合"多 DB + 可拓展"，并发查询/流式读取性能好；OSS 终端工具 Go 生态最成熟（usql/dblab 都是 Go）。TS 适合做 TUI 但分发和原生 DB 驱动不如 Go。 |
| D2 | **形态** | 纯命令式 CLI / 交互式 TUI / 两者结合 | **三者结合：CLI/REPL（已交付 v0.1/v0.2）+ 全屏 TUI（lazygit 式，提升为一等交付）** ✅（已修订 2026-06-16） | 原计划 TUI 后置；现用户明确目标形态为 lazygit 式有界面工具，TUI 从 Backlog 升为独立大阶段 **Phase T**。CLI/REPL 不废弃（脚本化/管道仍需要），TUI 经 `s9l tui [conn]` 进入。 |
| D10 | **TUI 框架** | tview / bubbletea / gocui | **`rivo/tview`** ✅（已拍板 2026-06-16） | DB 客户端最重的是"结果表格 + schema 树"，tview 内置 Table/TreeView/Flex，落地最快；k9s 同款、成熟纯 Go。bubbletea 更现代但表格/树需自拼、工期长；gocui(lazygit 本体)控件少。详见 [TUI.md](./TUI.md)。 |
| D3 | **首批数据库** | — | **SQLite + PostgreSQL + MySQL** ✅ | SQLite 零依赖最易测试做脚手架；PG/MySQL 覆盖绝大多数场景。其余（SQL Server/Oracle/ClickHouse 等）走插件式后加。 |
| D4 | **"可拓展性高"落地为** | 编译期 driver 抽象 / 运行期插件 | **先做编译期 Driver 接口抽象** ✅ | 一个清晰的 `Driver` interface + 注册机制即可满足"加新库只写一个适配文件"。运行期插件（plugin/wasm）复杂度高、收益不确定，明确放进 Backlog，不进 MVP。 |
| D5 | **配置/连接管理** | 每次输 DSN / 命名连接 profile | **两者都要：支持裸 DSN，也支持 `~/.config/s9l/config.yaml` 命名连接（遵循 XDG，尊重 `$XDG_CONFIG_HOME`）** ✅ | "命令便捷"的核心是 `s9l mydb` 就能连，靠命名 profile 实现。存储架构见下方专节。 |
| D7 | **配置格式** | TOML / YAML | **YAML** ✅（已拍板） | 用户可手动编辑、便于导入导出、适合 Git 管理（不含密码）、调试简单。 |
| D8 | **凭据存储** | 明文 / 环境变量 / 系统 Keychain | **系统 Keychain 为目标，v0.1 先起动时输入不保存** ✅（已拍板） | config 只存 `password_ref`，真密码进系统 Keychain（macOS Keychain / Windows Credential Manager / Linux Secret Service）。 |
| D9 | **历史/收藏存储** | 文件 / SQLite | **SQLite（`history.db`）** ✅（已拍板） | 历史量大、需搜索/排序/过滤/打标签/统计耗时，SQLite 最合适。 |
| D6 | **命令风格** | 子命令式(`s9l query ...`) / DSN 直连 + REPL | **`s9l <conn>` 进 REPL + `-e` 单次执行 + 管道** ✅ | 对标 `mysql`/`psql` 的肌肉记忆，学习成本最低。 |

**待确认（开工前请回复）**：D1 技术栈是否就定 **Go**？这是最大不可逆决策，其余都可后调。

## 存储与凭据架构（已拍板）

职责分离：**连接配置走 YAML（可手编/Git）、密码走系统 Keychain、历史/收藏走 SQLite**。

```
~/.config/s9l/
├── config.yaml        # 非敏感连接配置（不含明文密码）
└── history.db         # SQLite：查询历史 + 收藏 SQL（+ 可选 folders）
~/.cache/s9l/
└── schema.db          # 可选：schema cache（XDG cache 目录）
系统 Keychain          # 密码 / token（非文件）
```

### config.yaml（连接配置，YAML 列表形式）
```yaml
connections:
  - id: local-mysql
    name: Local MySQL
    driver: mysql
    host: localhost
    port: 3306
    user: root
    database: demo
    ssl: false
    charset: utf8mb4
    password_ref: keychain://s9l/connection.local-mysql.password
  - id: dev-postgres
    name: Dev Postgres
    driver: postgres
    host: 127.0.0.1
    port: 5432
    user: dev
    database: app
    ssl: true
    password_ref: keychain://s9l/connection.dev-postgres.password
```
对应 Go 结构（`internal/config`）：
```go
type ConnectionConfig struct {
    ID          string `yaml:"id"`
    Name        string `yaml:"name"`
    Driver      string `yaml:"driver"`
    Host        string `yaml:"host"`
    Port        int    `yaml:"port"`
    User        string `yaml:"user"`
    Database    string `yaml:"database"`
    SSL         bool   `yaml:"ssl"`
    Charset     string `yaml:"charset"`
    PasswordRef string `yaml:"password_ref"` // keychain://s9l/connection.<id>.password
}
```

### 凭据：SecretStore 抽象（`internal/secret`）
```go
type SecretStore interface {
    Get(service, key string) (string, error)
    Set(service, key, value string) error
    Delete(service, key string) error
}
// service = "s9l"；key = "connection.<id>.password"
```
实现：`keychain.go`（基于 `zalando/go-keyring`，跨 macOS/Windows/Linux）+ `memory.go`（v0.1 起动时输入、仅内存、不落盘，便于先跑通与测试）。

### 历史/收藏：SQLite（`internal/history`）
- `query_history`：每次执行的 SQL（connection_id / database_name / sql_text / executed_at / duration_ms / rows_affected / success / error_message）
- `saved_queries`：收藏 SQL（title / description / connection_id / database_name / sql_text / tags / created_at / updated_at）
- `query_folders`（可选，后置）：收藏分组，`saved_queries.folder_id` 关联
> 具体 DDL 见 [TASKS.md](./TASKS.md)。

### 分版落地（与 Phase 对齐）
- **v0.1（≈ MVP / Phase 1）**：config.yaml 连接 + 密码起动时输入（不保存，`SecretStore=memory`）+ SQLite `query_history`/`saved_queries`
- **v0.2~v0.4（Phase 2）**：系统 Keychain（`SecretStore=keychain`，✅）+ schema cache（✅）+ 补全/分页/收藏分组（✅）
- **v0.5（Phase T）**：全屏 TUI（lazygit 式，连接/树/结果/编辑器/历史/收藏，✅）
- **v0.6（≈ Phase 3）**：TUI 强化（lazygit 风格配色/布局、Connections 图标、SQL 编辑器扩大、界面内新增连接、结果过滤器）+ 新增 **SQL Server** 驱动；v0.6.1 增 TUI 连接编辑/删除 + MySQL 库→表树
- **v0.7（≈ Phase 4）**：TUI 交互重构——背景与终端/lazygit 一致；Connections 可展开到数据库；Schema 显示当前库表并可检索；使用手册同步
- **后续 Backlog（未排期）**：SSH Tunnel + TLS 配置 + AWS RDS IAM Auth + 更多数据库

## 需求拆解

把模糊需求拆成可执行模块：

### A. 连接与配置
- A1 DSN 解析 + 连接建立（基于 `database/sql`）
- A2 命名连接：`~/.config/s9l/config.yaml` 读取（遵循 XDG，`$XDG_CONFIG_HOME` 优先），`s9l <id>` 直连
- A3 连接管理命令：`s9l conn add/list/rm`
- A4 凭据：`SecretStore` 抽象 + `password_ref`；v0.1 起动时输入（memory 实现），v0.2 接系统 Keychain

### F. 历史与收藏（SQLite，v0.1 即纳入）
- F1 `history.db` 初始化 + 迁移（`query_history` / `saved_queries`）
- F2 执行后写历史（耗时/影响行数/成功失败/错误）
- F3 收藏命令：保存/列出/搜索/按连接·标签筛选/执行收藏的 SQL
- F4 `query_folders` 分组（可选，后置）

### B. 核心执行引擎
- B1 `Driver` 接口抽象（连接、查询、元数据、类型映射、方言差异）
- B2 SQLite 适配（脚手架基准）
- B3 PostgreSQL 适配
- B4 MySQL 适配
- B5 查询执行：流式读取 `rows`，避免全量进内存（效率关键）

### C. 交互与输出
- C1 单次执行 `s9l <conn> -e "SQL"`
- C2 REPL 模式（多行输入、历史、Ctrl-C 处理）
- C3 结果渲染：表格（默认）/ JSON / CSV，支持宽表截断与分页
- C4 管道友好：检测非 TTY 时输出可解析格式（默认 TSV/JSON）

### D. 便捷功能（提升"功能便捷"体感）
- D1 元数据快捷命令：`\dt`(列表)、`\d <table>`(结构)、`\l`(库列表) —— 对标 psql 反斜杠命令
- D2 自动补全（表名/列名）—— REPL 内，成本较高，可后置
- D3 查询历史、上一次结果重用

### E. 工程化（OSS 基建）
- E1 项目脚手架：Go module、目录结构、Makefile/Taskfile
- E2 CI：lint(golangci-lint) + test + build（GitHub Actions）
- E3 测试：SQLite 单测 + PG/MySQL 用 testcontainers/docker 集成测试
- E4 README + 安装方式（`go install` / release 二进制 / brew 后置）
- E5 完善 `CLAUDE.md`

## 优先级 & 分阶段路线图

### Phase 0 — 脚手架（先做，0.5~1 天）
**做什么**：E1 + E2 骨架 + B1 `Driver` 接口草案 + B2 SQLite 跑通最小查询。
**交付物**：`s9l ./test.db -e "select 1"` 能输出结果；CI 绿。
**验证标准**：clone → `go build` → 连 SQLite 跑通一条 SQL，CI 通过。

### Phase 1 — MVP（核心，建议 1~2 周）
**做什么**：A1/A2/A3 + B3(PostgreSQL) + B5 流式 + C1/C2/C3/C4 + D1。
**MVP 明确做**：SQLite + PostgreSQL 两个库；命名连接；单次执行 + REPL；表格/JSON/CSV 输出；`\dt`/`\d` 元数据命令。
**MVP 明确不做**：MySQL（放 Phase 2）、自动补全、TUI、运行期插件、事务/批量导入、多结果集、连接 SSH 隧道。
**交付物**：能日常用 s9l 替代 psql 做基本查询。
**验证标准**：`s9l mypg` 进 REPL 查询、`\dt` 列表、`-e` + 管道导出 CSV 全部可用；README 有完整使用示例。

### Phase 2 — 多库扩展 + 体验增强（进行中）
MySQL(P2-1 ✅) + 补全 + 系统 Keychain + 输出分页 + 错误打磨 + Homebrew。验证：新增 MySQL 仅改 driver 层，不动核心（已验证）。其余 P2 任务**暂保留**（用户优先做 TUI）。

### Phase T — 全屏 TUI（lazygit 式，新增的一等交付）
基于已有 driver/config/secret/history 层，加一层 tview 多面板界面：连接列表 + schema 树 + 结果表格 + SQL 编辑器 + 历史/收藏面板。经 `s9l tui [conn]` 进入。**先做 MVP 垂直切片**（连接→schema 树→选表查询→结果浏览），再迭代编辑器/历史/收藏/键位打磨。详见 [TUI.md](./TUI.md)，WBS 见 [TASKS.md](./TASKS.md) Phase T。

### Phase 3 — TUI 强化 + SQL Server（目标 v0.6）
在 Phase T 已交付的全屏 TUI 上做体验与视觉打磨，延续「只改 `internal/tui/`、不动核心」原则。TUI 五项：① lazygit 式配色/圆角/序号面板 + 底部键位栏；② Connections 仅显示名称 + 数据库类型图标；③ SQL 编辑器面积约翻倍；④ 界面内「新增连接」表单（写 config + 密码进 keychain）；⑤ 结果面板过滤器。另新增 **SQL Server 驱动**（`microsoft/go-mssqldb`，纯 Go 免 CGO），按既有「只加一个 driver 包」扩展模式（同 MySQL），核心零改动。详见 [TUI.md](./TUI.md) 「TUI 强化」节，WBS 见 [TASKS.md](./TASKS.md) Phase 3（T3-1~T3-5、P3-DB1）。

### Phase 4 — TUI 交互重构（目标 v0.7）
按用户反馈调整 TUI 层次：① 背景与终端/lazygit 一致（用终端默认背景）；② **Connections 可展开到数据库**（连接→库下拉）；③ **Schema 只显示所选库的表并可检索**（库层从 Schema 上移到 Connections，取代 B-7 的 Schema 库→表树）；④ 新功能用法同步进使用手册 [MANUAL.md](./MANUAL.md)。仍只改 `internal/tui/`，核心零改动。WBS 见 [TASKS.md](./TASKS.md) Phase 4（T4-1~T4-4）。

### Backlog（按需，未排期）
更多数据库（SQL Server/ClickHouse/Mongo 等）、运行期插件、SSH 隧道、TLS/IAM、数据导入导出、历史/收藏云同步。

## 依赖与阻塞

- **D1 技术栈拍板** → 阻塞一切开工（最高优先解决）。
- B1 `Driver` 接口设计 → 阻塞 B2/B3/B4，必须先稳定，否则多库适配返工。
- E3 集成测试需要 Docker 环境（PG/MySQL），CI 上用 testcontainers/service container。
- A4 密码安全方案 → 阻塞 A2 落地细节（可先用明文 + 0600 权限跑通，再迭代）。

## 风险

| 风险 | 等级 | 说明 & 缓解 |
|------|------|------|
| **R1 重复造轮子**（usql 已很全） | 高 | s9l 不拼 DB 数量，押"简单 + 便捷 + 可拓展"。**缓解**：MVP 前花 1 小时实际用一遍 usql/pgcli，明确我们的差异化体验点再动手。 |
| **R2 "多数据库"抽象失控** | 高 | 各库方言/类型/元数据查询差异大，接口设计不当会层层 if-else。**缓解**：Driver 接口只抽象"共性最小集"，方言差异下沉到各 driver；用 SQLite+PG 两个差异较大的库先验证抽象是否够用，再加第三个。 |
| **R3 "查询效率高"被误解为引擎优化** | 中 | 工具本身不决定 DB 查询速度，能控的是：流式读取不爆内存、连接复用、大结果分页/截断、不做无谓拷贝。**缓解**：明确"效率"= 工具开销低 + 大结果不卡，写进非功能需求，避免过度优化。 |
| **R4 凭据安全** | 中 | 明文存密码是 OSS 工具差评点。**缓解（已定方案）**：config.yaml 永不存明文密码，只存 `password_ref`；v0.1 起动时输入（不落盘）+ 支持环境变量/`$PGPASSWORD`；v0.2 接系统 Keychain（`zalando/go-keyring`）。 |
| **R5 范围蔓延**（TUI/补全/插件早做） | 中 | 这些都很诱人但拖慢 MVP。**缓解**：严格按阶段，Phase 1 不碰 TUI 和运行期插件。 |
| **R6 跨平台/驱动依赖**（如 MySQL/PG 驱动 CGO） | 低 | 选纯 Go 驱动（`jackc/pgx`、`go-sql-driver/mysql`、`modernc.org/sqlite` 纯 Go 版）避免 CGO，保证单二进制。 |

## 下一步行动（立即可执行）

1. **【你】拍板 D1**：确认技术栈 = Go？（其余 D2~D6 我已给默认，可一并确认或微调）
2. **【你/我】30~60 分钟竞品体感**：实际跑一遍 usql + pgcli，确认 s9l 差异化卖点（可选但强烈建议，降 R1）。
3. **【工程师，Phase 0】** 一旦 D1 确认：
   - `go mod init github.com/YangXplorer/s9l`
   - 建目录：`cmd/s9l/`（入口）、`internal/driver/`（接口 + sqlite）、`internal/repl/`、`internal/render/`、`internal/config/`
   - 起 `Driver` 接口草案 + SQLite 适配，跑通 `s9l ./x.db -e "select 1"`
   - 加 GitHub Actions（lint + test + build）
4. **【我，下一步可代办】** 你确认 D1 后，我可以直接交给 `@personal-tech-lead` 出 `Driver` 接口设计 + Phase 0 技术方案，再交给 `@personal-engineer` 落地脚手架。

---

## 假设清单（如与事实不符请纠正）

- A-1：这是个人/小团队的 OSS 项目，无硬性 deadline，可按阶段迭代。
- A-2（已修订 2026-06-16）：目标用户是开发者，但期望**像 lazygit 那样的全屏交互界面**作为主形态。CLI/REPL 保留用于脚本化，TUI 提升为一等交付（见 D2/Phase T）。
- A-3："适用于多数常用数据库" = 关系型为主（PG/MySQL/SQLite/SQL Server 等），NoSQL（Mongo/Redis）非首批目标。
- A-4：你接受"先把 1~2 个库做好用"，而不是一上来铺开十几个库。
