package db

import (
	"context"
	"database/sql"
	"fmt"
)

func (s *Store) ApproveSession(ctx context.Context, issueKey string, expectedVersion int) error {
	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin approve transaction: %w", err)
	}
	defer func() {
		if err != nil {
			_ = tx.Rollback()
		}
	}()

	var sessionID int64
	err = tx.QueryRowContext(ctx, `SELECT id FROM sessions WHERE issue_key = ?`, issueKey).Scan(&sessionID)
	if err != nil {
		return fmt.Errorf("load session for approval: %w", err)
	}

	result, err := tx.ExecContext(ctx, `
		UPDATE sessions
		SET state = 'IMPLEMENTING',
			version = version + 1,
			last_error = '',
			updated_at = CURRENT_TIMESTAMP
		WHERE issue_key = ? AND state = 'AWAITING_APPROVAL' AND version = ?
	`, issueKey, expectedVersion)
	if err != nil {
		return fmt.Errorf("approve session: %w", err)
	}
	rows, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("check approval result: %w", err)
	}
	if rows == 0 {
		return sql.ErrNoRows
	}

	if err := s.insertEventTx(ctx, tx, sessionID, "operator", "approved", "AWAITING_APPROVAL", "IMPLEMENTING", "Approved from Archon UI"); err != nil {
		return err
	}
	if err = tx.Commit(); err != nil {
		return fmt.Errorf("commit approve transaction: %w", err)
	}
	return nil
}

func (s *Store) RejectSession(ctx context.Context, issueKey string, expectedVersion int, reason string) error {
	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin reject transaction: %w", err)
	}
	defer func() {
		if err != nil {
			_ = tx.Rollback()
		}
	}()

	var sessionID int64
	err = tx.QueryRowContext(ctx, `SELECT id FROM sessions WHERE issue_key = ?`, issueKey).Scan(&sessionID)
	if err != nil {
		return fmt.Errorf("load session for rejection: %w", err)
	}

	result, err := tx.ExecContext(ctx, `
		UPDATE sessions
		SET state = 'WAITING_FOR_CLARIFICATION',
			version = version + 1,
			updated_at = CURRENT_TIMESTAMP
		WHERE issue_key = ? AND state = 'AWAITING_APPROVAL' AND version = ?
	`, issueKey, expectedVersion)
	if err != nil {
		return fmt.Errorf("reject session: %w", err)
	}
	rows, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("check rejection result: %w", err)
	}
	if rows == 0 {
		return sql.ErrNoRows
	}

	if err := s.insertEventTx(ctx, tx, sessionID, "operator", "rejected", "AWAITING_APPROVAL", "WAITING_FOR_CLARIFICATION", reason); err != nil {
		return err
	}
	if err = tx.Commit(); err != nil {
		return fmt.Errorf("commit reject transaction: %w", err)
	}
	return nil
}

func (s *Store) SetSessionLastError(ctx context.Context, issueKey, message string) error {
	_, err := s.DB.ExecContext(ctx, `
		UPDATE sessions
		SET last_error = ?, updated_at = CURRENT_TIMESTAMP
		WHERE issue_key = ?
	`, message, issueKey)
	if err != nil {
		return fmt.Errorf("set session last error: %w", err)
	}
	return nil
}
