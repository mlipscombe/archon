package watcher

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/availhealth/archon/internal/config"
	"github.com/availhealth/archon/internal/db"
	"github.com/availhealth/archon/internal/evaluator"
	gh "github.com/availhealth/archon/internal/github"
	"github.com/availhealth/archon/internal/jira"
	"github.com/availhealth/archon/internal/logx"
)

type Poller struct {
	cfg    config.Config
	log    *slog.Logger
	store  *db.Store
	jira   *jira.Client
	eval   *evaluator.Service
	github *gh.Client
}

func New(cfg config.Config, logger *slog.Logger, store *db.Store, jiraClient *jira.Client, evaluatorService *evaluator.Service, githubClient *gh.Client) *Poller {
	return &Poller{
		cfg:    cfg,
		log:    logx.WithComponent(logger, "watcher"),
		store:  store,
		jira:   jiraClient,
		eval:   evaluatorService,
		github: githubClient,
	}
}

func (p *Poller) Start(ctx context.Context) error {
	p.log.Info("starting jira watcher", slog.String("project_key", p.cfg.Jira.Projects[0].Key), slog.Int("poll_interval_seconds", p.cfg.Jira.PollIntervalSeconds))
	p.runCycle(ctx)

	ticker := time.NewTicker(time.Duration(p.cfg.Jira.PollIntervalSeconds) * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return nil
		case <-ticker.C:
			p.runCycle(ctx)
		}
	}
}

func (p *Poller) runCycle(ctx context.Context) {
	cycleCtx, cancel := context.WithTimeout(ctx, 60*time.Second)
	defer cancel()

	processed, skippedFresh, err := p.pollOnce(cycleCtx)
	if err != nil {
		p.log.Error("poll cycle failed", slog.Any("error", err))
		return
	}
	p.log.Info("poll cycle complete", slog.Int("processed", processed), slog.Int("skipped_fresh", skippedFresh))
}

func (p *Poller) pollOnce(ctx context.Context) (int, int, error) {
	project := p.cfg.Jira.Projects[0]
	state, err := p.store.ProjectState(ctx, project.Key)
	if err != nil {
		return 0, 0, err
	}

	searchJQL := ensureOrderedJQL(state.WatchFilter)
	stableBefore := time.Now().UTC().Add(-time.Duration(p.cfg.Jira.DebounceSeconds) * time.Second)
	startAt := 0
	processed := 0
	skippedFresh := 0
	var highWater *time.Time

	for {
		result, err := p.jira.SearchIssues(ctx, searchJQL, startAt, 50)
		if err != nil {
			return processed, skippedFresh, err
		}

		for _, item := range result.Issues {
			updatedAt, err := jira.ParseTime(item.Fields.Updated)
			if err != nil {
				p.log.Warn("skip issue with invalid updated timestamp", slog.String("issue_key", item.Key), slog.Any("error", err))
				continue
			}

			if updatedAt.After(stableBefore) {
				skippedFresh++
				break
			}
			if hasBranch, err := p.github.HasMatchingBranch(ctx, gitBranchPrefix(item.Key)); err == nil && hasBranch {
				continue
			}
			if state.LastIssueUpdated != nil && !updatedAt.After(*state.LastIssueUpdated) {
				continue
			}

			issue, err := p.jira.GetIssue(ctx, item.Key)
			if err != nil {
				p.log.Error("load issue detail failed", slog.String("issue_key", item.Key), slog.Any("error", err))
				continue
			}
			normalized, err := jira.NormalizeIssue(issue)
			if err != nil {
				p.log.Error("normalize issue failed", slog.String("issue_key", item.Key), slog.Any("error", err))
				continue
			}

			snapshotInserted, err := p.store.UpsertObservedIssue(ctx, db.ObservedIssue{
				IssueKey:               normalized.IssueKey,
				IssueSummary:           normalized.IssueSummary,
				IssueType:              normalized.IssueType,
				Priority:               normalized.Priority,
				JiraStatus:             normalized.JiraStatus,
				IssueUpdatedAt:         updatedAt,
				NormalizedText:         normalized.NormalizedText,
				DescriptionText:        normalized.DescriptionText,
				AcceptanceCriteriaText: normalized.AcceptanceCriteriaText,
				CommentsText:           normalized.CommentsText,
				Labels:                 normalized.Labels,
				Components:             normalized.Components,
			})
			if err != nil {
				p.log.Error("persist observed issue failed", slog.String("issue_key", item.Key), slog.Any("error", err))
				continue
			}
			if snapshotInserted {
				if err := p.eval.EvaluateIssue(ctx, item.Key); err != nil {
					p.log.Error("evaluate issue failed", slog.String("issue_key", item.Key), slog.Any("error", err))
				}
			}

			processed++
			if highWater == nil || updatedAt.After(*highWater) {
				copyTime := updatedAt
				highWater = &copyTime
			}
		}

		startAt += len(result.Issues)
		if startAt >= result.Total || len(result.Issues) == 0 {
			break
		}
	}

	if highWater != nil {
		if err := p.store.SetProjectHighWaterMark(ctx, project.Key, *highWater); err != nil {
			return processed, skippedFresh, err
		}
	}

	return processed, skippedFresh, nil
}

func gitBranchPrefix(issueKey string) string {
	return "archon/" + strings.TrimSpace(issueKey) + "-"
}

func ensureOrderedJQL(jql string) string {
	trimmed := strings.TrimSpace(jql)
	if trimmed == "" {
		return "ORDER BY updated ASC"
	}
	if strings.Contains(strings.ToLower(trimmed), "order by") {
		return trimmed
	}
	return fmt.Sprintf("%s ORDER BY updated ASC", trimmed)
}
