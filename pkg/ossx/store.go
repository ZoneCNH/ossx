package ossx

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/aliyun/aliyun-oss-go-sdk/oss"
)

type objectStore interface {
	putObject(key string, body io.Reader, contentType string) error
	getObject(key string) (storedObject, error)
	deleteObject(key string) error
	listObjects(input listRequest) (storedList, error)
	check() error
}

type listRequest struct {
	Prefix  string
	Marker  string
	MaxKeys int
}

type storedObject struct {
	Info ObjectInfo
	Body io.ReadCloser
}

type storedList struct {
	Objects     []ObjectInfo
	IsTruncated bool
	NextMarker  string
}

type aliyunBucket interface {
	PutObject(objectKey string, reader io.Reader, options ...oss.Option) error
	GetObject(objectKey string, options ...oss.Option) (io.ReadCloser, error)
	GetObjectDetailedMeta(objectKey string, options ...oss.Option) (http.Header, error)
	DeleteObject(objectKey string, options ...oss.Option) error
	ListObjects(options ...oss.Option) (oss.ListObjectsResult, error)
	IsBucketExist() (bool, error)
}

type sdkAliyunBucket struct {
	bucket *oss.Bucket
}

type aliyunStore struct {
	bucket aliyunBucket
}

var newAliyunBucketFunc = newSDKAliyunBucket

func newObjectStore(cfg Config) (objectStore, error) {
	switch cfg.Provider {
	case ProviderOSS:
		return newAliyunStore(cfg)
	case ProviderS3, ProviderMinIO, ProviderAzure, ProviderGCS:
		return nil, NewError(ErrorKindConfig, "ossx.New", fmt.Sprintf("provider %q is reserved but not implemented in v1.0.1", cfg.Provider), false)
	default:
		return nil, NewError(ErrorKindConfig, "ossx.New", "unsupported provider: "+string(cfg.Provider), false)
	}
}

func newAliyunStore(cfg Config) (objectStore, error) {
	bucket, err := newAliyunBucketFunc(cfg)
	if err != nil {
		return nil, mapStoreError("ossx.New", err)
	}
	return &aliyunStore{bucket: bucket}, nil
}

func newSDKAliyunBucket(cfg Config) (aliyunBucket, error) {
	client, err := oss.New(cfg.Endpoint, cfg.AccessKeyID, cfg.SecretAccessKey)
	if err != nil {
		return nil, err
	}
	bucket, err := client.Bucket(cfg.Bucket)
	if err != nil {
		return nil, err
	}
	return sdkAliyunBucket{bucket: bucket}, nil
}

func (b sdkAliyunBucket) PutObject(objectKey string, reader io.Reader, options ...oss.Option) error {
	return b.bucket.PutObject(objectKey, reader, options...)
}

func (b sdkAliyunBucket) GetObject(objectKey string, options ...oss.Option) (io.ReadCloser, error) {
	return b.bucket.GetObject(objectKey, options...)
}

func (b sdkAliyunBucket) GetObjectDetailedMeta(objectKey string, options ...oss.Option) (http.Header, error) {
	return b.bucket.GetObjectDetailedMeta(objectKey, options...)
}

func (b sdkAliyunBucket) DeleteObject(objectKey string, options ...oss.Option) error {
	return b.bucket.DeleteObject(objectKey, options...)
}

func (b sdkAliyunBucket) ListObjects(options ...oss.Option) (oss.ListObjectsResult, error) {
	return b.bucket.ListObjects(options...)
}

func (b sdkAliyunBucket) IsBucketExist() (bool, error) {
	return b.bucket.Client.IsBucketExist(b.bucket.BucketName)
}

func (s *aliyunStore) putObject(key string, body io.Reader, contentType string) error {
	options := make([]oss.Option, 0, 1)
	if contentType != "" {
		options = append(options, oss.ContentType(contentType))
	}
	return s.bucket.PutObject(key, body, options...)
}

func (s *aliyunStore) getObject(key string) (storedObject, error) {
	body, err := s.bucket.GetObject(key)
	if err != nil {
		return storedObject{}, err
	}
	headers, err := s.bucket.GetObjectDetailedMeta(key)
	if err != nil {
		_ = body.Close()
		return storedObject{}, err
	}
	return storedObject{
		Info: objectInfoFromHeaders(key, headers),
		Body: body,
	}, nil
}

