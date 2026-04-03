package approval

import (
	"context"
	"database/sql"
	"fmt"
	"log/slog"
	"strings"

	"github.com/availhealth/archon/internal/db"
	"github.com/availhealth/archon/internal/jira"
	"github.com/availhealth/archon/internal/logx"
)

type Service struct {
	log   *slog.Logger
	store *db.Store
	jira  *jira.Client
}

func New(logger *slog.Logger, store *db.Store, jiraClient *jira.Client) *Service {
	return &Service{
		log:   logx.WithComponent(logger, "approval"),
		store: store,
		jira:  jiraClient,
	}
}

func (s *Service) Approve(ctx context.Context, issueKey string, expectedVersion int) error {
	if err := s.store.ApproveSession(ctx, issueKey, expectedVersion); err != nil {
		if err == sql.ErrNoRows {
			return fmt.Errorf("approval conflict or session no longer awaiting approval")
		}
		return err
	}
	s.log.Info("session approved", slog.String("issue_key", issueKey))
	return nil
}

func (s *Service) Reject(ctx context.Context, issueKey string, expectedVersion int, reason string) error {
	reason = strings.TrimSpace(reason)
	if reason == "" {
		reason = "No rejection reason provided."
	}
	if err := s.store.RejectSession(ctx, issueKey, expectedVersion, reason); err != nil {
		if err == sql.ErrNoRows {
			return fmt.Errorf("rejection conflict or session no longer awaiting approval")
		}
		return err
	}

	comment := strings.Join([]string{
		"[ARCHON] Approval was rejected in the Archon UI.",
		"",
		"Reason:",
		reason,
	}, "\n")
	if _, err := s.jira.PostComment(ctx, issueKey, comment); err != nil {
		_ = s.store.SetSessionLastError(ctx, issueKey, fmt.Sprintf("rejection comment failed: %v", err))
		return fmt.Errorf("post rejection comment: %w", err)
	}

	s.log.Info("session rejected", slog.String("issue_key", issueKey))
	return nil
}
