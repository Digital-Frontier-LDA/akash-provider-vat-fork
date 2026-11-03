package dftelemetry

import (
	"bufio"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/gorilla/mux"

	ajwt "pkg.akt.dev/go/util/jwt"
)

// captureEmitter stands up a real UDS-backed Emitter and reads the NDJSON it
// writes, so a test can assert on the actual emitted Events.
type captureEmitter struct {
	em   *Emitter
	ln   net.Listener
	conn net.Conn
	sc   *bufio.Scanner
}

func newCaptureEmitter(t *testing.T) *captureEmitter {
	t.Helper()
	// A short socket path: a Unix-domain socket path has a ~108-byte limit and
	// t.TempDir() inside a sub-test embeds the (long) sub-test name. Use the
	// OS temp dir with a short unique name instead.
	sock := filepath.Join(os.TempDir(), fmt.Sprintf("dfmw-%d.sock", time.Now().UnixNano()))
	_ = os.Remove(sock)
	ln, err := net.Listen("unix", sock)
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	c := &captureEmitter{em: New(sock, 64), ln: ln}
	t.Cleanup(func() {
		if c.conn != nil {
			_ = c.conn.Close()
		}
		_ = ln.Close()
		_ = os.Remove(sock)
	})
	return c
}

// readEvents blocks until n events have been read off the socket.
func (c *captureEmitter) readEvents(t *testing.T, n int) []Event {
	t.Helper()
	if c.conn == nil {
		conn, err := c.ln.Accept()
		if err != nil {
			t.Fatalf("accept: %v", err)
		}
		c.conn = conn
		c.sc = bufio.NewScanner(conn)
	}
	_ = c.conn.SetReadDeadline(time.Now().Add(3 * time.Second))
	out := make([]Event, 0, n)
	for len(out) < n && c.sc.Scan() {
		var ev Event
		if err := json.Unmarshal(c.sc.Bytes(), &ev); err != nil {
			t.Fatalf("unmarshal emitted event: %v", err)
		}
		out = append(out, ev)
	}
	if len(out) != n {
		t.Fatalf("read %d events, want %d", len(out), n)
	}
	return out
}

// noopHandler is a trivial handler the middleware wraps.
func noopHandler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok"))
	})
}

// TestMiddleware_EmitsPreAuthThenVerified: exactly two events, first pre_auth
// with no verified wallet, second with a resolved auth_state.
func TestMiddleware_EmitsPreAuthThenVerified(t *testing.T) {
	cap := newCaptureEmitter(t)
	h := Middleware(cap.em, nil)(noopHandler())

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/status", nil)
	h.ServeHTTP(rec, req)

	events := cap.readEvents(t, 2)
	if events[0].AuthState != authPreAuth {
		t.Errorf("event 0 auth_state = %q, want pre_auth", events[0].AuthState)
	}
	if events[0].OwnerWalletVerified != nil {
		t.Errorf("event 0 owner_wallet_verified = %v, want nil on pre_auth", *events[0].OwnerWalletVerified)
	}
	if events[1].AuthState == authPreAuth {
		t.Errorf("event 1 auth_state = %q, want a resolved state (verified/auth_failed/unknown)", events[1].AuthState)
	}
}

// TestMiddleware_PopulatesAllSpec42SubsetFields: every required spec §0/§42
// field is populated on the emitted events.
func TestMiddleware_PopulatesAllSpec42SubsetFields(t *testing.T) {
	t.Setenv("DF_PROVIDER_ID", "provider-test")
	t.Setenv("DF_PROVIDER_WALLET", "akash1testproviderwallet")
	resetMiddlewareConfig()

	cap := newCaptureEmitter(t)
	h := Middleware(cap.em, nil)(noopHandler())

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPut, "/deployment/123/manifest", nil)
	h.ServeHTTP(rec, req)

	events := cap.readEvents(t, 2)
	for i, ev := range events {
		if ev.ProviderID != "provider-test" {
			t.Errorf("event %d provider_id = %q, want provider-test", i, ev.ProviderID)
		}
		if ev.ProviderWallet != "akash1testproviderwallet" {
			t.Errorf("event %d provider_wallet = %q", i, ev.ProviderWallet)
		}
		if ev.TSUTC == "" {
			t.Errorf("event %d ts_utc empty", i)
		}
		if ev.Route == "" {
			t.Errorf("event %d route empty", i)
		}
		if ev.HTTPMethod == "" {
			t.Errorf("event %d http_method empty", i)
		}
		if ev.EventType == "" {
			t.Errorf("event %d event_type empty", i)
		}
		if ev.SourceIPSource == "" {
			t.Errorf("event %d source_ip_source empty", i)
		}
		if ev.CaptureSchemaVersion != "v1.3" {
			t.Errorf("event %d capture_schema_version = %q, want v1.3", i, ev.CaptureSchemaVersion)
		}
		// Build-time fields carry their doc.go defaults in unit tests; real
		// SHAs are injected via -ldflags -X (Plan 03 Task 2). Assert at least
		// the defaults are present (non-empty).
		if ev.ProviderUpstreamVersion == "" || ev.ProviderUpstreamCommit == "" || ev.DFTelemetryCommit == "" {
			t.Errorf("event %d has an empty build-time field: upstreamVer=%q upstreamCommit=%q dfCommit=%q",
				i, ev.ProviderUpstreamVersion, ev.ProviderUpstreamCommit, ev.DFTelemetryCommit)
		}
	}
}

