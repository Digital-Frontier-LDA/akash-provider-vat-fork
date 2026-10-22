//go:build smoke

package dftelemetry

import (
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"github.com/gorilla/mux"
	"github.com/gorilla/websocket"
)

// TestSmoke_EndToEndControlPlaneRequests is the df-telemetry integration smoke
// test (CI-04 / ROADMAP v1.0-02 criterion 2).
//
// It replaces the kind-cluster harness. A kind boot needs the full forked-
// provider binary, and that build fails on a pre-existing upstream wasmd / Go
// toolchain break that has nothing to do with the fork. This in-process
// harness exercises the SAME wiring the real provider uses — the telemetry
// MiddlewareFunc registered on a gorilla/mux router via router.Use(...), the
// exact call gateway/rest/router.go's newRouter() makes — over a real
// httptest.Server and a real Unix-domain socket. It proves the pre_auth +
// verified two-event flow end to end, with every spec §0 field populated,
// without the heavyweight provider boot. (RESEARCH: "stripped REST-gateway
// harness".)
//
// Build-tagged `smoke` so it is excluded from the default unit run
// (the df-telemetry-tests CI job) and invoked explicitly by the ci.yaml
// kind-smoke job: `go test -tags smoke -run TestSmoke ./df-telemetry/...`.
func TestSmoke_EndToEndControlPlaneRequests(t *testing.T) {
	// MP-02: provider identity is parameterised, never hardcoded. CI sets the
	// DF_PROVIDER_* vars; the defaults keep a bare `go test` runnable.
	providerID := smokeEnvOr("DF_PROVIDER_ID", "df-smoke-provider")
	providerWallet := smokeEnvOr("DF_PROVIDER_WALLET", "akash1smokeproviderwallet")
	t.Setenv("DF_PROVIDER_ID", providerID)
	t.Setenv("DF_PROVIDER_WALLET", providerWallet)
	resetMiddlewareConfig() // pick up the env vars set above

	// A real UDS-backed Emitter + reader — the full path: Emit -> buffered
	// channel -> single writer goroutine -> Unix socket -> reader.
	cap := newCaptureEmitter(t)

	// The SAME registration gateway/rest/router.go performs: the telemetry
	// hook attached as gorilla/mux middleware via router.Use(...).
	router := mux.NewRouter()
	router.Use(Middleware(cap.em, nil))
	router.HandleFunc("/deployment/{dseq}/manifest", okHF).Methods(http.MethodPut)
	// shell/logs are WebSocket endpoints in production (leaseShellHandler /
	// leaseLogsHandler hijack the connection to upgrade). The smoke harness
	// used to register them as plain-HTTP stubs, which is exactly how the
	// 2026-07 fleet-wide shell/logs 500 (statusWriter hiding http.Hijacker)
	// sailed through CI — so these two routes now do a REAL upgrade.
	router.HandleFunc("/lease/{dseq}/{gseq}/{oseq}/shell", wsEchoHF)
	router.HandleFunc("/lease/{dseq}/{gseq}/{oseq}/logs", wsEchoHF).Methods(http.MethodGet)
	router.HandleFunc("/lease/{dseq}/{gseq}/{oseq}/status", okHF).Methods(http.MethodGet)

	// A real HTTP server — synthetic control-plane requests travel over a real
	// socket through the real mux stack, not an httptest.ResponseRecorder.
	srv := httptest.NewServer(router)
	defer srv.Close()

	requests := []struct {
		method, path, wantType string
	}{
		{http.MethodPut, "/deployment/100/manifest", eventManifestSubmit},
		{http.MethodGet, "/lease/100/1/1/status", eventLeaseStatusCheck},
	}
	for _, rq := range requests {
		req, err := http.NewRequest(rq.method, srv.URL+rq.path, nil)
		if err != nil {
			t.Fatalf("build request %s %s: %v", rq.method, rq.path, err)
		}
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatalf("request %s %s: %v", rq.method, rq.path, err)
		}
		_, _ = io.Copy(io.Discard, resp.Body)
		_ = resp.Body.Close()
	}

	// shell + logs: REAL WebSocket upgrades through the middleware, like
	// production. The upgrade must reach 101 (Hijacker resolvable through the
	// statusWriter wrapper) and the echo frame must come back.
	wsDials := []struct {
		path, wantType string
	}{
		{"/lease/100/1/1/shell", eventLeaseShellConnection},
		{"/lease/100/1/1/logs", eventLeaseLogsConnection},
	}
	for _, d := range wsDials {
		wsURL := "ws" + strings.TrimPrefix(srv.URL, "http") + d.path
		ws, resp, err := websocket.DefaultDialer.Dial(wsURL, nil)
		if err != nil {
			status := 0
			if resp != nil {
				status = resp.StatusCode
				_ = resp.Body.Close()
			}
			t.Fatalf("websocket upgrade %s failed (HTTP %d): %v", d.path, status, err)
		}
		_, msg, err := ws.ReadMessage()
		if err != nil {
			t.Fatalf("read echo frame on %s: %v", d.path, err)
		}
		if string(msg) != "ok" {
			t.Fatalf("echo frame on %s = %q, want %q", d.path, msg, "ok")
		}
		_ = ws.Close()
	}

	// Each request emits exactly two events: pre_auth then verified-flow.
	events := cap.readEvents(t, (len(requests)+len(wsDials))*2)

	for i, rq := range requests {
		pre := events[i*2]
		verified := events[i*2+1]

		if pre.AuthState != authPreAuth {
			t.Errorf("request %d (%s %s): event %d auth_state = %q, want pre_auth",
				i, rq.method, rq.path, i*2, pre.AuthState)
		}
		if pre.OwnerWalletVerified != nil {
			t.Errorf("request %d: pre_auth event has owner_wallet_verified set, want nil", i)
		}
		if verified.AuthState == authPreAuth {
			t.Errorf("request %d: event %d auth_state = pre_auth, want a resolved state", i, i*2+1)
		}
		if verified.StatusCode == nil {
			t.Errorf("request %d: verified event status_code is nil, want it populated", i)
		}
		for _, ev := range []Event{pre, verified} {
			if ev.EventType != rq.wantType {
				t.Errorf("request %d (%s %s): event_type = %q, want %q",
					i, rq.method, rq.path, ev.EventType, rq.wantType)
			}
		}
		assertSpec0FieldsPopulated(t, i*2, pre, providerID)
		assertSpec0FieldsPopulated(t, i*2+1, verified, providerID)
	}

	for j, d := range wsDials {
		base := (len(requests) + j) * 2
		pre, verified := events[base], events[base+1]
		if pre.AuthState != authPreAuth {
			t.Errorf("ws dial %s: event %d auth_state = %q, want pre_auth", d.path, base, pre.AuthState)
		}
		if verified.AuthState == authPreAuth {
			t.Errorf("ws dial %s: event %d auth_state = pre_auth, want a resolved state", d.path, base+1)
		}
		for _, ev := range []Event{pre, verified} {
			if ev.EventType != d.wantType {
				t.Errorf("ws dial %s: event_type = %q, want %q", d.path, ev.EventType, d.wantType)
			}
		}
		assertSpec0FieldsPopulated(t, base, pre, providerID)
		assertSpec0FieldsPopulated(t, base+1, verified, providerID)
	}

	// At least one pre_auth and one resolved line — the two-event-flow proof.
	var preCount, resolvedCount int
	for _, ev := range events {
		if ev.AuthState == authPreAuth {
			preCount++
		} else {
			resolvedCount++
		}
	}
	if preCount == 0 || resolvedCount == 0 {
		t.Fatalf("two-event-flow proof failed: pre_auth=%d resolved=%d", preCount, resolvedCount)
	}

	t.Logf("smoke OK: %d plain + %d websocket control-plane requests -> %d NDJSON events (pre_auth=%d resolved=%d), provider_id=%q",
		len(requests), len(wsDials), len(events), preCount, resolvedCount, providerID)
}

