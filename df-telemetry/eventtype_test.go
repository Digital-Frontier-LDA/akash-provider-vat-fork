package dftelemetry

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gorilla/mux"
)

// TestEventType_MapsControlPlaneRoutes: the route-to-event_type table maps
// every control-plane route and returns "other" for anything else — workload
// routes are never given a telemetry event_type (CLAUDE.md #8).
func TestEventType_MapsControlPlaneRoutes(t *testing.T) {
	cases := []struct {
		name, method, template, want string
	}{
		{"manifest submit", http.MethodPut, "/deployment/{dseq}/manifest", eventManifestSubmit},
		{"manifest get", http.MethodGet, "/lease/{dseq}/{gseq}/{oseq}/manifest", eventManifestGet},
		{"lease shell", http.MethodGet, "/lease/{dseq}/{gseq}/{oseq}/shell", eventLeaseShellConnection},
		{"lease logs", http.MethodGet, "/lease/{dseq}/{gseq}/{oseq}/logs", eventLeaseLogsConnection},
		{"lease status", http.MethodGet, "/lease/{dseq}/{gseq}/{oseq}/status", eventLeaseStatusCheck},
		{"lease kubeevents", http.MethodGet, "/lease/{dseq}/{gseq}/{oseq}/kubeevents", eventLeaseKubeEvents},
		{"unknown route", http.MethodGet, "/some/workload/path", eventOther},
		{"version", http.MethodGet, "/version", eventOther},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := EventTypeForRoute(tc.method, tc.template); got != tc.want {
				t.Errorf("EventTypeForRoute(%s, %s) = %q, want %q", tc.method, tc.template, got, tc.want)
			}
		})
	}
}

// TestEventType_MuxRouteTemplateUsed: EventTypeForMuxRoute matches on the route
// TEMPLATE, so concrete dseq/gseq/oseq values do not break the mapping.
func TestEventType_MuxRouteTemplateUsed(t *testing.T) {
	router := mux.NewRouter()
	var got string
	router.HandleFunc("/lease/{dseq}/{gseq}/{oseq}/shell", func(w http.ResponseWriter, r *http.Request) {
		got = EventTypeForMuxRoute(r)
	})

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/lease/9999/2/3/shell", nil)
	router.ServeHTTP(rec, req)

	if got != eventLeaseShellConnection {
		t.Errorf("EventTypeForMuxRoute on a concrete lease shell path = %q, want %q", got, eventLeaseShellConnection)
	}
}

// TestEventType_WorkloadRouteNotMapped: a workload-shaped route resolves to
// "other", never a control-plane event_type.
func TestEventType_WorkloadRouteNotMapped(t *testing.T) {
	for _, path := range []string{"/app/api/v1/users", "/healthz", "/metrics-of-the-workload"} {
		if got := EventTypeForRoute(http.MethodGet, path); got != eventOther {
			t.Errorf("workload route %s mapped to %q, want other", path, got)
		}
	}
}
