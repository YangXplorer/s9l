package main

import (
	"context"
	"fmt"
	"io"
	"strings"

	"github.com/YangXplorer/s9l/internal/driver"
	"github.com/YangXplorer/s9l/internal/render"
)

// runStatement runs one REPL/-e statement: a backslash meta-command (\l, \dt,
// \d, \?) or plain SQL. Returns the number of rows rendered.
func runStatement(ctx context.Context, out io.Writer, conn driver.Conn, stmt string, format render.Format) (int, error) {
	if strings.HasPrefix(strings.TrimSpace(stmt), `\`) {
		return runMeta(ctx, out, conn, strings.TrimSpace(stmt), format)
	}
	return execute(ctx, out, conn, stmt, format)
}

// runMeta handles backslash meta-commands by calling the driver's Metadata
// capability and rendering the result like any query.
func runMeta(ctx context.Context, out io.Writer, conn driver.Conn, line string, format render.Format) (int, error) {
	fields := strings.Fields(line)
	cmd := fields[0]

	if cmd == `\?` {
		_, err := io.WriteString(out, metaHelp)
		return 0, err
	}

	md, ok := conn.(driver.Metadata)
	if !ok {
		return 0, fmt.Errorf("driver does not support %s (no metadata capability)", cmd)
	}

	switch cmd {
	case `\l`:
		rows, err := md.Databases(ctx)
		return renderMeta(out, format, rows, err)
	case `\dt`:
		rows, err := md.Tables(ctx)
		return renderMeta(out, format, rows, err)
	case `\d`:
		if len(fields) < 2 {
			// \d with no argument lists tables, like psql.
			rows, err := md.Tables(ctx)
			return renderMeta(out, format, rows, err)
		}
		rows, err := md.Columns(ctx, fields[1])
		return renderMeta(out, format, rows, err)
	default:
		return 0, fmt.Errorf(`unknown command %q (try \?)`, cmd)
	}
}

func renderMeta(out io.Writer, format render.Format, rows driver.Rows, err error) (int, error) {
	if err != nil {
		return 0, err
	}
	return drainAndRender(out, format, rows)
}

const metaHelp = `commands:
  \l            list databases
  \dt           list tables
  \d [table]    list tables, or describe a table's columns
  \?            this help
  \q            quit (REPL)
`
