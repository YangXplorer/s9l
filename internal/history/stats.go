package history

import (
	"context"
	"fmt"
)

// ConnCount is a per-connection query count.
type ConnCount struct {
	ConnectionID string
	Count        int64
}

// QueryCount is a distinct SQL text with how often it ran and its mean duration.
type QueryCount struct {
	SQL   string
	Count int64
	AvgMs int64
}

// Stats summarizes the recorded query history.
type Stats struct {
	Total        int64
	Succeeded    int64
	Failed       int64
	AvgMs        int64
	ByConnection []ConnCount
	TopQueries   []QueryCount
}

// Stats computes aggregate statistics over query_history. topN bounds the
// most-frequent query list (<= 0 means a default of 10).
func (s *Store) Stats(ctx context.Context, topN int) (Stats, error) {
	if topN <= 0 {
		topN = 10
	}
	var st Stats

	var avg float64
	row := s.db.QueryRowContext(ctx, `SELECT
		COUNT(*),
		COALESCE(SUM(CASE WHEN success THEN 1 ELSE 0 END), 0),
		COALESCE(AVG(duration_ms), 0)
		FROM query_history`)
	if err := row.Scan(&st.Total, &st.Succeeded, &avg); err != nil {
		return Stats{}, fmt.Errorf("history: stats totals: %w", err)
	}
	st.Failed = st.Total - st.Succeeded
	st.AvgMs = int64(avg + 0.5)

	byConn, err := s.scanConnCounts(ctx, `SELECT connection_id, COUNT(*)
		FROM query_history GROUP BY connection_id ORDER BY COUNT(*) DESC, connection_id`)
	if err != nil {
		return Stats{}, err
	}
	st.ByConnection = byConn

	rows, err := s.db.QueryContext(ctx, `SELECT sql_text, COUNT(*) AS c, COALESCE(AVG(duration_ms), 0)
		FROM query_history GROUP BY sql_text ORDER BY c DESC, sql_text LIMIT ?`, topN)
	if err != nil {
		return Stats{}, fmt.Errorf("history: stats top queries: %w", err)
	}
	defer func() { _ = rows.Close() }()
	for rows.Next() {
		var q QueryCount
		var qavg float64
		if err := rows.Scan(&q.SQL, &q.Count, &qavg); err != nil {
			return Stats{}, fmt.Errorf("history: scan top query: %w", err)
		}
		q.AvgMs = int64(qavg + 0.5)
		st.TopQueries = append(st.TopQueries, q)
	}
	if err := rows.Err(); err != nil {
		return Stats{}, err
	}
	return st, nil
}

func (s *Store) scanConnCounts(ctx context.Context, query string) ([]ConnCount, error) {
	rows, err := s.db.QueryContext(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("history: stats by connection: %w", err)
	}
	defer func() { _ = rows.Close() }()
	var out []ConnCount
	for rows.Next() {
		var c ConnCount
		if err := rows.Scan(&c.ConnectionID, &c.Count); err != nil {
			return nil, fmt.Errorf("history: scan conn count: %w", err)
		}
		out = append(out, c)
	}
	return out, rows.Err()
}
