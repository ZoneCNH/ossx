package ossx

import (
	"errors"
	"fmt"
)

// ErrorKind classifies an ossx error for retry/routing decisions. The taxonomy
// mirrors kernel errx.ErrorKind so callers and resiliencx classifiers can map
// reliably. Keep this list in sync with the ErrorKind* constants below.
type ErrorKind string

const (
	// ErrorKindConfig is a construction/configuration error (not retryable).
	ErrorKindConfig ErrorKind = "config"
	// ErrorKindValidation is a client-side validation error (not retryable).
	ErrorKindValidation ErrorKind = "validation"
	// ErrorKindConnection is a provider connection failure (retryable).
	ErrorKindConnection ErrorKind = "connection"
	// ErrorKindUnavailable is provider unavailability (retryable).
	ErrorKindUnavailable ErrorKind = "unavailable"
	// ErrorKindTimeout is an operation timeout (retryable).
	ErrorKindTimeout ErrorKind = "timeout"
	// ErrorKindAuth is an authentication/authorization failure (not retryable).
	ErrorKindAuth ErrorKind = "auth"
	// ErrorKindConflict is a version/existence conflict (not retryable).
	ErrorKindConflict ErrorKind = "conflict"
	// ErrorKindRateLimit is provider-side throttling (retryable).
	ErrorKindRateLimit ErrorKind = "rate_limit"
	// ErrorKindCanceled is a context cancellation (fatal).
	ErrorKindCanceled ErrorKind = "canceled"
	// ErrorKindNotFound is a missing object (not retryable).
	ErrorKindNotFound ErrorKind = "not_found"
	// ErrorKindAlreadyExist is a duplicate object (not retryable).
	ErrorKindAlreadyExist ErrorKind = "already_exists"
	// ErrorKindInternal is an unexpected internal error (not retryable).
	ErrorKindInternal ErrorKind = "internal"
	// ErrorKindChecksum is a checksum mismatch (not retryable).
	ErrorKindChecksum ErrorKind = "checksum"
	// ErrorKindClosed is operation on a closed store (not retryable).
	ErrorKindClosed ErrorKind = "closed"
	// ErrorKindNotImplemented is an unimplemented operation (not retryable).
	ErrorKindNotImplemented ErrorKind = "not_implemented"
)

// isRetryableKind reports whether errors of this kind should be retried by the
// resiliencx retry wrapper. Mirrors the errx.Retryable convention.
func isRetryableKind(k ErrorKind) bool {
	switch k {
	case ErrorKindConnection, ErrorKindUnavailable, ErrorKindTimeout, ErrorKindRateLimit:
		return true
	default:
		return false
	}
}

// Error is the typed ossx error (SPEC §11). Provider-specific errors are
// translated to *Error at the adapter boundary so the public API exposes only
// stable types. Implements error, Unwrap (for errors.Is/As), and ErrorKind().
type Error struct {
	Kind      ErrorKind
	Code      string
	Op        string
	Message   string
	Cause     error
	Retryable bool
}

// Error implements error.
func (e *Error) Error() string {
	if e == nil {
		return "<nil>"
	}
	msg := e.Message
	if e.Op != "" {
		msg = e.Op + ": " + msg
	}
	if e.Cause != nil {
		msg += ": " + e.Cause.Error()
	}
	return "ossx: " + msg
}

// Unwrap allows errors.Is / errors.As to walk the cause chain.
func (e *Error) Unwrap() error { return e.Cause }

// IsKind reports whether this error is of the given kind.
func (e *Error) IsKind(k ErrorKind) bool { return e != nil && e.Kind == k }

// Is implements errors.Is: an *Error matches a sentinel *Error when they share
// the same Kind. This lets callers write `errors.Is(err, ErrInvalidConfig)` to
// check the category without comparing pointers, since ErrInvalidConfig is one
// specific instance. A nil *Error matches nothing.
func (e *Error) Is(target error) bool {
	if e == nil {
		return false
	}
	var t *Error
	if errors.As(target, &t) {
		return e.Kind == t.Kind
	}
	return false
}

// newError constructs a typed *Error.
func newError(kind ErrorKind, op, message string) *Error {
	return &Error{Kind: kind, Op: op, Message: message, Retryable: isRetryableKind(kind)}
}

// wrapError constructs a typed *Error wrapping a cause.
func wrapError(kind ErrorKind, op, message string, cause error) *Error {
	return &Error{Kind: kind, Op: op, Message: message, Cause: cause, Retryable: isRetryableKind(kind)}
}

