package evaluator

import (
	"context"
	"fmt"
	"log/slog"
	"strings"

	"github.com/availhealth/archon/internal/config"
	"github.com/availhealth/archon/internal/db"
	"github.com/availhealth/archon/internal/jira"
	"github.com/availhealth/archon/internal/logx"
	"github.com/availhealth/archon/internal/opencode"
)

type Service struct {
	cfg    config.Config
	log    *slog.Logger
	store  *db.Store
	jira   *jira.Client
	runner *opencode.Runner
}

func New(cfg config.Config, logger *slog.Logger, store *db.Store, jiraClient *jira.Client) *Service {
	return &Service{
		cfg:    cfg,
		log:    logx.WithComponent(logger, "evaluator"),
		store:  store,
		jira:   jiraClient,
		runner: opencode.NewRunner(cfg),
	}
}

func (s *Service) EvaluateIssue(ctx context.Context, issueKey string) error {
	input, err := s.store.LoadEvaluationInput(ctx, issueKey)
	if err != nil {
		return err
	}
	if err := s.store.MarkSessionEvaluating(ctx, issueKey); err != nil {
		return err
	}

	prompt := opencode.BuildEvaluationPrompt(opencode.PromptInput{
		IssueKey:        input.IssueKey,
		IssueSummary:    input.IssueSummary,
		NormalizedText:  input.NormalizedText,
		RubricCriteria:  input.RubricCriteria,
		PriorContext:    input.PriorContext,
		ConfidenceFloor: s.cfg.ConfidenceThreshold,
	})

	runResult, err := s.runner.RunEvaluation(ctx, prompt)
	if err != nil {
		recordErr := s.store.RecordEvaluationFailure(ctx, issueKey, db.EvaluationFailure{
			PromptSHA:  runResult.PromptSHA,
			Stdout:     runResult.Stdout,
			Stderr:     runResult.Stderr,
			ErrorText:  err.Error(),
			StartedAt:  runResult.StartedAt,
			FinishedAt: runResult.FinishedAt,
		})
		if recordErr != nil {
			return fmt.Errorf("record evaluation failure: %v (original: %w)", recordErr, err)
		}
		return err
	}

	parsed, rawJSON, err := opencode.ParseEvaluationOutput(runResult.Stdout)
	if err != nil {
		recordErr := s.store.RecordEvaluationFailure(ctx, issueKey, db.EvaluationFailure{
			PromptSHA:  runResult.PromptSHA,
			Stdout:     runResult.Stdout,
			Stderr:     runResult.Stderr,
			ErrorText:  err.Error(),
			StartedAt:  runResult.StartedAt,
			FinishedAt: runResult.FinishedAt,
		})
		if recordErr != nil {
			return fmt.Errorf("record evaluation parse failure: %v (original: %w)", recordErr, err)
		}
		return err
	}

	if err := s.store.RecordEvaluationSuccess(ctx, issueKey, db.EvaluationSuccess{
		PromptSHA:           runResult.PromptSHA,
		Stdout:              runResult.Stdout,
		Stderr:              runResult.Stderr,
		StartedAt:           runResult.StartedAt,
		FinishedAt:          runResult.FinishedAt,
		Decision:            parsed.Decision,
		Confidence:          parsed.Confidence,
		Reasoning:           parsed.Reasoning,
		MissingElements:     parsed.MissingElements,
		ClarifyingQuestions: convertQuestions(parsed.ClarifyingQuestions),
		ImplementationNotes: parsed.ImplementationNotes,
		SuggestedScope:      parsed.SuggestedScope,
		OutOfScope:          parsed.OutOfScopeAssumptions,
		RawJSON:             rawJSON,
		EffectiveDecision:   parsed.EffectiveDecision(s.cfg.ConfidenceThreshold),
		Mode:                s.cfg.Mode,
	}); err != nil {
		return err
	}

	effectiveDecision := parsed.EffectiveDecision(s.cfg.ConfidenceThreshold)
	if err := s.handlePostEvaluation(ctx, issueKey, effectiveDecision); err != nil {
		s.log.Error("post-evaluation follow-up failed", slog.String("issue_key", issueKey), slog.Any("error", err))
		_ = s.store.SetSessionLastError(ctx, issueKey, err.Error())
	}

	s.log.Info("evaluation completed", slog.String("issue_key", issueKey), slog.String("decision", effectiveDecision), slog.Float64("confidence", parsed.Confidence))
	return nil
}

