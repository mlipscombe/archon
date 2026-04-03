package implement

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/availhealth/archon/internal/db"
)

func BuildPrompt(input db.ImplementationInput) string {
	var builder strings.Builder
	builder.WriteString("You are implementing a software change described by the following Jira ticket.\n")
	builder.WriteString("Implement exactly what is described - no more, no less.\n")
	builder.WriteString("Do not create branches, commit, or push. Archon performs deterministic git operations outside the agent.\n")
	builder.WriteString("Write or update tests to cover the new behavior.\n")
	builder.WriteString("Respond with ONLY valid JSON using this schema:\n")
	builder.WriteString("{\n  \"task\": \"implementation\",\n  \"status\": \"success\" | \"failure\",\n  \"summary\": \"plain text summary\",\n  \"files_changed\": [\"path\"]\n}\n\n")
	builder.WriteString(fmt.Sprintf("Issue: %s\nSummary: %s\n\n", input.IssueKey, input.IssueSummary))
	builder.WriteString("Ticket context:\n")
	builder.WriteString(input.NormalizedText)
	builder.WriteString("\n\nArchon analysis:\n")
	if input.SuggestedScope != "" {
		builder.WriteString("Suggested scope:\n" + input.SuggestedScope + "\n")
	}
	if strings.TrimSpace(input.RevisionFeedback) != "" {
		builder.WriteString("Revision feedback to address:\n" + strings.TrimSpace(input.RevisionFeedback) + "\n")
	}
	if len(input.ImplementationNotes) > 0 {
		builder.WriteString("Implementation notes:\n")
		for _, item := range input.ImplementationNotes {
			builder.WriteString("- " + item + "\n")
		}
	}
	if len(input.OutOfScope) > 0 {
		builder.WriteString("Out of scope assumptions:\n")
		for _, item := range input.OutOfScope {
			builder.WriteString("- " + item + "\n")
		}
	}
	builder.WriteString("\nRepository context:\n")
	builder.WriteString(repoContext(input.WorktreePath))
	return builder.String()
}

func repoContext(worktreePath string) string {
	entries, err := os.ReadDir(worktreePath)
	if err != nil {
		return fmt.Sprintf("Repository root: %s", worktreePath)
	}
	topLevel := make([]string, 0, len(entries))
	for _, entry := range entries {
		name := entry.Name()
		if strings.HasPrefix(name, ".git") {
			continue
		}
		if entry.IsDir() {
			topLevel = append(topLevel, name+"/")
		} else {
			topLevel = append(topLevel, name)
		}
	}
	sort.Strings(topLevel)
	manager := "unknown"
	switch {
	case exists(filepath.Join(worktreePath, "go.work")) || exists(filepath.Join(worktreePath, "go.mod")):
		manager = "go"
	case exists(filepath.Join(worktreePath, "pnpm-workspace.yaml")):
		manager = "pnpm"
	case exists(filepath.Join(worktreePath, "package.json")):
		manager = "npm"
	}
	return fmt.Sprintf("Repository root: %s\nDetected build/package manager: %s\nTop-level entries: %s", worktreePath, manager, strings.Join(topLevel, ", "))
}

func exists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}
