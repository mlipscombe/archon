package db

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"
	"time"
)

type ClarifyingQuestion struct {
	Question        string `json:"question"`
	SuggestedAnswer string `json:"suggested_answer"`
	Rationale       string `json:"rationale"`
}

type EvaluationInput struct {
	IssueKey       string
	IssueSummary   string
	NormalizedText string
	RubricCriteria []string
	PriorContext   string
}

type EvaluationSuccess struct {
	PromptSHA           string
	Stdout              string
	Stderr              string
	StartedAt           time.Time
	FinishedAt          time.Time
	Decision            string
	Confidence          float64
	Reasoning           string
	MissingElements     []string
	ClarifyingQuestions []ClarifyingQuestion
	ImplementationNotes []string
	SuggestedScope      string
	OutOfScope          []string
	RawJSON             string
	EffectiveDecision   string
	Mode                string
}

type EvaluationRecord struct {
	ID                  int64
	Decision            string
	Confidence          float64
	Reasoning           string
	MissingElements     []string
	ClarifyingQuestions []ClarifyingQuestion
	ImplementationNotes []string
	SuggestedScope      string
	OutOfScope          []string
	RawJSON             string
	CreatedAt           time.Time
}

type SessionDetail struct {
	SessionSummary
	SuggestedScope      string
	MissingElements     []string
	ClarifyingQuestions []ClarifyingQuestion
	ImplementationNotes []string
	OutOfScope          []string
	Events              []SessionEventRecord
	Runs                []OpencodeRunRecord
	ClarificationCycles []ClarificationCycleRecord
}

type SessionEventRecord struct {
	Actor     string
	EventType string
	FromState string
	ToState   string
	Detail    string
	CreatedAt time.Time
}

type OpencodeRunRecord struct {
	TaskType   string
	Status     string
	PromptSHA  string
	Stdout     string
	Stderr     string
	ErrorText  string
	StartedAt  time.Time
	FinishedAt time.Time
}

type ClarificationCycleRecord struct {
	JiraCommentID string
	CommentBody   string
	CreatedAt     time.Time
}

type EvaluationFailure struct {
	PromptSHA  string
	Stdout     string
	Stderr     string
	ErrorText  string
	StartedAt  time.Time
	FinishedAt time.Time
}

func (s *Store) LoadEvaluationInput(ctx context.Context, issueKey string) (EvaluationInput, error) {
	var input EvaluationInput
	err := s.DB.QueryRowContext(ctx, `
		SELECT se.issue_key, se.issue_summary, ts.normalized_text
		FROM sessions se
		JOIN ticket_snapshots ts ON ts.session_id = se.id
		WHERE se.issue_key = ?
		ORDER BY ts.issue_updated_at DESC, ts.id DESC
		LIMIT 1
	`, issueKey).Scan(&input.IssueKey, &input.IssueSummary, &input.NormalizedText)
	if err != nil {
		return EvaluationInput{}, fmt.Errorf("load evaluation input: %w", err)
	}

	rows, err := s.DB.QueryContext(ctx, `
		SELECT description, applies_to
		FROM rubric_criteria
		WHERE is_enabled = 1 AND (project_key IS NULL OR project_key = '')
		ORDER BY id
	`)
	if err != nil {
		return EvaluationInput{}, fmt.Errorf("load rubric criteria: %w", err)
	}
	defer rows.Close()

	for rows.Next() {
		var description string
		var appliesTo string
		if err := rows.Scan(&description, &appliesTo); err != nil {
			return EvaluationInput{}, fmt.Errorf("scan rubric criterion: %w", err)
		}
		input.RubricCriteria = append(input.RubricCriteria, fmt.Sprintf("%s (%s)", description, appliesTo))
	}
	if err := rows.Err(); err != nil {
		return EvaluationInput{}, fmt.Errorf("iterate rubric criteria: %w", err)
	}

	priorContext, err := s.latestEvaluationContext(ctx, issueKey)
	if err != nil {
		return EvaluationInput{}, err
	}
	input.PriorContext = priorContext

	return input, nil
}

