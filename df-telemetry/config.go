package dftelemetry

import (
	"errors"
	"os"
	"strconv"
)

// Config holds the runtime configuration the emitter and middleware read.
//
// MP-01/MP-02: every emitted event must carry provider_id and provider_wallet,
// so a missing DF_PROVIDER_ID / DF_PROVIDER_WALLET is a misconfiguration that
// Load reports. The emitter's Shared() still fails open on that error — it
// returns a drop-everything emitter rather than crashing the provider.
type Config struct {
	ProviderID     string
	ProviderWallet string
	ProviderRegion string
	ProviderSite   string

	SocketPath          string
	SourceIPPolicyPath  string
	ChannelCapacity     int
}

// ErrMissingProviderIdentity is returned by Load when DF_PROVIDER_ID or
// DF_PROVIDER_WALLET is empty.
var ErrMissingProviderIdentity = errors.New("df-telemetry: DF_PROVIDER_ID and DF_PROVIDER_WALLET must be set")

// Load reads the telemetry configuration from the environment.
//
// MP-02: the four DF_PROVIDER_* variables identify which provider emitted an
// event. Load returns ErrMissingProviderIdentity when the two mandatory
// identity vars are absent — callers that must not crash (Shared) handle the
// error by failing open.
func Load() (Config, error) {
	c := Config{
		ProviderID:         os.Getenv("DF_PROVIDER_ID"),
		ProviderWallet:     os.Getenv("DF_PROVIDER_WALLET"),
		ProviderRegion:     os.Getenv("DF_PROVIDER_REGION"),
		ProviderSite:       os.Getenv("DF_PROVIDER_SITE"),
		SocketPath:         os.Getenv("DF_TELEMETRY_SOCKET"),
		SourceIPPolicyPath: os.Getenv("DF_SOURCE_IP_POLICY_PATH"),
		ChannelCapacity:    defaultChannelCapacity,
	}
	if c.SocketPath == "" {
		c.SocketPath = defaultSocketPath
	}
	if v := os.Getenv("DF_TELEMETRY_CHANNEL_CAPACITY"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			c.ChannelCapacity = n
		}
	}
	if c.ProviderID == "" || c.ProviderWallet == "" {
		return c, ErrMissingProviderIdentity
	}
	return c, nil
}
