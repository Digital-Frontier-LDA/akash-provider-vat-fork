package dftelemetry

import (
	"errors"
	"fmt"
	"net"
	"net/http"
	"os"
	"strings"

	yaml "gopkg.in/yaml.v3"
)

// Source-IP source values as recorded on the EVENT (Event.SourceIPSource).
// Note these differ from the config-field enum in the source_ip_policy schema:
// the schema's source_ip_source config field uses "x_forwarded_for_trusted",
// but the event field carries "trusted_x_forwarded_for". The mapping is:
//   policy mode proxy_protocol -> event source "proxy_protocol"
//   policy mode trusted_xff    -> event source "trusted_x_forwarded_for"
//   policy mode remote_addr    -> event source "remote_addr"
//   any resolution failure     -> event source "unknown" (event still emits)
const (
	srcProxyProtocol = "proxy_protocol"
	srcTrustedXFF    = "trusted_x_forwarded_for"
	srcRemoteAddr    = "remote_addr"
	srcUnknown       = "unknown"
)

// ProxyProtocolPolicy mirrors the schema's optional proxy_protocol block.
type ProxyProtocolPolicy struct {
	Version      int  `yaml:"version"`
	RequiredOnLB bool `yaml:"required_on_lb"`
}

// TrustedXFFPolicy mirrors the schema's optional trusted_xff block.
type TrustedXFFPolicy struct {
	Proxies             []string `yaml:"proxies"`
	Depth               int      `yaml:"depth"`
	StripClientSupplied bool     `yaml:"strip_client_supplied"`

	// parsed is the net.ParseCIDR'd form of Proxies, filled by LoadSourceIPPolicy.
	parsed []*net.IPNet `yaml:"-"`
}

// SourceIPPolicy is the DEC-06 source_ip_policy the resolver honours. Its shape
// matches vat-evidence-pipeline/schemas/source_ip_policy.schema.json.
type SourceIPPolicy struct {
	SchemaVersion  int                  `yaml:"schema_version"`
	Mode           string               `yaml:"mode"`
	Env            string               `yaml:"env"`
	SourceIPSource string               `yaml:"source_ip_source"`
	ProxyProtocol  *ProxyProtocolPolicy `yaml:"proxy_protocol"`
	TrustedXFF     *TrustedXFFPolicy    `yaml:"trusted_xff"`
}

// LoadSourceIPPolicy reads and validates a DEC-06 source_ip_policy YAML file.
//
// It is fail-CLOSED on policy validity: an invalid policy (e.g. mode=remote_addr
// outside env=test) is a config error and returns a non-nil error. The caller
// (the middleware) handles that error by emitting events with
// source_ip_source=unknown rather than crashing the provider — fail-OPEN on
// telemetry delivery. The two disciplines are distinct (CLAUDE.md #9, DEC-06).
func LoadSourceIPPolicy(path string) (SourceIPPolicy, error) {
	var p SourceIPPolicy
	raw, err := os.ReadFile(path)
	if err != nil {
		return p, fmt.Errorf("df-telemetry: read source_ip_policy: %w", err)
	}
	if err := yaml.Unmarshal(raw, &p); err != nil {
		return p, fmt.Errorf("df-telemetry: parse source_ip_policy: %w", err)
	}
	if err := validatePolicy(&p); err != nil {
		return p, err
	}
	return p, nil
}

