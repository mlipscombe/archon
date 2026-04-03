package opencode

import (
	"encoding/json"
	"fmt"
	"strings"
)

func ParseEvaluationOutput(output string) (EvaluationResult, string, error) {
	output = strings.TrimSpace(output)
	if output == "" {
		return EvaluationResult{}, "", fmt.Errorf("empty opencode output")
	}

	if result, raw, err := parseDirect(output); err == nil {
		return result, raw, nil
	}

	lines := strings.Split(output, "\n")
	for i := len(lines) - 1; i >= 0; i-- {
		line := strings.TrimSpace(lines[i])
		if line == "" {
			continue
		}
		if result, raw, err := parseDirect(line); err == nil {
			return result, raw, nil
		}

		var payload any
		if err := json.Unmarshal([]byte(line), &payload); err == nil {
			if result, raw, ok := extractEvaluation(payload); ok {
				return result, raw, nil
			}
		}
	}

	return EvaluationResult{}, "", fmt.Errorf("no evaluation json object found in opencode output")
}

func parseDirect(candidate string) (EvaluationResult, string, error) {
	var result EvaluationResult
	if err := json.Unmarshal([]byte(candidate), &result); err != nil {
		return EvaluationResult{}, "", err
	}
	if err := ValidateEvaluation(&result); err != nil {
		return EvaluationResult{}, "", err
	}
	return result, candidate, nil
}

func extractEvaluation(payload any) (EvaluationResult, string, bool) {
	switch value := payload.(type) {
	case map[string]any:
		if candidate, ok := maybeDecodeEvaluationMap(value); ok {
			return candidate, mustMarshal(value), true
		}
		for _, nested := range value {
			if candidate, raw, ok := extractEvaluation(nested); ok {
				return candidate, raw, true
			}
		}
	case []any:
		for _, nested := range value {
			if candidate, raw, ok := extractEvaluation(nested); ok {
				return candidate, raw, true
			}
		}
	}
	return EvaluationResult{}, "", false
}

func maybeDecodeEvaluationMap(value map[string]any) (EvaluationResult, bool) {
	_, hasDecision := value["decision"]
	_, hasConfidence := value["confidence"]
	if !hasDecision || !hasConfidence {
		return EvaluationResult{}, false
	}

	data, err := json.Marshal(value)
	if err != nil {
		return EvaluationResult{}, false
	}
	var result EvaluationResult
	if err := json.Unmarshal(data, &result); err != nil {
		return EvaluationResult{}, false
	}
	if err := ValidateEvaluation(&result); err != nil {
		return EvaluationResult{}, false
	}
	return result, true
}

func mustMarshal(value any) string {
	data, err := json.Marshal(value)
	if err != nil {
		return ""
	}
	return string(data)
}
