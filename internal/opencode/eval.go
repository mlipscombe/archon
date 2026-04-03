package opencode

type EvaluationResult struct {
	Decision              string               `json:"decision"`
	Confidence            float64              `json:"confidence"`
	Reasoning             string               `json:"reasoning"`
	MissingElements       []string             `json:"missing_elements"`
	ClarifyingQuestions   []ClarifyingQuestion `json:"clarifying_questions"`
	ImplementationNotes   []string             `json:"implementation_notes"`
	SuggestedScope        string               `json:"suggested_scope"`
	OutOfScopeAssumptions []string             `json:"out_of_scope_assumptions"`
}

type ClarifyingQuestion struct {
	Question        string `json:"question"`
	SuggestedAnswer string `json:"suggested_answer"`
	Rationale       string `json:"rationale"`
}

func (r EvaluationResult) EffectiveDecision(confidenceThreshold float64) string {
	if r.Confidence < confidenceThreshold {
		return "NOT_READY"
	}
	if r.Decision == "READY" {
		return "READY"
	}
	return "NOT_READY"
}
