package dftelemetry

import (
	"errors"
	"testing"
)

// TestConfig_LoadReadsAllProviderEnvVars asserts Load reads all four MP-02
// DF_PROVIDER_* vars plus the socket and policy paths.
func TestConfig_LoadReadsAllProviderEnvVars(t *testing.T) {
	t.Setenv("DF_PROVIDER_ID", "provider-alpha")
	t.Setenv("DF_PROVIDER_WALLET", "akash1exampleproviderwallet000000000000000")
	t.Setenv("DF_PROVIDER_REGION", "eu-west")
	t.Setenv("DF_PROVIDER_SITE", "lis-1")
	t.Setenv("DF_TELEMETRY_SOCKET", "/tmp/df-telemetry-test.sock")
	t.Setenv("DF_SOURCE_IP_POLICY_PATH", "/etc/df/source_ip_policy.yaml")
	t.Setenv("DF_TELEMETRY_CHANNEL_CAPACITY", "1024")

	c, err := Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	checks := map[string][2]string{
		"ProviderID":         {c.ProviderID, "provider-alpha"},
		"ProviderWallet":     {c.ProviderWallet, "akash1exampleproviderwallet000000000000000"},
		"ProviderRegion":     {c.ProviderRegion, "eu-west"},
		"ProviderSite":       {c.ProviderSite, "lis-1"},
		"SocketPath":         {c.SocketPath, "/tmp/df-telemetry-test.sock"},
		"SourceIPPolicyPath": {c.SourceIPPolicyPath, "/etc/df/source_ip_policy.yaml"},
	}
	for field, pair := range checks {
		if pair[0] != pair[1] {
			t.Errorf("%s = %q, want %q", field, pair[0], pair[1])
		}
	}
	if c.ChannelCapacity != 1024 {
		t.Errorf("ChannelCapacity = %d, want 1024", c.ChannelCapacity)
	}
}

// TestConfig_LoadErrorsOnMissingProviderID asserts Load reports a misconfigured
// provider identity (MP-01/MP-02). The error is non-fatal — Shared fails open.
func TestConfig_LoadErrorsOnMissingProviderID(t *testing.T) {
	t.Setenv("DF_PROVIDER_ID", "")
	t.Setenv("DF_PROVIDER_WALLET", "akash1somewallet")

	c, err := Load()
	if err == nil {
		t.Fatal("Load accepted an empty DF_PROVIDER_ID — must return ErrMissingProviderIdentity")
	}
	if !errors.Is(err, ErrMissingProviderIdentity) {
		t.Errorf("error = %v, want ErrMissingProviderIdentity", err)
	}
	// Load still returns usable defaults so Shared can fail open.
	if c.SocketPath == "" {
		t.Error("Load returned an empty SocketPath on error — Shared needs a usable default")
	}
}
