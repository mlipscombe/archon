package revision

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/availhealth/archon/internal/config"
	"github.com/availhealth/archon/internal/db"
	gh "github.com/availhealth/archon/internal/github"
	"github.com/availhealth/archon/internal/jira"
	"github.com/availhealth/archon/internal/logx"
)

type Service struct {
	cfg    config.Config
	log    *slog.Logger
	store  *db.Store
	github *gh.Client
	jira   *jira.Client
}

func New(cfg config.Config, logger *slog.Logger, store *db.Store, githubClient *gh.Client, jiraClient *jira.Client) *Service {
	return &Service{
		cfg:    cfg,
		log:    logx.WithComponent(logger, "revision"),
		store:  store,
		github: githubClient,
		jira:   jiraClient,
	}
}

func (s *Service) Start(ctx context.Context) error {
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()
	s.runCycle(ctx)
	for {
		select {
		case <-ctx.Done():
			return nil
		case <-ticker.C:
			s.runCycle(ctx)
		}
	}
}

func (s *Service) runCycle(ctx context.Context) {
	sessions, err := s.store.ListSessionsByState(ctx, "PR_OPEN", 20)
	if err != nil {
		s.log.Error("list pr-open sessions failed", slog.Any("error", err))
		return
	}
	for _, session := range sessions {
		if session.PRURL == "" {
			continue
		}
		if err := s.inspectSession(ctx, session); err != nil {
			s.log.Error("inspect revision candidate failed", slog.String("issue_key", session.IssueKey), slog.Any("error", err))
		}
	}
}

func (s *Service) inspectSession(ctx context.Context, session db.SessionSummary) error {
	if session.PRURL == "" {
		return nil
	}
	if session.RevisionCount >= s.cfg.Opencode.MaxRevisionCycles {
		return nil
	}
	labels, err := s.github.ListLabels(ctx, session.PRNumber)
	if err != nil {
		return err
	}
	if !hasLabel(labels, "archon-revise") {
		return nil
	}
	comments, err := s.github.ListReviewComments(ctx, session.PRNumber)
	if err != nil {
		return err
	}
	feedback := buildFeedback(comments)
	if strings.TrimSpace(feedback) == "" {
		feedback = "GitHub reviewers requested changes with the archon-revise label, but no review comments were available. Re-read the PR discussion and update the implementation carefully."
	}
	if err := s.store.QueueRevision(ctx, session.IssueKey, feedback); err != nil {
		return err
	}
	comment := strings.Join([]string{
		"[ARCHON] GitHub review feedback requested changes and `archon-revise` is present.",
		"",
		"Queued revision feedback:",
		feedback,
	}, "\n")
	if _, err := s.jira.PostComment(ctx, session.IssueKey, comment); err != nil {
		_ = s.store.SetSessionLastError(ctx, session.IssueKey, fmt.Sprintf("revision jira comment failed: %v", err))
	}
	s.log.Info("queued revision", slog.String("issue_key", session.IssueKey))
	return nil
}

func hasLabel(labels []gh.Label, expected string) bool {
	for _, label := range labels {
		if strings.EqualFold(strings.TrimSpace(label.Name), expected) {
			return true
		}
	}
	return false
}

func buildFeedback(comments []gh.ReviewComment) string {
	parts := make([]string, 0, len(comments))
	for _, comment := range comments {
		body := strings.TrimSpace(comment.Body)
		if body != "" {
			parts = append(parts, "- "+body)
		}
	}
	return strings.Join(parts, "\n")
}
