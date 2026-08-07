package dftelemetry

import (
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// writePolicy writes a source_ip_policy YAML to a temp file and returns its path.
func writePolicy(t *testing.T, body string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "source_ip_policy.yaml")
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatalf("write policy: %v", err)
	}
	return path
}

// TestSourceIP_ProxyProtocolResolvesAndSetsSource: PROXY-protocol mode takes
// r.RemoteAddr as the real client (the L4 LB rewrote it) and sets source.
func TestSourceIP_ProxyProtocolResolvesAndSetsSource(t *testing.T) {
	path := writePolicy(t, `schema_version: 1
mode: proxy_protocol
env: prod
source_ip_source: proxy_protocol
proxy_protocol:
  version: 2
  required_on_lb: true
`)
	p, err := LoadSourceIPPolicy(path)
	if err != nil {
		t.Fatalf("LoadSourceIPPolicy: %v", err)
	}

	r := &http.Request{RemoteAddr: "203.0.113.7:54321", Header: http.Header{}}
	ip, source, _ := ResolveSourceIP(r, p)

	if ip != "203.0.113.7" {
		t.Errorf("ip = %q, want 203.0.113.7", ip)
	}
	if source != "proxy_protocol" {
		t.Errorf("source = %q, want proxy_protocol", source)
	}
}

// TestSourceIP_TrustedXffResolvesHopAndStripsClientSupplied: trusted_xff mode
// walks the chain right-to-left, accepts the client hop before the trusted
// proxy, and never trusts a client-forged hop.
func TestSourceIP_TrustedXffResolvesHopAndStripsClientSupplied(t *testing.T) {
	path := writePolicy(t, `schema_version: 1
mode: trusted_xff
env: prod
source_ip_source: x_forwarded_for_trusted
trusted_xff:
  proxies:
    - 10.0.0.0/8
  depth: 4
  strip_client_supplied: true
`)
	p, err := LoadSourceIPPolicy(path)
	if err != nil {
		t.Fatalf("LoadSourceIPPolicy: %v", err)
	}

	t.Run("resolves hop before trusted proxy", func(t *testing.T) {
		r := &http.Request{
			RemoteAddr: "10.1.2.3:443",
			Header:     http.Header{"X-Forwarded-For": {"198.51.100.4, 10.1.2.3"}},
		}
		ip, source, _ := ResolveSourceIP(r, p)
		if ip != "198.51.100.4" {
			t.Errorf("ip = %q, want 198.51.100.4 (hop before the trusted 10.x proxy)", ip)
		}
		if source != "trusted_x_forwarded_for" {
			t.Errorf("source = %q, want trusted_x_forwarded_for", source)
		}
	})

	t.Run("client-forged leading hop is not trusted", func(t *testing.T) {
		// A malicious client prepends a forged hop "1.2.3.4". Walking
		// right-to-left, 10.1.2.3 is a trusted proxy and 198.51.100.4 is the
		// first untrusted hop -> the real client. The forged 1.2.3.4 sits
		// further left and is never reached / never trusted.
		r := &http.Request{
			RemoteAddr: "10.1.2.3:443",
			Header:     http.Header{"X-Forwarded-For": {"1.2.3.4, 198.51.100.4, 10.1.2.3"}},
		}
		ip, _, _ := ResolveSourceIP(r, p)
		if ip == "1.2.3.4" {
			t.Fatal("resolver trusted a client-forged X-Forwarded-For hop (CLAUDE.md #9 violation)")
		}
		if ip != "198.51.100.4" {
			t.Errorf("ip = %q, want 198.51.100.4", ip)
		}
	})
}

// TestSourceIP_RemoteAddrRejectedOutsideEnvTest: the loader fails closed on a
// remote_addr policy outside env=test.
func TestSourceIP_RemoteAddrRejectedOutsideEnvTest(t *testing.T) {
	path := writePolicy(t, `schema_version: 1
mode: remote_addr
env: prod
source_ip_source: remote_addr
`)
	_, err := LoadSourceIPPolicy(path)
	if err == nil {
		t.Fatal("LoadSourceIPPolicy accepted mode=remote_addr with env=prod — must be rejected (DEC-06)")
	}
	if !strings.Contains(err.Error(), "remote_addr") {
		t.Errorf("error %q does not mention remote_addr", err.Error())
	}

	// And the same policy with env=test must be accepted, and resolve.
	okPath := writePolicy(t, `schema_version: 1
mode: remote_addr
env: test
source_ip_source: remote_addr
`)
	p, err := LoadSourceIPPolicy(okPath)
	if err != nil {
		t.Fatalf("LoadSourceIPPolicy rejected a valid env=test remote_addr policy: %v", err)
	}
	r := &http.Request{RemoteAddr: "192.0.2.55:1234", Header: http.Header{}}
	ip, source, _ := ResolveSourceIP(r, p)
	if ip != "192.0.2.55" || source != "remote_addr" {
		t.Errorf("ResolveSourceIP = (%q, %q), want (192.0.2.55, remote_addr)", ip, source)
	}
}