func (s *aliyunStore) deleteObject(key string) error {
	return s.bucket.DeleteObject(key)
}

func (s *aliyunStore) listObjects(input listRequest) (storedList, error) {
	options := []oss.Option{oss.Prefix(input.Prefix)}
	if input.Marker != "" {
		options = append(options, oss.Marker(input.Marker))
	}
	if input.MaxKeys > 0 {
		options = append(options, oss.MaxKeys(input.MaxKeys))
	}
	result, err := s.bucket.ListObjects(options...)
	if err != nil {
		return storedList{}, err
	}
	objects := make([]ObjectInfo, 0, len(result.Objects))
	for _, object := range result.Objects {
		objects = append(objects, objectInfoFromOSSObject(object))
	}
	return storedList{
		Objects:     objects,
		IsTruncated: result.IsTruncated,
		NextMarker:  result.NextMarker,
	}, nil
}

func (s *aliyunStore) check() error {
	exists, err := s.bucket.IsBucketExist()
	if err != nil {
		return err
	}
	if !exists {
		return NewError(ErrorKindBucketNotFound, "ossx.HealthCheck", "bucket not found", false)
	}
	return nil
}

func objectInfoFromHeaders(key string, headers http.Header) ObjectInfo {
	size, _ := strconv.ParseInt(headers.Get("Content-Length"), 10, 64)
	return ObjectInfo{
		Key:          key,
		Size:         size,
		ETag:         trimETag(headers.Get("ETag")),
		ContentType:  headers.Get("Content-Type"),
		LastModified: headers.Get("Last-Modified"),
	}
}

func objectInfoFromOSSObject(object oss.ObjectProperties) ObjectInfo {
	return ObjectInfo{
		Key:          object.Key,
		Size:         object.Size,
		ETag:         trimETag(object.ETag),
		LastModified: object.LastModified.UTC().Format(time.RFC3339),
	}
}

func trimETag(etag string) string {
	return strings.Trim(etag, `"`)
}

func mapStoreError(op string, err error) error {
	if err == nil {
		return nil
	}
	var existing *Error
	if errors.As(err, &existing) {
		return existing
	}
	if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return contextError(op, err)
	}
	var netErr net.Error
	if errors.As(err, &netErr) && netErr.Timeout() {
		return WrapError(ErrorKindTimeout, op, "object storage operation timed out", true, err)
	}
	switch serviceErr := err.(type) {
	case oss.ServiceError:
		return mapServiceError(op, serviceErr, err)
	case *oss.ServiceError:
		if serviceErr != nil {
			return mapServiceError(op, *serviceErr, err)
		}
	}
	return WrapError(ErrorKindTransfer, op, "object storage operation failed", true, err)
}

func mapServiceError(op string, err oss.ServiceError, cause error) error {
	kind := ErrorKindTransfer
	retryable := false
	switch {
	case err.Code == "NoSuchBucket":
		kind = ErrorKindBucketNotFound
	case err.Code == "NoSuchKey" || err.Code == "NoSuchObject":
		kind = ErrorKindNotFound
	case err.StatusCode == http.StatusForbidden:
		kind = ErrorKindAuth
	case err.StatusCode == http.StatusNotFound:
		kind = ErrorKindNotFound
	case err.StatusCode == http.StatusConflict:
		kind = ErrorKindConflict
	case err.StatusCode == http.StatusRequestEntityTooLarge:
		kind = ErrorKindObjectTooLarge
	case err.StatusCode == http.StatusTooManyRequests:
		kind = ErrorKindRateLimit
		retryable = true
	case err.StatusCode == http.StatusRequestTimeout || err.StatusCode == http.StatusGatewayTimeout:
		kind = ErrorKindTimeout
		retryable = true
	case err.StatusCode >= http.StatusInternalServerError:
		kind = ErrorKindUnavailable
		retryable = true
	case err.StatusCode == http.StatusBadRequest:
		kind = ErrorKindValidation
	}
	return WrapError(kind, op, serviceErrorMessage(err), retryable, cause)
}

func serviceErrorMessage(err oss.ServiceError) string {
	if err.Message != "" {
		return err.Message
	}
	if err.Code != "" {
		return err.Code
	}
	return "object storage service error"
}
