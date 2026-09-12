package cmd

import (
	"net/http/pprof"

	"github.com/gorilla/mux"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

// makeMetricsRouter builds the handler served on --metrics-listener.
//
// withPprof adds net/http/pprof under /debug/pprof. It is OFF by default: this
// process decides bids, and /debug/pprof/profile blocks for 30s of CPU
// profiling, so the debug surface is opt-in even though the listener it rides
// on is already opt-in.
func makeMetricsRouter(withPprof bool) *mux.Router {
	router := mux.NewRouter()
	router.Handle("/metrics", promhttp.HandlerFor(
		prometheus.DefaultGatherer,
		promhttp.HandlerOpts{
			// Opt into OpenMetrics to support exemplars.
			EnableOpenMetrics: true,
		},
	))

	if withPprof {
		// ⛔ TWO ROUTING TRAPS HERE, BOTH LOAD-BEARING.
		//
		// 1. Do NOT use HandleFunc("/debug/pprof/", pprof.Index) alone.
		//    gorilla/mux matches paths EXACTLY, so that serves only the literal
		//    index path and every NAMED profile (heap, goroutine, allocs,
		//    block, mutex) 404s -- they are dispatched by pprof.Index off the
		//    remaining path segment and must actually be routed to it.
		//    operator/inventory/cmd.go registers it that way, and on a live
		//    cluster /debug/pprof/ returns 200 while /debug/pprof/heap
		//    returns 404. Heap is the profile worth having.
		//
		// 2. The catch-all prefix MUST keep its trailing slash. PathPrefix is a
		//    plain string prefix, so "/debug/pprof" would also match
		//    "/debug/pprofx" and serve profiles from an unintended route.
		router.HandleFunc("/debug/pprof", pprof.Index)
		router.HandleFunc("/debug/pprof/cmdline", pprof.Cmdline)
		router.HandleFunc("/debug/pprof/profile", pprof.Profile)
		router.HandleFunc("/debug/pprof/symbol", pprof.Symbol)
		router.HandleFunc("/debug/pprof/trace", pprof.Trace)
		// Index also serves the named profiles from the trailing path segment.
		router.PathPrefix("/debug/pprof/").HandlerFunc(pprof.Index)
	}

	return router
}
