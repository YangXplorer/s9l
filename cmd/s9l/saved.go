package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"strconv"

	"github.com/YangXplorer/s9l/internal/history"
	"github.com/YangXplorer/s9l/internal/render"
)

// runSaved dispatches `s9l saved <add|list|search|rm|run|mv|folder|folders>`.
func runSaved(args []string, out, errOut io.Writer) error {
	if len(args) < 1 {
		return errors.New("usage: s9l saved <add|list|search|rm|run|mv|folder|folders>")
	}
	switch args[0] {
	case "add":
		return savedAdd(args[1:], errOut)
	case "list":
		return savedList(args[1:], out, errOut)
	case "search":
		return savedSearch(args[1:], out, errOut)
	case "rm":
		return savedRm(args[1:])
	case "run":
		return savedRun(args[1:], out, errOut)
	case "mv":
		return savedMv(args[1:], errOut)
	case "folder":
		return savedFolder(args[1:], errOut)
	case "folders":
		return savedFolders(out)
	default:
		return fmt.Errorf("unknown saved subcommand %q (want add|list|search|rm|run|mv|folder|folders)", args[0])
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
	fs.Int64Var(&q.FolderID, "folder", 0, "folder id to file under (0 = none)")
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

func savedList(args []string, out, errOut io.Writer) error {
	fs := flag.NewFlagSet("s9l saved list", flag.ContinueOnError)
	fs.SetOutput(errOut)
	folder := fs.Int64("folder", -1, "filter by folder id (0 = unfiled; default: all)")
	if err := fs.Parse(args); err != nil {
		return err
	}

	store, err := history.OpenDefault()
	if err != nil {
		return err
	}
	defer func() { _ = store.Close() }()

	var items []history.SavedQuery
	if *folder >= 0 {
		items, err = store.ListSavedByFolder(context.Background(), *folder)
	} else {
		items, err = store.ListSaved(context.Background())
	}
	if err != nil {
		return err
	}
	return printSaved(out, items)
}

// savedMv reassigns a saved query to a folder: `s9l saved mv <id> --folder N`
// (--folder 0 unfiles it).
func savedMv(args []string, errOut io.Writer) error {
	fs := flag.NewFlagSet("s9l saved mv", flag.ContinueOnError)
	fs.SetOutput(errOut)
	folder := fs.Int64("folder", 0, "destination folder id (0 = unfile)")
	positionals, err := parseFlagsInterspersed(fs, args)
	if err != nil {
		return err
	}
	if len(positionals) < 1 {
		return errors.New("usage: s9l saved mv <id> --folder N")
	}
	id, err := strconv.ParseInt(positionals[0], 10, 64)
	if err != nil {
		return fmt.Errorf("saved mv: invalid id %q", positionals[0])
	}

	store, err := history.OpenDefault()
	if err != nil {
		return err
	}
	defer func() { _ = store.Close() }()

	if err := store.SetSavedFolder(context.Background(), id, *folder); err != nil {
		return err
	}
	if *folder == 0 {
		_, err = fmt.Fprintf(errOut, "unfiled query #%d\n", id)
	} else {
		_, err = fmt.Fprintf(errOut, "moved query #%d to folder %d\n", id, *folder)
	}
	return err
}

// savedFolder dispatches `s9l saved folder <add|rm>`.
func savedFolder(args []string, errOut io.Writer) error {
	if len(args) < 1 {
		return errors.New("usage: s9l saved folder <add <name>|rm <id>>")
	}
	store, err := history.OpenDefault()
	if err != nil {
		return err
	}
	defer func() { _ = store.Close() }()

	switch args[0] {
	case "add":
		if len(args) != 2 {
			return errors.New("usage: s9l saved folder add <name>")
		}
		id, err := store.CreateFolder(context.Background(), args[1])
		if err != nil {
			return err
		}
		_, err = fmt.Fprintf(errOut, "folder #%d %q\n", id, args[1])
		return err
	case "rm":
		if len(args) != 2 {
			return errors.New("usage: s9l saved folder rm <id>")
		}
		id, err := strconv.ParseInt(args[1], 10, 64)
		if err != nil {
			return fmt.Errorf("saved folder rm: invalid id %q", args[1])
		}
		ok, err := store.DeleteFolder(context.Background(), id)
		if err != nil {
			return err
		}
		if !ok {
			return fmt.Errorf("folder #%d not found", id)
		}
		return nil
	default:
		return fmt.Errorf("unknown saved folder subcommand %q (want add|rm)", args[0])
	}
}

// savedFolders lists folders: `s9l saved folders`.
func savedFolders(out io.Writer) error {
	store, err := history.OpenDefault()
	if err != nil {
		return err
	}
	defer func() { _ = store.Close() }()

	folders, err := store.ListFolders(context.Background())
	if err != nil {
		return err
	}
	if len(folders) == 0 {
		_, err := fmt.Fprintln(out, "no folders")
		return err
	}
	for _, f := range folders {
		if _, err := fmt.Fprintf(out, "#%d\t%s\n", f.ID, f.Name); err != nil {
			return err
		}
	}
	return nil
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
	return runQuery(context.Background(), out, errOut, target, "sqlite", q.SQL, render.Options{Format: format}, 0)
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
		if q.FolderID != 0 {
			meta += fmt.Sprintf(" (folder %d)", q.FolderID)
		}
		if _, err := fmt.Fprintf(out, "#%d\t%s\t%s\t%s\n", q.ID, q.Title, meta, singleLine(q.SQL)); err != nil {
			return err
		}
	}
	return nil
}
