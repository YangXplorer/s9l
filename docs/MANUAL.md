# s9l 使用说明书

> 终端数据库客户端。一条短命令连上数据库跑查询——简单、可脚本化、易扩展。
> 当前支持数据库：**SQLite · PostgreSQL · MySQL · SQL Server · ClickHouse**（纯 Go 驱动，单静态二进制，免 CGO）。
> 本文覆盖 v0.9.0 的全部命令、输出形式、全屏 TUI 与配置。命令速查见 [README](../README.md)，路线图见 [PLAN.md](./PLAN.md)。

---

## 目录

1. [安装](#1-安装)
2. [三种使用形态](#2-三种使用形态)
3. [命令与参数速查](#3-命令与参数速查)
4. [连接管理 `conn` 与配置文件](#4-连接管理-conn-与配置文件)
5. [凭据与密码](#5-凭据与密码)
6. [一次性执行 `-e`](#6-一次性执行--e)
7. [交互式 REPL](#7-交互式-repl)
8. [输出格式与分页](#8-输出格式与分页)
9. [查询历史 `history`](#9-查询历史-history)
10. [收藏与分组 `saved`](#10-收藏与分组-saved)
11. [全屏 TUI `tui`](#11-全屏-tui-tui)
12. [文件、目录与环境变量](#12-文件目录与环境变量)
13. [常见场景速查](#13-常见场景速查)

---

## 1. 安装

```bash
# Homebrew（macOS / Linux）
brew install YangXplorer/tap/s9l

# Go
go install github.com/YangXplorer/s9l/cmd/s9l@latest
```

也可从 [Releases 页面](https://github.com/YangXplorer/s9l/releases) 下载对应平台的预编译二进制（darwin/linux/windows × amd64/arm64）。

验证安装：

```bash
s9l --version       # 形如：s9l 0.9.0 (commit abc1234, built 2026-06-19)
s9l help            # 顶层帮助概览
```

---

## 2. 三种使用形态

s9l 同一个二进制提供三种用法，按使用场景选择：

| 形态 | 命令 | 适用场景 |
|------|------|----------|
| **一次性执行** | `s9l <连接|DSN> -e "SQL"` | 脚本、管道、一行查询 |
| **交互式 REPL** | `s9l <连接|DSN>` | 连续敲多条 SQL、探索数据 |
| **全屏 TUI** | `s9l tui [连接]` | lazygit 式可视化操作，键盘驱动 |

这里的 `<连接>` 既可以是 `config.yaml` 里配好的**连接 id**（如 `pg`），也可以是一个**裸 DSN**（如 SQLite 文件路径 `./app.db`）。裸 DSN 默认按 `--driver`（缺省 `sqlite`）解释。

---

## 3. 命令与参数速查

### 子命令

| 命令 | 说明 |
|------|------|
| `s9l <连接|DSN> -e "SQL"` | 跑一条 SQL 后退出 |
| `s9l <连接|DSN>` | 进入交互式 REPL |
| `s9l conn list\|add\|rm` | 管理命名连接 |
| `s9l history [--limit N]` | 查看最近查询历史 |
| `s9l history stats [--top N]` | 历史统计（计数/成功率/平均耗时/高频查询） |
| `s9l saved add\|list\|search\|rm\|run` | 管理与运行收藏查询 |
| `s9l saved folder add\|rm` · `folders` · `mv` | 收藏查询的文件夹分组 |
| `s9l import <连接|DSN> --table T --file f` | 批量导入 CSV/JSON 到表 |
| `s9l tui [连接]` | 启动全屏 TUI |
| `s9l help` · `-h` · `--help` | 顶层帮助 |
| `s9l --version` | 打印版本 |

### 查询相关参数（用于 `-e` / REPL / `saved run`）

| 参数 | 含义 |
|------|------|
| `-e "SQL"` | 执行 SQL 后退出（不带则进 REPL） |
| `--driver NAME` | 裸 DSN 使用的驱动，默认 `sqlite`（可选 `sqlite`/`postgres`/`mysql`/`sqlserver`/`clickhouse`） |
| `--format FMT` | 输出格式：`table` \| `json` \| `csv` \| `tsv`。默认：TTY→`table`，管道→`tsv` |
| `--max-col-width N` | 把表格单元格截断到 N 个字符（仅 `table` 格式；0=不限） |
| `--timeout DUR` | 超过该时长则中断查询（如 `30s`；0=不限）。`Ctrl-C` 也能取消 |
| `--no-pager` | 在终端上不使用 `$PAGER` 分页 |

> 参数可放在连接/DSN 的前面或后面，s9l 会自动识别（例如 `s9l --format json db.sqlite -e "..."` 与 `s9l db.sqlite -e "..." --format json` 等价）。

---

## 4. 连接管理 `conn` 与配置文件

### 4.1 命令

```bash
# 列出已配置的连接
s9l conn list

# 新增一个连接
s9l conn add --id pg --driver postgres --host localhost --port 5432 \
    --user dev --database app --ssl --password-ref env:PGPASSWORD

# 删除连接（同时清掉它在系统 keychain 里的密码）
s9l conn rm pg
```

`conn add` 的参数：

| 参数 | 必填 | 说明 |
|------|:---:|------|
| `--id` | ✅ | 连接 id（唯一），后续 `s9l <id>` 用它引用 |
| `--driver` | ✅ | `sqlite` / `postgres` / `mysql` / `sqlserver` / `clickhouse` |
| `--name` | | 显示名（备注用） |
| `--host` `--port` `--user` | | 网络数据库的连接信息 |
| `--database` | | 数据库名；**SQLite 时填文件路径** |
| `--ssl` | | 启用 SSL/TLS（postgres→`sslmode=require`，mysql→`tls=true`） |
| `--charset` | | 字符集（mysql 用） |
| `--password` | | 把密码**存入系统 keychain**，并自动设置 `password_ref` |
| `--password-ref` | | 直接指定密码引用，如 `env:PGPASSWORD` 或 `keychain://s9l/...` |
| `--ssl-mode` | | TLS 模式（比 `--ssl` 更细）：postgres `disable\|require\|verify-ca\|verify-full`；mysql `require\|skip-verify\|preferred`；sqlserver `disable\|require\|verify-full` |
| `--tls-ca` | | CA 证书文件（postgres、sqlserver） |
| `--tls-cert` `--tls-key` | | 客户端证书/私钥（仅 postgres） |

> **TLS 细化**：`--ssl` 是开关（等价 `ssl-mode` 的 require/verify 默认）；`--ssl-mode` 精确控制。
> `ssl: true` 行为不变（postgres=require、mysql=tls、sqlserver=encrypt 并验证证书）。
> mysql 的自定义 CA/客户端证书需用裸 DSN（`RegisterTLSConfig`），config 仅支持其内置模式。

### 4.2 配置文件

连接保存在 `$XDG_CONFIG_HOME/s9l/config.yaml`（回退 `~/.config/s9l/config.yaml`），权限 `0600`。可手动编辑：

```yaml
connections:
  - id: local            # SQLite：database 是文件路径
    driver: sqlite
    database: ./app.db

  - id: pg               # PostgreSQL
    name: Dev Postgres
    driver: postgres
    host: localhost
    port: 5432
    user: dev
    database: app
    ssl: true
    password_ref: env:PGPASSWORD

  - id: my               # MySQL
    driver: mysql
    host: 127.0.0.1
    port: 3306
    user: root
    database: shop
    charset: utf8mb4
    password_ref: keychain://s9l/connection.my.password

  - id: ms               # SQL Server
    driver: sqlserver
    host: localhost
    port: 1433
    user: sa
    database: app
    password_ref: env:MSSQL_PASSWORD

  - id: ch               # ClickHouse
    driver: clickhouse
    host: localhost
    port: 9000
    user: default
    database: app
    password_ref: env:CLICKHOUSE_PASSWORD
```

> **配置文件里绝不存明文密码**，只存 `password_ref`（见下一节）。

各驱动生成的 DSN 形式（自动构建，了解即可）：
- **sqlite**：`database` 即文件路径。
- **postgres**：`postgres://user:pass@host:port/db?sslmode=require|disable`。
- **mysql**：`user:pass@tcp(host:port)/db?parseTime=true[&tls=true][&charset=...]`。
- **sqlserver**：`sqlserver://user:pass@host:port?database=db&encrypt=disable|true`。
- **clickhouse**：`clickhouse://user:pass@host:9000/db[?secure=true]`。

---

## 5. 凭据与密码

密码永远不进 `config.yaml`。连接通过 `password_ref` 指向真正的密钥，支持两种形式：

| 形式 | 含义 | 例子 |
|------|------|------|
| `env:NAME` | 从环境变量 `NAME` 读取 | `env:PGPASSWORD` |
| `keychain://s9l/<key>` | 从系统 keychain 读取（macOS Keychain / Windows 凭据管理器 / Linux Secret Service） | `keychain://s9l/connection.pg.password` |

把密码存进 keychain（推荐，不留痕）：

```bash
s9l conn add --id pg --driver postgres --host localhost --user dev \
    --database app --password 'your-password'
# password_ref 会被自动设为 keychain://...，明文只进 keychain，不进 config.yaml
```

> 在命令行直接传 `--password` 可能落进 shell 历史；CI/脚本里更推荐用 `--password-ref env:PGPASSWORD`，把密码放到环境变量。

---

## 6. 一次性执行 `-e`

适合脚本与管道：

```bash
# SQLite 文件，一次性查询
s9l ./app.db -e "select * from users limit 5"

# 命名连接
s9l pg -e "select version()"

# 管道：非 TTY 默认输出 tsv（可解析）；按需切 json 喂给 jq
s9l ./app.db -e "select * from users" --format json | jq '.[0]'

# 限时 + 裸 DSN 指定驱动
s9l "postgres://dev@localhost/app?sslmode=disable" --driver postgres \
    -e "select count(*) from orders" --timeout 10s
```

要点：
- 多条以 `;` 分隔的语句会依次执行。
- DDL/DML（无结果集）不打印空表格，只是静默成功。
- 每次执行都会被记入历史（见 §9）。
- `Ctrl-C` 取消正在跑的查询。

---

## 7. 交互式 REPL

不带 `-e` 直接进入 REPL：

```text
$ s9l pg
s9l> select * from orders order by created_at desc limit 10;
... 结果表格 ...
s9l> \dt
s9l> \q
```

### 7.1 输入规则
- 以 `;` 结束一条 SQL；可跨多行输入，遇到 `;` 才执行。
- `\q` / `quit` / `exit` 退出 REPL。
- `Ctrl-C` 丢弃当前正在输入的内容 / 取消正在跑的查询（**不退出**会话）。
- `Ctrl-D`（EOF）退出。

### 7.2 反斜杠元命令

| 命令 | 作用 |
|------|------|
| `\l` | 列出数据库 |
| `\dt` | 列出当前库的表 |
| `\d [表名]` | 不带参数=列出表；带表名=查看该表的列结构 |
| `\?` | 元命令帮助 |
| `\q` | 退出 REPL |

> 元命令依赖驱动的元数据能力；SQLite/PostgreSQL/MySQL/SQL Server/ClickHouse 均已支持。

### 7.3 Tab 自动补全

在 REPL 里按 `Tab` 补全：
- **SQL 关键字**（`SELECT`/`FROM`/`WHERE`/`JOIN` 等）；
- **表名**（来自当前连接的元数据）；
- **列名**：支持 `表名.列前缀` 限定补全；也会把当前语句里**出现过的表**的列纳入候选；
- **反斜杠命令**（输入 `\` 开头时）。

补全的 schema 会缓存到 `~/.cache/s9l/schema.db`（仅命名连接），所以**跨会话仍然可用**，即使某次元数据查询失败也能用上次的结果（详见 §12）。

---

## 8. 输出格式与分页

### 8.1 四种格式

用 `--format` 选择，默认随是否为终端自动决定（TTY→`table`，管道→`tsv`）。

- **`table`**：对齐的表格，适合人眼阅读。NULL 显示为空。

  ```text
  id | name  | email
  ---+-------+------------------
   1 | Alice | alice@example.com
   2 | Bob   |
  ```

- **`json`**：保留列顺序的对象数组，NULL→`null`。

  ```json
  [{"id":1,"name":"Alice","email":"alice@example.com"},
   {"id":2,"name":"Bob","email":null}]
  ```

- **`csv`** / **`tsv`**：逗号 / 制表符分隔，NULL→空字段。适合喂给其他工具或导出。

`--max-col-width N` 仅作用于 `table`，把过宽单元格截断（结尾用 `…`），机器格式（json/csv/tsv）不截断以保证数据完整。

### 8.2 大结果分页（pager）

在**终端**上，输出会经过 `$PAGER` 分页，默认 `less -FIRX`：
- `-F` 表示**一屏装得下就直接打印**，不进入全屏 pager——小结果体验和不分页一样；
- 大结果可上下滚动浏览，按 `q` 退出 pager（提前退出不算查询出错）。

控制方式：

| 想要 | 做法 |
|------|------|
| 换 pager | `export PAGER='less -S'` 或 `export S9L_PAGER='bat'` |
| 本次不分页 | 加 `--no-pager` |
| 永久禁用 | `export S9L_PAGER=`（设为空值） |

> 管道 / 重定向 / 非 TTY 场景**绝不分页**，脚本行为不受影响。`S9L_PAGER` 优先级高于 `PAGER`。

---

## 9. 查询历史 `history`

每条执行过的查询（成功或失败）都会自动记录到 `~/.config/s9l/history.db`。

```bash
s9l history              # 最近 20 条
s9l history --limit 100  # 最近 100 条
s9l history --limit 0    # 全部
```

输出每行（制表符分隔）：

```text
2026-06-18 14:03:21   ok    12ms   pg    select * from orders limit 10
2026-06-18 14:02:55   ERR   3ms    pg    select * from no_such_table
```

字段依次为：**执行时间 · 状态(ok/ERR) · 耗时(ms) · 连接 id · SQL（折成单行）**。

统计（只读聚合本地历史）：

```bash
s9l history stats            # 默认 Top 10 高频查询
s9l history stats --top 20
```

输出：总数 / 成功 / 失败 / 成功率 / 平均耗时；**按连接计数**；**最高频查询**（次数 · 平均耗时 · SQL）。

---

## 10. 收藏与分组 `saved`

把常用查询存下来，随时按 id 运行；可用文件夹归类。

### 10.1 收藏查询

```bash
# 新增收藏
s9l saved add --title "日活" --conn pg --sql "select count(*) from active_users" \
    --tags "metrics,daily"

# 列出 / 搜索（按 标题/标签/SQL 模糊匹配，可加 --conn 过滤）
s9l saved list
s9l saved search daily
s9l saved search orders --conn pg

# 运行第 1 条收藏（可临时换连接 / 换格式）
s9l saved run 1
s9l saved run 1 --conn pg --format json

# 删除
s9l saved rm 1
```

`saved list` / `search` 每行：`#id  标题  连接 [标签] (folder N)  SQL`。

`saved add` 参数：`--title`（必填）、`--sql`（必填）、`--desc`、`--conn`、`--db`、`--tags`、`--folder N`。

### 10.2 文件夹分组

```bash
# 新建 / 列出 / 删除文件夹
s9l saved folder add reports     # 输出：folder #1 "reports"
s9l saved folders                # 列出所有文件夹
s9l saved folder rm 1            # 删文件夹（里面的查询不删，只是变成“未归档”）

# 新增时直接归档到文件夹 1
s9l saved add --title "月报" --sql "select ..." --folder 1

# 按文件夹过滤；--folder 0 表示“未归档”
s9l saved list --folder 1
s9l saved list --folder 0

# 把第 2 条收藏移动到文件夹 1；--folder 0 取消归档
s9l saved mv 2 --folder 1
s9l saved mv 2 --folder 0
```

> 删除文件夹**不会**删除其中的收藏查询，只是把它们重置为未归档，避免误删。

---

## 11. 全屏 TUI `tui`

lazygit 式的全屏界面，全键盘操作：

```bash
s9l tui          # 进界面后再选连接
s9l tui pg       # 直接连上命名连接 pg
```

界面使用**终端自身的背景色**（像 lazygit 一样融入终端）；面板带圆角、序号，聚焦面板高亮，底部是快捷键栏。设 `NO_COLOR` 关闭配色。

### 11.1 界面布局

```text
┌─[1] Connections ─────┬─[3] Results ──────────────────────────────┐
│ ▾ [my] neohub        │ id | name  | email                        │
│     app              │ ---+-------+-------------------            │
│   ▸ logs             │  1 | Alice | alice@example.com            │
│ ▸ [pg] dev           │  2 | Bob   |                              │
├─[2] Schema ──────────┤                                           │
│ /tbl: ord ── 2/18    │                                           │
│   orders             ├─[4] SQL (F5 run) ─────────────────────────┤
│   order_items        │ select * from orders                      │
│                      │ limit 200;                                │
└──────────────────────┴───────────────────────────────────────────┘
 [状态栏]  [快捷键栏：Tab panel  n new  / filter  ^R history  ? help  q quit]
```

四个面板：
- **Connections（左上）**：`config.yaml` 里的连接树（无树形连线；可展开的连接前显示开合三角 `▾`/`▸`；`↑`/`↓` 或 `j`/`k` 上下选择）。每行 `图标 + 名称`（图标按驱动 `[pg]/[my]/[sq]/[ms]`，`S9L_TUI_ICONS=nerd` 用 Nerd Font 字形、`=off` 关闭）。`Enter` 连接；对**多库引擎（MySQL）会展开其数据库列表**，再 `Enter` 选中某数据库 → 刷新 Schema 为该库的表（解决“连接没指定默认库时看不到表”）。单库引擎（SQLite/PostgreSQL/SQL Server）直接列当前库的表。
- **Schema（左下）**：当前所选数据库的**表列表**。`Enter` 预览选中表（自动按方言取前 200 行）。按 `/` **检索表名**（子串、大小写不敏感，状态栏显示 `tables M/N`）。
- **Results（右上）**：查询/预览结果表格。按 `/` **过滤结果行**（跨列子串）。
- **SQL (F5 run)（右下）**：SQL 编辑器，`F5` 执行。
- **状态栏 + 快捷键栏（最底部两行）**：当前连接/库、行数/耗时、错误（连接失败等只进状态栏不崩溃）；下面一行常驻快捷键。

### 11.2 按键

| 按键 | 作用 |
|------|------|
| `Tab` / `Shift-Tab` | 在面板间切换焦点 |
| `1` / `2` / `3` / `4` | 跳到 Connections / Schema / Results / SQL 编辑器 |
| `↑`/`↓` 或 `j`/`k` | 在当前面板内上下移动（编辑器内 `j`/`k` 是普通文本） |
| `Enter` | Connections=连接并展开数据库 / 选中数据库刷新 Schema；Schema=预览选中表 |
| `n` / `e` / `d` | 在 Connections 面板：新增 / 编辑 / 删除连接（密码存系统 keychain） |
| `/` | 检索：Schema 聚焦时过滤**表名**，否则过滤**结果行**（`Enter` 保留、`Esc` 清空） |
| `F5` | 执行 SQL 编辑器里的语句 |
| `Esc` | 取消正在执行的查询 / 关闭浮层 / 清空过滤 |
| `Ctrl-R` | 打开查询历史；`Enter` 把选中项**载入编辑器** |
| `Ctrl-F` | 打开收藏查询；`Enter` 直接**运行**选中项 |
| `Ctrl-S` | 把编辑器里的 SQL **存为收藏** |
| `Ctrl-E` | 把当前结果**导出到文件**（按扩展名 .csv/.json/.tsv 选格式） |
| `?` | 显示/关闭帮助浮层 |
| `q` / `Ctrl-C` | 退出 TUI |

### 11.3 在界面里管理连接
- `n` 打开「新增连接」表单（id/name/driver 下拉/host/port/user/database/ssl/password 或 password-ref）；填好 **Save** 即写入 `config.yaml`，填了密码则存入系统 keychain（配置只留引用）。
- `e` 编辑选中连接（预填；密码留空=保留原引用）；`d` 删除（确认弹窗，连同 keychain 密码）。

### 11.4 典型操作流
1. `s9l tui neohub-dev` 进界面（自动连接）。
2. MySQL：Connections 下展开出数据库，选一个库（`Enter`）→ Schema 列出该库的表；单库引擎直接出表。
3. 焦点切到 Schema（`2`），需要时按 `/` 检索表名，选表 `Enter` 预览到 Results。
4. 或切到 SQL 编辑器（`4`），写 SQL，`F5` 运行；结果里按 `/` 过滤行。
5. `Ctrl-R` 调历史、`Ctrl-F` 跑收藏、`Ctrl-S` 存收藏；查询太久 `Esc` 取消；`q` 退出。

> 查询在后台异步执行，期间界面不卡顿，可用 `Esc` 取消。

---

## 12. 文件、目录与环境变量

### 文件位置

| 用途 | 路径 | 说明 |
|------|------|------|
| 连接配置 | `$XDG_CONFIG_HOME/s9l/config.yaml`（回退 `~/.config/s9l/config.yaml`） | 权限 0600，无明文密码 |
| 查询历史 + 收藏 | `~/.config/s9l/history.db`（同样遵循 `$XDG_CONFIG_HOME`） | SQLite |
| 补全 schema 缓存 | `$XDG_CACHE_HOME/s9l/schema.db`（回退 `~/.cache/s9l/schema.db`） | SQLite，仅命名连接，加速并兜底补全 |
| 密码 | 系统 keychain | 仅当用 `keychain://` 引用时 |

### 环境变量

| 变量 | 作用 |
|------|------|
| `XDG_CONFIG_HOME` | 配置/历史目录根（默认 `~/.config`） |
| `XDG_CACHE_HOME` | 缓存目录根（默认 `~/.cache`） |
| `PAGER` | 分页器命令（默认 `less -FIRX`） |
| `S9L_PAGER` | 覆盖 `PAGER`；设为空值则禁用分页 |
| `NO_COLOR` | 关闭 TUI 配色（背景/文字用终端默认） |
| `S9L_TUI_ICONS` | TUI 连接图标：默认 ASCII 标签；`nerd`=Nerd Font 字形；`off`=不显示 |
| `password_ref` 里的 `env:NAME` | 提供数据库密码的环境变量 |

---

## 13. 常见场景速查

```bash
# 1. 快速看一眼 SQLite 文件里的表
s9l ./app.db -e "\dt"

# 2. 导出查询结果为 CSV
s9l pg -e "select * from orders" --format csv > orders.csv

# 3. 把查询结果喂给 jq
s9l pg -e "select id,email from users" --format json | jq -r '.[].email'

# 4. 存一个常用查询并随时跑
s9l saved add --title "今日订单" --conn pg --sql "select * from orders where created_at::date = current_date"
s9l saved run 1

# 5. 给团队成员配一个走环境变量密码的连接（不落明文）
s9l conn add --id prod --driver postgres --host db.prod --user app \
    --database app --ssl --password-ref env:PGPASSWORD

# 6. 可视化探索：进 TUI 选连接、点表预览、写 SQL F5 运行
s9l tui prod

# 7. 防止误跑超长查询
s9l prod -e "select * from huge_table" --timeout 30s

# 8. 批量导入 CSV / JSON 到已存在的表
s9l import prod --table users --file users.csv
s9l import prod --table events --file events.json --batch 1000
```

> **导入 `import`**：`s9l <连接|DSN> import --table T --file f [--format csv|json] [--batch N]`。
> CSV 首行为列名、其余为字符串值；JSON 为对象数组（列取首个对象的键、排序；缺失键→NULL）。
> 按 `--batch`（默认 500）分批多行 INSERT，占位符/标识符按方言自动适配。
> 表需**预先存在**；导入中途出错会报告已成功行数（无整体事务回滚）。

---

更多文档：[README](../README.md) · 计划 [PLAN.md](./PLAN.md) · 任务分解 [TASKS.md](./TASKS.md) · 测试 [TESTING.md](./TESTING.md) · 发布 [RELEASE.md](./RELEASE.md) · TUI 设计 [TUI.md](./TUI.md)。
