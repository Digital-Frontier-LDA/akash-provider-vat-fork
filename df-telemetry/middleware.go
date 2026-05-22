package dftelemetry

import (
	"net/http"
	"sync"
	"time"

	"github.com/gorilla/mux"

	ajwt "pkg.akt.dev/go/util/jwt"
)

// ClaimsFunc reads the provider's already-verified JWT/mTLS claims from a
// request. The provider stores claims under an UNEXPORTED gorilla/context key
// in package gateway/rest, so dftelemetry cannot read them directly — the
// single router.go call site passes the provider's own `requestClaims`
// accessor. SEC-04: this is the provider's ALREADY cryptographically-verified
// claims object; the hook never re-parses the JWT or re-extracts the token.
//
// requestClaims panics (an unchecked type assertion) when no claims are in
// context — i.e. before prepareAuthMiddleware runs, or on an unauthenticated
// request. Middleware calls ClaimsFunc inside a recover so that absence is
// resolved to auth_state="unknown"/"auth_failed", never a provider panic.
type ClaimsFunc func(*http.Request) *ajwt.Claims

// auth_state values per spec.
const (
	authPreAuth    = "pre_auth"
	authVerified   = "verified"
	authAuthFailed = "auth_failed"
	authUnknown    = "unknown"
)

// Middleware returns the telemetry hook as a single gorilla/mux MiddlewareFunc.
//
// It emits TWO events per request: a pre_auth event at entry (before the
// handler chain, owner_wallet_verified=nil) and a verified event AFTER
// next.ServeHTTP returns — emitting the second event earlier would capture an
// unresolved auth_state (RESEARCH Pitfall 2).
//
// claimsFn may be nil (e.g. unit tests that do not exercise the verified-wallet
// path); when nil every request resolves to auth_state="unknown".
func Middleware(em *Emitter, claimsFn ClaimsFunc) mux.MiddlewareFunc {
	cfg := loadMiddlewareConfig()
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			base := cfg.extractBase(r)

			// EVENT 1 — pre_auth. owner_wallet_verified is nil: identity is
			// not yet resolved at request entry.
			pre := base
			pre.AuthState = authPreAuth
			em.Emit(pre)

			sw := &statusWriter{ResponseWriter: w, code: http.StatusOK}
			next.ServeHTTP(sw, r) // run the real handler chain

			// EVENT 2 — verified / auth_failed / unknown. Claims are read
			// only AFTER the chain has run prepareAuthMiddleware.
			state, claimed, verified := resolveAuth(r, claimsFn)
			ev := base
			ev.AuthState = state
			ev.OwnerWalletClaimed = claimed
			ev.OwnerWalletVerified = verified
			ev.StatusCode = &sw.code
			em.Emit(ev)
		})
	}
}

// middlewareConfig caches the provider identity + source-IP policy resolved
// once at startup, so per-request work stays minimal.
type middlewareConfig struct {
	providerID     string
	providerWallet string
	providerRegion string
	providerSite   string

	policy        SourceIPPolicy
	policyValid   bool
}

var (
	mwCfgOnce sync.Once
	mwCfg     middlewareConfig
)

// loadMiddlewareConfig loads Config + the source_ip_policy once. A config or
// policy error never crashes the provider (CLAUDE.md #3): an invalid policy
// just means source_ip resolution yields source_ip_source="unknown".
func loadMiddlewareConfig() middlewareConfig {
	mwCfgOnce.Do(func() {
		cfg, _ := Load() // a config error is non-fatal; fields may be empty
		mwCfg.providerID = cfg.ProviderID
		mwCfg.providerWallet = cfg.ProviderWallet
		mwCfg.providerRegion = cfg.ProviderRegion
		mwCfg.providerSite = cfg.ProviderSite
		if cfg.SourceIPPolicyPath != "" {
			if p, err := LoadSourceIPPolicy(cfg.SourceIPPolicyPath); err == nil {
				mwCfg.policy = p
				mwCfg.policyValid = true
			}
		}
	})
	return mwCfg
}

