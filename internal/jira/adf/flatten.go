package adf

import (
	"fmt"
	"strings"
)

func PlainText(value any) string {
	text := flatten(value, 0)
	return normalizeSpacing(text)
}

func flatten(value any, depth int) string {
	switch node := value.(type) {
	case map[string]any:
		typ, _ := node["type"].(string)
		switch typ {
		case "doc":
			return joinBlocks(contentSlice(node), depth)
		case "paragraph":
			return joinInline(contentSlice(node), depth)
		case "text":
			text, _ := node["text"].(string)
			return text
		case "hardBreak":
			return "\n"
		case "heading":
			attrs := nodeMap(node["attrs"])
			level := intNumber(attrs["level"])
			if level <= 0 {
				level = 1
			}
			return fmt.Sprintf("%s %s", strings.Repeat("#", level), strings.TrimSpace(joinInline(contentSlice(node), depth)))
		case "bulletList":
			return flattenList(contentSlice(node), false, depth)
		case "orderedList":
			return flattenList(contentSlice(node), true, depth)
		case "listItem":
			return joinBlocks(contentSlice(node), depth+1)
		case "codeBlock":
			attrs := nodeMap(node["attrs"])
			lang, _ := attrs["language"].(string)
			body := strings.TrimSpace(joinInline(contentSlice(node), depth))
			return fmt.Sprintf("```%s\n%s\n```", lang, body)
		case "blockquote":
			text := joinBlocks(contentSlice(node), depth)
			lines := splitNonEmptyLines(text)
			for i, line := range lines {
				lines[i] = "> " + line
			}
			return strings.Join(lines, "\n")
		case "table":
			return flattenTable(contentSlice(node), depth)
		case "tableRow", "tableCell", "tableHeader", "panel":
			return joinBlocks(contentSlice(node), depth)
		case "rule":
			return "---"
		default:
			if text, ok := node["text"].(string); ok {
				return text
			}
			return joinBlocks(contentSlice(node), depth)
		}
	case []any:
		parts := make([]string, 0, len(node))
		for _, item := range node {
			flattened := strings.TrimSpace(flatten(item, depth))
			if flattened != "" {
				parts = append(parts, flattened)
			}
		}
		return strings.Join(parts, "\n")
	case string:
		return node
	default:
		return ""
	}
}

func joinBlocks(items []any, depth int) string {
	parts := make([]string, 0, len(items))
	for _, item := range items {
		flattened := strings.TrimSpace(flatten(item, depth))
		if flattened != "" {
			parts = append(parts, flattened)
		}
	}
	return strings.Join(parts, "\n\n")
}

func joinInline(items []any, depth int) string {
	var builder strings.Builder
	for _, item := range items {
		builder.WriteString(flatten(item, depth))
	}
	return builder.String()
}

func flattenList(items []any, ordered bool, depth int) string {
	lines := make([]string, 0, len(items))
	for idx, item := range items {
		text := strings.TrimSpace(flatten(item, depth))
		if text == "" {
			continue
		}
		prefix := "- "
		if ordered {
			prefix = fmt.Sprintf("%d. ", idx+1)
		}
		indent := strings.Repeat("  ", depth)
		formatted := prefixMultiline(text, indent, prefix)
		lines = append(lines, formatted)
	}
	return strings.Join(lines, "\n")
}

func flattenTable(rows []any, depth int) string {
	lines := make([]string, 0, len(rows))
	for _, row := range rows {
		cells := contentSlice(row)
		parts := make([]string, 0, len(cells))
		for _, cell := range cells {
			text := strings.TrimSpace(flatten(cell, depth))
			text = strings.ReplaceAll(text, "\n", " ")
			parts = append(parts, text)
		}
		if len(parts) > 0 {
			lines = append(lines, "| "+strings.Join(parts, " | ")+" |")
		}
	}
	return strings.Join(lines, "\n")
}

func contentSlice(value any) []any {
	node := nodeMap(value)
	content, _ := node["content"].([]any)
	return content
}

func nodeMap(value any) map[string]any {
	if node, ok := value.(map[string]any); ok {
		return node
	}
	return map[string]any{}
}

func intNumber(value any) int {
	switch v := value.(type) {
	case int:
		return v
	case int64:
		return int(v)
	case float64:
		return int(v)
	default:
		return 0
	}
}

func prefixMultiline(text, indent, prefix string) string {
	lines := strings.Split(text, "\n")
	for i, line := range lines {
		trimmed := strings.TrimRight(line, " ")
		if i == 0 {
			lines[i] = indent + prefix + trimmed
			continue
		}
		lines[i] = indent + strings.Repeat(" ", len(prefix)) + trimmed
	}
	return strings.Join(lines, "\n")
}

func splitNonEmptyLines(text string) []string {
	lines := strings.Split(text, "\n")
	filtered := make([]string, 0, len(lines))
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if trimmed != "" {
			filtered = append(filtered, trimmed)
		}
	}
	return filtered
}

func normalizeSpacing(text string) string {
	lines := strings.Split(text, "\n")
	result := make([]string, 0, len(lines))
	blankCount := 0
	for _, line := range lines {
		trimmedRight := strings.TrimRight(line, " \t")
		if strings.TrimSpace(trimmedRight) == "" {
			blankCount++
			if blankCount > 1 {
				continue
			}
			result = append(result, "")
			continue
		}
		blankCount = 0
		result = append(result, trimmedRight)
	}
	return strings.TrimSpace(strings.Join(result, "\n"))
}
