package cmd

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

// The named profiles (heap, goroutine, allocs, block, mutex) are the reason to
// have pprof at all, and they are the easiest thing to get wrong: gorilla/mux
// matches paths EXACTLY, so registering HandleFunc("/pprof/", pprof.Index)
// serves only that literal path and every named profile 404s.
//
// operator/inventory/cmd.go registers it that way. Measured against a live
// cluster: /debug/pprof/ returns 200 and /debug/pprof/heap returns 404. This
// test exists so the provider does not inherit that.
func TestMetricsRouterPprof(t *testing.T) {
	for _, tc := range []struct {
		name    string
		pprof   bool
		path    string
		want    int
	}{
		{"metrics always served", false, "/metrics", http.StatusOK},
		{"metrics still served with pprof", true, "/metrics", http.StatusOK},

		// off by default
		{"index absent when disabled", false, "/debug/pprof/", http.StatusNotFound},
		{"heap absent when disabled", false, "/debug/pprof/heap", http.StatusNotFound},

		// on when asked for -- including the NAMED profiles
		{"index served", true, "/debug/pprof/", http.StatusOK},
		{"heap served", true, "/debug/pprof/heap", http.StatusOK},
		{"goroutine served", true, "/debug/pprof/goroutine", http.StatusOK},
		{"allocs served", true, "/debug/pprof/allocs", http.StatusOK},
		{"cmdline served", true, "/debug/pprof/cmdline", http.StatusOK},

		// PathPrefix is a plain string prefix: without the trailing slash on
		// "/debug/pprof/", these would match and serve profiles from routes
		// nobody registered.
		{"lookalike prefix not matched", true, "/debug/pprofx", http.StatusNotFound},
		{"lookalike subpath not matched", true, "/debug/pprofx/heap", http.StatusNotFound},
		{"bare index still served", true, "/debug/pprof", http.StatusOK},
	} {
		t.Run(tc.name, func(t *testing.T) {
			rec := httptest.NewRecorder()
			req := httptest.NewRequest(http.MethodGet, tc.path, nil)
			makeMetricsRouter(tc.pprof).ServeHTTP(rec, req)
			if rec.Code != tc.want {
				t.Fatalf("%s: got %d, want %d", tc.path, rec.Code, tc.want)
			}
		})
	}
}
