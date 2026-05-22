package dftelemetry

import "net/http"

// Middleware returns the telemetry HTTP middleware.
//
// Skeleton stub: a passthrough. The real hook is a gorilla/mux MiddlewareFunc;
// the plain func(http.Handler) http.Handler signature keeps the skeleton
// compiling without resolving the gorilla dependency surface prematurely.
// TODO(Plan v1.0-02-03): gorilla/mux MiddlewareFunc, pre_auth + verified emission.
func Middleware(em *Emitter) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return next
	}
}
