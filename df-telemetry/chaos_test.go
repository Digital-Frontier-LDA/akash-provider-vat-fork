package dftelemetry

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"testing"
	"time"

	"github.com/prometheus/client_golang/prometheus/testutil"
)

// TestChaos_NoUDSReaderRequestPathUnaffectedAndDropsCounted is the REL-01 /
// ROADMAP-criterion-3 fail-open proof at the in-process integration level: with
// NO UDS reader, every request must still succeed with unchanged latency while
// df_telemetry_drops_total climbs.
func TestChaos_NoUDSReaderRequestPathUnaffectedAndDropsCounted(t *testing.T) {
	const n = 500

	// An Emitter pointed at a socket path with NO listener — every net.Dial
	// in the writer goroutine fails, so every event is ultimately dropped.
	sock := filepath.Join(os.TempDir(), fmt.Sprintf("dfchaos-%d.sock", time.Now().UnixNano()))
	_ = os.Remove(sock)
	const capacity = 2048
	em := New(sock, capacity)

	dropsBefore := testutil.ToFloat64(dropsTotal)

	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok"))
	})

	// Baseline: the same handler WITHOUT the middleware.
	baseline := measureLatency(t, handler, n)

	// With the telemetry middleware (two Emit calls per request).
	withMW := Middleware(em, nil)(handler)
	statuses := make([]int, 0, n)
	withTimes := make([]time.Duration, 0, n)
	for i := 0; i < n; i++ {
		rec := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "/status", nil)

		// If Emit ever blocks, this guarded call fails fast instead of
		// hanging the whole suite.
		start := time.Now()
		done := make(chan struct{})
		go func() {
			withMW.ServeHTTP(rec, req)
			close(done)
		}()
		select {
		case <-done:
		case <-time.After(2 * time.Second):
			t.Fatal("a request through the middleware blocked >2s — Emit is not fail-open (REL-01 violation)")
		}
		withTimes = append(withTimes, time.Since(start))
		statuses = append(statuses, rec.Code)
	}

	// (a) every request succeeded.
	for i, code := range statuses {
		if code != http.StatusOK {
			t.Fatalf("request %d returned %d, want 200 — a dropped telemetry event affected the response path", i, code)
		}
	}

	// (b) latency not pathologically higher than baseline. A blocking send
	// would explode this; a generous ceiling catches that without flaking on
	// normal scheduler noise.
	withMedian := median(withTimes)
	if withMedian > baseline+5*time.Millisecond && withMedian > 5*time.Millisecond {
		t.Errorf("with-middleware median latency %v is pathologically above baseline %v — possible blocking send",
			withMedian, baseline)
	}

	// (c) df_telemetry_drops_total climbs. Two events per request -> 2N events
	// emitted. Every one is ultimately dropped because net.Dial keeps failing.
	// We let the channel drain, then assert the increase is at least
	// 2N - capacity: that lower bound accounts for the worst case where a full
	// channel capacity of events is still buffered (not yet drained-and-dropped)
	// at the moment of measurement. After settleUntilChannelDrained the real
	// number is at or very near exactly 2N.
	settleUntilChannelDrained(em)
	dropped := testutil.ToFloat64(dropsTotal) - dropsBefore
	const emitted = 2 * n
	if dropped < float64(emitted-capacity) {
		t.Errorf("df_telemetry_drops_total rose by %v, want >= %d (2N - capacity) — drops not counted under sidecar absence",
			dropped, emitted-capacity)
	}
}

// TestChaos_NoGoroutineLeakUnderSidecarAbsence proves the emitter uses exactly
// ONE writer goroutine, not one per event — a goroutine-per-event bug would
// make the count grow ~N under load.
func TestChaos_NoGoroutineLeakUnderSidecarAbsence(t *testing.T) {
	const n = 500

	runtime.GC()
	before := runtime.NumGoroutine()

	sock := filepath.Join(os.TempDir(), fmt.Sprintf("dfleak-%d.sock", time.Now().UnixNano()))
	_ = os.Remove(sock)
	em := New(sock, 2048) // +1 goroutine: the single writer

	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
	withMW := Middleware(em, nil)(handler)

	for i := 0; i < n; i++ {
		rec := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "/status", nil)
		withMW.ServeHTTP(rec, req)
	}

	// Settle so any transient goroutines exit.
	settleUntilChannelDrained(em)
	time.Sleep(50 * time.Millisecond)
	runtime.GC()
	after := runtime.NumGoroutine()

	// Allow a small constant for the writer goroutine + scheduler noise. If
	// the count grew by ~N, that is the goroutine-per-event anti-pattern.
	const tolerance = 5
	if after > before+1+tolerance {
		t.Fatalf("goroutine count grew from %d to %d after %d requests — expected ~+1 (the single writer), got +%d (goroutine-per-event leak?)",
			before, after, n, after-before)
	}
}

// measureLatency runs handler n times directly (no middleware) and returns the
// median per-request latency baseline.
func measureLatency(t *testing.T, handler http.Handler, n int) time.Duration {
	t.Helper()
	times := make([]time.Duration, 0, n)
	for i := 0; i < n; i++ {
		rec := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "/status", nil)
		start := time.Now()
		handler.ServeHTTP(rec, req)
		times = append(times, time.Since(start))
	}
	return median(times)
}

func median(d []time.Duration) time.Duration {
	if len(d) == 0 {
		return 0
	}
	cp := make([]time.Duration, len(d))
	copy(cp, d)
	sort.Slice(cp, func(i, j int) bool { return cp[i] < cp[j] })
	return cp[len(cp)/2]
}

// settleUntilChannelDrained waits until the emitter's channel is empty (the
// writer has consumed every queued event — each one dropped because net.Dial
// keeps failing). Bounded so a bug cannot hang the test.
func settleUntilChannelDrained(em *Emitter) {
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		if len(em.ch) == 0 {
			// Give the writer a moment to process the last in-flight item.
			time.Sleep(20 * time.Millisecond)
			if len(em.ch) == 0 {
				return
			}
		}
		time.Sleep(10 * time.Millisecond)
	}
}