func (s *Store) latestEvaluationContext(ctx context.Context, issueKey string) (string, error) {
	var decision string
	var confidence float64
	var reasoning string
	err := s.DB.QueryRowContext(ctx, `
		SELECT evaluation_decision, confidence, evaluation_reasoning
		FROM sessions
		WHERE issue_key = ?
	`, issueKey).Scan(&decision, &confidence, &reasoning)
	if err != nil {
		return "", fmt.Errorf("load prior evaluation context: %w", err)
	}
	decision = strings.TrimSpace(decision)
	if decision == "" && strings.TrimSpace(reasoning) == "" {
		return "", nil
	}
	return fmt.Sprintf("Last decision: %s\nLast confidence: %.2f\nLast reasoning: %s", decision, confidence, reasoning), nil
}

func (s *Store) MarkSessionEvaluating(ctx context.Context, issueKey string) error {
	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin mark evaluating transaction: %w", err)
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
		return fmt.Errorf("load session for evaluation: %w", err)
	}

	_, err = tx.ExecContext(ctx, `
		UPDATE sessions
		SET state = 'EVALUATING', updated_at = CURRENT_TIMESTAMP
		WHERE id = ?
	`, sessionID)
	if err != nil {
		return fmt.Errorf("set session evaluating: %w", err)
	}
	if err := s.insertEventTx(ctx, tx, sessionID, "system", "evaluation_started", currentState, "EVALUATING", "Started readiness evaluation"); err != nil {
		return err
	}
	if err = tx.Commit(); err != nil {
		return fmt.Errorf("commit mark evaluating transaction: %w", err)
	}
	return nil
}

func (s *Store) RecordEvaluationFailure(ctx context.Context, issueKey string, failure EvaluationFailure) error {
	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin evaluation failure transaction: %w", err)
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
		return fmt.Errorf("load session for evaluation failure: %w", err)
	}

	if err := s.insertRunTx(ctx, tx, sessionID, "evaluation", "failure", failure.PromptSHA, failure.Stdout, failure.Stderr, failure.ErrorText, failure.StartedAt, failure.FinishedAt); err != nil {
		return err
	}
	_, err = tx.ExecContext(ctx, `
		UPDATE sessions
		SET state = 'FAILED', last_error = ?, updated_at = CURRENT_TIMESTAMP
		WHERE id = ?
	`, failure.ErrorText, sessionID)
	if err != nil {
		return fmt.Errorf("update failed evaluation session: %w", err)
	}
	if err := s.insertEventTx(ctx, tx, sessionID, "system", "evaluation_failed", currentState, "FAILED", failure.ErrorText); err != nil {
		return err
	}
	if err = tx.Commit(); err != nil {
		return fmt.Errorf("commit evaluation failure transaction: %w", err)
	}
	return nil
}

