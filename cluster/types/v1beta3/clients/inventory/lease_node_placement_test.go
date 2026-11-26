package inventory

import (
	"testing"

	"github.com/stretchr/testify/require"

	ctypes "github.com/akash-network/provider/cluster/types/v1beta3"
)

func TestInventoryRequiredNodeDoesNotFallBack(t *testing.T) {
	state := archCluster("amd64")
	inv := &inventory{Cluster: state}
	require.ErrorIs(t, inv.Adjust(archTestOrder(""), ctypes.WithRequiredNode("missing")), ctypes.ErrInsufficientCapacity)
	require.NoError(t, inv.Adjust(archTestOrder(""), ctypes.WithRequiredNode("node")))
	snapshot := inv.Snapshot()
	require.EqualValues(t, 500, snapshot.Nodes[0].Resources.CPU.Quantity.Allocated.MilliValue())
}
