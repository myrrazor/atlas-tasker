package apperr

import (
	"errors"
	"testing"
)

func TestCodeOfHeuristicsAndEnvelope(t *testing.T) {
	tests := []struct {
		name string
		err  error
		want Code
		exit int
	}{
		{name: "typed", err: New(CodeBusy, "workspace is busy"), want: CodeBusy, exit: 6},
		{name: "not found string", err: errors.New("ticket APP-1 not found"), want: CodeNotFound, exit: 3},
		{name: "conflict string", err: errors.New("ticket APP-1 is already claimed by agent:builder-1"), want: CodeConflict, exit: 4},
		{name: "invalid string", err: errors.New("invalid actor: nope"), want: CodeInvalidInput, exit: 2},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := CodeOf(tt.err); got != tt.want {
				t.Fatalf("CodeOf() = %s, want %s", got, tt.want)
			}
			if got := ExitCode(tt.err); got != tt.exit {
				t.Fatalf("ExitCode() = %d, want %d", got, tt.exit)
			}
			env := Envelope(tt.err)
			errorPayload := env["error"].(map[string]any)
			if errorPayload["code"] != tt.want {
				t.Fatalf("Envelope code = %v, want %s", errorPayload["code"], tt.want)
			}
		})
	}
}
