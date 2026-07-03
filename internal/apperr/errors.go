package apperr

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strings"
)

type Code string

const (
	CodeInvalidInput     Code = "invalid_input"
	CodeNotFound         Code = "not_found"
	CodeConflict         Code = "conflict"
	CodePermissionDenied Code = "permission_denied"
	CodeBusy             Code = "busy"
	CodeRepairNeeded     Code = "repair_needed"
	CodeInternal         Code = "internal"
)

type Error struct {
	Code    Code
	Message string
	Cause   error
}

func (e *Error) Error() string {
	if strings.TrimSpace(e.Message) != "" {
		return e.Message
	}
	if e.Cause != nil {
		return e.Cause.Error()
	}
	return string(e.Code)
}

func (e *Error) Unwrap() error {
	return e.Cause
}

func New(code Code, message string) error {
	return &Error{Code: code, Message: strings.TrimSpace(message)}
}

func Wrap(code Code, err error, format string, args ...any) error {
	message := strings.TrimSpace(fmt.Sprintf(format, args...))
	return &Error{Code: code, Message: message, Cause: err}
}

func CodeOf(err error) Code {
	if err == nil {
		return ""
	}
	var appErr *Error
	if errors.As(err, &appErr) && appErr.Code != "" {
		return appErr.Code
	}
	if errors.Is(err, context.DeadlineExceeded) {
		return CodeBusy
	}
	if errors.Is(err, os.ErrNotExist) {
		return CodeNotFound
	}
	text := strings.ToLower(strings.TrimSpace(err.Error()))
	switch {
	case strings.Contains(text, "not found"):
		return CodeNotFound
	case strings.Contains(text, "already exists"), strings.Contains(text, "already claimed"), strings.Contains(text, "claimed by"):
		return CodeConflict
	case strings.Contains(text, "invalid "), strings.Contains(text, " is required"), strings.Contains(text, "requires a reason"), strings.Contains(text, "must be "):
		return CodeInvalidInput
	case strings.Contains(text, "only the assigned reviewer"), strings.Contains(text, "not allowed"), strings.Contains(text, "must belong to"), strings.Contains(text, "permission"):
		return CodePermissionDenied
	case strings.Contains(text, "repair needed"), strings.Contains(text, "repair required"):
		return CodeRepairNeeded
	case strings.Contains(text, "busy"), strings.Contains(text, "locked"), strings.Contains(text, "timeout"):
		return CodeBusy
	default:
		return CodeInternal
	}
}

func ExitCode(err error) int {
	switch CodeOf(err) {
	case CodeInvalidInput:
		return 2
	case CodeNotFound:
		return 3
	case CodeConflict:
		return 4
	case CodePermissionDenied:
		return 5
	case CodeBusy:
		return 6
	case CodeRepairNeeded:
		return 7
	case "":
		return 0
	default:
		return 1
	}
}

func Envelope(err error) map[string]any {
	if err == nil {
		return map[string]any{"ok": true}
	}
	code := CodeOf(err)
	return map[string]any{
		"ok": false,
		"error": map[string]any{
			"code":    code,
			"message": err.Error(),
			"exit":    ExitCode(err),
		},
	}
}
