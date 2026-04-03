package implement

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
	"sync"
	"time"

	"github.com/availhealth/archon/internal/config"
	"github.com/availhealth/archon/internal/db"
	gh "github.com/availhealth/archon/internal/github"
	"github.com/availhealth/archon/internal/gitops"
	"github.com/availhealth/archon/internal/jira"
	"github.com/availhealth/archon/internal/logx"
	"github.com/availhealth/archon/internal/sessionlog"
	"github.com/availhealth/archon/internal/verify"
)

type Service struct {
	cfg     config.Config
	log     *slog.Logger
	store   *db.Store
	git     *gitops.Manager
	github  *gh.Client
	jira    *jira.Client
	logs    *sessionlog.Hub
	running map[string]struct{}
	mu      sync.Mutex
}

func New(cfg config.Config, logger *slog.Logger, store *db.Store, githubClient *gh.Client, jiraClient *jira.Client, logHub *sessionlog.Hub) *Service {
	return &Service{
		cfg:     cfg,
		log:     logx.WithComponent(logger, "implement"),
		store:   store,
		git:     gitops.NewManager(cfg.Repo.Path, cfg.StateStore.Path, cfg.GitHub.BaseBranch),
		github:  githubClient,
		jira:    jiraClient,
		logs:    logHub,
		running: map[string]struct{}{},
	}
}

func (s *Service) Start(ctx context.Context) error {
	ticker := time.NewTicker(5 * time.Second)
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
	implementing, err := s.store.ListSessionsByState(ctx, "IMPLEMENTING", 10)
	if err != nil {
		s.log.Error("list implementing sessions failed", slog.Any("error", err))
		return
	}
	revising, err := s.store.ListSessionsByState(ctx, "REVISING", 10)
	if err != nil {
		s.log.Error("list revising sessions failed", slog.Any("error", err))
		return
	}
	sessions := append(implementing, revising...)
	for _, session := range sessions {
		if !s.claim(session.IssueKey) {
			continue
		}
		go func(issueKey string) {
			defer s.release(issueKey)
			runCtx, cancel := context.WithTimeout(ctx, time.Duration(s.cfg.Opencode.TimeoutMinutes)*time.Minute)
			defer cancel()
			if err := s.Execute(runCtx, issueKey); err != nil {
				s.log.Error("implementation run failed", slog.String("issue_key", issueKey), slog.Any("error", err))
			}
		}(session.IssueKey)
	}
}

