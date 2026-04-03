package db

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"
	"time"
)

type ProjectState struct {
	ProjectKey       string
	WatchFilter      string
	LastIssueUpdated *time.Time
}

type ObservedIssue struct {
	IssueKey               string
	IssueSummary           string
	IssueType              string
	Priority               string
	JiraStatus             string
	IssueUpdatedAt         time.Time
	NormalizedText         string
	DescriptionText        string
	AcceptanceCriteriaText string
	CommentsText           string
	Labels                 []string
	Components             []string
}

type SessionSummary struct {
	IssueKey              string
	IssueSummary          string
	State                 string
	Version               int
	IssueType             string
	Priority              string
	JiraStatus            string
	BranchName            string
	PRNumber              int
	PRURL                 string
	ImplementationSummary string
	RevisionCount         int
	Decision              string
	Reasoning             string
	LastError             string
	Confidence            float64
	UpdatedAt             time.Time
	IssueUpdatedAt        *time.Time
}

func (s *Store) ProjectState(ctx context.Context, projectKey string) (ProjectState, error) {
	var state ProjectState
	var lastUpdated string
	err := s.DB.QueryRowContext(ctx, `
		SELECT project_key, watch_filter, last_issue_updated
		FROM projects
		WHERE project_key = ?
	`, projectKey).Scan(&state.ProjectKey, &state.WatchFilter, &lastUpdated)
	if err != nil {
		return ProjectState{}, fmt.Errorf("load project state: %w", err)
	}
	if parsed, err := parseStoredTime(lastUpdated); err == nil {
		state.LastIssueUpdated = &parsed
	}
	return state, nil
}

func (s *Store) SetProjectHighWaterMark(ctx context.Context, projectKey string, updatedAt time.Time) error {
	_, err := s.DB.ExecContext(ctx, `
		UPDATE projects
		SET last_issue_updated = ?, updated_at = CURRENT_TIMESTAMP
		WHERE project_key = ?
	`, formatStoredTime(updatedAt), projectKey)
	if err != nil {
		return fmt.Errorf("set project high-water mark: %w", err)
	}
	return nil
}

func (s *Store) UpsertObservedIssue(ctx context.Context, input ObservedIssue) (bool, error) {
	labelsJSON, err := json.Marshal(input.Labels)
	if err != nil {
		return false, fmt.Errorf("marshal labels: %w", err)
	}
	componentsJSON, err := json.Marshal(input.Components)
	if err != nil {
		return false, fmt.Errorf("marshal components: %w", err)
	}

	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return false, fmt.Errorf("begin observed issue transaction: %w", err)
	}
	defer func() {
		if err != nil {
			_ = tx.Rollback()
		}
	}()

	var sessionID int64
	var existingUpdated string
	created := false

	scanErr := tx.QueryRowContext(ctx, `
		SELECT id, issue_updated_at
		FROM sessions
		WHERE issue_key = ?
	`, input.IssueKey).Scan(&sessionID, &existingUpdated)
	if scanErr != nil {
		if scanErr != sql.ErrNoRows {
			err = fmt.Errorf("load existing session: %w", scanErr)
			return false, err
		}

		result, execErr := tx.ExecContext(ctx, `
			INSERT INTO sessions (
				issue_key,
				issue_summary,
				state,
				issue_type,
				priority,
				jira_status,
				issue_updated_at
			) VALUES (?, ?, 'OBSERVED', ?, ?, ?, ?)
		`, input.IssueKey, input.IssueSummary, input.IssueType, input.Priority, input.JiraStatus, formatStoredTime(input.IssueUpdatedAt))
		if execErr != nil {
			err = fmt.Errorf("insert session: %w", execErr)
			return false, err
		}
		sessionID, err = result.LastInsertId()
		if err != nil {
			err = fmt.Errorf("read new session id: %w", err)
			return false, err
		}
		created = true
	} else {
		_, execErr := tx.ExecContext(ctx, `
			UPDATE sessions
			SET issue_summary = ?,
				issue_type = ?,
				priority = ?,
				jira_status = ?,
				issue_updated_at = ?,
				updated_at = CURRENT_TIMESTAMP
			WHERE id = ?
		`, input.IssueSummary, input.IssueType, input.Priority, input.JiraStatus, formatStoredTime(input.IssueUpdatedAt), sessionID)
		if execErr != nil {
			err = fmt.Errorf("update session: %w", execErr)
			return false, err
		}
	}

	result, execErr := tx.ExecContext(ctx, `
		INSERT INTO ticket_snapshots (
			session_id,
			issue_key,
			issue_updated_at,
			normalized_text,
			description_text,
			acceptance_criteria_text,
			comments_text,
			labels_json,
			components_json
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(issue_key, issue_updated_at) DO NOTHING
	`, sessionID, input.IssueKey, formatStoredTime(input.IssueUpdatedAt), input.NormalizedText, input.DescriptionText, input.AcceptanceCriteriaText, input.CommentsText, string(labelsJSON), string(componentsJSON))
	if execErr != nil {
		err = fmt.Errorf("insert ticket snapshot: %w", execErr)
		return false, err
	}

	rows, execErr := result.RowsAffected()
	if execErr != nil {
		err = fmt.Errorf("check snapshot insert result: %w", execErr)
		return false, err
	}

	if created {
		if execErr := s.insertEventTx(ctx, tx, sessionID, "system", "observed", "", "OBSERVED", "New ticket observed from Jira watcher"); execErr != nil {
			err = execErr
			return false, err
		}
	} else if rows > 0 && existingUpdated != formatStoredTime(input.IssueUpdatedAt) {
		if execErr := s.insertEventTx(ctx, tx, sessionID, "system", "ticket_updated", "", "", "Observed a new Jira ticket update"); execErr != nil {
			err = execErr
			return false, err
		}
	}

	if err = tx.Commit(); err != nil {
		return false, fmt.Errorf("commit observed issue transaction: %w", err)
	}

	return rows > 0, nil
}

