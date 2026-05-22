// Package dftelemetry is the Digital Frontier VAT-evidence telemetry hook.
// It extracts minimal verified semantic facts from provider control-plane
// requests and emits them over a Unix domain socket. It does NO enrichment
// (no GeoIP/ASN/HMAC/confidence) — see CLAUDE.md guardrail #2.
package dftelemetry

// Injected at build time via -ldflags -X (mirrors the provider's version pkg).
// Empty values on an emitted event mean ldflags injection failed — OBS-04.
var (
	Commit               = "dev"
	UpstreamCommit       = "unknown"
	UpstreamVersion      = "unknown"
	CaptureSchemaVersion = "v1.3"
)
