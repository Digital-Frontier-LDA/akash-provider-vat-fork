package inventory

import (
	"testing"

	"github.com/stretchr/testify/require"
	"k8s.io/apimachinery/pkg/api/resource"

	inventoryV1 "pkg.akt.dev/go/inventory/v1"
	dvbeta "pkg.akt.dev/go/node/deployment/v1beta4"
	mtypes "pkg.akt.dev/go/node/market/v1"
	attrtypes "pkg.akt.dev/go/node/types/attributes/v1"
	rtypes "pkg.akt.dev/go/node/types/resources/v1beta4"
	"pkg.akt.dev/go/node/types/unit"

	ctypes "github.com/akash-network/provider/cluster/types/v1beta3"
)

type fakeReservation struct {
	resources dvbeta.GroupSpec
	allocated dvbeta.ResourceUnits
	cparams   interface{}
}

var _ ctypes.Reservation = (*fakeReservation)(nil)

func (r *fakeReservation) OrderID() mtypes.OrderID                      { return mtypes.OrderID{} }
func (r *fakeReservation) Resources() dvbeta.ResourceGroup              { return r.resources }
func (r *fakeReservation) SetAllocatedResources(v dvbeta.ResourceUnits) { r.allocated = v }
func (r *fakeReservation) GetAllocatedResources() dvbeta.ResourceUnits  { return r.allocated }
func (r *fakeReservation) Allocated() bool                              { return false }
func (r *fakeReservation) SetClusterParams(v interface{})               { r.cparams = v }
func (r *fakeReservation) ClusterParams() interface{}                   { return r.cparams }

func gpuNode(name, model string, memory int64) inventoryV1.Node {
	return inventoryV1.Node{
		Name: name,
		Resources: inventoryV1.NodeResources{
			CPU:    inventoryV1.CPU{Quantity: inventoryV1.NewResourcePairMilli(32000, 32000, 0, resource.DecimalSI)},
			Memory: inventoryV1.Memory{Quantity: inventoryV1.NewResourcePair(memory, memory, 0, resource.DecimalSI)},
			GPU: inventoryV1.GPU{
				Quantity: inventoryV1.NewResourcePair(1, 1, 0, resource.DecimalSI),
				Info:     inventoryV1.GPUInfoS{{Vendor: "nvidia", Name: model, Interface: "PCIe", MemorySize: "24Gi"}},
			},
			EphemeralStorage: inventoryV1.NewResourcePair(500*unit.Gi, 500*unit.Gi, 0, resource.DecimalSI),
			VolumesAttached:  inventoryV1.NewResourcePair(0, 0, 0, resource.DecimalSI),
			VolumesMounted:   inventoryV1.NewResourcePair(0, 0, 0, resource.DecimalSI),
		},
	}
}

// The first node matches the GPU but cannot hold 16Gi beside it. Before the
// fix, that match narrowed the any-model request to rtx4000ada and the p40
// node after it was never considered.
func TestAdjust_AnyModelGPU_SkipsNodeThatMatchesGPUButLacksMemory(t *testing.T) {
	inv := &inventory{Cluster: inventoryV1.Cluster{Nodes: inventoryV1.Nodes{
		gpuNode("ares", "rtx4000ada", 8*unit.Gi),
		gpuNode("hestia", "p40", 128*unit.Gi),
	}}}
	res := &fakeReservation{resources: dvbeta.GroupSpec{
		Name: "gpu",
		Resources: dvbeta.ResourceUnits{{
			Resources: rtypes.Resources{
				ID:     1,
				CPU:    &rtypes.CPU{Units: rtypes.NewResourceValue(2000)},
				Memory: &rtypes.Memory{Quantity: rtypes.NewResourceValue(16 * unit.Gi)},
				GPU: &rtypes.GPU{
					Units:      rtypes.NewResourceValue(1),
					Attributes: attrtypes.Attributes{{Key: "vendor/nvidia/model/*", Value: "true"}},
				},
				Storage: []rtypes.Storage{{Name: "default", Quantity: rtypes.NewResourceValue(32 * unit.Gi)}},
			},
			Count: 1,
		}},
	}}

	require.NoError(t, inv.Adjust(res))
	require.Equal(t, attrtypes.Attributes{{Key: "vendor/nvidia/model/p40", Value: "true"}},
		res.GetAllocatedResources()[0].GPU.Attributes)
}
