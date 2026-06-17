# s9l

A fast terminal database client. Connect to a database and run queries with one
short command — simple to use, scriptable, and easy to extend to new databases.

> Status: early development (v0.x). SQLite, PostgreSQL and MySQL are supported
> today; more databases are on the roadmap. See [docs/PLAN.md](docs/PLAN.md).

## Features

- **Three ways to use it** — one-shot `s9l <conn> -e "SQL"` for scripts, a REPL
  with `s9l <conn>`, or a full-screen lazygit-style TUI with `s9l tui` (see below).
- **Named connections** — store connections in `~/.config/s9l/config.yaml` and
  connect with `s9l mydb`. Passwords never live in the config (see below).
- **Multiple output formats** — `--format table|json|csv|tsv`. Defaults to a
  table on a terminal and TSV when piped, so output stays parseable.
- **Metadata commands** — `\l` (databases), `\dt` (tables), `\d <table>`
  (columns), `\?` (help).
- **Query history & saved queries** — every query is recorded; favorite the
  ones you reuse.
- **Single static binary** — pure-Go drivers, no CGO.

## Install

```bash
go install github.com/YangXplorer/s9l/cmd/s9l@latest
```

Or download a prebuilt binary from the [releases page](https://github.com/YangXplorer/s9l/releases).

## Quick start

```bash
# Run a one-off query against a SQLite file
s9l ./app.db -e "select * from users limit 5"

# Pipe-friendly output (TSV when not a TTY); JSON on demand
s9l ./app.db -e "select * from users" --format json | jq '.[0]'

# Add a named connection, then use it
s9l conn add --id pg --driver postgres --host localhost --port 5432 \
    --user dev --database app --ssl --password-ref env:PGPASSWORD
s9l pg -e "select version()"

# Interactive REPL
s9l pg
s9l> \dt
s9l> select * from orders order by created_at desc limit 10;
s9l> \q
```

## Configuration

Connections live in `$XDG_CONFIG_HOME/s9l/config.yaml` (falling back to
`~/.config/s9l/config.yaml`), written with `0600` permissions:

```yaml
connections:
  - id: local
    driver: sqlite
    database: ./app.db
  - id: pg
    name: Dev Postgres
    driver: postgres
    host: localhost
    port: 5432
    user: dev
    database: app
    ssl: true
    password_ref: env:PGPASSWORD
```

**Passwords are never stored in the config.** `password_ref` points at the
secret instead:

- `env:NAME` — read from environment variable `NAME` (e.g. `env:PGPASSWORD`).
- `keychain://s9l/<key>` — read from a secret store (in-memory in v0.1; system
  keychain planned for v0.2).

Manage connections with `s9l conn add|list|rm`.

## Commands

| Command | Description |
|---------|-------------|
| `s9l <conn\|dsn> -e "SQL"` | Run a query and exit |
| `s9l <conn\|dsn>` | Start the interactive REPL |
| `s9l conn add\|list\|rm` | Manage named connections |
| `s9l history [--limit N]` | Show recent query history |
| `s9l saved add\|list\|search\|rm\|run` | Manage and run saved queries |
| `s9l tui [connection]` | Launch the full-screen TUI |
| `s9l --version` | Print version |

In the REPL / with `-e`: `\l`, `\dt`, `\d [table]`, `\?`, and `\q` (REPL quit).

Flags: `--format table|json|csv|tsv`, `--max-col-width N` (truncate table cells),
`--timeout 30s` (abort a slow query). Press `Ctrl-C` to cancel a running query.

## Terminal UI

A full-screen, lazygit-style interface — connections, schema tree, results and a
SQL editor, all keyboard-driven:

```bash
s9l tui          # choose a connection inside the UI
s9l tui mydb     # auto-connect to a named connection
```

Panels: **Connections** (from your config) · **Schema** (databases → tables) ·
**Results** · **SQL editor**. Select a table to preview it, or write SQL and run
it with F5. Queries run asynchronously and can be cancelled with Esc.

| Key | Action |
|-----|--------|
| `Tab` / `Shift-Tab` | switch panel |
| `1` / `2` / `3` / `4` | Connections / Schema / Results / SQL editor |
| `Up`/`Down` · `j`/`k` | navigate within a panel |
| `Enter` | connect (Connections) · preview table (Schema) |
| `F5` | run the SQL editor |
| `Esc` | cancel a running query |
| `Ctrl-R` | query history — `Enter` loads an entry into the editor |
| `Ctrl-F` | saved queries — `Enter` runs the selected one |
| `Ctrl-S` | save the editor's SQL as a favorite |
| `?` | help overlay |
| `q` / `Ctrl-C` | quit |

## Development

```bash
go build ./...
go test -short ./...   # unit + in-memory SQLite (no Docker)
go test ./...          # also runs container-based PostgreSQL tests (needs Docker)
golangci-lint run
```

See [docs/](docs/) for the plan ([PLAN.md](docs/PLAN.md)), task breakdown
([TASKS.md](docs/TASKS.md)), testing strategy ([TESTING.md](docs/TESTING.md)),
and release/CI design ([RELEASE.md](docs/RELEASE.md)).

## License

MIT — see [LICENSE](LICENSE).
