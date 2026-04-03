package db

import (
	"context"
	"encoding/json"
	"fmt"
	"time"
)

type ImplementationInput struct {
	IssueKey            string
	IssueSummary        string
	NormalizedText      string
	SuggestedScope      string
	ImplementationNotes []string
	OutOfScope          []string
	RevisionFeedback    string
	BranchName          string
	WorktreePath        string
	ExistingBranch      bool
}

type ImplementationSuccess struct {
	PromptSHA           string
	Stdout              string
	StartedAt           time.Time
	FinishedAt          time.Time
	Summary             string
	BranchName          string
	WorktreePath        string
	VerificationSummary string
	PRNumber            int
	PRURL               string
}

type ImplementationFailure struct {
	PromptSHA    string
	Stdout       string
	ErrorText    string
	StartedAt    time.Time
	FinishedAt   time.Time
	BranchName   string
	WorktreePath string
}

func (s *Store) ListSessionsByState(ctx context.Context, state string, limit int) ([]SessionSummary, error) {
	if limit <= 0 {
		limit = 20
	}
	rows, err := s.DB.QueryContext(ctx, `
		SELECT issue_key, issue_summary, state, version, issue_type, priority, jira_status, branch_name, pr_number, pr_url, implementation_summary, revision_count, evaluation_decision, evaluation_reasoning, last_error, confidence, updated_at, issue_updated_at
		FROM sessions
		WHERE state = ?
		ORDER BY updated_at ASC, id ASC
		LIMIT ?
	`, state, limit)
	if err != nil {
		return nil, fmt.Errorf("list sessions by state: %w", err)
	}
	defer rows.Close()
	var sessions []SessionSummary
	for rows.Next() {
		var summary SessionSummary
		var updatedAt string
		var issueUpdatedAt string
		if err := rows.Scan(&summary.IssueKey, &summary.IssueSummary, &summary.State, &summary.Version, &summary.IssueType, &summary.Priority, &summary.JiraStatus, &summary.BranchName, &summary.PRNumber, &summary.PRURL, &summary.ImplementationSummary, &summary.RevisionCount, &summary.Decision, &summary.Reasoning, &summary.LastError, &summary.Confidence, &updatedAt, &issueUpdatedAt); err != nil {
			return nil, fmt.Errorf("scan session by state: %w", err)
		}
		if parsed, err := parseStoredTime(updatedAt); err == nil {
			summary.UpdatedAt = parsed
		}
		if parsed, err := parseStoredTime(issueUpdatedAt); err == nil {
			summary.IssueUpdatedAt = &parsed
		}
		sessions = append(sessions, summary)
	}
	return sessions, rows.Err()
}

func (s *Store) LoadImplementationInput(ctx context.Context, issueKey string) (ImplementationInput, error) {
	var input ImplementationInput
	err := s.DB.QueryRowContext(ctx, `
		SELECT se.issue_key, se.issue_summary, se.suggested_scope, se.branch_name, se.revision_feedback, ts.normalized_text
		FROM sessions se
		JOIN ticket_snapshots ts ON ts.session_id = se.id
		WHERE se.issue_key = ?
		ORDER BY ts.issue_updated_at DESC, ts.id DESC
		LIMIT 1
	`, issueKey).Scan(&input.IssueKey, &input.IssueSummary, &input.SuggestedScope, &input.BranchName, &input.RevisionFeedback, &input.NormalizedText)
	if err != nil {
		return ImplementationInput{}, fmt.Errorf("load implementation input: %w", err)
	}
	record, err := s.LatestEvaluationResult(ctx, issueKey)
	if err != nil {
		return ImplementationInput{}, err
	}
	input.ImplementationNotes = record.ImplementationNotes
	input.OutOfScope = record.OutOfScope
	input.ExistingBranch = input.BranchName != ""
	return input, nil
}

func (s *Store) RecordWorktree(ctx context.Context, issueKey, path, branchName, baseRef, status string) error {
	var sessionID int64
	err := s.DB.QueryRowContext(ctx, `SELECT id FROM sessions WHERE issue_key = ?`, issueKey).Scan(&sessionID)
	if err != nil {
		return fmt.Errorf("load session for worktree: %w", err)
	}
	_, err = s.DB.ExecContext(ctx, `
		INSERT INTO worktrees (session_id, path, branch_name, base_ref, status)
		VALUES (?, ?, ?, ?, ?)
	`, sessionID, path, branchName, baseRef, status)
	if err != nil {
		return fmt.Errorf("record worktree: %w", err)
	}
	return nil
}

