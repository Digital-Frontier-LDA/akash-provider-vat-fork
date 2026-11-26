package v1beta3

import (
	"strings"
	"testing"

	"github.com/cosmos/cosmos-sdk/types/bech32"
	"github.com/stretchr/testify/require"
)

func TestLeaseNodePlacementIsOperatorScoped(t *testing.T) {
	owner, err := bech32.ConvertAndEncode("akash", make([]byte, 20))
	require.NoError(t, err)
	rule, err := ParseLeaseNodePlacement(owner + "/island-b=node-b")
	require.NoError(t, err)
	require.Equal(t, "node-b", rule.NodeFor(owner, "island-b"))
	require.Empty(t, rule.NodeFor(owner, "island-a"))
	require.Empty(t, rule.NodeFor("another-owner", "island-b"))
	require.Empty(t, LeaseNodePlacement{}.NodeFor(owner, "island-b"))
	_, err = ParseLeaseNodePlacement("")
	require.NoError(t, err)

	for _, value := range []string{
		owner + "/island-b", owner + "/=node-b", owner + "/Island-B=node-b",
		owner + "/island-b=", owner + "/island-b=node-b,node-c",
		owner + "/island-b=node-b=x", strings.ToUpper(owner) + "/island-b=node-b",
		"akash1foreign/island-b=node-b", " " + owner + "/island-b=node-b",
	} {
		_, err := ParseLeaseNodePlacement(value)
		require.Error(t, err, "must reject %q", value)
	}
}
