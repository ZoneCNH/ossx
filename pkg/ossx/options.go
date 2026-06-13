package ossx

// Option configures the Client.
type Option func(*options)

type options struct {
	metrics Metrics
}

func defaultOptions() options {
	return options{
		metrics: NoopMetrics{},
	}
}

// WithMetrics sets the Metrics implementation for the client.
func WithMetrics(metrics Metrics) Option {
	return func(o *options) {
		if metrics != nil {
			o.metrics = metrics
		}
	}
}