// assertSpec0FieldsPopulated checks every spec §0/§42-subset field that must be
// non-empty on an emitted event, plus the MP-02 provider_id round-trip and the
// fixed capture_schema_version. Mirrors the old assert-ndjson.sh checks.
func assertSpec0FieldsPopulated(t *testing.T, idx int, ev Event, wantProviderID string) {
	t.Helper()
	nonEmpty := map[string]string{
		"provider_id":               ev.ProviderID,
		"provider_wallet":           ev.ProviderWallet,
		"ts_utc":                    ev.TSUTC,
		"route":                     ev.Route,
		"http_method":               ev.HTTPMethod,
		"event_type":                ev.EventType,
		"source_ip_source":          ev.SourceIPSource,
		"auth_state":                ev.AuthState,
		"capture_schema_version":    ev.CaptureSchemaVersion,
		"provider_upstream_version": ev.ProviderUpstreamVersion,
		"provider_upstream_commit":  ev.ProviderUpstreamCommit,
		"df_telemetry_commit":       ev.DFTelemetryCommit,
	}
	for field, val := range nonEmpty {
		if val == "" {
			t.Errorf("event %d: %s is empty", idx, field)
		}
	}
	if ev.CaptureSchemaVersion != "v1.3" {
		t.Errorf("event %d: capture_schema_version = %q, want v1.3", idx, ev.CaptureSchemaVersion)
	}
	if ev.ProviderID != wantProviderID {
		t.Errorf("event %d: provider_id = %q, want %q (MP-02 round-trip)", idx, ev.ProviderID, wantProviderID)
	}
}

func smokeEnvOr(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}

// wsEchoHF upgrades to a WebSocket and writes one "ok" frame — the minimal
// stand-in for leaseShellHandler/leaseLogsHandler, which hijack the connection
// the same way via gorilla/websocket.
func wsEchoHF(w http.ResponseWriter, r *http.Request) {
	upgrader := websocket.Upgrader{}
	ws, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		// Upgrade already wrote its own error response.
		return
	}
	defer func() { _ = ws.Close() }()
	_ = ws.WriteMessage(websocket.TextMessage, []byte("ok"))
}
