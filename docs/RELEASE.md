# s9l 安装、发布与 CI/CD 方案

> 配套：[PLAN.md](./PLAN.md) · [TASKS.md](./TASKS.md) · [TESTING.md](./TESTING.md)。
> 本文件定义安装渠道、发布流程与 CI/CD 实现，含可直接落地的配置示例。

---

## 1. 安装渠道

| 渠道 | 安装方式 | 适用 | 上线阶段 |
|------|---------|------|---------|
| **`go install`** | `go install github.com/YangXplorer/s9l/cmd/s9l@latest` | Go 开发者，零基建 | v0.1 |
| **GitHub Releases 二进制** | 下载对应平台压缩包解压 | 所有人，无需 Go 环境 | v0.1 |
| **Homebrew** | `brew install YangXplorer/tap/s9l` | macOS/Linux 主力 | v0.2 |
| **install.sh** | `curl -fsSL https://raw.githubusercontent.com/YangXplorer/s9l/main/install.sh \| sh` | Linux/快速试用 | v0.2 |
| **Scoop**（Windows） | `scoop install s9l` | Windows | v0.3 按需 |
| **deb/rpm**（nfpm） | `apt`/`yum` | Linux 发行版 | v0.3 按需 |
| **Docker** | `docker run ghcr.io/yangxplorer/s9l` | CI/容器 | v0.3 按需 |

> Homebrew 先用自建 tap 仓库 `YangXplorer/homebrew-tap`（goreleaser 自动维护），稳定后再议 homebrew-core。

---

## 2. 版本管理

- **SemVer + git tag**：`vMAJOR.MINOR.PATCH`，与 v0.1/v0.2/v0.3 路线对齐。
- 预发布用 `-rc.N`（如 `v0.1.0-rc.1`），goreleaser 自动标记为 prerelease。
- 版本信息通过 `-ldflags` 注入，`s9l --version` 可查。入口 `cmd/s9l/main.go` 预留变量：

```go
package main

// 由 goreleaser / ldflags 注入；本地构建为默认值
var (
    version = "dev"
    commit  = "none"
    date    = "unknown"
)
```

---

## 3. 发布流程（端到端）

### 触发方式：打 tag 即发布

```bash
# 1. 确认 main 绿、CHANGELOG 就绪
# 2. 打带注释的 tag
git tag -a v0.1.0 -m "s9l v0.1.0"
# 3. 推送 tag（触发 release workflow）
git push origin v0.1.0
```

推 tag 后全自动，无需手动操作。

### 自动流程（GitHub Actions + goreleaser）

```
push tag v* 
  → release.yml 触发
  → 全量测试（go test ./...，含容器化 IT）
  → goreleaser：
      ├─ 交叉编译 linux/darwin/windows × amd64/arm64
      ├─ 打包 tar.gz(unix)/zip(win) + 注入 version/commit/date
      ├─ 生成 checksums.txt
      ├─ 创建 GitHub Release + 自动 changelog（按 commit）
      ├─ 更新 Homebrew tap（v0.2 起）
      └─ (可选) nfpm deb/rpm、ghcr docker 镜像
  → 发布完成，各渠道可安装
```

### 发布前检查清单（Release Checklist）

- [ ] `main` 分支 CI 全绿（unit + integration）
- [ ] 版本号符合 SemVer，未与已有 tag 冲突
- [ ] `CHANGELOG.md` 更新（或确认自动 changelog 足够）
- [ ] 破坏性变更已在 release notes 标注
- [ ] 本地 `goreleaser release --snapshot --clean` 干跑通过（不发布，只验证配置）

### 回滚策略

- 二进制有问题：在 GitHub Release 页标记该版本为 pre-release/draft，发补丁版 `v0.1.1`（**不复用/删除已发 tag**，避免 `go install` 缓存与下游混乱）。
- Homebrew：tap 由 goreleaser 维护，发新版即覆盖；紧急可手动 revert tap 仓库 commit。

---

## 4. CI/CD 实现方案

### 4.1 总览：两条流水线

| 流水线 | 文件 | 触发 | 职责 |
|--------|------|------|------|
| **CI** | `.github/workflows/ci.yml` | push / PR → 任意分支 | lint + 单测 + 快速 IT + 完整 IT |
| **Release** | `.github/workflows/release.yml` | push tag `v*` | 全量测试 + goreleaser 发布 |

分离原因：日常开发只跑 CI（快、频繁）；发布是低频、强校验的独立流程。

### 4.2 CI 流水线（`.github/workflows/ci.yml`）

```yaml
name: CI
on:
  push:
    branches: [main]
  pull_request:

permissions:
  contents: read

concurrency:
  group: ci-${{ github.ref }}
  cancel-in-progress: true

jobs:
  lint:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4
      - uses: actions/setup-go@v5
        with: { go-version: 'stable', cache: true }
      - uses: golangci/golangci-lint-action@v6
        with: { version: latest }

  unit:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4
      - uses: actions/setup-go@v5
        with: { go-version: 'stable', cache: true }
      # UT + 快速 IT(SQLite 内存库)，秒级，无需 Docker
      - run: go test -short -race ./...

  integration:
    runs-on: ubuntu-latest   # GitHub runner 自带 Docker，testcontainers 可直接用
    steps:
      - uses: actions/checkout@v4
      - uses: actions/setup-go@v5
        with: { go-version: 'stable', cache: true }
      # 完整 IT：testcontainers 起 PG/MySQL 容器
      - run: go test -race ./...

  build:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4
      - uses: actions/setup-go@v5
        with: { go-version: 'stable', cache: true }
      # 验证 goreleaser 配置 + 交叉编译可过（不发布）
      - uses: goreleaser/goreleaser-action@v6
        with: { version: latest, args: release --snapshot --clean }
```