// TestMiddleware_EventTypeRouteMapping: event_type resolves from the matched
// mux route template.
func TestMiddleware_EventTypeRouteMapping(t *testing.T) {
	cap := newCaptureEmitter(t)

	router := mux.NewRouter()
	router.Use(Middleware(cap.em, nil))
	router.HandleFunc("/deployment/{dseq}/manifest", okHF).Methods(http.MethodPut)
	router.HandleFunc("/lease/{dseq}/{gseq}/{oseq}/shell", okHF)
	router.HandleFunc("/lease/{dseq}/{gseq}/{oseq}/logs", okHF).Methods(http.MethodGet)
	router.HandleFunc("/lease/{dseq}/{gseq}/{oseq}/status", okHF).Methods(http.MethodGet)

	cases := []struct {
		method, path, wantType string
	}{
		{http.MethodPut, "/deployment/55/manifest", eventManifestSubmit},
		{http.MethodGet, "/lease/55/1/1/shell", eventLeaseShellConnection},
		{http.MethodGet, "/lease/55/1/1/logs", eventLeaseLogsConnection},
		{http.MethodGet, "/lease/55/1/1/status", eventLeaseStatusCheck},
	}
	for _, tc := range cases {
		rec := httptest.NewRecorder()
		req := httptest.NewRequest(tc.method, tc.path, nil)
		router.ServeHTTP(rec, req)
		events := cap.readEvents(t, 2)
		if events[0].EventType != tc.wantType {
			t.Errorf("%s %s -> event_type %q, want %q", tc.method, tc.path, events[0].EventType, tc.wantType)
		}
	}
}

// TestMiddleware_VerifiedWalletOnlySetWhenClaimsPresent: owner_wallet_verified
// is nil without claims and set from the provider's verified claims with them.
func TestMiddleware_VerifiedWalletOnlySetWhenClaimsPresent(t *testing.T) {
	t.Run("no claims -> verified wallet nil", func(t *testing.T) {
		cap := newCaptureEmitter(t)
		// claimsFn returns nil -> no verified identity.
		h := Middleware(cap.em, func(*http.Request) *ajwt.Claims { return nil })(noopHandler())

		rec := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "/status", nil)
		h.ServeHTTP(rec, req)

		events := cap.readEvents(t, 2)
		if events[1].OwnerWalletVerified != nil {
			t.Errorf("verified event owner_wallet_verified = %v, want nil without claims", *events[1].OwnerWalletVerified)
		}
		if events[1].AuthState != authUnknown && events[1].AuthState != authAuthFailed {
			t.Errorf("verified event auth_state = %q, want unknown or auth_failed", events[1].AuthState)
		}
	})

	t.Run("claims with empty issuer -> auth_failed", func(t *testing.T) {
		cap := newCaptureEmitter(t)
		// A claims object that exists but yields an empty IssuerAddress.
		h := Middleware(cap.em, func(*http.Request) *ajwt.Claims { return &ajwt.Claims{} })(noopHandler())

		rec := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "/status", nil)
		h.ServeHTTP(rec, req)

		events := cap.readEvents(t, 2)
		if events[1].OwnerWalletVerified != nil {
			t.Errorf("owner_wallet_verified = %v, want nil for empty issuer", *events[1].OwnerWalletVerified)
		}
		if events[1].AuthState != authAuthFailed {
			t.Errorf("auth_state = %q, want auth_failed for a claims object with no verified issuer", events[1].AuthState)
		}
	})
}

func okHF(w http.ResponseWriter, r *http.Request) { w.WriteHeader(http.StatusOK) }

// resetMiddlewareConfig clears the sync.Once-cached middleware config so a test
// that sets DF_PROVIDER_* env vars sees them.
func resetMiddlewareConfig() {
	mwCfgOnce = sync.Once{}
	mwCfg = middlewareConfig{}
}