func (s *Service) Execute(ctx context.Context, issueKey string) error {
	s.logLine(issueKey, "system", "Preparing isolated worktree")
	input, err := s.store.LoadImplementationInput(ctx, issueKey)
	if err != nil {
		return err
	}
	worktreePath, branchName, err := s.git.PrepareWorktree(ctx, input.IssueKey, input.IssueSummary, input.BranchName)
	if err != nil {
		s.logLine(issueKey, "system", "Worktree preparation failed: "+err.Error())
		return s.store.RecordImplementationFailure(ctx, issueKey, db.ImplementationFailure{ErrorText: err.Error(), StartedAt: time.Now().UTC(), FinishedAt: time.Now().UTC()})
	}
	s.logLine(issueKey, "system", fmt.Sprintf("Worktree ready at %s on branch %s", worktreePath, branchName))
	input.WorktreePath = worktreePath
	input.BranchName = branchName
	if err := s.store.RecordWorktree(ctx, issueKey, worktreePath, branchName, s.cfg.GitHub.BaseBranch, "active"); err != nil {
		return err
	}

	_ = s.jira.TransitionIssueByName(ctx, issueKey, s.cfg.Jira.Projects[0].Transitions.InProgress)

	prompt := BuildPrompt(input)
	s.logLine(issueKey, "system", "Starting sandbox implementation run")
	runResult, err := RunWithLogger(ctx, s.cfg, worktreePath, prompt, func(stream, line string) {
		s.logLine(issueKey, stream, line)
	})
	if err != nil {
		s.logLine(issueKey, "system", "Implementation failed: "+err.Error())
		return s.store.RecordImplementationFailure(ctx, issueKey, db.ImplementationFailure{PromptSHA: runResult.PromptSHA, Stdout: runResult.Stdout, ErrorText: err.Error(), StartedAt: runResult.StartedAt, FinishedAt: runResult.FinishedAt, BranchName: branchName, WorktreePath: worktreePath})
	}

	s.logLine(issueKey, "system", "Detecting verification commands")
	commands, err := verify.Detect(worktreePath)
	if err != nil {
		s.logLine(issueKey, "system", "Verification detection failed: "+err.Error())
		return s.store.RecordImplementationFailure(ctx, issueKey, db.ImplementationFailure{PromptSHA: runResult.PromptSHA, Stdout: runResult.Stdout, ErrorText: err.Error(), StartedAt: runResult.StartedAt, FinishedAt: runResult.FinishedAt, BranchName: branchName, WorktreePath: worktreePath})
	}
	for _, command := range commands {
		s.logLine(issueKey, "system", "Running verification: "+command.Name+" "+strings.Join(command.Args, " "))
	}
	verifyResults, err := verify.Run(ctx, worktreePath, commands)
	if err != nil {
		s.logLine(issueKey, "system", "Verification failed: "+err.Error())
		return s.store.RecordImplementationFailure(ctx, issueKey, db.ImplementationFailure{PromptSHA: runResult.PromptSHA, Stdout: runResult.Stdout, ErrorText: err.Error(), StartedAt: runResult.StartedAt, FinishedAt: time.Now().UTC(), BranchName: branchName, WorktreePath: worktreePath})
	}
	for _, item := range verifyResults {
		s.logLine(issueKey, "system", "Verification passed: "+item.Command)
	}

	hasChanges, err := s.git.HasChanges(ctx, worktreePath)
	if err != nil {
		s.logLine(issueKey, "system", "Git status failed: "+err.Error())
		return s.store.RecordImplementationFailure(ctx, issueKey, db.ImplementationFailure{PromptSHA: runResult.PromptSHA, Stdout: runResult.Stdout, ErrorText: err.Error(), StartedAt: runResult.StartedAt, FinishedAt: time.Now().UTC(), BranchName: branchName, WorktreePath: worktreePath})
	}
	if !hasChanges {
		s.logLine(issueKey, "system", "No file changes detected after implementation")
		return s.store.RecordImplementationFailure(ctx, issueKey, db.ImplementationFailure{PromptSHA: runResult.PromptSHA, Stdout: runResult.Stdout, ErrorText: "implementation completed without file changes", StartedAt: runResult.StartedAt, FinishedAt: time.Now().UTC(), BranchName: branchName, WorktreePath: worktreePath})
	}

	commitMessage := fmt.Sprintf("[%s] implement approved scope", input.IssueKey)
	s.logLine(issueKey, "system", "Creating commit and pushing branch")
	if err := s.git.CommitAndPush(ctx, worktreePath, branchName, commitMessage); err != nil {
		s.logLine(issueKey, "system", "Commit or push failed: "+err.Error())
		return s.store.RecordImplementationFailure(ctx, issueKey, db.ImplementationFailure{PromptSHA: runResult.PromptSHA, Stdout: runResult.Stdout, ErrorText: err.Error(), StartedAt: runResult.StartedAt, FinishedAt: time.Now().UTC(), BranchName: branchName, WorktreePath: worktreePath})
	}

	s.logLine(issueKey, "system", "Creating GitHub pull request")
	prBody := buildPRBody(issueKey, input, runResult.Summary)
	pr, err := s.github.CreatePullRequest(ctx, gh.CreatePRInput{Title: fmt.Sprintf("[%s] %s", input.IssueKey, input.IssueSummary), Head: branchName, Base: s.cfg.GitHub.BaseBranch, Body: prBody, Draft: s.cfg.GitHub.DraftPRs})
	if err != nil {
		s.logLine(issueKey, "system", "Pull request creation failed: "+err.Error())
		return s.store.RecordImplementationFailure(ctx, issueKey, db.ImplementationFailure{PromptSHA: runResult.PromptSHA, Stdout: runResult.Stdout, ErrorText: err.Error(), StartedAt: runResult.StartedAt, FinishedAt: time.Now().UTC(), BranchName: branchName, WorktreePath: worktreePath})
	}
	_ = s.github.AddLabels(ctx, pr.Number, s.cfg.GitHub.Labels)
	_ = s.jira.TransitionIssueByName(ctx, issueKey, s.cfg.Jira.Projects[0].Transitions.InReview)
	s.logLine(issueKey, "system", "Pull request opened: "+pr.HTMLURL)

	verificationSummary := make([]string, 0, len(verifyResults))
	for _, item := range verifyResults {
		verificationSummary = append(verificationSummary, item.Command)
	}
	return s.store.RecordImplementationSuccess(ctx, issueKey, db.ImplementationSuccess{
		PromptSHA:           runResult.PromptSHA,
		Stdout:              runResult.Stdout,
		StartedAt:           runResult.StartedAt,
		FinishedAt:          runResult.FinishedAt,
		Summary:             runResult.Summary,
		BranchName:          branchName,
		WorktreePath:        worktreePath,
		VerificationSummary: strings.Join(verificationSummary, ", "),
		PRNumber:            pr.Number,
		PRURL:               pr.HTMLURL,
	})
}

func (s *Service) logLine(issueKey, stream, message string) {
	if s.logs == nil || strings.TrimSpace(message) == "" {
		return
	}
	s.logs.Publish(issueKey, stream, message)
}

func (s *Service) claim(issueKey string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.running[issueKey]; ok {
		return false
	}
	s.running[issueKey] = struct{}{}
	return true
}

func (s *Service) release(issueKey string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.running, issueKey)
}

func buildPRBody(issueKey string, input db.ImplementationInput, summary string) string {
	parts := []string{"## Summary", "", summary, "", "## Jira Ticket", "", issueKey + " - " + input.IssueSummary}
	if strings.TrimSpace(input.SuggestedScope) != "" {
		parts = append(parts, "", "## Archon Notes", "", "Scope as implemented: "+input.SuggestedScope)
	}
	if len(input.OutOfScope) > 0 {
		parts = append(parts, "", "Out of scope (by design):")
		for _, item := range input.OutOfScope {
			parts = append(parts, "- "+item)
		}
	}
	return strings.Join(parts, "\n")
}
