package db

import (
	"context"
	"fmt"

	"github.com/availhealth/archon/internal/config"
)

type RubricCriterion struct {
	ID          string
	Description string
	AppliesTo   string
	IsRequired  bool
}

func (s *Store) SyncProject(ctx context.Context, project config.JiraProject) error {
	_, err := s.DB.ExecContext(ctx, `
		INSERT INTO projects (
			project_key,
			watch_filter,
			auto_transition,
			transition_in_progress,
			transition_in_review,
			transition_done
		) VALUES (?, ?, ?, ?, ?, ?)
		ON CONFLICT(project_key) DO UPDATE SET
			watch_filter = excluded.watch_filter,
			auto_transition = excluded.auto_transition,
			transition_in_progress = excluded.transition_in_progress,
			transition_in_review = excluded.transition_in_review,
			transition_done = excluded.transition_done,
			updated_at = CURRENT_TIMESTAMP
	`,
		project.Key,
		project.WatchFilter,
		project.AutoTransition,
		project.Transitions.InProgress,
		project.Transitions.InReview,
		project.Transitions.Done,
	)
	if err != nil {
		return fmt.Errorf("sync project: %w", err)
	}
	return nil
}

func (s *Store) SeedDefaultRubric(ctx context.Context, projectKey string) error {
	criteria := []RubricCriterion{
		{ID: "clear_goal", Description: "The problem or objective is unambiguously stated", AppliesTo: "all", IsRequired: true},
		{ID: "testable_acceptance_criteria", Description: "Acceptance criteria exist and are specific enough to write a test", AppliesTo: "all", IsRequired: true},
		{ID: "scope_defined", Description: "In-scope and out-of-scope behavior is explicitly or inferably defined", AppliesTo: "all", IsRequired: true},
		{ID: "dependencies_named", Description: "All referenced systems, APIs, or data models are named", AppliesTo: "all", IsRequired: true},
		{ID: "steps_to_reproduce", Description: "Steps to reproduce are provided", AppliesTo: "Bug", IsRequired: true},
		{ID: "expected_vs_actual", Description: "Expected and actual behavior are described", AppliesTo: "Bug", IsRequired: true},
		{ID: "environment_specified", Description: "The environment where the bug occurs is specified", AppliesTo: "Bug", IsRequired: true},
		{ID: "research_question", Description: "A specific research question is stated", AppliesTo: "Spike", IsRequired: true},
		{ID: "defined_deliverable", Description: "A defined deliverable exists (doc, decision, or PoC)", AppliesTo: "Spike", IsRequired: true},
	}

	for _, criterion := range criteria {
		_, err := s.DB.ExecContext(ctx, `
			INSERT OR IGNORE INTO rubric_criteria (
				id,
				description,
				applies_to,
				is_required,
				project_key,
				is_enabled
			) VALUES (?, ?, ?, ?, NULL, 1)
		`, criterion.ID, criterion.Description, criterion.AppliesTo, criterion.IsRequired)
		if err != nil {
			return fmt.Errorf("seed rubric criterion %s: %w", criterion.ID, err)
		}
	}

	s.log.Info("default rubric ready", "project_key", projectKey, "criteria_count", len(criteria))
	return nil
}