func (s *Store) RecordEvaluationSuccess(ctx context.Context, issueKey string, success EvaluationSuccess) error {
	missingJSON, err := json.Marshal(success.MissingElements)
	if err != nil {
		return fmt.Errorf("marshal missing elements: %w", err)
	}
	questionsJSON, err := json.Marshal(success.ClarifyingQuestions)
	if err != nil {
		return fmt.Errorf("marshal clarifying questions: %w", err)
	}
	notesJSON, err := json.Marshal(success.ImplementationNotes)
	if err != nil {
		return fmt.Errorf("marshal implementation notes: %w", err)
	}
	outOfScopeJSON, err := json.Marshal(success.OutOfScope)
	if err != nil {
		return fmt.Errorf("marshal out of scope assumptions: %w", err)
	}

	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin evaluation success transaction: %w", err)
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
		return fmt.Errorf("load session for evaluation success: %w", err)
	}

	if err := s.insertRunTx(ctx, tx, sessionID, "evaluation", "success", success.PromptSHA, success.Stdout, success.Stderr, "", success.StartedAt, success.FinishedAt); err != nil {
		return err
	}
	_, err = tx.ExecContext(ctx, `
		INSERT INTO evaluation_results (
			session_id,
			decision,
			confidence,
			reasoning,
			missing_elements_json,
			clarifying_questions_json,
			implementation_notes_json,
			suggested_scope,
			out_of_scope_assumptions_json,
			raw_json
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`, sessionID, success.Decision, success.Confidence, success.Reasoning, string(missingJSON), string(questionsJSON), string(notesJSON), success.SuggestedScope, string(outOfScopeJSON), success.RawJSON)
	if err != nil {
		return fmt.Errorf("insert evaluation result: %w", err)
	}

	toState := "WAITING_FOR_CLARIFICATION"
	detail := "Evaluation determined the ticket is not ready"
	if success.EffectiveDecision == "READY" {
		if success.Mode == "sandbox" {
			toState = "IMPLEMENTING"
			detail = "Evaluation determined the ticket is ready and sandbox mode would auto-proceed"
		} else {
			toState = "AWAITING_APPROVAL"
			detail = "Evaluation determined the ticket is ready and awaiting approval"
		}
	}

	_, err = tx.ExecContext(ctx, `
		UPDATE sessions
		SET state = ?,
			confidence = ?,
			suggested_scope = ?,
			evaluation_decision = ?,
			evaluation_reasoning = ?,
			last_evaluated_at = ?,
			last_error = '',
			updated_at = CURRENT_TIMESTAMP
		WHERE id = ?
	`, toState, success.Confidence, success.SuggestedScope, success.EffectiveDecision, success.Reasoning, formatStoredTime(success.FinishedAt), sessionID)
	if err != nil {
		return fmt.Errorf("update session after evaluation: %w", err)
	}
	if err := s.insertEventTx(ctx, tx, sessionID, "system", "evaluation_completed", currentState, toState, detail); err != nil {
		return err
	}
	if err = tx.Commit(); err != nil {
		return fmt.Errorf("commit evaluation success transaction: %w", err)
	}
	return nil
}

func (s *Store) insertRunTx(ctx context.Context, tx *sql.Tx, sessionID int64, taskType, status, promptSHA, stdout, stderr, errorText string, startedAt, finishedAt time.Time) error {
	_, err := tx.ExecContext(ctx, `
		INSERT INTO opencode_runs (
			session_id,
			task_type,
			status,
			prompt_sha256,
			stdout_text,
			stderr_text,
			error_text,
			started_at,
			finished_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)
	`, sessionID, taskType, status, promptSHA, stdout, stderr, errorText, formatStoredTime(startedAt), formatStoredTime(finishedAt))
	if err != nil {
		return fmt.Errorf("insert opencode run: %w", err)
	}
	return nil
}

func (s *Store) LatestEvaluationResult(ctx context.Context, issueKey string) (EvaluationRecord, error) {
	var record EvaluationRecord
	var missingJSON string
	var questionsJSON string
	var notesJSON string
	var outOfScopeJSON string
	var createdAt string
	err := s.DB.QueryRowContext(ctx, `
		SELECT er.id, er.decision, er.confidence, er.reasoning, er.missing_elements_json,
			er.clarifying_questions_json, er.implementation_notes_json, er.suggested_scope,
			er.out_of_scope_assumptions_json, er.raw_json, er.created_at
		FROM evaluation_results er
		JOIN sessions se ON se.id = er.session_id
		WHERE se.issue_key = ?
		ORDER BY er.id DESC
		LIMIT 1
	`, issueKey).Scan(
		&record.ID,
		&record.Decision,
		&record.Confidence,
		&record.Reasoning,
		&missingJSON,
		&questionsJSON,
		&notesJSON,
		&record.SuggestedScope,
		&outOfScopeJSON,
		&record.RawJSON,
		&createdAt,
	)
	if err != nil {
		return EvaluationRecord{}, fmt.Errorf("load latest evaluation result: %w", err)
	}
	if err := json.Unmarshal([]byte(missingJSON), &record.MissingElements); err != nil {
		return EvaluationRecord{}, fmt.Errorf("decode missing elements: %w", err)
	}
	if err := json.Unmarshal([]byte(questionsJSON), &record.ClarifyingQuestions); err != nil {
		return EvaluationRecord{}, fmt.Errorf("decode clarifying questions: %w", err)
	}
	if err := json.Unmarshal([]byte(notesJSON), &record.ImplementationNotes); err != nil {
		return EvaluationRecord{}, fmt.Errorf("decode implementation notes: %w", err)
	}
	if err := json.Unmarshal([]byte(outOfScopeJSON), &record.OutOfScope); err != nil {
		return EvaluationRecord{}, fmt.Errorf("decode out of scope assumptions: %w", err)
	}
	if parsed, err := parseStoredTime(createdAt); err == nil {
		record.CreatedAt = parsed
	}
	return record, nil
}

