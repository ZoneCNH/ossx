package ossx

import (
	"context"
	"errors"
	"testing"
)

func TestErrorFormattingAndWrapping(t *testing.T) {
	cause := errors.New("root cause")

	tests := []struct {
		name string
		err  *Error
		want string
	}{
		{name: "nil", err: nil, want: ""},
		{name: "kind only", err: &Error{Kind: ErrorKindInternal}, want: "internal"},
		{name: "with op message", err: &Error{Kind: ErrorKindConfig, Op: "op", Message: "bad"}, want: "config: op: bad"},
		{name: "cause fallback", err: &Error{Kind: ErrorKindTransfer, Op: "op", Cause: cause}, want: "transfer: op: root cause"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.err.Error(); got != tt.want {
				t.Fatalf("Error() = %q, want %q", got, tt.want)
			}
		})
	}

	if (*Error)(nil).Unwrap() != nil {
		t.Fatal("nil error should unwrap to nil")
	}
	wrapped := WrapError(ErrorKindTransfer, "op", "wrapped", true, cause)
	if !errors.Is(wrapped, cause) {
		t.Fatalf("wrapped error should contain cause")
	}
	if !wrapped.Retryable {
		t.Fatal("retryable flag not preserved")
	}
}

func TestErrorHelpers(t *testing.T) {
	cause := errors.New("missing")
	err := NewError(ErrorKindNotFound, "get", "", false)
	if err.Message != "" {
		t.Fatalf("unexpected message: %q", err.Message)
	}

	wrapped := newError(ErrorKindTransfer, "put", "", true, cause)
	if wrapped.Message != "missing" {
		t.Fatalf("cause message not promoted: %q", wrapped.Message)
	}
	if validationError("op", "bad input", cause).Kind != ErrorKindValidation {
		t.Fatal("validationError should use validation kind")
	}
	if contextError("op", context.Canceled).Kind != ErrorKindUnavailable {
		t.Fatal("context canceled should map to unavailable")
	}
	timeout := contextError("op", context.DeadlineExceeded)
	if timeout.Kind != ErrorKindTimeout || !timeout.Retryable {
		t.Fatalf("deadline should be retryable timeout: %#v", timeout)
	}
	if !IsKind(wrapped, ErrorKindTransfer) {
		t.Fatal("IsKind should find structured error")
	}
	if IsKind(errors.New("plain"), ErrorKindTransfer) {
		t.Fatal("IsKind should reject plain errors")
	}
	if errorKind(errors.New("plain")) != ErrorKindInternal {
		t.Fatal("plain error should map to internal")
	}
}
