package dftelemetry

import "encoding/json"

// Event is the spec §0/§42 pre-enrichment subset the fork emits over the UDS.
// Fork emits a SUBSET of spec §42 — never source_ip_hmac/geo_*/asn*/is_*/
// confidence_* (those are sidecar fields, added downstream). The shape MUST
// stay in sync with vat-evidence-pipeline/schemas/telemetry_event.schema.json.
type Event struct {
	TSUTC                   string  `json:"ts_utc"`
	ProviderID              string  `json:"provider_id"`
	ProviderWallet          string  `json:"provider_wallet"`
	OwnerWalletClaimed      *string `json:"owner_wallet_claimed"`
	OwnerWalletVerified     *string `json:"owner_wallet_verified"`
	AuthState               string  `json:"auth_state"`
	DSEQ                    *string `json:"dseq"`
	GSEQ                    *uint32 `json:"gseq"`
	OSEQ                    *uint32 `json:"oseq"`
	EventType               string  `json:"event_type"`
	SourceIP                string  `json:"source_ip"`
	SourceIPSource          string  `json:"source_ip_source"`
	TrustedProxyID          *string `json:"trusted_proxy_id"`
	Route                   string  `json:"route"`
	HTTPMethod              string  `json:"http_method"`
	StatusCode              *int    `json:"status_code"`
	ProviderUpstreamVersion string  `json:"provider_upstream_version"`
	ProviderUpstreamCommit  string  `json:"provider_upstream_commit"`
	DFTelemetryCommit       string  `json:"df_telemetry_commit"`
	CaptureSchemaVersion    string  `json:"capture_schema_version"`
}

// ToNDJSON marshals the event as a single line-delimited JSON object: one JSON
// object followed by a '\n'. The Phase v1.0-03 sidecar reads the UDS stream
// line-by-line, so the trailing newline is the record delimiter.
func (e Event) ToNDJSON() ([]byte, error) {
	b, err := json.Marshal(e)
	if err != nil {
		return nil, err
	}
	return append(b, '\n'), nil
}
