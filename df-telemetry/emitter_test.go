package dftelemetry

import (
	"bufio"
	"encoding/json"
	"net"
	"path/filepath"
	"testing"
	"time"

	"github.com/prometheus/client_golang/prometheus/testutil"
)

// TestEmitter_EmitNeverBlocksWhenChannelFull is the core REL-01/PERF-01 unit
// test: Emit must return immediately even when the buffered channel is full and
// nothing is draining it.
func TestEmitter_EmitNeverBlocksWhenChannelFull(t *testing.T) {
	// Construct the struct directly WITHOUT starting run(), so nothing drains
	// the channel. Capacity 1 — the first send fills it, every subsequent
	// Emit must hit the select/default drop branch.
	e := &Emitter{ch: make(chan []byte, 1), sock: "unused"}

	e.Emit(Event{}) // fills the channel

	done := make(chan struct{})
	go func() {
		e.Emit(Event{}) // channel full -> must drop, not block
		close(done)
	}()

	select {
	case <-done:
		// Emit returned promptly — fail-open send works.
	case <-time.After(100 * time.Millisecond):
		t.Fatal("Emit blocked on a full channel — the non-blocking select/default send is broken (REL-01 violation)")
	}
}

// TestEmitter_DropCounterIncrementsOnFullChannel asserts df_telemetry_drops_total
// climbs by the number of events that overflowed the channel.
func TestEmitter_DropCounterIncrementsOnFullChannel(t *testing.T) {
	e := &Emitter{ch: make(chan []byte, 1), sock: "unused"}

	e.Emit(Event{}) // fills the channel (buffered, not a drop)

	before := testutil.ToFloat64(dropsTotal)
	const overflow = 5
	for i := 0; i < overflow; i++ {
		e.Emit(Event{}) // each one drops
	}
	after := testutil.ToFloat64(dropsTotal)

	if got := after - before; got != float64(overflow) {
		t.Fatalf("df_telemetry_drops_total increased by %v, want %d", got, overflow)
	}
}

// TestEmitter_NDJSONFramingOneObjectPerLine stands up a real UDS listener and
// asserts each Emit produces exactly one newline-delimited JSON object.
func TestEmitter_NDJSONFramingOneObjectPerLine(t *testing.T) {
	sock := filepath.Join(t.TempDir(), "framing.sock")
	ln, err := net.Listen("unix", sock)
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	defer ln.Close()

	e := New(sock, 16)

	const n = 3
	for i := 0; i < n; i++ {
		e.Emit(Event{EventType: "manifest_submit"})
	}

	conn, err := ln.Accept()
	if err != nil {
		t.Fatalf("accept: %v", err)
	}
	defer conn.Close()
	_ = conn.SetReadDeadline(time.Now().Add(2 * time.Second))

	sc := bufio.NewScanner(conn)
	lines := 0
	for lines < n && sc.Scan() {
		var ev Event
		if err := json.Unmarshal(sc.Bytes(), &ev); err != nil {
			t.Fatalf("line %d is not a valid JSON Event: %v", lines, err)
		}
		lines++
	}
	if lines != n {
		t.Fatalf("read %d NDJSON lines, want %d", lines, n)
	}
}

// TestEmitter_SingleWriterPreservesOrder asserts the single writer goroutine
// preserves emission order — no goroutine-per-event reordering.
func TestEmitter_SingleWriterPreservesOrder(t *testing.T) {
	sock := filepath.Join(t.TempDir(), "order.sock")
	ln, err := net.Listen("unix", sock)
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	defer ln.Close()

	e := New(sock, 64)

	const n = 20
	for i := 0; i < n; i++ {
		seq := strconvItoa(i)
		e.Emit(Event{DSEQ: &seq})
	}

	conn, err := ln.Accept()
	if err != nil {
		t.Fatalf("accept: %v", err)
	}
	defer conn.Close()
	_ = conn.SetReadDeadline(time.Now().Add(2 * time.Second))

	sc := bufio.NewScanner(conn)
	for i := 0; i < n; i++ {
		if !sc.Scan() {
			t.Fatalf("expected %d lines, got %d", n, i)
		}
		var ev Event
		if err := json.Unmarshal(sc.Bytes(), &ev); err != nil {
			t.Fatalf("line %d unmarshal: %v", i, err)
		}
		if ev.DSEQ == nil || *ev.DSEQ != strconvItoa(i) {
			got := "<nil>"
			if ev.DSEQ != nil {
				got = *ev.DSEQ
			}
			t.Fatalf("event %d arrived out of order: dseq=%s, want %d", i, got, i)
		}
	}
}

// strconvItoa is a tiny local helper so the test file needs no extra import
// beyond what the assertions require.
func strconvItoa(i int) string {
	if i == 0 {
		return "0"
	}
	neg := i < 0
	if neg {
		i = -i
	}
	var buf [20]byte
	pos := len(buf)
	for i > 0 {
		pos--
		buf[pos] = byte('0' + i%10)
		i /= 10
	}
	if neg {
		pos--
		buf[pos] = '-'
	}
	return string(buf[pos:])
}
