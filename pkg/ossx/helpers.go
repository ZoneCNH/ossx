package ossx

import (
	"crypto/md5"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"hash"
	"hash/crc32"
	"io"
	"sort"
)

// helpers.go holds internal helpers shared by blobstore.go and adapters.

// copyStringMap returns a defensive copy (nil-safe).
func copyStringMap(m map[string]string) map[string]string {
	if m == nil {
		return nil
	}
	out := make(map[string]string, len(m))
	for k, v := range m {
		out[k] = v
	}
	return out
}

// sortedObjectInfos returns items sorted by Key for stable List output (BR-006).
func sortedObjectInfos(items []ObjectInfo) []ObjectInfo {
	out := make([]ObjectInfo, len(items))
	copy(out, items)
	sort.Slice(out, func(i, j int) bool { return out[i].Key < out[j].Key })
	return out
}

// validateChecksumAlgo checks alg against the policy allowlist.
func validateChecksumAlgo(alg ChecksumAlgorithm, allowed []ChecksumAlgorithm) error {
	if len(allowed) == 0 {
		switch alg {
		case ChecksumSHA256, ChecksumMD5, ChecksumCRC32:
			return nil
		default:
			return newError(ErrorKindConfig, "checksum", fmt.Sprintf("unsupported algorithm %q", alg))
		}
	}
	for _, a := range allowed {
		if a == alg {
			return nil
		}
	}
	return newError(ErrorKindConfig, "checksum", fmt.Sprintf("algorithm %q not in policy", alg))
}

// computeChecksum returns the hex digest of data under alg.
func computeChecksum(alg ChecksumAlgorithm, data []byte) string {
	switch alg {
	case ChecksumSHA256:
		sum := sha256.Sum256(data)
		return hex.EncodeToString(sum[:])
	case ChecksumMD5:
		sum := md5.Sum(data)
		return hex.EncodeToString(sum[:])
	case ChecksumCRC32:
		sum := crc32.ChecksumIEEE(data)
		return fmt.Sprintf("%08x", sum)
	default:
		return ""
	}
}

// newHasher returns a streaming hash for alg, or nil if unsupported.
func newHasher(alg ChecksumAlgorithm) hash.Hash {
	switch alg {
	case ChecksumSHA256:
		return sha256.New()
	case ChecksumMD5:
		return md5.New()
	case ChecksumCRC32:
		return crc32.NewIEEE()
	default:
		return nil
	}
}

// wrapChecksumVerifier wraps an ObjectReader so that reading it verifies the
// checksum and sets ChecksumVerified on EOF. The underlying reader is tee'd
// through the hasher without buffering the whole object (FR-004). On checksum
// mismatch, Read returns an *Error of kind checksum and the reader is closed.
func wrapChecksumVerifier(r *ObjectReader, info ObjectInfo) *ObjectReader {
	h := newHasher(info.ChecksumAlgo)
	if h == nil {
		return r
	}
	orig := r.ReadCloser
	verified := false
	r.ReadCloser = &checksumReader{
		base:     orig,
		hasher:   h,
		expected: info.ChecksumHex,
		onDone: func(ok bool) { verified = ok },
		verify:   func() bool { return verified },
	}
	return r
}

// checksumReader tees reads through a hasher and verifies the digest at EOF.
type checksumReader struct {
	base     io.ReadCloser
	hasher   hash.Hash
	expected string
	onDone   func(bool)
	verify   func() bool
	checked  bool
}

func (c *checksumReader) Read(p []byte) (int, error) {
	n, err := c.base.Read(p)
	if n > 0 {
		if _, werr := c.hasher.Write(p[:n]); werr != nil {
			return n, werr
		}
	}
	if err == io.EOF && !c.checked {
		c.checked = true
		got := hex.EncodeToString(c.hasher.Sum(nil))
		if got != c.expected {
			c.onDone(false)
			return n, newError(ErrorKindChecksum, "checksum", "streaming checksum mismatch")
		}
		c.onDone(true)
	}
	return n, err
}

func (c *checksumReader) Close() error { return c.base.Close() }
