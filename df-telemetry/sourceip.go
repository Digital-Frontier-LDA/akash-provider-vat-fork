package dftelemetry

import "net/http"

// SourceIPPolicy is the DEC-06 source_ip_policy the resolver honours.
type SourceIPPolicy struct{}

// ResolveSourceIP resolves the client source IP per the DEC-06 policy.
//
// Skeleton stub. Never blind-trust X-Forwarded-For — CLAUDE.md guardrail #9.
// TODO(Plan v1.0-02-02): DEC-06 source_ip_policy loader + resolver.
func ResolveSourceIP(r *http.Request, p SourceIPPolicy) (ip string, source string, trustedProxyID *string) {
	return "", "unknown", nil
}
