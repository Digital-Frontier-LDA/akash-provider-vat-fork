package dftelemetry

// Config holds the runtime configuration the emitter and middleware read.
type Config struct{}

// Load reads the telemetry configuration from the environment.
//
// Skeleton stub.
// TODO(Plan v1.0-02-02): load DF_PROVIDER_* env + socket path + source_ip_policy file.
func Load() (Config, error) {
	return Config{}, nil
}
