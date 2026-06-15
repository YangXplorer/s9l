package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"strconv"

	"github.com/YangXplorer/s9l/internal/history"
)

// runSaved dispatches `s9l saved <add|list|search|rm|run>`.
func runSaved(args []string, out, errOut io.Writer) error {
	if len(args) < 1 {
		return errors.New("usage: s9l saved <add|list|search|rm|run>")
	}
	switch args[0] {
	case "add":
		return savedAdd(args[1:], errOut)
	case "list":
		return savedList(out)
	case "search":
		return savedSearch(args[1:], out, errOut)
	case "rm":
		return savedRm(args[1:])
	case "run":
		return savedRun(args[1:], out, errOut)
	default:
		return fmt.Errorf("unknown saved subcommand %q (want add|list|search|rm|run)", args[0])
	}
}

func savedAdd(args []string, errOut io.Writer) error {
	fs := flag.NewFlagSet("s9l saved add", flag.ContinueOnError)
	fs.SetOutput(errOut)
	var q history.SavedQuery
	fs.StringVar(&q.Title, "title", "", "title (required)")
	fs.StringVar(&q.Description, "desc", "", "description")
	fs.StringVar(&q.ConnectionID, "conn", "", "connection id to run against")
	fs.StringVar(&q.DatabaseName, "db", "", "database name")
	fs.StringVar(&q.SQL, "sql", "", "SQL text (required)")
	fs.StringVar(&q.Tags, "tags", "", "comma-separated tags")
	if err := fs.Parse(args); err != nil {
		return err
	}

	store, err := history.OpenDefault()
	if err != nil {
		return err
	}
	defer func() { _ = store.Close() }()

	id, err := store.SaveQuery(context.Background(), q)
	if err != nil {
		return err
	}
	_, err = fmt.Fprintf(errOut, "saved query #%d\n", id)
	return err
}

func savedList(out io.Writer) error {
	store, err := history.OpenDefault()
	if err != nil {
		return err
	}
	defer func() { _ = store.Close() }()

	items, err := store.ListSaved(context.Background())
	if err != nil {
		return err
	}
	return printSaved(out, items)
}

func savedSearch(args []string, out, errOut io.Writer) error {
	fs := flag.NewFlagSet("s9l saved search", flag.ContinueOnError)
	fs.SetOutput(errOut)
	conn := fs.String("conn", "", "filter by connection id")
	positionals, err := parseFlagsInterspersed(fs, args)
	if err != nil {
		return err
	}
	if len(positionals) < 1 {
		return errors.New("usage: s9l saved search <term> [--conn id]")
	}

	store, err := history.OpenDefault()
	if err != nil {
		return err
	}
	defer func() { _ = store.Close() }()

	items, err := store.SearchSaved(context.Background(), positionals[0], *conn)
	if err != nil {
		return err
	}
	return printSaved(out, items)
}

func savedRm(args []string) error {
	if len(args) != 1 {
		return errors.New("usage: s9l saved rm <id>")
	}
	id, err := strconv.ParseInt(args[0], 10, 64)
	if err != nil {
		return fmt.Errorf("saved rm: invalid id %q", args[0])
	}

	store, err := history.OpenDefault()
	if err != nil {
		return err
	}
	defer func() { _ = store.Close() }()

	ok, err := store.DeleteSaved(context.Background(), id)
	if err != nil {
		return err
	}
	if !ok {
		return fmt.Errorf("saved query #%d not found", id)
	}
	return nil
}

func savedRun(args []string, out, errOut io.Writer) error {
	fs := flag.NewFlagSet("s9l saved run", flag.ContinueOnError)
	fs.SetOutput(errOut)
	connOverride := fs.String("conn", "", "override the connection id to run against")
	formatFlag := fs.String("format", "", "output format: table|json|csv|tsv")
	positionals, err := parseFlagsInterspersed(fs, args)
	if err != nil {
		return err
	}
	if len(positionals) < 1 {
		return errors.New("usage: s9l saved run <id> [--conn id] [--format fmt]")
	}
	id, err := strconv.ParseInt(positionals[0], 10, 64)
	if err != nil {
		return fmt.Errorf("saved run: invalid id %q", positionals[0])
	}

	store, err := history.OpenDefault()
	if err != nil {
		return err
	}
	q, err := store.GetSaved(context.Background(), id)
	_ = store.Close()
	if err != nil {
		return err
	}

	target := q.ConnectionID
	if *connOverride != "" {
		target = *connOverride
	}
	if target == "" {
		return fmt.Errorf("saved query #%d has no connection; pass --conn", id)
	}

	format, err := outputFormat(*formatFlag, out)
	if err != nil {
		return err
	}
	return runQuery(context.Background(), out, errOut, target, "sqlite", q.SQL, format)
}

func printSaved(out io.Writer, items []history.SavedQuery) error {
	if len(items) == 0 {
		_, err := fmt.Fprintln(out, "no saved queries")
		return err
	}
	for _, q := range items {
		meta := q.ConnectionID
		if q.Tags != "" {
			meta += " [" + q.Tags + "]"
		}
		if _, err := fmt.Fprintf(out, "#%d\t%s\t%s\t%s\n", q.ID, q.Title, meta, singleLine(q.SQL)); err != nil {
			return err
		}
	}
	return nil
}
