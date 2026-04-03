package db

import (
	"context"
	"fmt"
	"sort"
)

type MetricsSnapshot struct {
	SessionsTotal            int
	SessionsByState          map[string]int
	EvaluationResultsTotal   int
	ClarificationCyclesTotal int
	OpencodeRunsTotal        int
	OpencodeRunsSuccess      int
	WorktreesTotal           int
}

func (s *Store) RecoverInterruptedSessions(ctx context.Context) error {
	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin restart recovery transaction: %w", err)
	}
	defer func() {
		if err != nil {
			_ = tx.Rollback()
		}
	}()

	rows, err := tx.QueryContext(ctx, `SELECT id, state FROM sessions WHERE state IN ('IMPLEMENTING', 'REVISING')`)
	if err != nil {
		return fmt.Errorf("query interrupted sessions: %w", err)
	}
	defer rows.Close()

	type interrupted struct {
		id    int64
		state string
	}
	var sessions []interrupted
	for rows.Next() {
		var item interrupted
		if err := rows.Scan(&item.id, &item.state); err != nil {
			return fmt.Errorf("scan interrupted session: %w", err)
		}
		sessions = append(sessions, item)
	}
	if err := rows.Err(); err != nil {
		return fmt.Errorf("iterate interrupted sessions: %w", err)
	}

	for _, session := range sessions {
		_, err = tx.ExecContext(ctx, `
			UPDATE sessions
			SET state = 'FAILED',
				last_error = 'archon restarted during active execution',
				updated_at = CURRENT_TIMESTAMP
			WHERE id = ?
		`, session.id)
		if err != nil {
			return fmt.Errorf("mark interrupted session failed: %w", err)
		}
		if err := s.insertEventTx(ctx, tx, session.id, "system", "restart_recovery", session.state, "FAILED", "Archon restarted while execution was in progress"); err != nil {
			return err
		}
	}

	if err = tx.Commit(); err != nil {
		return fmt.Errorf("commit restart recovery transaction: %w", err)
	}
	if len(sessions) > 0 {
		s.log.Warn("recovered interrupted sessions", "count", len(sessions))
	}
	return nil
}

func (s *Store) MetricsSnapshot(ctx context.Context) (MetricsSnapshot, error) {
	snapshot := MetricsSnapshot{SessionsByState: map[string]int{}}
	queries := []struct {
		query  string
		target *int
	}{
		{`SELECT COUNT(1) FROM sessions`, &snapshot.SessionsTotal},
		{`SELECT COUNT(1) FROM evaluation_results`, &snapshot.EvaluationResultsTotal},
		{`SELECT COUNT(1) FROM clarification_cycles`, &snapshot.ClarificationCyclesTotal},
		{`SELECT COUNT(1) FROM opencode_runs`, &snapshot.OpencodeRunsTotal},
		{`SELECT COUNT(1) FROM opencode_runs WHERE status = 'success'`, &snapshot.OpencodeRunsSuccess},
		{`SELECT COUNT(1) FROM worktrees`, &snapshot.WorktreesTotal},
	}
	for _, item := range queries {
		if err := s.DB.QueryRowContext(ctx, item.query).Scan(item.target); err != nil {
			return MetricsSnapshot{}, fmt.Errorf("collect metrics snapshot: %w", err)
		}
	}

	rows, err := s.DB.QueryContext(ctx, `SELECT state, COUNT(1) FROM sessions GROUP BY state`)
	if err != nil {
		return MetricsSnapshot{}, fmt.Errorf("collect session state counts: %w", err)
	}
	defer rows.Close()
	for rows.Next() {
		var state string
		var count int
		if err := rows.Scan(&state, &count); err != nil {
			return MetricsSnapshot{}, fmt.Errorf("scan session state count: %w", err)
		}
		snapshot.SessionsByState[state] = count
	}
	if err := rows.Err(); err != nil {
		return MetricsSnapshot{}, fmt.Errorf("iterate session state counts: %w", err)
	}
	return snapshot, nil
}

func SortedStates(counts map[string]int) []string {
	states := make([]string, 0, len(counts))
	for state := range counts {
		states = append(states, state)
	}
	sort.Strings(states)
	return states
}
