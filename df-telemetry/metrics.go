package dftelemetry

import "github.com/prometheus/client_golang/prometheus"

// dropsTotal is the SOLE Prometheus metric the fork exposes (OBS-02).
//
// It is a bare counter with NO labels: per-route or per-provider labels would
// add cardinality the tiny fork must not carry. A "drop" is any telemetry event
// that did not reach the sidecar — channel full, marshal error, or UDS write
// failure. A drop NEVER blocks the provider request path (fail-open, CLAUDE.md
// guardrail #3); this counter is the only observable trace it leaves.
var dropsTotal = prometheus.NewCounter(prometheus.CounterOpts{
	Name: "df_telemetry_drops_total",
	Help: "Total telemetry events dropped (channel full, marshal error, or UDS write failure). Fail-open: a drop never blocks the provider.",
})

func init() {
	// Register on the default registry. MustRegister panics only on a
	// duplicate registration, which cannot happen for a single package-level
	// counter registered exactly once.
	prometheus.MustRegister(dropsTotal)
}

// incDrops records one dropped telemetry event.
func incDrops() { dropsTotal.Inc() }