// WrapExport is the exported error wrapper for external adapters (e.g.,
// adapters/aliyun) to translate provider errors at the boundary (SPEC §11).
func WrapExport(kind ErrorKind, op, message string, cause error) *Error {
	return wrapError(kind, op, message, cause)
}

// NewExport is the exported error constructor for external adapters.
func NewExport(kind ErrorKind, op, message string) *Error {
	return newError(kind, op, message)
}

// ErrorKindExport re-exports ErrorKind for adapter packages that reference it
// via ossx.ErrorKindExport (alias). Use ossx.ErrorKind directly instead.
type ErrorKindExport = ErrorKind

// Convenience re-exports of the kind constants for adapter use.
var (
	ErrorKindConfigExport       = ErrorKindConfig
	ErrorKindValidationExport   = ErrorKindValidation
	ErrorKindConnectionExport   = ErrorKindConnection
	ErrorKindUnavailableExport  = ErrorKindUnavailable
	ErrorKindTimeoutExport      = ErrorKindTimeout
	ErrorKindAuthExport         = ErrorKindAuth
	ErrorKindConflictExport     = ErrorKindConflict
	ErrorKindRateLimitExport    = ErrorKindRateLimit
	ErrorKindCanceledExport     = ErrorKindCanceled
	ErrorKindNotFoundExport     = ErrorKindNotFound
	ErrorKindAlreadyExistExport = ErrorKindAlreadyExist
	ErrorKindInternalExport     = ErrorKindInternal
)

// errorKind inspects err and returns its ErrorKind, defaulting to Internal.
// Walks Unwrap chains to find an *Error; falls back to context classification.
func errorKind(err error) ErrorKind {
	if err == nil {
		return ErrorKindInternal
	}
	var e *Error
	if errors.As(err, &e) {
		return e.Kind
	}
	return ErrorKindInternal
}

// isRetryable inspects err to decide retry eligibility.
func isRetryable(err error) bool {
	if err == nil {
		return false
	}
	var e *Error
	if errors.As(err, &e) {
		return e.Retryable
	}
	return false
}

// kindClass maps an ossx ErrorKind to a retry classification for resiliencx.
type retryClass int

const (
	retryClassRetryable retryClass = iota
	retryClassNonRetryable
	retryClassFatal
)

func kindToClass(k ErrorKind) retryClass {
	switch k {
	case ErrorKindCanceled:
		return retryClassFatal
	case ErrorKindConnection, ErrorKindUnavailable, ErrorKindTimeout, ErrorKindRateLimit:
		return retryClassRetryable
	default:
		return retryClassNonRetryable
	}
}

// classifyError inspects err and returns its retry class.
func classifyError(err error) retryClass {
	if err == nil {
		return retryClassNonRetryable
	}
	var e *Error
	if errors.As(err, &e) {
		return kindToClass(e.Kind)
	}
	return retryClassNonRetryable
}

// --- Sentinel errors (kept for backward compatibility with v1.0.2-alpha
// tests and any external callers that use errors.Is). New code should prefer
// *Error + ErrorKind. ---

var (
	ErrInvalidConfig    = newError(ErrorKindConfig, "config", "invalid config")
	ErrNotFound         = newError(ErrorKindNotFound, "", "object not found")
	ErrConflict         = newError(ErrorKindConflict, "", "conflict")
	ErrPermission       = newError(ErrorKindAuth, "", "permission denied")
	ErrChecksumMismatch = newError(ErrorKindChecksum, "", "checksum mismatch")
	ErrTimeout          = newError(ErrorKindTimeout, "", "operation timeout")
	ErrCancelled        = newError(ErrorKindCanceled, "", "operation cancelled")
	ErrProviderFailure  = newError(ErrorKindUnavailable, "", "provider failure")
	ErrClosed           = newError(ErrorKindClosed, "", "blobstore closed")
	ErrInvalidKey       = newError(ErrorKindValidation, "", "invalid key")
	ErrInvalidMetadata  = newError(ErrorKindValidation, "", "invalid metadata")
	ErrNotImplemented   = newError(ErrorKindNotImplemented, "", "not implemented in this release")
)

// ctxCancelledError is returned when context is cancelled (errors.Is checks
// context.Canceled via Cause walk in blobstore.go).
func ctxCancelledError(err error) error {
	return wrapError(ErrorKindCanceled, "", "operation cancelled", err)
}

// fmtErrorf wraps fmt.Errorf for the few call sites that need formatting.
func fmtErrorf(format string, args ...any) error {
	return fmt.Errorf(format, args...)
}
