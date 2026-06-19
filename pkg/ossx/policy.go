package ossx

import (
	"time"
)

// policy.go enforces lifecycle / retention / permission policies BEFORE adapter
// calls (FR-007 / AC-OSS-007). These checks run on the public BlobStore layer
// so every adapter inherits them for free. They return typed *Error (kind
// validation) so callers can distinguish policy rejection from provider errors.

// validateKeyPolicy enforces the permission policy (FR-007) for a write or
// presign operation against the given key. Returns nil if allowed, or a typed
// validation error if the key is denied or outside allowed prefixes.
func validateKeyPolicy(perm PermissionPolicy, key string) error {
	for _, p := range perm.DeniedPrefixes {
		if hasPrefix(key, p) {
			return newError(ErrorKindValidation, "policy", "key denied by permission policy")
		}
	}
	if len(perm.AllowedPrefixes) > 0 {
		allowed := false
		for _, p := range perm.AllowedPrefixes {
			if hasPrefix(key, p) {
				allowed = true
				break
			}
		}
		if !allowed {
			return newError(ErrorKindValidation, "policy", "key not in allowed prefixes")
		}
	}
	return nil
}

// validateRetentionDelete enforces the retention policy (FR-007) before a
// delete. A non-none retention mode forbids deleting objects younger than
// MaxDays. info.CreatedAt zero means unknown age — allow (provider enforces
// hard retention server-side when configured).
func validateRetentionDelete(ret RetentionPolicy, info ObjectInfo, now time.Time) error {
	if ret.Mode == RetentionModeNone || ret.Mode == "" {
		return nil
	}
	if ret.MaxDays <= 0 || info.CreatedAt.IsZero() {
		return nil
	}
	age := now.Sub(info.CreatedAt)
	if age < time.Duration(ret.MaxDays)*24*time.Hour {
		return newError(ErrorKindValidation, "policy", "delete forbidden by retention policy (object younger than MaxDays)")
	}
	return nil
}
