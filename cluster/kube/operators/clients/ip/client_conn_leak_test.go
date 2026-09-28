package ip

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"pkg.akt.dev/go/testutil"
)

// TestIPOperatorClientErrorResponsesDoNotLeakConnections reproduces the
// 2026-09-28 alphavps bid outage. The operator answers 500 with a JSON body but
// no Content-Type, so it is sniffed as text/plain and the client takes the
// "unspecified error" path without reading the body. The shared transport
// allows only 2 connections per host, so before the fix the third request
// blocked forever waiting for a connection that was never released.
//
// All requests share one long-lived context, as the inventory run loop does.
// Cancelling a request's context makes the transport close its connection,
// which would hide the leak, so the test must not cancel between requests.
func TestIPOperatorClientErrorResponsesDoNotLeakConnections(t *testing.T) {
	fake := fakeIPOperatorHandler()
	fake.setHealthStatus(http.StatusOK)
	fake.setIPLeaseStatusResponse(http.StatusInternalServerError,
		[]byte(`{"error":"service \"gateway-ip-443-tcp\" has 0 load balancers and is invalid","code":-1}`+"\n"))
	fake.setIPUsageResponse(http.StatusInternalServerError,
		[]byte(`{"error":"usage unavailable","code":-1}`+"\n"))

	server := httptest.NewServer(fake.mux)
	defer server.Close()
	defer server.CloseClientConnections()

	host, portStr, err := net.SplitHostPort(server.Listener.Addr().String())
	require.NoError(t, err)
	port, err := strconv.ParseUint(portStr, 10, 16)
	require.NoError(t, err)

	ipop, err := NewClient(context.Background(), testutil.Logger(t), &net.SRV{
		Target: host,
		Port:   uint16(port),
	})
	require.NoError(t, err)
	defer ipop.Stop()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	done := make(chan error, 1)
	go func() {
		// Well past MaxConnsPerHost (2), across both endpoints that share the pool.
		for i := 0; i < 6; i++ {
			if _, err := ipop.GetIPAddressStatus(ctx, testutil.OrderID(t)); !isRemoteErr(err) {
				done <- fmt.Errorf("status request %d: %v", i, err)
				return
			}
			if _, err := ipop.GetIPAddressUsage(ctx); !isRemoteErr(err) {
				done <- fmt.Errorf("usage request %d: %v", i, err)
				return
			}
		}

		// The operator recovering must be visible, not queued behind dead conns.
		fake.setIPLeaseStatusResponse(http.StatusNoContent, nil)
		status, err := ipop.GetIPAddressStatus(ctx, testutil.OrderID(t))
		if err != nil || status != nil {
			done <- fmt.Errorf("after recovery: status=%v err=%v", status, err)
			return
		}
		done <- nil
	}()

	select {
	case err := <-done:
		require.NoError(t, err)
	case <-time.After(10 * time.Second):
		t.Fatal("requests blocked: error responses leaked the operator's connection pool")
	}
}

func isRemoteErr(err error) bool {
	return err != nil && errors.Is(err, errIPOperatorRemote)
}
