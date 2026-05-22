package dftelemetry

import (
	"log"
	"net"
	"os"
	"strconv"
	"sync"
	"time"
)

// defaultChannelCapacity is the buffered-channel size when none is configured.
// RESEARCH gives a 1024-4096 working range; 2048 balances burst tolerance
// against bounded memory under a sidecar outage.
const defaultChannelCapacity = 2048

// defaultSocketPath is the UDS the sidecar listens on when DF_TELEMETRY_SOCKET
// is unset.
const defaultSocketPath = "/var/run/digitalfrontier/vat-telemetry.sock"

// dialBackoff is the minimum wait between UDS dial attempts after a failure.
// Without it, a long sidecar outage would dial on every single event.
const dialBackoff = time.Second

// Emitter ships telemetry events to the sidecar over a Unix domain socket.
//
// The fail-open guarantee (REL-01/PERF-01) lives here: Emit performs a
// non-blocking send onto a buffered channel and a SINGLE background writer
// goroutine owns the UDS connection. Emit never blocks the provider request
// path — a full channel or an absent sidecar produces a drop, not a wait.
type Emitter struct {
	ch   chan []byte
	sock string
}

// New constructs an Emitter and starts its single writer goroutine. capacity
// is the buffered-channel size; capacity <= 0 falls back to the default.
func New(socketPath string, capacity int) *Emitter {
	if capacity <= 0 {
		capacity = defaultChannelCapacity
	}
	e := &Emitter{ch: make(chan []byte, capacity), sock: socketPath}
	go e.run()
	return e
}

// Emit serialises evt and hands it to the writer goroutine without blocking.
//
// The select/default below is the load-bearing REL-01/PERF-01 guarantee: a
// plain `e.ch <- line` would block the provider's request goroutine when the
// channel is full. That is a BANNED pattern. A marshal error or a full channel
// is a drop, never a block.
func (e *Emitter) Emit(evt Event) {
	line, err := evt.ToNDJSON()
	if err != nil {
		incDrops()
		return
	}
	select {
	case e.ch <- line: // fast path
	default: // channel full -> FAIL OPEN, never block the provider
		incDrops()
	}
}

// run is the SINGLE writer goroutine. It owns the UDS connection for the
// process lifetime: there is exactly one run() goroutine, never one per event.
// It lazily dials the socket, writes one NDJSON line per event, and reconnects
// on failure with a small backoff so a long outage does not dial per event.
func (e *Emitter) run() {
	var (
		conn        net.Conn
		lastDialErr time.Time
	)
	for line := range e.ch {
		if conn == nil {
			// Skip dialing if the last dial failed recently — a drop is
			// cheaper than a syscall storm during a sidecar outage.
			if !lastDialErr.IsZero() && time.Since(lastDialErr) < dialBackoff {
				incDrops()
				continue
			}
			c, err := net.Dial("unix", e.sock)
			if err != nil {
				// Reader absent -> drop, never block.
				lastDialErr = time.Now()
				incDrops()
				continue
			}
			conn = c
			lastDialErr = time.Time{}
		}
		if _, err := conn.Write(line); err != nil {
			// Write failure -> close, drop this event, reconnect next loop.
			_ = conn.Close()
			conn = nil
			lastDialErr = time.Now()
			incDrops()
		}
	}
	if conn != nil {
		_ = conn.Close()
	}
}

var (
	sharedOnce sync.Once
	sharedEm   *Emitter
)

// Shared returns the process-wide Emitter, initialised exactly once.
//
// Telemetry config failure must NEVER crash the provider (CLAUDE.md guardrail
// #3): if the environment is misconfigured, Shared still returns a working
// Emitter pointed at the default socket — it simply drops every event when no
// sidecar is present. The config error is logged once.
func Shared() *Emitter {
	sharedOnce.Do(func() {
		socket := os.Getenv("DF_TELEMETRY_SOCKET")
		if socket == "" {
			socket = defaultSocketPath
		}
		capacity := defaultChannelCapacity
		if v := os.Getenv("DF_TELEMETRY_CHANNEL_CAPACITY"); v != "" {
			if n, err := strconv.Atoi(v); err == nil && n > 0 {
				capacity = n
			} else {
				log.Printf("df-telemetry: invalid DF_TELEMETRY_CHANNEL_CAPACITY %q, using default %d", v, defaultChannelCapacity)
			}
		}
		sharedEm = New(socket, capacity)
	})
	return sharedEm
}
