package dftelemetry

// Emitter ships telemetry events to the sidecar over a Unix domain socket.
type Emitter struct{}

// Emit sends one event. Skeleton stub — must never block the provider.
// TODO(Plan v1.0-02-02): channel-backed async UDS emitter, non-blocking send.
func (e *Emitter) Emit(evt Event) {}

// Shared returns the process-wide emitter.
// TODO(Plan v1.0-02-02): sync.Once-initialised emitter reading Config.Load().
func Shared() *Emitter { return &Emitter{} }