func (s *Service) handlePostEvaluation(ctx context.Context, issueKey, effectiveDecision string) error {
	record, err := s.store.LatestEvaluationResult(ctx, issueKey)
	if err != nil {
		return err
	}

	switch {
	case effectiveDecision == "NOT_READY":
		body := buildClarificationComment(record)
		commentID, err := s.jira.PostComment(ctx, issueKey, body)
		if err != nil {
			return fmt.Errorf("post clarification comment: %w", err)
		}
		if err := s.store.RecordClarificationCycle(ctx, issueKey, record.ID, commentID, body); err != nil {
			return err
		}
	case effectiveDecision == "READY" && s.cfg.Mode == "approval":
		body := buildApprovalReadyComment(s.cfg, issueKey, record)
		if _, err := s.jira.PostComment(ctx, issueKey, body); err != nil {
			return fmt.Errorf("post approval-ready comment: %w", err)
		}
	}

	return nil
}

func convertQuestions(items []opencode.ClarifyingQuestion) []db.ClarifyingQuestion {
	result := make([]db.ClarifyingQuestion, 0, len(items))
	for _, item := range items {
		result = append(result, db.ClarifyingQuestion{
			Question:        item.Question,
			SuggestedAnswer: item.SuggestedAnswer,
			Rationale:       item.Rationale,
		})
	}
	return result
}

func buildClarificationComment(record db.EvaluationRecord) string {
	parts := []string{"[ARCHON] Archon needs a few things clarified before implementation can begin."}
	if len(record.MissingElements) > 0 {
		parts = append(parts, "", "Missing elements:")
		for _, item := range record.MissingElements {
			parts = append(parts, "- "+strings.TrimSpace(item))
		}
	}
	for idx, question := range record.ClarifyingQuestions {
		parts = append(parts, "", fmt.Sprintf("Question %d: %s", idx+1, strings.TrimSpace(question.Question)))
		if text := strings.TrimSpace(question.SuggestedAnswer); text != "" {
			parts = append(parts, "Suggested answer: "+text)
		}
		if text := strings.TrimSpace(question.Rationale); text != "" {
			parts = append(parts, "Why this matters: "+text)
		}
	}
	if text := strings.TrimSpace(record.SuggestedScope); text != "" {
		parts = append(parts, "", "What Archon understands so far:", text)
	}
	if len(record.OutOfScope) > 0 {
		parts = append(parts, "", "Assumed out of scope:")
		for _, item := range record.OutOfScope {
			parts = append(parts, "- "+strings.TrimSpace(item))
		}
	}
	parts = append(parts, "", "Reply to this comment or update the ticket description. Archon will re-evaluate automatically.")
	return strings.Join(parts, "\n")
}

func buildApprovalReadyComment(cfg config.Config, issueKey string, record db.EvaluationRecord) string {
	parts := []string{"[ARCHON] Archon has evaluated this ticket and is ready to implement."}
	if text := strings.TrimSpace(record.SuggestedScope); text != "" {
		parts = append(parts, "", "Planned scope:", text)
	}
	if len(record.OutOfScope) > 0 {
		parts = append(parts, "", "Out of scope (by design):")
		for _, item := range record.OutOfScope {
			parts = append(parts, "- "+strings.TrimSpace(item))
		}
	}
	if len(record.ImplementationNotes) > 0 {
		parts = append(parts, "", "Implementation notes:")
		for _, item := range record.ImplementationNotes {
			parts = append(parts, "- "+strings.TrimSpace(item))
		}
	}
	parts = append(parts, "", fmt.Sprintf("Running in approval mode. Approve or reject this plan in the Archon dashboard: %s/sessions/%s", uiBaseURL(cfg), issueKey))
	return strings.Join(parts, "\n")
}

func uiBaseURL(cfg config.Config) string {
	host := strings.TrimSpace(cfg.UI.Host)
	if host == "" || host == "0.0.0.0" {
		host = "localhost"
	}
	return fmt.Sprintf("http://%s:%d", host, cfg.UI.Port)
}
