package ossx

import (
	"context"
	"fmt"
	"sync"
	"time"
)

// InMemoryAdapter is a fully in-memory ObjectStorageAdapter intended for
// tests and the v1.0.2-alpha BlobStore default. Production deployments
// must use a real adapter (adapters/s3, adapters/aliyun) once shipped in v1.1.0.
type InMemoryAdapter struct {
	mu      sync.Mutex
	objects map[string]inMemoryObject
}

type inMemoryObject struct {
	body        []byte
	contentType string
	metadata    map[string]string
	etag        string
	createdAt   time.Time
}

// NewInMemoryAdapter returns a zero-state in-memory adapter.
func NewInMemoryAdapter() *InMemoryAdapter {
	return &InMemoryAdapter{objects: make(map[string]inMemoryObject)}
}

// Name returns the adapter identifier.
func (a *InMemoryAdapter) Name() string { return "in-memory" }

// PutObject stores the body and returns a synthetic etag.
func (a *InMemoryAdapter) PutObject(_ context.Context, key string, body []byte, contentType string, metadata map[string]string) (string, error) {
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.objects == nil {
		return "", ErrClosed
	}
	etag := fmt.Sprintf("\"in-memory-%s\"", computeChecksum(ChecksumMD5, body))
	a.objects[key] = inMemoryObject{
		body:        append([]byte(nil), body...),
		contentType: contentType,
		metadata:    copyStringMap(metadata),
		etag:        etag,
		createdAt:   time.Now().UTC(),
	}
	return etag, nil
}

// GetObject returns the body and ObjectInfo or ErrNotFound.
func (a *InMemoryAdapter) GetObject(_ context.Context, key string) ([]byte, ObjectInfo, error) {
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.objects == nil {
		return nil, ObjectInfo{}, ErrClosed
	}
	obj, ok := a.objects[key]
	if !ok {
		return nil, ObjectInfo{}, ErrNotFound
	}
	info := ObjectInfo{
		Key:         Key(key),
		Size:        int64(len(obj.body)),
		ContentType: obj.contentType,
		Metadata:    copyStringMap(obj.metadata),
		ETag:        obj.etag,
		CreatedAt:   obj.createdAt,
		ModifiedAt:  obj.createdAt,
	}
	return append([]byte(nil), obj.body...), info, nil
}

// DeleteObject is idempotent: returns ErrNotFound only when the key was missing.
// The BlobStore wrapper consults DeleteOptions.StrictNotFound to decide.
func (a *InMemoryAdapter) DeleteObject(_ context.Context, key string) error {
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.objects == nil {
		return ErrClosed
	}
	if _, ok := a.objects[key]; !ok {
		return ErrNotFound
	}
	delete(a.objects, key)
	return nil
}

// HeadObject returns metadata without body bytes.
func (a *InMemoryAdapter) HeadObject(_ context.Context, key string) (ObjectInfo, error) {
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.objects == nil {
		return ObjectInfo{}, ErrClosed
	}
	obj, ok := a.objects[key]
	if !ok {
		return ObjectInfo{}, ErrNotFound
	}
	return ObjectInfo{
		Key:         Key(key),
		Size:        int64(len(obj.body)),
		ContentType: obj.contentType,
		Metadata:    copyStringMap(obj.metadata),
		ETag:        obj.etag,
		CreatedAt:   obj.createdAt,
		ModifiedAt:  obj.createdAt,
	}, nil
}

// ListObjects returns prefix-matched keys, bounded by max, with continuation token.
func (a *InMemoryAdapter) ListObjects(_ context.Context, prefix string, max int, token string) ([]ObjectInfo, string, error) {
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.objects == nil {
		return nil, "", ErrClosed
	}
	all := make([]ObjectInfo, 0, len(a.objects))
	for k, obj := range a.objects {
		if !HasPrefix(k, prefix) {
			continue
		}
		all = append(all, ObjectInfo{
			Key:         Key(k),
			Size:        int64(len(obj.body)),
			ContentType: obj.contentType,
			Metadata:    copyStringMap(obj.metadata),
			ETag:        obj.etag,
			CreatedAt:   obj.createdAt,
			ModifiedAt:  obj.createdAt,
		})
	}
	all = SortedKeys(all)
	startIdx := 0
	if token != "" {
		for i, info := range all {
			if string(info.Key) == token {
				startIdx = i + 1
				break
			}
		}
	}
	if max <= 0 {
		max = 1000
	}
	end := startIdx + max
	next := ""
	if end < len(all) {
		next = string(all[end-1].Key)
	} else {
		end = len(all)
	}
	return all[startIdx:end], next, nil
}

// CloseAdapter clears the in-memory state.
func (a *InMemoryAdapter) CloseAdapter(_ context.Context) error {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.objects = nil
	return nil
}
