package ossx

const (
	// Standard metric constants from xlib-standard.
	MetricClientCreatedTotal           = "client_created_total"
	MetricClientClosedTotal            = "client_closed_total"
	MetricClientErrorsTotal            = "client_errors_total"
	MetricClientHealthStatus           = "client_health_status"
	MetricClientHealthLatencyMS        = "client_health_latency_ms"
	MetricClientRequestsTotal          = "client_requests_total"
	MetricClientRequestDurationSeconds = "client_request_duration_seconds"
	MetricClientRetriesTotal           = "client_retries_total"
	MetricClientInflight               = "client_inflight"

	// OSS-specific metric constants.
	MetricOSSUploadsTotal     = "ossx_uploads_total"
	MetricOSSDownloadsTotal   = "ossx_downloads_total"
	MetricOSSBytesTransferred = "ossx_bytes_transferred"
)

// Metrics defines the interface for recording observability data.
type Metrics interface {
	IncCounter(name string, labels map[string]string)
	ObserveHistogram(name string, value float64, labels map[string]string)
	SetGauge(name string, value float64, labels map[string]string)
}

// NoopMetrics is a Metrics implementation that does nothing.
type NoopMetrics struct{}

func (NoopMetrics) IncCounter(name string, labels map[string]string) {
	_ = name
	_ = labels
}

func (NoopMetrics) ObserveHistogram(name string, value float64, labels map[string]string) {
	_ = name
	_ = value
	_ = labels
}

func (NoopMetrics) SetGauge(name string, value float64, labels map[string]string) {
	_ = name
	_ = value
	_ = labels
}