// extractBase populates the Event fields available at request entry.
func (c middlewareConfig) extractBase(r *http.Request) Event {
	ip, source, trustedProxyID := "", srcUnknown, (*string)(nil)
	if c.policyValid {
		ip, source, trustedProxyID = ResolveSourceIP(r, c.policy)
	}

	ev := Event{
		TSUTC:          time.Now().UTC().Format(time.RFC3339),
		ProviderID:     c.providerID,
		ProviderWallet: c.providerWallet,
		EventType:      EventTypeForMuxRoute(r),
		Route:          r.URL.Path,
		HTTPMethod:     r.Method,
		SourceIP:       ip,
		SourceIPSource: source,
		TrustedProxyID: trustedProxyID,

		// Build-time fields injected via -ldflags -X (doc.go globals).
		ProviderUpstreamVersion: UpstreamVersion,
		ProviderUpstreamCommit:  UpstreamCommit,
		DFTelemetryCommit:       Commit,
		CaptureSchemaVersion:    CaptureSchemaVersion,
	}

	// dseq/gseq/oseq are mux route vars on lease/deployment routes.
	if vars := mux.Vars(r); len(vars) > 0 {
		if d := vars["dseq"]; d != "" {
			ev.DSEQ = &d
		}
		if g, ok := parseUint32(vars["gseq"]); ok {
			ev.GSEQ = &g
		}
		if o, ok := parseUint32(vars["oseq"]); ok {
			ev.OSEQ = &o
		}
	}
	return ev
}

// resolveAuth reads the provider's already-verified claims AFTER the handler
// chain ran. SEC-04: owner_wallet_verified is set ONLY from claims the provider
// already cryptographically verified — the hook never re-parses the JWT.
//
// owner_wallet_claimed and owner_wallet_verified are both set to
// IssuerAddress() for an authenticated request: Akash exposes no separate
// trustworthy "claimed" identity, so the two are equal once verification has
// succeeded (and both nil before it has). This keeps the schema's
// claimed/verified distinction structurally honest.
func resolveAuth(r *http.Request, claimsFn ClaimsFunc) (state string, claimed, verified *string) {
	if claimsFn == nil {
		return authUnknown, nil, nil
	}

	// requestClaims panics when no claims are in context (unauthenticated
	// request, or a public route that never ran prepareAuthMiddleware).
	var claims *ajwt.Claims
	func() {
		defer func() { _ = recover() }()
		claims = claimsFn(r)
	}()

	if claims == nil {
		return authUnknown, nil, nil
	}
	addr := claims.IssuerAddress()
	if addr.Empty() {
		// Claims object present but no verified issuer — authorization did
		// not yield an identity.
		return authAuthFailed, nil, nil
	}
	s := addr.String()
	return authVerified, &s, &s
}

// statusWriter wraps http.ResponseWriter to capture the status code. Default
// 200 — a handler that never calls WriteHeader still returns 200.
type statusWriter struct {
	http.ResponseWriter
	code        int
	wroteHeader bool
}

func (w *statusWriter) WriteHeader(code int) {
	if !w.wroteHeader {
		w.code = code
		w.wroteHeader = true
	}
	w.ResponseWriter.WriteHeader(code)
}

func (w *statusWriter) Write(b []byte) (int, error) {
	if !w.wroteHeader {
		w.wroteHeader = true
	}
	return w.ResponseWriter.Write(b)
}

func parseUint32(s string) (uint32, bool) {
	if s == "" {
		return 0, false
	}
	var n uint64
	for _, c := range s {
		if c < '0' || c > '9' {
			return 0, false
		}
		n = n*10 + uint64(c-'0')
		if n > 0xFFFFFFFF {
			return 0, false
		}
	}
	return uint32(n), true
}
