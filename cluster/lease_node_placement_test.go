package cluster

import (
	"testing"

	"github.com/cosmos/cosmos-sdk/types/bech32"
	"github.com/stretchr/testify/require"

	dtypes "pkg.akt.dev/go/node/deployment/v1beta4"
	mtypes "pkg.akt.dev/go/node/market/v1"

	ctypes "github.com/akash-network/provider/cluster/types/v1beta3"
)

func TestLeaseNodePlacementUsedForInitialAndRefreshedReservation(t *testing.T) {
	owner, err := bech32.ConvertAndEncode("akash", make([]byte, 20))
	require.NoError(t, err)
	rule, err := ctypes.ParseLeaseNodePlacement(owner + "/island-b=node-b")
	require.NoError(t, err)
	service := &inventoryService{config: Config{LeaseNodePlacement: rule}}

	for _, tc := range []struct {
		owner string
		group string
		node  string
	}{
		{owner, "island-b", "node-b"},
		{owner, "island-a", ""},
		{"another-owner", "island-b", ""},
	} {
		group := dtypes.GroupSpec{Name: tc.group}
		reservation := newReservation(mtypes.OrderID{Owner: tc.owner}, service.resourcesToCommit(group))
		settings := &ctypes.InventoryOptions{}
		for _, option := range service.adjustOpts(reservation) {
			settings = option(settings)
		}
		require.Equal(t, tc.node, settings.RequiredNode)
	}
}
