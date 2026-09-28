package kube

import (
	"context"
	"sort"
	"testing"

	"github.com/stretchr/testify/require"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	k8stesting "k8s.io/client-go/testing"

	manifest "pkg.akt.dev/go/manifest/v2beta3"
	mtypes "pkg.akt.dev/go/node/market/v1"
	"pkg.akt.dev/go/testutil"

	afake "github.com/akash-network/provider/pkg/client/clientset/versioned/fake"
)

// honourDeleteCollectionSelector makes the fake clientset apply the label
// selector of a DeleteCollection the way the API server does. The generated
// fake ignores it, so without this a purge scoped to one entry and a purge of
// everything are indistinguishable in a test.
func honourDeleteCollectionSelector(t *testing.T, ac *afake.Clientset) {
	t.Helper()
	gvr := schema.GroupVersionResource{Group: "akash.network", Version: "v2beta2", Resource: "providerleasedips"}
	ac.PrependReactor("delete-collection", "providerleasedips", func(action k8stesting.Action) (bool, runtime.Object, error) {
		dc := action.(k8stesting.DeleteCollectionAction)
		sel := dc.GetListRestrictions().Labels
		objs, err := ac.Tracker().List(gvr, schema.GroupVersionKind{Group: "akash.network", Version: "v2beta2", Kind: "ProviderLeasedIP"}, action.GetNamespace())
		require.NoError(t, err)
		items, err := metaItems(objs)
		require.NoError(t, err)
		for _, item := range items {
			if sel.Matches(labels.Set(item.GetLabels())) {
				require.NoError(t, ac.Tracker().Delete(gvr, action.GetNamespace(), item.GetName()))
			}
		}
		return true, nil, nil
	})
}

func TestPurgeDeclaredIPRemovesOnlyThatEntry(t *testing.T) {
	ctx := context.Background()
	ac := afake.NewSimpleClientset() // nolint: staticcheck // NewClientset needs an apply schema this CRD does not generate
	honourDeleteCollectionSelector(t, ac)

	c := &client{ac: ac, ns: "lease", log: testutil.Logger(t)}

	updated := testutil.LeaseID(t)
	other := testutil.LeaseID(t)

	require.NoError(t, c.DeclareIP(ctx, updated, "web", 80, 80, manifest.TCP, "updated-web", false))
	require.NoError(t, c.DeclareIP(ctx, updated, "web", 443, 443, manifest.TCP, "updated-web", false))
	require.NoError(t, c.DeclareIP(ctx, other, "db", 5432, 5432, manifest.TCP, "other-db", false))
	require.NoError(t, c.DeclareIP(ctx, other, "db", 5433, 5433, manifest.UDP, "other-db", false))

	// A deployment update drops one endpoint of one lease.
	require.NoError(t, c.PurgeDeclaredIP(ctx, updated, "web", 80, manifest.TCP))

	require.Equal(t, []uint32{443}, declaredPorts(ctx, t, c, updated), "only the dropped endpoint may go")
	require.Equal(t, []uint32{5432, 5433}, declaredPorts(ctx, t, c, other), "another lease's IPs must be untouched")
}

func TestPurgeDeclaredIPsRemovesOnlyThatLease(t *testing.T) {
	ctx := context.Background()
	ac := afake.NewSimpleClientset() // nolint: staticcheck // NewClientset needs an apply schema this CRD does not generate
	honourDeleteCollectionSelector(t, ac)

	c := &client{ac: ac, ns: "lease", log: testutil.Logger(t)}

	closed := testutil.LeaseID(t)
	other := testutil.LeaseID(t)
	require.NoError(t, c.DeclareIP(ctx, closed, "web", 80, 80, manifest.TCP, "closed-web", false))
	require.NoError(t, c.DeclareIP(ctx, other, "db", 5432, 5432, manifest.TCP, "other-db", false))

	require.NoError(t, c.PurgeDeclaredIPs(ctx, closed))

	require.Empty(t, declaredPorts(ctx, t, c, closed))
	require.Equal(t, []uint32{5432}, declaredPorts(ctx, t, c, other))
}

func declaredPorts(ctx context.Context, t *testing.T, c *client, lID mtypes.LeaseID) []uint32 {
	t.Helper()
	specs, err := c.GetDeclaredIPs(ctx, lID)
	require.NoError(t, err)
	ports := make([]uint32, 0, len(specs))
	for _, s := range specs {
		ports = append(ports, s.ExternalPort)
	}
	sort.Slice(ports, func(i, j int) bool { return ports[i] < ports[j] })
	return ports
}

func metaItems(list runtime.Object) ([]metav1.Object, error) {
	objs, err := meta.ExtractList(list)
	if err != nil {
		return nil, err
	}
	out := make([]metav1.Object, 0, len(objs))
	for _, o := range objs {
		m, err := meta.Accessor(o)
		if err != nil {
			return nil, err
		}
		out = append(out, m)
	}
	return out, nil
}
