package builder

import (
	"testing"

	"github.com/cosmos/cosmos-sdk/types/bech32"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	mani "pkg.akt.dev/go/manifest/v2beta3"
	mtypes "pkg.akt.dev/go/node/market/v1"

	ctypes "github.com/akash-network/provider/cluster/types/v1beta3"
	crd "github.com/akash-network/provider/pkg/apis/akash.network/v2beta2"
)

func TestLeaseNodePlacementKeepsRequiredHardwareSelectors(t *testing.T) {
	owner, err := bech32.ConvertAndEncode("akash", make([]byte, 20))
	require.NoError(t, err)
	placement, err := ctypes.ParseLeaseNodePlacement(owner + "/island-b=node-b")
	require.NoError(t, err)
	group := &mani.Group{Name: "island-b", Services: []mani.Service{{Name: "vllm"}}}
	deployment := &ClusterDeployment{Lid: mtypes.LeaseID{Owner: owner}, Group: group}
	workload := &Workload{builder: builder{
		settings:   Settings{LeaseNodePlacement: placement},
		deployment: deployment,
		group:      *group,
		sparams: []*crd.SchedulerParams{{
			Resources: &crd.SchedulerResources{GPU: &crd.SchedulerResourceGPU{Vendor: "nvidia", Model: "v100"}},
		}},
	}}

	terms := func() []corev1.NodeSelectorTerm {
		return workload.affinity().NodeAffinity.RequiredDuringSchedulingIgnoredDuringExecution.NodeSelectorTerms
	}
	pinned := terms()
	require.Len(t, pinned, 1)
	require.Equal(t, []corev1.NodeSelectorRequirement{
		{Key: "metadata.name", Operator: corev1.NodeSelectorOpIn, Values: []string{"node-b"}},
	}, pinned[0].MatchFields)
	require.Greater(t, len(pinned[0].MatchExpressions), 1)

	workload.group.Name = "island-a"
	require.Empty(t, terms()[0].MatchFields)
	require.Equal(t, pinned[0].MatchExpressions, terms()[0].MatchExpressions)
	workload.group.Name = "island-b"
	deployment.Lid.Owner = "other"
	require.Empty(t, terms()[0].MatchFields)
}
