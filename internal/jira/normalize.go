package jira

import (
	"fmt"
	"strings"

	"github.com/availhealth/archon/internal/jira/adf"
)

type NormalizedIssue struct {
	IssueKey               string
	IssueSummary           string
	IssueType              string
	Priority               string
	JiraStatus             string
	Labels                 []string
	Components             []string
	DescriptionText        string
	AcceptanceCriteriaText string
	CommentsText           string
	NormalizedText         string
}

func NormalizeIssue(issue Issue) (NormalizedIssue, error) {
	descriptionText := strings.TrimSpace(adf.PlainText(issue.Fields.Description))
	commentsText := strings.TrimSpace(normalizeComments(issue.Fields.Comment.Comments))
	components := make([]string, 0, len(issue.Fields.Components))
	for _, component := range issue.Fields.Components {
		if strings.TrimSpace(component.Name) != "" {
			components = append(components, component.Name)
		}
	}

	normalized := NormalizedIssue{
		IssueKey:        issue.Key,
		IssueSummary:    issue.Fields.Summary,
		IssueType:       issue.Fields.IssueType.Name,
		Priority:        issue.Fields.Priority.Name,
		JiraStatus:      issue.Fields.Status.Name,
		Labels:          append([]string(nil), issue.Fields.Labels...),
		Components:      components,
		DescriptionText: descriptionText,
		CommentsText:    commentsText,
	}
	normalized.NormalizedText = buildNormalizedText(issue, normalized)
	return normalized, nil
}

func normalizeComments(comments []IssueComment) string {
	if len(comments) == 0 {
		return ""
	}

	var sections []string
	for _, comment := range comments {
		body := strings.TrimSpace(adf.PlainText(comment.Body))
		if body == "" {
			continue
		}
		author := strings.TrimSpace(comment.Author.DisplayName)
		if author == "" {
			author = "Unknown author"
		}
		sections = append(sections, fmt.Sprintf("%s:\n%s", author, body))
	}
	return strings.Join(sections, "\n\n")
}

func buildNormalizedText(issue Issue, normalized NormalizedIssue) string {
	var parts []string
	parts = append(parts, fmt.Sprintf("Issue: %s", issue.Key))
	parts = append(parts, fmt.Sprintf("Summary: %s", issue.Fields.Summary))
	if normalized.IssueType != "" {
		parts = append(parts, fmt.Sprintf("Issue Type: %s", normalized.IssueType))
	}
	if normalized.Priority != "" {
		parts = append(parts, fmt.Sprintf("Priority: %s", normalized.Priority))
	}
	if normalized.JiraStatus != "" {
		parts = append(parts, fmt.Sprintf("Status: %s", normalized.JiraStatus))
	}
	if parent := issue.Fields.Parent; parent != nil {
		parts = append(parts, fmt.Sprintf("Parent: %s %s", parent.Key, strings.TrimSpace(parent.Fields.Summary)))
	}
	if len(normalized.Labels) > 0 {
		parts = append(parts, fmt.Sprintf("Labels: %s", strings.Join(normalized.Labels, ", ")))
	}
	if len(normalized.Components) > 0 {
		parts = append(parts, fmt.Sprintf("Components: %s", strings.Join(normalized.Components, ", ")))
	}
	linked := normalizedLinkedIssues(issue.Fields.IssueLinks)
	if len(linked) > 0 {
		parts = append(parts, fmt.Sprintf("Linked Issues: %s", strings.Join(linked, "; ")))
	}
	if normalized.DescriptionText != "" {
		parts = append(parts, "Description:\n"+normalized.DescriptionText)
	}
	if normalized.AcceptanceCriteriaText != "" {
		parts = append(parts, "Acceptance Criteria:\n"+normalized.AcceptanceCriteriaText)
	}
	if normalized.CommentsText != "" {
		parts = append(parts, "Comments:\n"+normalized.CommentsText)
	}
	return strings.Join(parts, "\n\n")
}

func normalizedLinkedIssues(links []IssueLink) []string {
	items := make([]string, 0, len(links))
	for _, link := range links {
		switch {
		case link.OutwardIssue != nil:
			items = append(items, fmt.Sprintf("%s %s", link.OutwardIssue.Key, strings.TrimSpace(link.OutwardIssue.Fields.Summary)))
		case link.InwardIssue != nil:
			items = append(items, fmt.Sprintf("%s %s", link.InwardIssue.Key, strings.TrimSpace(link.InwardIssue.Fields.Summary)))
		}
	}
	return items
}
