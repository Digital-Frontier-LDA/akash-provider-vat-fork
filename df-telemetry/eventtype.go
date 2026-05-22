package dftelemetry

import (
	"net/http"
	"strings"

	"github.com/gorilla/mux"
)

// event_type values per spec §21. The fork captures CONTROL-PLANE / operator
// interactions only — manifest submission, lease shell, lease logs, lease
// status. It never maps or captures workload application routes (CLAUDE.md #8).
const (
	eventManifestSubmit       = "manifest_submit"
	eventLeaseShellConnection = "lease_shell_connection"
	eventLeaseLogsConnection  = "lease_logs_connection"
	eventLeaseStatusCheck     = "lease_status_check"
	eventManifestGet          = "manifest_get"
	eventLeaseKubeEvents      = "lease_kube_events"
	eventLeaseServiceStatus   = "lease_service_status"
	eventOther                = "other"
)

// EventTypeForRoute maps a matched route template + HTTP method to a spec §21
// event_type. The path argument is the mux route TEMPLATE (e.g.
// "/lease/{...}/shell"), not a concrete path — matching on the template keeps
// route-param values from breaking the mapping. An unmatched route returns
// "other": the hook never crashes and never invents a workload event_type.
func EventTypeForRoute(method, path string) string {
	p := strings.ToLower(path)
	switch {
	case strings.HasSuffix(p, "/manifest") && method == http.MethodPut:
		return eventManifestSubmit
	case strings.HasSuffix(p, "/manifest") && method == http.MethodGet:
		return eventManifestGet
	case strings.HasSuffix(p, "/shell"):
		return eventLeaseShellConnection
	case strings.HasSuffix(p, "/logs"):
		return eventLeaseLogsConnection
	case strings.HasSuffix(p, "/kubeevents"):
		return eventLeaseKubeEvents
	case strings.HasSuffix(p, "/service/{servicename}/status"):
		return eventLeaseServiceStatus
	case strings.HasSuffix(p, "/status"):
		return eventLeaseStatusCheck
	default:
		return eventOther
	}
}

// EventTypeForMuxRoute derives the event_type from the request's matched mux
// route. It uses the route TEMPLATE via mux.CurrentRoute so concrete dseq/gseq
// values do not affect the mapping. If no route matched it falls back to the
// raw URL path (still yielding "other" for anything unrecognised).
func EventTypeForMuxRoute(r *http.Request) string {
	method := r.Method
	if route := mux.CurrentRoute(r); route != nil {
		if tmpl, err := route.GetPathTemplate(); err == nil {
			return EventTypeForRoute(method, tmpl)
		}
	}
	return EventTypeForRoute(method, r.URL.Path)
}