要点：
- `-race` 开竞态检测（DB 连接/REPL 并发的保险）。
- `integration` 若耗时过长，可改为只在 `main` push 或 nightly 跑（`pull_request` 只跑 unit）。
- `build` 用 `--snapshot` 干跑 goreleaser，**提前暴露发布配置错误**，不等到打 tag 才发现。

### 4.3 Release 流水线（`.github/workflows/release.yml`）

```yaml
name: Release
on:
  push:
    tags: ['v*']

permissions:
  contents: write    # 创建 Release 需要
  packages: write    # 推 ghcr docker 镜像需要(可选)

jobs:
  release:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4
        with: { fetch-depth: 0 }   # goreleaser 需完整历史生成 changelog
      - uses: actions/setup-go@v5
        with: { go-version: 'stable', cache: true }

      # 发布前全量测试，防止发出坏版本
      - run: go test ./...

      - uses: goreleaser/goreleaser-action@v6
        with:
          version: latest
          args: release --clean
        env:
          GITHUB_TOKEN: ${{ secrets.GITHUB_TOKEN }}
          # v0.2 起：推送 Homebrew tap 需要有 tap 仓库写权限的 PAT
          HOMEBREW_TAP_TOKEN: ${{ secrets.HOMEBREW_TAP_TOKEN }}
```

### 4.4 goreleaser 配置（`.goreleaser.yaml`）

v0.1 最小可用版（后续渠道按阶段解注释）：

```yaml
version: 2

before:
  hooks:
    - go mod tidy

builds:
  - id: s9l
    main: ./cmd/s9l
    binary: s9l
    env: [CGO_ENABLED=0]          # 纯 Go 驱动，免 CGO，保证交叉编译与单二进制
    goos: [linux, darwin, windows]
    goarch: [amd64, arm64]
    ldflags:
      - -s -w
      - -X main.version={{.Version}}
      - -X main.commit={{.ShortCommit}}
      - -X main.date={{.Date}}

archives:
  - formats: [tar.gz]
    format_overrides:
      - goos: windows
        formats: [zip]
    name_template: "{{ .ProjectName }}_{{ .Version }}_{{ .Os }}_{{ .Arch }}"

checksum:
  name_template: 'checksums.txt'

changelog:
  sort: asc
  filters:
    exclude: ['^docs:', '^test:', '^chore:', 'Merge pull request']

release:
  github:
    owner: YangXplorer
    name: s9l
  prerelease: auto              # tag 含 -rc/-beta 自动标 prerelease

# ── v0.2 起启用：Homebrew tap ──────────────────────────────
# brews:
#   - repository:
#       owner: YangXplorer
#       name: homebrew-tap
#       token: "{{ .Env.HOMEBREW_TAP_TOKEN }}"
#     homepage: "https://github.com/YangXplorer/s9l"
#     description: "Fast terminal database client"
#     install: |
#       bin.install "s9l"

# ── v0.3 起按需启用：deb/rpm ──────────────────────────────
# nfpms:
#   - formats: [deb, rpm]
#     maintainer: YangXplorer
#     description: "Fast terminal database client"
#     license: MIT

# ── v0.3 起按需启用：docker ──────────────────────────────
# dockers:
#   - image_templates: ["ghcr.io/yangxplorer/s9l:{{ .Version }}", "ghcr.io/yangxplorer/s9l:latest"]
```

### 4.5 Secrets 配置（GitHub repo Settings → Secrets）

| Secret | 何时需要 | 说明 |
|--------|---------|------|
| `GITHUB_TOKEN` | 始终（自动注入） | 创建 Release，无需手配 |
| `HOMEBREW_TAP_TOKEN` | v0.2 起 | 一个对 `YangXplorer/homebrew-tap` 有写权限的 PAT（classic，`repo` scope 或 fine-grained 限定该仓库），让 goreleaser 推 formula |
| `GHCR`（用 `GITHUB_TOKEN`） | v0.3 docker | `packages: write` 权限即可，无需额外 PAT |

---

## 5. 分阶段落地（对应 TASKS）

| 阶段 | 交付 | 对应任务 |
|------|------|---------|
| **v0.1** | `ci.yml` + `release.yml` + `.goreleaser.yaml`(最小) → `go install` & GitHub Releases 可用 | P0-6, P1-E3 |
| **v0.2** | 启用 Homebrew tap + install.sh + 配 `HOMEBREW_TAP_TOKEN` | 新增 P2 任务 |
| **v0.3** | 按需启用 nfpm(deb/rpm) / Scoop / docker | Backlog |

## 6. 前置要求 / 风险

- 仓库已 public ✅，`go install` 天然可用。
- **必须用纯 Go 驱动**（`modernc.org/sqlite`/`pgx`/`go-sql-driver/mysql`，`CGO_ENABLED=0`），否则交叉编译与单二进制分发受阻——已在计划锁定。
- 入口固定 `cmd/s9l/`，否则 `go install .../cmd/s9l@latest` 路径不对。
- Homebrew tap 需提前建空仓库 `YangXplorer/homebrew-tap` 并配 PAT。
- `integration` job 在 CI 上的耗时需观察，过长则降频（nightly / 仅 main）。

## 7. 待确认

1. ~~许可证~~ → 已定 **MIT**，已加 `LICENSE`（Copyright 2026 YangXplorer）。
2. Docker 镜像仓库用 **ghcr.io**（与 GitHub 同源，免额外配 Docker Hub）还是 Docker Hub？默认 ghcr。
3. `integration` job 默认每个 PR 都跑，还是仅 main/nightly？默认每 PR 跑，过慢再降。
