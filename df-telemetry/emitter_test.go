package dftelemetry

import "testing"

// Wave 0 RED stubs — Plan v1.0-02-02 flips these to real assertions.

func TestEmitter_EmitNeverBlocksWhenChannelFull(t *testing.T) {
	t.Skip("TODO(Plan v1.0-02-02): implement non-blocking drop assertion")
}

func TestEmitter_NDJSONFramingOneObjectPerLine(t *testing.T) {
	t.Skip("TODO(Plan v1.0-02-02): implement NDJSON framing assertion")
}

func TestEmitter_SingleWriterPreservesOrder(t *testing.T) {
	t.Skip("TODO(Plan v1.0-02-02): implement single-writer ordering assertion")
}

func TestEmitter_DropCounterIncrementsOnFullChannel(t *testing.T) {
	t.Skip("TODO(Plan v1.0-02-02): implement drop-counter increment assertion")
}