func (s *Store) RecordImplementationFailure(ctx context.Context, issueKey string, failure ImplementationFailure) error {
	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin implementation failure transaction: %w", err)
	}
	defer func() {
		if err != nil {
			_ = tx.Rollback()
		}
	}()
	var sessionID int64
	var currentState string
	err = tx.QueryRowContext(ctx, `SELECT id, state FROM sessions WHERE issue_key = ?`, issueKey).Scan(&sessionID, &currentState)
	if err != nil {
		return fmt.Errorf("load session for implementation failure: %w", err)
	}
	if err := s.insertRunTx(ctx, tx, sessionID, "implementation", "failure", failure.PromptSHA, failure.Stdout, "", failure.ErrorText, failure.StartedAt, failure.FinishedAt); err != nil {
		return err
	}
	_, err = tx.ExecContext(ctx, `UPDATE sessions SET state = 'FAILED', branch_name = COALESCE(NULLIF(?, ''), branch_name), last_error = ?, updated_at = CURRENT_TIMESTAMP WHERE id = ?`, failure.BranchName, failure.ErrorText, sessionID)
	if err != nil {
		return fmt.Errorf("update session implementation failure: %w", err)
	}
	if failure.WorktreePath != "" {
		_, err = tx.ExecContext(ctx, `UPDATE worktrees SET status = 'failed', updated_at = CURRENT_TIMESTAMP WHERE session_id = ? AND path = ?`, sessionID, failure.WorktreePath)
		if err != nil {
			return fmt.Errorf("mark worktree failed: %w", err)
		}
	}
	if err := s.insertEventTx(ctx, tx, sessionID, "system", "implementation_failed", currentState, "FAILED", failure.ErrorText); err != nil {
		return err
	}
	if err = tx.Commit(); err != nil {
		return fmt.Errorf("commit implementation failure transaction: %w", err)
	}
	return nil
}

func (s *Store) RecordImplementationSuccess(ctx context.Context, issueKey string, success ImplementationSuccess) error {
	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin implementation success transaction: %w", err)
	}
	defer func() {
		if err != nil {
			_ = tx.Rollback()
		}
	}()
	var sessionID int64
	var currentState string
	err = tx.QueryRowContext(ctx, `SELECT id, state FROM sessions WHERE issue_key = ?`, issueKey).Scan(&sessionID, &currentState)
	if err != nil {
		return fmt.Errorf("load session for implementation success: %w", err)
	}
	if err := s.insertRunTx(ctx, tx, sessionID, "implementation", "success", success.PromptSHA, success.Stdout, "", "", success.StartedAt, success.FinishedAt); err != nil {
		return err
	}
	_, err = tx.ExecContext(ctx, `
		UPDATE sessions
		SET state = 'PR_OPEN',
			branch_name = ?,
			implementation_summary = ?,
			pr_number = ?,
			pr_url = ?,
			revision_feedback = '',
			last_error = '',
			updated_at = CURRENT_TIMESTAMP
		WHERE id = ?
	`, success.BranchName, success.Summary, success.PRNumber, success.PRURL, sessionID)
	if err != nil {
		return fmt.Errorf("update session implementation success: %w", err)
	}
	_, err = tx.ExecContext(ctx, `UPDATE worktrees SET status = 'completed', updated_at = CURRENT_TIMESTAMP WHERE session_id = ? AND path = ?`, sessionID, success.WorktreePath)
	if err != nil {
		return fmt.Errorf("mark worktree completed: %w", err)
	}
	if err := s.insertEventTx(ctx, tx, sessionID, "system", "implementation_completed", currentState, "PR_OPEN", success.VerificationSummary); err != nil {
		return err
	}
	if err = tx.Commit(); err != nil {
		return fmt.Errorf("commit implementation success transaction: %w", err)
	}
	return nil
}

func (s *Store) LatestSuccessfulRun(ctx context.Context, issueKey, taskType string) (bool, error) {
	var count int
	err := s.DB.QueryRowContext(ctx, `
		SELECT COUNT(1)
		FROM opencode_runs r
		JOIN sessions s ON s.id = r.session_id
		WHERE s.issue_key = ? AND r.task_type = ? AND r.status = 'success'
	`, issueKey, taskType).Scan(&count)
	if err != nil {
		return false, fmt.Errorf("check latest successful run: %w", err)
	}
	return count > 0, nil
}

func (s *Store) QueueRevision(ctx context.Context, issueKey, feedback string) error {
	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin queue revision transaction: %w", err)
	}
	defer func() {
		if err != nil {
			_ = tx.Rollback()
		}
	}()
	var sessionID int64
	var currentState string
	err = tx.QueryRowContext(ctx, `SELECT id, state FROM sessions WHERE issue_key = ?`, issueKey).Scan(&sessionID, &currentState)
	if err != nil {
		return fmt.Errorf("load session for revision queue: %w", err)
	}
	if currentState != "PR_OPEN" {
		return nil
	}
	_, err = tx.ExecContext(ctx, `
		UPDATE sessions
		SET state = 'REVISING',
			revision_feedback = ?,
			revision_count = revision_count + 1,
			updated_at = CURRENT_TIMESTAMP
		WHERE id = ?
	`, feedback, sessionID)
	if err != nil {
		return fmt.Errorf("queue revision: %w", err)
	}
	if err := s.insertEventTx(ctx, tx, sessionID, "system", "revision_queued", "PR_OPEN", "REVISING", feedback); err != nil {
		return err
	}
	if err = tx.Commit(); err != nil {
		return fmt.Errorf("commit queue revision transaction: %w", err)
	}
	return nil
}

func (s *Store) VerificationText(results []string) string {
	data, _ := json.Marshal(results)
	return string(data)
}
