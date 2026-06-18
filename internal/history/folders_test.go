package history_test

import (
	"context"
	"errors"
	"testing"

	"github.com/YangXplorer/s9l/internal/history"
)

func TestFolderCRUDAndAssignment(t *testing.T) {
	s := openTemp(t)
	ctx := context.Background()

	// Create folders.
	reports, err := s.CreateFolder(ctx, "reports")
	if err != nil {
		t.Fatalf("create folder: %v", err)
	}
	if _, err := s.CreateFolder(ctx, "adhoc"); err != nil {
		t.Fatalf("create folder 2: %v", err)
	}

	// Empty name rejected; duplicate name rejected (UNIQUE).
	if _, err := s.CreateFolder(ctx, ""); err == nil {
		t.Error("empty folder name should error")
	}
	if _, err := s.CreateFolder(ctx, "reports"); err == nil {
		t.Error("duplicate folder name should error")
	}

	// Listed alphabetically.
	folders, err := s.ListFolders(ctx)
	if err != nil {
		t.Fatalf("list folders: %v", err)
	}
	if len(folders) != 2 || folders[0].Name != "adhoc" || folders[1].Name != "reports" {
		t.Fatalf("folders = %+v, want [adhoc, reports]", folders)
	}

	// Save filed under reports + an unfiled one.
	filed, err := s.SaveQuery(ctx, history.SavedQuery{
		Title: "daily", SQL: "SELECT 1", FolderID: reports,
	})
	if err != nil {
		t.Fatalf("save filed: %v", err)
	}
	unfiled, err := s.SaveQuery(ctx, history.SavedQuery{Title: "scratch", SQL: "SELECT 2"})
	if err != nil {
		t.Fatalf("save unfiled: %v", err)
	}

	// FolderID round-trips.
	got, err := s.GetSaved(ctx, filed)
	if err != nil || got.FolderID != reports {
		t.Fatalf("get filed: folder=%d err=%v, want %d", got.FolderID, err, reports)
	}

	// ListSavedByFolder filters.
	inReports, err := s.ListSavedByFolder(ctx, reports)
	if err != nil || len(inReports) != 1 || inReports[0].ID != filed {
		t.Fatalf("by folder reports: %+v err=%v", inReports, err)
	}
	none, err := s.ListSavedByFolder(ctx, 0)
	if err != nil || len(none) != 1 || none[0].ID != unfiled {
		t.Fatalf("unfiled: %+v err=%v", none, err)
	}

	// Move the unfiled one into reports.
	if err := s.SetSavedFolder(ctx, unfiled, reports); err != nil {
		t.Fatalf("set folder: %v", err)
	}
	inReports, _ = s.ListSavedByFolder(ctx, reports)
	if len(inReports) != 2 {
		t.Fatalf("after move: %d in reports, want 2", len(inReports))
	}

	// Errors.
	if err := s.SetSavedFolder(ctx, 9999, reports); !errors.Is(err, history.ErrNotFound) {
		t.Errorf("set folder missing query = %v, want ErrNotFound", err)
	}
	if err := s.SetSavedFolder(ctx, filed, 9999); !errors.Is(err, history.ErrFolderNotFound) {
		t.Errorf("set missing folder = %v, want ErrFolderNotFound", err)
	}
}

func TestDeleteFolderUnfilesQueries(t *testing.T) {
	s := openTemp(t)
	ctx := context.Background()

	fid, err := s.CreateFolder(ctx, "tmp")
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	qid, err := s.SaveQuery(ctx, history.SavedQuery{Title: "q", SQL: "SELECT 1", FolderID: fid})
	if err != nil {
		t.Fatalf("save: %v", err)
	}

	ok, err := s.DeleteFolder(ctx, fid)
	if err != nil || !ok {
		t.Fatalf("delete folder: ok=%v err=%v", ok, err)
	}
	// Query survives, now unfiled.
	got, err := s.GetSaved(ctx, qid)
	if err != nil {
		t.Fatalf("get after folder delete: %v", err)
	}
	if got.FolderID != 0 {
		t.Errorf("query folder = %d after folder delete, want 0", got.FolderID)
	}

	// Deleting again reports false.
	ok, _ = s.DeleteFolder(ctx, fid)
	if ok {
		t.Error("delete missing folder should report false")
	}
}
