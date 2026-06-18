package history_test

import (
	"context"
	"testing"
	"time"

	"github.com/YangXplorer/s9l/internal/history"
)

func TestStats(t *testing.T) {
	s := openTemp(t)
	ctx := context.Background()

	add := func(conn, sql string, ms int64, ok bool) {
		t.Helper()
		if _, err := s.AddHistory(ctx, history.HistoryEntry{
			ConnectionID: conn, SQL: sql, ExecutedAt: time.Now(),
			Duration: time.Duration(ms) * time.Millisecond, Success: ok,
		}); err != nil {
			t.Fatalf("add: %v", err)
		}
	}
	// pg: SELECT 1 ×2 (10ms, 30ms), SELECT 2 ×1 (fail). my: SELECT 1 ×1.
	add("pg", "SELECT 1", 10, true)
	add("pg", "SELECT 1", 30, true)
	add("pg", "SELECT 2", 5, false)
	add("my", "SELECT 1", 20, true)

	st, err := s.Stats(ctx, 10)
	if err != nil {
		t.Fatalf("stats: %v", err)
	}
	if st.Total != 4 || st.Succeeded != 3 || st.Failed != 1 {
		t.Errorf("totals = %+v, want total 4 / ok 3 / err 1", st)
	}
	// avg of 10,30,5,20 = 16.25 → 16
	if st.AvgMs != 16 {
		t.Errorf("avg = %dms, want 16", st.AvgMs)
	}

	// by connection: pg(3) before my(1).
	if len(st.ByConnection) != 2 || st.ByConnection[0].ConnectionID != "pg" || st.ByConnection[0].Count != 3 {
		t.Fatalf("by connection = %+v, want pg=3 first", st.ByConnection)
	}

	// top queries: "SELECT 1" has 3 occurrences (most frequent), avg of 10,30,20 = 20.
	top := st.TopQueries[0]
	if top.SQL != "SELECT 1" || top.Count != 3 || top.AvgMs != 20 {
		t.Errorf("top query = %+v, want SELECT 1 ×3 @20ms", top)
	}
}

func TestStatsEmpty(t *testing.T) {
	s := openTemp(t)
	st, err := s.Stats(context.Background(), 10)
	if err != nil {
		t.Fatalf("stats: %v", err)
	}
	if st.Total != 0 || len(st.ByConnection) != 0 || len(st.TopQueries) != 0 {
		t.Errorf("empty stats = %+v, want zero", st)
	}
}
