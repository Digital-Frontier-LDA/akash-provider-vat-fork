package cluster

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"pkg.akt.dev/go/testutil"

	cip "github.com/akash-network/provider/cluster/types/v1beta3/clients/ip"
	cipmocks "github.com/akash-network/provider/mocks/cluster/types/clients/ip"
)

// Test_runCheck_BoundedWhenIPOperatorHangs guards the reservation gate. The run
// loop stops taking reservations until runCheck delivers, and hands it a
// context that is never cancelled. An IP operator request that never returns
// (on alphavps, 2026-09-28: every connection held by leaked response bodies)
// must fail the pass, not block it, or the provider bids on nothing until it
// is restarted.
func Test_runCheck_BoundedWhenIPOperatorHangs(t *testing.T) {
	prev := ipCheckTimeout
	ipCheckTimeout = 100 * time.Millisecond
	t.Cleanup(func() { ipCheckTimeout = prev })

	ipClient := &cipmocks.Client{}
	ipClient.On("GetIPAddressUsage", mock.Anything).
		Run(func(args mock.Arguments) {
			<-args.Get(0).(context.Context).Done()
		}).
		Return(cip.AddressUsage{}, context.DeadlineExceeded)

	is := &inventoryService{log: testutil.Logger(t)}
	is.clients.ip = ipClient

	state := &inventoryServiceState{
		reservations: []*reservation{{
			order:            testutil.OrderID(t),
			allocated:        true,
			endpointQuantity: 1,
		}},
	}

	select {
	case res := <-is.runCheck(context.Background(), state):
		require.ErrorIs(t, res.Error(), context.DeadlineExceeded)
	case <-time.After(5 * time.Second):
		t.Fatal("runCheck did not return: a hung IP operator request would stop all reservations")
	}
}
