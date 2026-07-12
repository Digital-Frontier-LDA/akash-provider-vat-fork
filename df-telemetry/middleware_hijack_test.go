package dftelemetry

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gorilla/mux"
	"github.com/gorilla/websocket"
)

// The provider's shell/logs handlers hijack the connection for the WebSocket
// upgrade (gorilla/websocket via http.ResponseController, which resolves
// Hijacker through Unwrap chains). A middleware wrapper that hides Hijacker
// makes every upgrade fail with a 500 before the handler runs — the exact
// fleet-wide lease-shell/lease-logs outage of 2026-07. This test dials a real
// WebSocket through the middleware so that regression cannot ship again.
func TestMiddlewareAllowsWebsocketHijack(t *testing.T) {
	c := newCaptureEmitter(t)

	router := mux.NewRouter()
	router.Use(Middleware(c.em, nil))
	upgrader := websocket.Upgrader{}
	router.HandleFunc("/lease/{dseq}/{gseq}/{oseq}/shell", func(w http.ResponseWriter, r *http.Request) {
		ws, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			// Upgrade has already written its own error response.
			return
		}
		defer func() { _ = ws.Close() }()
		_ = ws.WriteMessage(websocket.TextMessage, []byte("ok"))
	})

	srv := httptest.NewServer(router)
	defer srv.Close()

	url := "ws" + strings.TrimPrefix(srv.URL, "http") + "/lease/123/1/1/shell"
	ws, resp, err := websocket.DefaultDialer.Dial(url, nil)
	if err != nil {
		status := 0
		if resp != nil {
			status = resp.StatusCode
		}
		t.Fatalf("websocket upgrade through telemetry middleware failed (HTTP %d): %v", status, err)
	}
	defer func() { _ = ws.Close() }()

	_, msg, err := ws.ReadMessage()
	if err != nil {
		t.Fatalf("read after upgrade: %v", err)
	}
	if string(msg) != "ok" {
		t.Fatalf("frame = %q, want %q", msg, "ok")
	}

	// Both telemetry events must still flow for a hijacked request.
	evs := c.readEvents(t, 2)
	if evs[0].AuthState != authPreAuth {
		t.Fatalf("event 1 auth_state = %q, want %q", evs[0].AuthState, authPreAuth)
	}
	if evs[1].DSEQ == nil || *evs[1].DSEQ != "123" {
		t.Fatalf("event 2 dseq = %v, want 123", evs[1].DSEQ)
	}
}