// validatePolicy applies the DEC-06 cross-field rules. These mirror the
// vat-evidence-pipeline policy-validator; the resolver re-implements only the
// checks it needs (it does not import that binary).
func validatePolicy(p *SourceIPPolicy) error {
	switch p.Mode {
	case "proxy_protocol":
		if p.Env == "prod" {
			if p.ProxyProtocol == nil || !p.ProxyProtocol.RequiredOnLB {
				return errors.New("df-telemetry: source_ip_policy mode=proxy_protocol with env=prod requires proxy_protocol.required_on_lb=true")
			}
		}
	case "trusted_xff":
		if p.TrustedXFF == nil || len(p.TrustedXFF.Proxies) == 0 {
			return errors.New("df-telemetry: source_ip_policy mode=trusted_xff requires a non-empty trusted_xff.proxies array")
		}
		// Never hand-roll CIDR parsing — net.ParseCIDR for full IPv4/IPv6
		// correctness (RESEARCH "Don't Hand-Roll").
		parsed := make([]*net.IPNet, 0, len(p.TrustedXFF.Proxies))
		for _, c := range p.TrustedXFF.Proxies {
			_, ipnet, err := net.ParseCIDR(strings.TrimSpace(c))
			if err != nil {
				return fmt.Errorf("df-telemetry: source_ip_policy trusted_xff.proxies entry %q is not a valid CIDR: %w", c, err)
			}
			parsed = append(parsed, ipnet)
		}
		p.TrustedXFF.parsed = parsed
		if p.Env == "prod" && !p.TrustedXFF.StripClientSupplied {
			return errors.New("df-telemetry: source_ip_policy mode=trusted_xff with env=prod requires trusted_xff.strip_client_supplied=true")
		}
	case "remote_addr":
		// The fork refuses to emit with remote_addr outside a test env: behind
		// an L4 LB remote_addr is the LB's IP, not the operator's (DEC-06,
		// CLAUDE.md #9, PITFALLS Pitfall 15).
		if p.Env != "test" {
			return fmt.Errorf("df-telemetry: source_ip_policy mode=remote_addr is only permitted with env=test, got env=%q", p.Env)
		}
	default:
		return fmt.Errorf("df-telemetry: source_ip_policy mode %q is not one of proxy_protocol|trusted_xff|remote_addr", p.Mode)
	}
	return nil
}

// ResolveSourceIP resolves the client source IP per the DEC-06 policy.
//
// On any resolution failure it returns ip="" and source="unknown": the event
// still emits (fail-open), it just records that the source IP was unresolvable.
// Never blind-trust X-Forwarded-For — CLAUDE.md guardrail #9.
func ResolveSourceIP(r *http.Request, p SourceIPPolicy) (ip string, source string, trustedProxyID *string) {
	switch p.Mode {
	case "proxy_protocol":
		// The L4 LB terminated PROXY protocol at the listener, so r.RemoteAddr
		// is already the real client.
		host := hostOnly(r.RemoteAddr)
		if host == "" {
			return "", srcUnknown, nil
		}
		return host, srcProxyProtocol, nil

	case "trusted_xff":
		clientIP := resolveTrustedXFF(r, p.TrustedXFF)
		if clientIP == "" {
			return "", srcUnknown, nil
		}
		return clientIP, srcTrustedXFF, nil

	case "remote_addr":
		host := hostOnly(r.RemoteAddr)
		if host == "" {
			return "", srcUnknown, nil
		}
		return host, srcRemoteAddr, nil

	default:
		return "", srcUnknown, nil
	}
}

// resolveTrustedXFF walks the X-Forwarded-For chain RIGHT-to-LEFT. A hop counts
// as a trusted proxy only when it falls inside one of the configured CIDRs; the
// first hop NOT in the allowlist (bounded by depth) is the client IP. Hops the
// client may have forged at the start of the header are never trusted — they
// are simply not in the allowlist — and strip_client_supplied makes that
// explicit. Never blind-trust XFF (CLAUDE.md #9, RESEARCH Pitfall 5).
func resolveTrustedXFF(r *http.Request, x *TrustedXFFPolicy) string {
	if x == nil || len(x.parsed) == 0 {
		return ""
	}
	xff := r.Header.Get("X-Forwarded-For")
	if xff == "" {
		return ""
	}
	hops := strings.Split(xff, ",")
	for i := range hops {
		hops[i] = strings.TrimSpace(hops[i])
	}

	depth := x.Depth
	if depth <= 0 {
		depth = len(hops)
	}

	// Walk right-to-left: peel off trusted-proxy hops, stop at the first
	// untrusted hop — that is the client.
	stepped := 0
	for i := len(hops) - 1; i >= 0 && stepped < depth; i-- {
		ip := net.ParseIP(hops[i])
		if ip == nil {
			// Unparseable hop: treat as the client boundary rather than
			// silently trusting it.
			return ""
		}
		if !ipInAny(ip, x.parsed) {
			// First untrusted hop walking inward — this is the client.
			return ip.String()
		}
		stepped++
	}
	// Every inspected hop was a trusted proxy and no client hop was found
	// within depth: nothing trustworthy to report.
	return ""
}

func ipInAny(ip net.IP, nets []*net.IPNet) bool {
	for _, n := range nets {
		if n.Contains(ip) {
			return true
		}
	}
	return false
}

// hostOnly strips the port from a host:port address. If the value has no port
// it is returned unchanged.
func hostOnly(addr string) string {
	if addr == "" {
		return ""
	}
	if host, _, err := net.SplitHostPort(addr); err == nil {
		return host
	}
	return addr
}
