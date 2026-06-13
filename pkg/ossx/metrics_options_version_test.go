package ossx

import "testing"

func TestNoopMetricsAndOptions(t *testing.T) {
	noop := NoopMetrics{}
	noop.IncCounter("counter", map[string]string{"k": "v"})
	noop.ObserveHistogram("histogram", 1, nil)
	noop.SetGauge("gauge", 1, nil)

	opts := defaultOptions()
	if opts.metrics == nil {
		t.Fatal("default metrics should be configured")
	}
	WithMetrics(nil)(&opts)
	if opts.metrics == nil {
		t.Fatal("nil metrics option should keep default")
	}
	metrics := &testMetrics{}
	WithMetrics(metrics)(&opts)
	opts.metrics.IncCounter("custom", nil)
	if metrics.countCounter("custom") != 1 {
		t.Fatal("custom metrics not installed")
	}
}

func TestModuleIdentity(t *testing.T) {
	if ModuleName != "github.com/ZoneCNH/ossx" {
		t.Fatalf("unexpected module name: %s", ModuleName)
	}
	if Version != "v1.0.1" {
		t.Fatalf("unexpected version: %s", Version)
	}
}
