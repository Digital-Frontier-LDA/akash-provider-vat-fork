package v1beta3

import (
	"fmt"
	"strings"

	"github.com/cosmos/cosmos-sdk/types/bech32"
	"k8s.io/apimachinery/pkg/util/validation"
)

// LeaseNodePlacement is an optional, operator-only node pin for one wallet and
// deployment group. The same pin is used by the bid inventory and the pod builder.
type LeaseNodePlacement struct {
	owner string
	group string
	node  string
}

// ParseLeaseNodePlacement accepts an empty string or akash-owner/group=node.
func ParseLeaseNodePlacement(value string) (LeaseNodePlacement, error) {
	if value == "" {
		return LeaseNodePlacement{}, nil
	}

	key, node, hasNode := strings.Cut(value, "=")
	owner, group, hasGroup := strings.Cut(key, "/")
	prefix, address, err := bech32.DecodeAndConvert(owner)
	if !hasNode || !hasGroup || err != nil || prefix != "akash" || owner != strings.ToLower(owner) ||
		(len(address) != 20 && len(address) != 32) || len(validation.IsDNS1123Label(group)) != 0 ||
		len(validation.IsDNS1123Subdomain(node)) != 0 {
		return LeaseNodePlacement{}, fmt.Errorf("invalid lease node placement: expected akash-owner/group=node")
	}

	return LeaseNodePlacement{owner: owner, group: group, node: node}, nil
}

func (placement LeaseNodePlacement) NodeFor(owner, group string) string {
	if placement.owner == owner && placement.group == group {
		return placement.node
	}
	return ""
}
