package opencode

import (
	"fmt"
	"strings"
)

func ValidateEvaluation(result *EvaluationResult) error {
	result.Decision = strings.ToUpper(strings.TrimSpace(result.Decision))
	if result.Decision != "READY" && result.Decision != "NOT_READY" {
		return fmt.Errorf("decision must be READY or NOT_READY")
	}
	if result.Confidence < 0 || result.Confidence > 1 {
		return fmt.Errorf("confidence must be between 0 and 1")
	}
	return nil
}
