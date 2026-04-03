package opencode

import (
	"fmt"
	"strings"
)

type PromptInput struct {
	IssueKey        string
	IssueSummary    string
	NormalizedText  string
	RubricCriteria  []string
	PriorContext    string
	ConfidenceFloor float64
}

func BuildEvaluationPrompt(input PromptInput) string {
	var builder strings.Builder
	builder.WriteString("You are Archon's requirements evaluator.\n")
	builder.WriteString("Read the Jira ticket carefully and inspect the repository in the current working directory before deciding whether the ticket is ready to implement autonomously.\n")
	builder.WriteString("Return ONLY valid JSON matching this exact schema and do not include markdown fences or commentary.\n\n")
	builder.WriteString("{\n")
	builder.WriteString("  \"decision\": \"READY\" | \"NOT_READY\",\n")
	builder.WriteString("  \"confidence\": <float 0.0-1.0>,\n")
	builder.WriteString("  \"reasoning\": \"plain text\",\n")
	builder.WriteString("  \"missing_elements\": [\"...\"],\n")
	builder.WriteString("  \"clarifying_questions\": [\n")
	builder.WriteString("    {\n")
	builder.WriteString("      \"question\": \"...\",\n")
	builder.WriteString("      \"suggested_answer\": \"repo-aware prose grounded in the codebase\",\n")
	builder.WriteString("      \"rationale\": \"...\"\n")
	builder.WriteString("    }\n")
	builder.WriteString("  ],\n")
	builder.WriteString("  \"implementation_notes\": [\"...\"],\n")
	builder.WriteString("  \"suggested_scope\": \"plain text\",\n")
	builder.WriteString("  \"out_of_scope_assumptions\": [\"...\"]\n")
	builder.WriteString("}\n\n")
	builder.WriteString(fmt.Sprintf("Confidence below %.2f will be treated as NOT_READY, so be honest.\n", input.ConfidenceFloor))
	builder.WriteString("Rules:\n")
	builder.WriteString("- Ask at most 5 clarifying questions.\n")
	builder.WriteString("- Suggested answers must be repo-aware prose. Exact file/line references are optional.\n")
	builder.WriteString("- Never ask questions already answered by the ticket.\n")
	builder.WriteString("- If the codebase already makes a decision obvious, reflect that in the suggested answer.\n")
	builder.WriteString("- When READY, keep implementation notes focused and concrete.\n\n")
	builder.WriteString(fmt.Sprintf("Issue: %s\nSummary: %s\n\n", input.IssueKey, input.IssueSummary))
	builder.WriteString("Readiness rubric:\n")
	for _, criterion := range input.RubricCriteria {
		builder.WriteString("- ")
		builder.WriteString(criterion)
		builder.WriteString("\n")
	}
	builder.WriteString("\nTicket context:\n")
	builder.WriteString(input.NormalizedText)
	if strings.TrimSpace(input.PriorContext) != "" {
		builder.WriteString("\n\nPrior evaluation context:\n")
		builder.WriteString(strings.TrimSpace(input.PriorContext))
	}
	return builder.String()
}
