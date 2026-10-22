package inventory

import (
	"testing"

	"github.com/stretchr/testify/require"
	"k8s.io/apimachinery/pkg/api/resource"

	inventoryV1 "pkg.akt.dev/go/inventory/v1"
	dvbeta "pkg.akt.dev/go/node/deployment/v1beta4"
	attrtypes "pkg.akt.dev/go/node/types/attributes/v1"
	rtypes "pkg.akt.dev/go/node/types/resources/v1beta4"
	"pkg.akt.dev/go/node/types/unit"

	ctypes "github.com/akash-network/provider/cluster/types/v1beta3"
	crd "github.com/akash-network/provider/pkg/apis/akash.network/v2beta2"
)

// tryAdjustGPU narrows res.GPU.Attributes to the model it matched, and Adjust
// passes the same res to every node it tries. These tests pin that a node
// which matches the GPU but then fails a later check does not narrow the
// request for the nodes after it.
//
// Shape taken from oni_dc on 2026-10-14: "ares" sorts first and has a free
// rtx4000ada, but almost no free memory. "hestia" has a free p40 and plenty of
// memory. An any-model order fits only on hestia.

func gpuAttrNode(name, model string, memory int64) inventoryV1.Node {
	return inventoryV1.Node{
		Name: name,
		Resources: inventoryV1.NodeResources{
			CPU:    inventoryV1.CPU{Quantity: inventoryV1.NewResourcePairMilli(32000, 32000, 0, resource.DecimalSI)},
			Memory: inventoryV1.Memory{Quantity: inventoryV1.NewResourcePair(memory, memory, 0, resource.DecimalSI)},
			GPU: inventoryV1.GPU{
				Quantity: inventoryV1.NewResourcePair(1, 1, 0, resource.DecimalSI),
				Info: inventoryV1.GPUInfoS{{
					Vendor: "nvidia", Name: model, Interface: "PCIe", MemorySize: "24Gi",
				}},
			},
			EphemeralStorage: inventoryV1.NewResourcePair(500*unit.Gi, 500*unit.Gi, 0, resource.DecimalSI),
			VolumesAttached:  inventoryV1.NewResourcePair(0, 0, 0, resource.DecimalSI),
			VolumesMounted:   inventoryV1.NewResourcePair(0, 0, 0, resource.DecimalSI),
		},
	}
}

// aresAndHestia: the first node holds a free GPU it cannot host a 16Gi order
// beside; the second can.
func aresAndHestia() inventoryV1.Cluster {
	return inventoryV1.Cluster{Nodes: inventoryV1.Nodes{
		gpuAttrNode("ares", "rtx4000ada", 8*unit.Gi),
		gpuAttrNode("hestia", "p40", 128*unit.Gi),
	}}
}

func gpuAttrReservation(gpuAttrs ...string) *testReservation {
	attrs := make(attrtypes.Attributes, 0, len(gpuAttrs))
	for _, key := range gpuAttrs {
		attrs = append(attrs, attrtypes.Attribute{Key: key, Value: "true"})
	}

	return &testReservation{
		resources: dvbeta.GroupSpec{
			Name: "gpu",
			Resources: dvbeta.ResourceUnits{{
				Resources: rtypes.Resources{
					ID:     1,
					CPU:    &rtypes.CPU{Units: rtypes.NewResourceValue(2000)},
					Memory: &rtypes.Memory{Quantity: rtypes.NewResourceValue(16 * unit.Gi)},
					GPU: &rtypes.GPU{
						Units:      rtypes.NewResourceValue(1),
						Attributes: attrs,
					},
					Storage: []rtypes.Storage{{
						Name:     "default",
						Quantity: rtypes.NewResourceValue(32 * unit.Gi),
					}},
				},
				Count: 1,
			}},
		},
	}
}

// requireLandedOn checks the committed reservation chose `model`: the
// allocated attributes and the scheduler params both name it.
func requireLandedOn(t *testing.T, res *testReservation, model string) {
	t.Helper()

	allocated := res.GetAllocatedResources()
	require.Len(t, allocated, 1)
	require.Equal(t, attrtypes.Attributes{{Key: "vendor/nvidia/model/" + model, Value: "true"}},
		allocated[0].GPU.Attributes)

	cparams, ok := res.ClusterParams().(crd.ReservationClusterSettings)
	require.True(t, ok)
	sparams := cparams[1]
	require.NotNil(t, sparams)
	require.Equal(t, model, sparams.Resources.GPU.Model)
}

func TestAdjust_AnyModelGPU_SkipsNodeThatMatchesGPUButLacksMemory(t *testing.T) {
	inv := newInventory(aresAndHestia())
	res := gpuAttrReservation("vendor/nvidia/model/*")

	require.NoError(t, inv.Adjust(res))
	requireLandedOn(t, res, "p40")
}

func TestAdjust_MultiModelGPU_SkipsNodeThatMatchesGPUButLacksMemory(t *testing.T) {
	inv := newInventory(aresAndHestia())
	res := gpuAttrReservation("vendor/nvidia/model/rtx4000ada", "vendor/nvidia/model/p40")

	require.NoError(t, inv.Adjust(res))
	requireLandedOn(t, res, "p40")
}

// The restore must not widen a request that named one model: ares is the only
// rtx4000ada and it lacks memory, so this order still has nowhere to go.
func TestAdjust_SingleModelGPU_StillRejectedWhenItsOnlyNodeLacksMemory(t *testing.T) {
	inv := newInventory(aresAndHestia())
	res := gpuAttrReservation("vendor/nvidia/model/rtx4000ada")

	require.ErrorIs(t, inv.Adjust(res), ctypes.ErrInsufficientCapacity)
}

// When the first matching node does fit, the request is still narrowed to that
// node's model, as before.
func TestAdjust_AnyModelGPU_FirstFittingNodeStillNarrowsTheRequest(t *testing.T) {
	inv := newInventory(inventoryV1.Cluster{Nodes: inventoryV1.Nodes{
		gpuAttrNode("ares", "rtx4000ada", 128*unit.Gi),
		gpuAttrNode("hestia", "p40", 128*unit.Gi),
	}})
	res := gpuAttrReservation("vendor/nvidia/model/*")

	require.NoError(t, inv.Adjust(res))
	requireLandedOn(t, res, "rtx4000ada")
}