func (s *Store) ListSessions(ctx context.Context, limit int) ([]SessionSummary, error) {
	if limit <= 0 {
		limit = 50
	}

	rows, err := s.DB.QueryContext(ctx, `
		SELECT issue_key, issue_summary, state, version, issue_type, priority, jira_status, branch_name, pr_number, pr_url, implementation_summary, revision_count, evaluation_decision, evaluation_reasoning, last_error, confidence, updated_at, issue_updated_at
		FROM sessions
		ORDER BY updated_at DESC, id DESC
		LIMIT ?
	`, limit)
	if err != nil {
		return nil, fmt.Errorf("list sessions: %w", err)
	}
	defer rows.Close()

	var sessions []SessionSummary
	for rows.Next() {
		var summary SessionSummary
		var updatedAt string
		var issueUpdatedAt string
		if err := rows.Scan(&summary.IssueKey, &summary.IssueSummary, &summary.State, &summary.Version, &summary.IssueType, &summary.Priority, &summary.JiraStatus, &summary.BranchName, &summary.PRNumber, &summary.PRURL, &summary.ImplementationSummary, &summary.RevisionCount, &summary.Decision, &summary.Reasoning, &summary.LastError, &summary.Confidence, &updatedAt, &issueUpdatedAt); err != nil {
			return nil, fmt.Errorf("scan session summary: %w", err)
		}
		if parsed, err := parseStoredTime(updatedAt); err == nil {
			summary.UpdatedAt = parsed
		}
		if parsed, err := parseStoredTime(issueUpdatedAt); err == nil {
			summary.IssueUpdatedAt = &parsed
		}
		sessions = append(sessions, summary)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate session summaries: %w", err)
	}

	return sessions, nil
}

func (s *Store) insertEventTx(ctx context.Context, tx *sql.Tx, sessionID int64, actor, eventType, fromState, toState, detail string) error {
	_, err := tx.ExecContext(ctx, `
		INSERT INTO session_events (session_id, actor, event_type, from_state, to_state, detail)
		VALUES (?, ?, ?, ?, ?, ?)
	`, sessionID, actor, eventType, fromState, toState, detail)
	if err != nil {
		return fmt.Errorf("insert session event: %w", err)
	}
	return nil
}

func formatStoredTime(t time.Time) string {
	if t.IsZero() {
		return ""
	}
	return t.UTC().Format(time.RFC3339Nano)
}

func parseStoredTime(value string) (time.Time, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return time.Time{}, fmt.Errorf("empty time")
	}
	layouts := []string{
		time.RFC3339Nano,
		time.RFC3339,
		"2006-01-02 15:04:05",
	}
	for _, layout := range layouts {
		if parsed, err := time.Parse(layout, value); err == nil {
			return parsed.UTC(), nil
		}
	}
	return time.Time{}, fmt.Errorf("unsupported time format %q", value)
}
