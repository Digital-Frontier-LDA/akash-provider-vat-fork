package ip

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/require"

	"pkg.akt.dev/go/testutil"

	ipoptypes "github.com/akash-network/provider/operator/ip/types"
)

// TestHandleHTTPErrorDeclaresJSON guards the Content-Type on error responses.
// Set after WriteHeader it is silently dropped, the body is sniffed as
// text/plain, and the provider's client reports "unspecified error" without
// ever reading the operator's message.
func TestHandleHTTPErrorDeclaresJSON(t *testing.T) {
	op := &ipOperator{log: testutil.Logger(t)}

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/ip-lease-status/owner/1/1/1", nil)

	msg := `service "gateway-ip-443-tcp" has 0 load balancers and is invalid`
	handleHTTPError(op, rec, req, errors.New(msg), http.StatusInternalServerError)

	require.Equal(t, http.StatusInternalServerError, rec.Code)
	require.Equal(t, "application/json", rec.Result().Header.Get("Content-Type"))

	body := ipoptypes.IPOperatorErrorResponse{}
	require.NoError(t, json.NewDecoder(rec.Body).Decode(&body))
	require.Equal(t, msg, body.Error)
}
