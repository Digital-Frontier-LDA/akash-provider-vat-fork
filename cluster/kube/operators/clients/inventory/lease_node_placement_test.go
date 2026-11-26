package inventory

import (
	"testing"

	"github.com/stretchr/testify/require"
	inventoryV1 "pkg.akt.dev/go/inventory/v1"

	ctypes "github.com/akash-network/provider/cluster/types/v1beta3"
)

func TestInventoryRequiredNodeDoesNotFallBack(t *testing.T) {
	for _, capacity := range []int64{100, 1000} {
		inv := newInventory(inventoryV1.Cluster{Nodes: inventoryV1.Nodes{
			archNode("foreign", 2000, "amd64"),
			archNode("allowed", capacity, "amd64"),
		}})
		err := inv.Adjust(archReservation(500, "", 1), ctypes.WithRequiredNode("allowed"))
		if capacity < 500 {
			require.ErrorIs(t, err, ctypes.ErrInsufficientCapacity)
		} else {
			require.NoError(t, err)
			snapshot := inv.Snapshot()
			require.Zero(t, snapshot.Nodes[0].Resources.CPU.Quantity.Allocated.MilliValue())
			require.EqualValues(t, 500, snapshot.Nodes[1].Resources.CPU.Quantity.Allocated.MilliValue())
		}
		require.ErrorIs(t, inv.Adjust(archReservation(500, "", 1), ctypes.WithRequiredNode("missing")), ctypes.ErrInsufficientCapacity)
	}
}
