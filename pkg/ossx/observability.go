package ossx

import (
	"context"
	"time"
)

// Hooks captures observability hook points (FR-009). All fields are optional;
// nil fields are no-ops. The interfaces are signature-compatible with
// observex (IncCounter/AddCounter/ObserveHistogram/SetGauge with map labels)
// so a caller may pass an observex implementation directly. ossx keeps these
// local to stay provider-SDK-only in go.mod (mirrors sibling adapters).
//
// BR-009: secrets / signatures / credentials / full signed URLs / raw
// metadata values MUST never reach a hook. The blobStore sanitizes keys
// (via Key.SanitizedScope) before emitting.
type Hooks struct {
	Metrics Metrics
	Tracer  Tracer
	Logger  Logger
}

// Metrics records operation metrics. Signature mirrors observex.Metrics.
type Metrics interface {
	IncCounter(name string, labels map[string]string)
	AddCounter(name string, delta float64, labels map[string]string)
	ObserveHistogram(name string, value float64, labels map[string]string)
	SetGauge(name string, value float64, labels map[string]string)
}

// Tracer records span lifecycle. Signature mirrors observex.Tracer.
type Tracer interface {
	Start(ctx context.Context, name string, fields ...Field) (context.Context, Span)
}

// Span is a trace span handle.
type Span interface {
	SetField(field Field)
	AddEvent(name string, fields ...Field)
	End(fields ...Field)
}

// Logger is a structured logger. Signature mirrors observex.Logger.
type Logger interface {
	Debug(ctx context.Context, msg string, fields ...Field)
	Info(ctx context.Context, msg string, fields ...Field)
	Warn(ctx context.Context, msg string, fields ...Field)
	Error(ctx context.Context, msg string, fields ...Field)
}

// Field is a structured key/value with optional secret flag (BR-009 redaction).
type Field struct {
	Key    string
	Value  any
	Secret bool
}

// noopSpan is a no-op span returned by NoopTracer.
type noopSpan struct{}

func (noopSpan) SetField(Field)            {}
func (noopSpan) AddEvent(string, ...Field) {}
func (noopSpan) End(...Field)              {}

// NoopMetrics is a Metrics implementation that does nothing.
type NoopMetrics struct{}

func (NoopMetrics) IncCounter(string, map[string]string)                {}
func (NoopMetrics) AddCounter(string, float64, map[string]string)       {}
func (NoopMetrics) ObserveHistogram(string, float64, map[string]string) {}
func (NoopMetrics) SetGauge(string, float64, map[string]string)         {}

// NoopTracer is a Tracer implementation that does nothing.
type NoopTracer struct{}

func (NoopTracer) Start(ctx context.Context, _ string, _ ...Field) (context.Context, Span) {
	return ctx, noopSpan{}
}

// NoopLogger is a Logger implementation that does nothing.
type NoopLogger struct{}

func (NoopLogger) Debug(context.Context, string, ...Field) {}
func (NoopLogger) Info(context.Context, string, ...Field)  {}
func (NoopLogger) Warn(context.Context, string, ...Field)  {}
func (NoopLogger) Error(context.Context, string, ...Field) {}

// AuditEvent records an auditable operation (FR-006 / FR-009). Secrets and
// full signed URLs are NEVER included; only sanitized scopes.
type AuditEvent struct {
	Operation     string
	Result        string // "ok" | error kind string
	KeyScope      string // sanitized (Key.SanitizedScope)
	ObjectSize    int64
	Latency       time.Duration
	TTLSeconds    int64            // presign only
	Method        string           // presign method (GET/PUT)
	ActorFields   map[string]string // caller-supplied, already sanitized
	CorrelationID string
	OccurredAt    time.Time
}

// metric name constants (lower-snake-case, validated shape).
const (
	metricRequestsTotal   = "ossx_requests_total"
	metricRequestDuration = "ossx_request_duration_seconds"
	metricObjectSize      = "ossx_object_size_bytes"
	metricRetriesTotal    = "ossx_retries_total"
	metricHealthStatus    = "ossx_health_status"
	metricInflight        = "ossx_inflight"
)

// emit fires metrics for a completed operation. keyScope MUST already be
// sanitized. Secrets must never be passed.
func (h *Hooks) emit(operation, result, keyScope string, size int64, latency time.Duration) {
	if h == nil {
		return
	}
	labels := map[string]string{
		"operation": operation,
		"result":    result,
		"key_scope": keyScope,
	}
	if h.Metrics != nil {
		h.Metrics.IncCounter(metricRequestsTotal, labels)
		h.Metrics.ObserveHistogram(metricRequestDuration, latency.Seconds(), labels)
		if size > 0 {
			h.Metrics.ObserveHistogram(metricObjectSize, float64(size), labels)
		}
	}
}

// emitAudit logs an AuditEvent through the Logger (if any). The caller is
// responsible for sanitizing ActorFields and never including secrets.
func (h *Hooks) emitAudit(ctx context.Context, ev AuditEvent) {
	if h == nil || h.Logger == nil {
		return
	}
	fields := []Field{
		{Key: "operation", Value: ev.Operation},
		{Key: "result", Value: ev.Result},
		{Key: "key_scope", Value: ev.KeyScope},
		{Key: "latency_ms", Value: ev.Latency.Milliseconds()},
	}
	if ev.TTLSeconds > 0 {
		fields = append(fields, Field{Key: "ttl_seconds", Value: ev.TTLSeconds})
	}
	if ev.Method != "" {
		fields = append(fields, Field{Key: "method", Value: ev.Method})
	}
	if ev.CorrelationID != "" {
		fields = append(fields, Field{Key: "correlation_id", Value: ev.CorrelationID})
	}
	for k, v := range ev.ActorFields {
		fields = append(fields, Field{Key: k, Value: v})
	}
	h.Logger.Info(ctx, "ossx audit", fields...)
}

// withDefaults fills nil hook fields with no-ops so emit/emitAudit are nil-safe.
func (h Hooks) withDefaults() Hooks {
	out := h
	if out.Metrics == nil {
		out.Metrics = NoopMetrics{}
	}
	if out.Tracer == nil {
		out.Tracer = NoopTracer{}
	}
	if out.Logger == nil {
		out.Logger = NoopLogger{}
	}
	return out
}
