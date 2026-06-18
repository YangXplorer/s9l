# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## 项目状态

s9l 是一个**终端数据库客户端**（快速连接数据库并进行数据操作，目标：操作简单、命令便捷、适配多数常用数据库、可拓展性高、查询效率高）。

**当前处于实现前的规划阶段**：仓库尚无源码，只有 `docs/` 下的设计文档与 `.claude/` 下的自审机制。下文的命令/目录是**已拍板的目标规格**，实现时按此落地（一旦有代码，重跑 `/init` 校准）。

## 设计文档（动手前必读）

| 文档 | 内容 |
|------|------|
| `docs/PLAN.md` | 需求收敛、关键决策(D1~D9)、存储与凭据架构、路线图、风险 |
| `docs/TASKS.md` | WBS：Phase 0~3 细化到可执行 task，含 DoD/依赖/`history.db` DDL/关键路径 |
| `docs/TESTING.md` | 测试分层、框架选型、Driver 一致性套件 |
| `docs/RELEASE.md` | 安装渠道、发布流程、CI/CD 实现与配置示例 |

## 已拍板的关键决策（不要擅自推翻）

- **语言：Go**。纯 Go 驱动、`CGO_ENABLED=0`（保证交叉编译与单二进制）。
- **形态**：CLI/REPL（已交付）+ **全屏 TUI（lazygit 式，Phase T，一等交付，框架 `rivo/tview`）**。`s9l <id>` 进 REPL，`s9l <id> -e "SQL"` 单次执行（非 TTY 管道友好），`s9l tui [conn]` 进全屏界面。TUI 只新增 `internal/tui/` 展示层，复用 driver/config/secret/history，不改核心。详见 `docs/TUI.md`。
- **配置**：`~/.config/s9l/config.yaml`（YAML，遵循 XDG，尊重 `$XDG_CONFIG_HOME`）。**绝不存明文密码**，只存 `password_ref`。
- **凭据**：`SecretStore` 接口抽象。`memory`（内存）+ `keychain`（系统 Keychain，`zalando/go-keyring`，`secret.Default()`）已实现；`password_ref` 支持 `env:NAME` 与 `keychain://s9l/connection.<id>.password`；`conn add --password` 存 keychain。CI 用 go-keyring `MockInit`，真实 keychain 手动验证。
- **历史/收藏**：SQLite `~/.config/s9l/history.db`（表 `query_history` / `saved_queries`，可选 `query_folders`）。
- **可拓展性**：编译期 `Driver` 接口抽象 + 注册机制。**新增数据库只新增一个 driver 文件，不改核心层**。运行期插件进 Backlog。
- **查询效率**：流式读取 rows（逐行消费，不全量进内存）、连接复用、大结果分页/截断。
- **已支持数据库**：SQLite、PostgreSQL、MySQL（纯 Go 驱动 `modernc.org/sqlite`、`jackc/pgx/v5/stdlib`、`go-sql-driver/mysql`，均免 CGO）。

## 架构（目标）

核心抽象是 `Driver` 接口——所有数据库差异下沉到各 driver，核心层（CLI/REPL/render/history）只依赖接口。这是"可拓展性高"和"适配多库"的支点，改动它要格外谨慎。

计划的包结构（`internal/`）：
- `cli/` 命令解析 · `repl/` 交互 · `render/` 输出(table/json/csv)
- `driver/` DB 适配：`driver.go`(接口+注册) + `drivertest/`(一致性套件) + 各库子包(`sqlite/`,`postgres/`,`mysql/`)
- `config/` YAML 连接配置 · `secret/` 凭据(`store.go`+`memory.go`+`keychain.go`)
- `history/` SQLite 历史仓储 · `query/` 历史/收藏逻辑
- 入口：`cmd/s9l/main.go`（含 ldflags 注入的 `version/commit/date`）

## 常用命令（实现后适用）

```bash
go test -short ./...          # UT + SQLite 快速 IT（秒级，无需 Docker）
go test ./...                 # 含容器化 IT（testcontainers 起 PG/MySQL，需 Docker）
go test ./internal/render -update   # 刷新 render golden 文件
go test -run TestXxx ./internal/...  # 跑单个测试
golangci-lint run             # lint
goreleaser release --snapshot --clean   # 本地干跑发布配置（不发布）
go install github.com/YangXplorer/s9l/cmd/s9l@latest  # 安装
```

发布：打 tag `vX.Y.Z` 并 push 即触发 `release.yml` → goreleaser 全自动出多平台二进制。

## 测试约定

- 框架：标准 `testing` + `google/go-cmp`（深比较）+ `testcontainers-go`（IT）+ golden file。**不使用 testify**。
- 任何 `Driver` 实现/改动**必须通过 `internal/driver/drivertest` 的 `RunConformance` 一致性套件**——这是验证多库抽象是否成立的闸门。
- 容器化 IT 用 `testing.Short()` 隔离（`-short` 跳过）。系统 Keychain 真实读写不进自动化测试（只测 `memory` 实现与 `password_ref` 解析）。

## 完成判定机制（重要行为约束）

本项目有一套**目标-标准双闸门自审机制**。在把任何交付标记为"完成"、commit、push 或发版之前：

- 运行 `/goal-check` skill，或调用 `s9l-goalkeeper` agent。
- 规则：**目标(Goal) 与 标准(Standard) 两者都达成才算 ゴール**。任一未满足 → 列出具体 gap，修复后重审，循环直到达成。诚实暂停优于虚假达成。
- 权威定义在 `.claude/agents/s9l-goalkeeper.md`。

## Git 约定

- 提交到 `develop` 分支（`main` 为发布分支）。**小粒度提交**：一个逻辑单元/文档一个 commit；实现阶段在 message 中带任务 ID（如 `P0-3`）。
- 远程认证：该仓库属于 `YangXplorer`，但本机 SSH key 绑定的是 `yangxianglong`。push 前需 `gh auth switch --user YangXplorer && gh auth setup-git`，走 HTTPS。
