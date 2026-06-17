package ossx

import "errors"

// Typed errors per SPEC §11. Provider-specific errors are translated
// to these stable types at adapter boundaries.
var (
	ErrInvalidConfig    = errors.New("ossx: invalid config")
	ErrNotFound         = errors.New("ossx: object not found")
	ErrConflict         = errors.New("ossx: conflict")
	ErrPermission       = errors.New("ossx: permission denied")
	ErrChecksumMismatch = errors.New("ossx: checksum mismatch")
	ErrTimeout          = errors.New("ossx: operation timeout")
	ErrCancelled        = errors.New("ossx: operation cancelled")
	ErrProviderFailure  = errors.New("ossx: provider failure")
	ErrClosed           = errors.New("ossx: blobstore closed")
	ErrInvalidKey       = errors.New("ossx: invalid key")
	ErrInvalidMetadata  = errors.New("ossx: invalid metadata")
	ErrNotImplemented   = errors.New("ossx: not implemented in this release")
)
