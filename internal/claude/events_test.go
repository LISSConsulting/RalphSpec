package claude

import "testing"

func TestClassifyOutcome(t *testing.T) {
	tests := []struct {
		message string
		want    OutcomeKind
	}{
		{"success", OutcomeSuccess},
		{"Usage limit reached for 5-hour window", OutcomeQuotaExhausted},
		{"OAuth session expired and could not be refreshed", OutcomeAuthenticationRequired},
		{"API Error: The model has reached its context window limit.", OutcomeContextExhausted},
		{"provider temporarily unavailable: status 503", OutcomeTransientProvider},
		{"context canceled", OutcomeCancelled},
		{"process exited with status 1", OutcomeAgentFailure},
	}

	for _, tt := range tests {
		t.Run(tt.message, func(t *testing.T) {
			if got := ClassifyOutcome(tt.message).Kind; got != tt.want {
				t.Fatalf("ClassifyOutcome(%q) = %q, want %q", tt.message, got, tt.want)
			}
		})
	}
}

func TestAsOutcomeWraps(t *testing.T) {
	want := TerminalOutcome{Kind: OutcomeQuotaExhausted, Message: "quota reached"}
	err := &OutcomeError{Outcome: want}
	got, ok := AsOutcome(err)
	if !ok || got != want {
		t.Fatalf("AsOutcome = %#v, %v; want %#v, true", got, ok, want)
	}
}