func (s *Store) LoadSessionDetail(ctx context.Context, issueKey string) (SessionDetail, error) {
	var detail SessionDetail
	var updatedAt string
	var issueUpdatedAt string
	err := s.DB.QueryRowContext(ctx, `
		SELECT issue_key, issue_summary, state, version, issue_type, priority, jira_status,
			branch_name, pr_number, pr_url, implementation_summary, revision_count,
			evaluation_decision, evaluation_reasoning, last_error, confidence, updated_at, issue_updated_at, suggested_scope
		FROM sessions
		WHERE issue_key = ?
	`, issueKey).Scan(
		&detail.IssueKey,
		&detail.IssueSummary,
		&detail.State,
		&detail.Version,
		&detail.IssueType,
		&detail.Priority,
		&detail.JiraStatus,
		&detail.BranchName,
		&detail.PRNumber,
		&detail.PRURL,
		&detail.ImplementationSummary,
		&detail.RevisionCount,
		&detail.Decision,
		&detail.Reasoning,
		&detail.LastError,
		&detail.Confidence,
		&updatedAt,
		&issueUpdatedAt,
		&detail.SuggestedScope,
	)
	if err != nil {
		return SessionDetail{}, fmt.Errorf("load session detail: %w", err)
	}
	if parsed, err := parseStoredTime(updatedAt); err == nil {
		detail.UpdatedAt = parsed
	}
	if parsed, err := parseStoredTime(issueUpdatedAt); err == nil {
		detail.IssueUpdatedAt = &parsed
	}

	record, err := s.LatestEvaluationResult(ctx, issueKey)
	if err == nil {
		detail.SuggestedScope = record.SuggestedScope
		detail.MissingElements = record.MissingElements
		detail.ClarifyingQuestions = record.ClarifyingQuestions
		detail.ImplementationNotes = record.ImplementationNotes
		detail.OutOfScope = record.OutOfScope
	}
	if detail.Events, err = s.ListSessionEvents(ctx, issueKey, 100); err != nil {
		return SessionDetail{}, err
	}
	if detail.Runs, err = s.ListOpencodeRuns(ctx, issueKey, 20); err != nil {
		return SessionDetail{}, err
	}
	if detail.ClarificationCycles, err = s.ListClarificationCycles(ctx, issueKey, 20); err != nil {
		return SessionDetail{}, err
	}
	return detail, nil
}

func (s *Store) RecordClarificationCycle(ctx context.Context, issueKey string, evaluationResultID int64, jiraCommentID, body string) error {
	var sessionID int64
	err := s.DB.QueryRowContext(ctx, `SELECT id FROM sessions WHERE issue_key = ?`, issueKey).Scan(&sessionID)
	if err != nil {
		return fmt.Errorf("load session for clarification cycle: %w", err)
	}
	_, err = s.DB.ExecContext(ctx, `
		INSERT INTO clarification_cycles (session_id, evaluation_result_id, jira_comment_id, comment_body)
		VALUES (?, ?, ?, ?)
	`, sessionID, evaluationResultID, jiraCommentID, body)
	if err != nil {
		return fmt.Errorf("record clarification cycle: %w", err)
	}
	return nil
}

