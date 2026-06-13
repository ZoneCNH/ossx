package ossx

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"
)

func TestHealthCheckStates(t *testing.T) {
	tests := []struct {
		name          string
		client        *Client
		ctx           context.Context
		wantStatus    HealthStatusValue
		wantMessage   string
		wantGauge     float64
		wantErrorHits int
	}{
		{
			name:        "nil context",
			client:      &Client{cfg: Config{Name: "nil-context"}, metrics: &testMetrics{}},
			wantStatus:  HealthUnhealthy,
			wantMessage: "context is required",
			wantGauge:   0,
		},
		{
			name:        "canceled context",
			client:      &Client{cfg: Config{Name: "canceled"}, metrics: &testMetrics{}},
			ctx:         canceledContext(t),
			wantStatus:  HealthUnhealthy,
			wantMessage: context.Canceled.Error(),
			wantGauge:   0,
		},
		{
			name:        "uninitialized nil client",
			client:      nil,
			ctx:         t.Context(),
			wantStatus:  HealthUnhealthy,
			wantMessage: "client is not initialized",
			wantGauge:   0,
		},
		{
			name:        "uninitialized",
			client:      &Client{cfg: Config{Name: "not-ready"}, metrics: &testMetrics{}},
			ctx:         t.Context(),
			wantStatus:  HealthUnhealthy,
			wantMessage: "client is not initialized",
			wantGauge:   0,
		},
		{
			name:        "closed",
			client:      &Client{cfg: Config{Name: "closed"}, metrics: &testMetrics{}, initialized: true, closed: true},
			ctx:         t.Context(),
			wantStatus:  HealthUnhealthy,
			wantMessage: "client is closed",
			wantGauge:   0,
		},
		{
			name:          "store check failed",
			client:        &Client{cfg: Config{Name: "provider"}, metrics: &testMetrics{}, initialized: true, store: fakeStore{checkFn: func() error { return errors.New("check failed") }}},
			ctx:           t.Context(),
			wantStatus:    HealthUnhealthy,
			wantMessage:   "object storage operation failed",
			wantGauge:     0,
			wantErrorHits: 1,
		},
		{
			name:        "healthy without provider check",
			client:      &Client{cfg: Config{Name: ""}, metrics: &testMetrics{}, initialized: true},
			ctx:         t.Context(),
			wantStatus:  HealthHealthy,
			wantMessage: "ok",
			wantGauge:   1,
		},
		{
			name:        "healthy with provider check",
			client:      &Client{cfg: Config{Name: "provider"}, metrics: &testMetrics{}, initialized: true, store: fakeStore{}},
			ctx:         t.Context(),
			wantStatus:  HealthHealthy,
			wantMessage: "ok",
			wantGauge:   1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			status := tt.client.HealthCheck(tt.ctx)
			if status.Status != tt.wantStatus {
				t.Fatalf("expected status %s, got %#v", tt.wantStatus, status)
			}
			if !strings.Contains(status.Message, tt.wantMessage) {
				t.Fatalf("expected message containing %q, got %#v", tt.wantMessage, status)
			}
			if status.Name == "" || status.CheckedAt.IsZero() || status.LatencyMs < 0 {
				t.Fatalf("health status missing common fields: %#v", status)
			}
			var metrics *testMetrics
			if tt.client != nil {
				metrics, _ = tt.client.metrics.(*testMetrics)
			}
			if metrics != nil {
				if len(metrics.gauges) == 0 {
					t.Fatal("health gauge not recorded")
				}
				lastGauge := metrics.gauges[len(metrics.gauges)-1]
				if lastGauge.name != MetricClientHealthStatus || lastGauge.value != tt.wantGauge {
					t.Fatalf("unexpected health gauge: %#v", lastGauge)
				}
				if metrics.countCounter(MetricClientErrorsTotal) != tt.wantErrorHits {
					t.Fatalf("unexpected error metric count: %#v", metrics.counters)
				}
			}
		})
	}
}

func TestHealthCheckDeadlineStates(t *testing.T) {
	metrics := &testMetrics{}
	client := &Client{
		cfg:         Config{Name: "deadline", Timeout: time.Hour},
		metrics:     metrics,
		initialized: true,
		store:       fakeStore{},
	}

	expired, cancelExpired := context.WithDeadline(t.Context(), time.Now().Add(-time.Millisecond))
	defer cancelExpired()
	status := client.HealthCheck(expired)
	if status.Status != HealthUnhealthy || status.Message != context.DeadlineExceeded.Error() {
		t.Fatalf("unexpected expired status: %#v", status)
	}

	status = client.HealthCheck(staleDeadlineContext{Context: t.Context()})
	if status.Status != HealthUnhealthy || status.Message != context.DeadlineExceeded.Error() {
		t.Fatalf("unexpected stale deadline status: %#v", status)
	}

	lateErr := &lateErrDeadlineContext{Context: t.Context()}
	status = client.HealthCheck(lateErr)
	if status.Status != HealthUnhealthy || status.Message != context.Canceled.Error() {
		t.Fatalf("unexpected late err deadline status: %#v", status)
	}

	short, cancelShort := context.WithDeadline(t.Context(), time.Now().Add(time.Minute))
	defer cancelShort()
	status = client.HealthCheck(short)
	if status.Status != HealthDegraded {
		t.Fatalf("expected degraded status, got %#v", status)
	}
	if status.Metadata["reason"] != "deadline_below_timeout" || status.Metadata["timeout"] != time.Hour.String() {
		t.Fatalf("unexpected degraded metadata: %#v", status.Metadata)
	}

	if len(metrics.gauges) != 4 || metrics.gauges[0].value != 0 || metrics.gauges[1].value != 0 || metrics.gauges[2].value != 0 || metrics.gauges[3].value != 0 {
		t.Fatalf("unexpected deadline health gauges: %#v", metrics.gauges)
	}
}

type staleDeadlineContext struct {
	context.Context
}

func (staleDeadlineContext) Deadline() (time.Time, bool) {
	return time.Now().Add(-time.Millisecond), true
}

func (staleDeadlineContext) Err() error {
	return nil
}

type lateErrDeadlineContext struct {
	context.Context
	errChecks int
}

func (*lateErrDeadlineContext) Deadline() (time.Time, bool) {
	return time.Now().Add(-time.Millisecond), true
}

func (c *lateErrDeadlineContext) Err() error {
	c.errChecks++
	if c.errChecks == 1 {
		return nil
	}
	return context.Canceled
}

func TestHealthMetricHelpers(t *testing.T) {
	recordHealthMetric(nil, HealthStatus{})

	metrics := &testMetrics{}
	recordHealthMetric(metrics, HealthStatus{Name: "helper", Status: HealthHealthy})
	recordHealthMetric(metrics, HealthStatus{Name: "helper", Status: HealthDegraded})
	recordHealthMetric(metrics, HealthStatus{Name: "helper", Status: HealthUnhealthy})

	if healthGaugeValue(HealthHealthy) != 1 || healthGaugeValue(HealthDegraded) != 0 || healthGaugeValue(HealthUnhealthy) != 0 {
		t.Fatal("unexpected health gauge values")
	}
	if len(metrics.gauges) != 3 || len(metrics.histograms) != 3 {
		t.Fatalf("health metrics not recorded: gauges=%#v histograms=%#v", metrics.gauges, metrics.histograms)
	}
}
