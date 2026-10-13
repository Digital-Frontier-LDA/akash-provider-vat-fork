package rest

import (
	"net/http"
	"net/http/httptest"
	"testing"

	gcontext "github.com/gorilla/context"
	"github.com/stretchr/testify/require"

	sdk "github.com/cosmos/cosmos-sdk/types"

	"pkg.akt.dev/go/testutil"
)

// TestRouterClearsRequestContext pins the fix for the provider's REST memory leak
// (akash-providers-IaC#202, upstream akash-network/support#658).
//
// gorilla/context keeps request values in a package-level map keyed by *http.Request,
// and only Clear removes an entry. gorilla/mux stopped calling Clear when it moved to
// Request.Context(), so every request through this router stayed in that map for the
// life of the process, together with its claims, TLS state and the values restMiddleware
// stores. prepareAuthMiddleware stores claims on EVERY request, unauthenticated routes
// included, so plain /status polling was enough to grow the heap.
func TestRouterClearsRequestContext(t *testing.T) {
	mocks := createMocks()
	addr := sdk.AccAddress(testutil.Key(t).PubKey().Address())

	var seen []*http.Request
	capture := func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// Positive control: the chain above this point did store values, so an
			// empty map after the request means they were cleared, not never set.
			require.NotEmpty(t, gcontext.GetAll(r), "no gorilla/context values were stored during the request")
			seen = append(seen, r)
			next.ServeHTTP(w, r)
			// Not cleared too early either: middleware that reads after its handler
			// returns (df-telemetry reads the verified claims there) must still see them.
			require.NotEmpty(t, gcontext.GetAll(r), "gorilla/context was cleared before the middleware chain unwound")
		})
	}

	router := newRouter(testutil.Logger(t), addr, mocks.pclient, map[interface{}]interface{}{}, capture)

	for i := 0; i < 3; i++ {
		req := httptest.NewRequest(http.MethodGet, "https://provider.test/address", nil)
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, req)
		require.Equal(t, http.StatusOK, rec.Code)
	}

	require.Len(t, seen, 3)
	for _, r := range seen {
		require.Empty(t, gcontext.GetAll(r), "gorilla/context still holds values for a finished request")
	}
}