func (s *Store) ListSessionEvents(ctx context.Context, issueKey string, limit int) ([]SessionEventRecord, error) {
	if limit <= 0 {
		limit = 100
	}
	rows, err := s.DB.QueryContext(ctx, `
		SELECT ev.actor, ev.event_type, ev.from_state, ev.to_state, ev.detail, ev.created_at
		FROM session_events ev
		JOIN sessions se ON se.id = ev.session_id
		WHERE se.issue_key = ?
		ORDER BY ev.id DESC
		LIMIT ?
	`, issueKey, limit)
	if err != nil {
		return nil, fmt.Errorf("list session events: %w", err)
	}
	defer rows.Close()
	var events []SessionEventRecord
	for rows.Next() {
		var record SessionEventRecord
		var createdAt string
		if err := rows.Scan(&record.Actor, &record.EventType, &record.FromState, &record.ToState, &record.Detail, &createdAt); err != nil {
			return nil, fmt.Errorf("scan session event: %w", err)
		}
		if parsed, err := parseStoredTime(createdAt); err == nil {
			record.CreatedAt = parsed
		}
		events = append(events, record)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate session events: %w", err)
	}
	return events, nil
}

func (s *Store) ListOpencodeRuns(ctx context.Context, issueKey string, limit int) ([]OpencodeRunRecord, error) {
	if limit <= 0 {
		limit = 20
	}
	rows, err := s.DB.QueryContext(ctx, `
		SELECT r.task_type, r.status, r.prompt_sha256, r.stdout_text, r.stderr_text, r.error_text, r.started_at, r.finished_at
		FROM opencode_runs r
		JOIN sessions s ON s.id = r.session_id
		WHERE s.issue_key = ?
		ORDER BY r.id DESC
		LIMIT ?
	`, issueKey, limit)
	if err != nil {
		return nil, fmt.Errorf("list opencode runs: %w", err)
	}
	defer rows.Close()
	var runs []OpencodeRunRecord
	for rows.Next() {
		var record OpencodeRunRecord
		var startedAt string
		var finishedAt string
		if err := rows.Scan(&record.TaskType, &record.Status, &record.PromptSHA, &record.Stdout, &record.Stderr, &record.ErrorText, &startedAt, &finishedAt); err != nil {
			return nil, fmt.Errorf("scan opencode run: %w", err)
		}
		if parsed, err := parseStoredTime(startedAt); err == nil {
			record.StartedAt = parsed
		}
		if parsed, err := parseStoredTime(finishedAt); err == nil {
			record.FinishedAt = parsed
		}
		runs = append(runs, record)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate opencode runs: %w", err)
	}
	return runs, nil
}

func (s *Store) ListClarificationCycles(ctx context.Context, issueKey string, limit int) ([]ClarificationCycleRecord, error) {
	if limit <= 0 {
		limit = 20
	}
	rows, err := s.DB.QueryContext(ctx, `
		SELECT c.jira_comment_id, c.comment_body, c.created_at
		FROM clarification_cycles c
		JOIN sessions s ON s.id = c.session_id
		WHERE s.issue_key = ?
		ORDER BY c.id DESC
		LIMIT ?
	`, issueKey, limit)
	if err != nil {
		return nil, fmt.Errorf("list clarification cycles: %w", err)
	}
	defer rows.Close()
	var cycles []ClarificationCycleRecord
	for rows.Next() {
		var record ClarificationCycleRecord
		var createdAt string
		if err := rows.Scan(&record.JiraCommentID, &record.CommentBody, &createdAt); err != nil {
			return nil, fmt.Errorf("scan clarification cycle: %w", err)
		}
		if parsed, err := parseStoredTime(createdAt); err == nil {
			record.CreatedAt = parsed
		}
		cycles = append(cycles, record)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate clarification cycles: %w", err)
	}
	return cycles, nil
}
