# s9l 测试策略

> 配套：[PLAN.md](./PLAN.md) · [TASKS.md](./TASKS.md)。本文件定义测试分层、框架选型与运行方式。

## 框架选型（已拍板）

```
testing + google/go-cmp + testcontainers-go
```

| 用途 | 选型 | 说明 |
|------|------|------|
| 测试主框架 | 标准库 `testing` | 表驱动 + `t.Run` 子测试为主，零依赖、最地道 |
| 深度比较 | `google/go-cmp` | 替代断言库做结构体/slice/map 深比较（`cmp.Diff`），失败信息清晰 |
| 容器化 IT | `testcontainers-go` | 测试内起 PG/MySQL 容器，自动销毁，CI/本地一致 |
| render 输出断言 | inline `go-cmp` 字面量 | render(table/json/csv/tsv) 用 `cmp.Diff` 对照内联期望字符串；如未来宽表/大输出确有需要再引 golden file |

**明确不用**：`testify`（保持零断言框架、贴合极简偏好）、Ginkgo/Gomega（BDD 太重）。
**备选（按需才引）**：`go-sqlmock`/`gomock` —— 优先用真 SQLite 内存库与手写假实现，少用 mock，避免"假绿"。

## 测试分层

| 层 | 测什么 | 依赖 | 何时跑 |
|----|--------|------|--------|
| **UT** | 纯逻辑：DSN/`password_ref` 解析、config YAML 往返、render、SecretStore(memory)、CLI 参数、各 driver 元数据 SQL 拼接 | 无外部依赖 | 每次 `go test`（秒级） |
| **快速 IT（SQLite）** | 真实 SQL、流式 rows、history.db 读写、Driver 接口契约 | `modernc.org/sqlite` 内存库（纯 Go，无 CGO/容器） | 每次 `go test` |
| **完整 IT（PG/MySQL）** | 真连接、类型映射、方言差异、元数据命令、ctx 取消 | `testcontainers-go` 起容器 | CI + 本地按需（默认 `-short` 跳过） |

**关键点**：SQLite 是纯 Go 内存库，"真实 SQL 行为"几乎能以 UT 速度测，无 Docker 环境也能覆盖大部分执行逻辑。

## Driver 一致性测试套件（核心，对冲"多库抽象失控"风险 R2）

一套与具体 driver 无关、任何 `Driver` 实现都必须通过的测试：

```go
// internal/driver/drivertest/conformance.go
func RunConformance(t *testing.T, open func() (driver.Conn, error)) {
    // 建表 / insert / select 流式读取 / 类型映射 / NULL / ctx 取消 / 元数据
}
```

- SQLite：`go test` 每次都跑（快）
- PG/MySQL：IT 里用同一套 `RunConformance`
- 收益：新增数据库时**先过 conformance**，抽象是否够用立刻暴露，避免集成阶段才返工。
- 落地时机：P0 脚手架即建套件 + SQLite 基线；P1-E1 接入 testcontainers PG/MySQL。

## 隔离与运行

完整 IT 用 `testing.Short()`（或 build tag `//go:build integration`）隔离：

```go
if testing.Short() { t.Skip("skip IT: needs docker") }
```

```bash
# 本地快速回归（UT + SQLite IT，秒级，无需 Docker）
go test -short ./...

# 完整（含容器化 IT，需 Docker）
go test ./...

# 只跑 render 输出格式测试
go test ./internal/render

# 大结果流式不 OOM 的冒烟基准
go test -bench=. ./internal/driver/...
```

## CI 编排（GitHub Actions，对应 P0-6 / P1-E1）

- **Job `unit`**：`go test -short ./...` + `golangci-lint`（每个 PR，快）
- **Job `integration`**：`go test ./...`，runner 自带 Docker，testcontainers 直接用（PR 必跑或 nightly，视耗时定）

## 边界 / 不测什么

- **系统 Keychain 真实读写不进自动化测试**：CI 无 GUI keyring、跨平台 flaky。`keychain.go` 靠接口隔离 + macOS 本地手动冒烟；自动化只测 `memory` 实现与 `password_ref` 解析。
- 不测 DB 引擎本身性能；"效率"相关只做冒烟基准（10w 行流式不 OOM，用 SQLite 造大表）。
