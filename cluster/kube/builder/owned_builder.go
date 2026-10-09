package builder

import (
	"regexp"
	"strings"

	corev1 "k8s.io/api/core/v1"
)

const ownedBuilderOwner = "akash14n4rkmz64rn0tey0r5g07l8q5x0fh2h4hu44kt"
const ownedBuilderService = "owned-buildkit"

var ownedBuilderPullSecretPattern = regexp.MustCompile(`^[a-z0-9](?:[a-z0-9-]{0,61}[a-z0-9])?$`)

var ownedBuilderImagePattern = regexp.MustCompile(`^ghcr\.io/digital-frontier-lda/df-akash-builder@sha256:[0-9a-f]{64}$`)

// isOwnedBuilder grants only the operator-approved immutable setup contract.
// It never derives privilege authority from tenant flags or environment values.
func (b *Workload) isOwnedBuilder() bool {
	image := b.settings.OwnedBuilderImage
	if image == "" || !ownedBuilderImagePattern.MatchString(image) || b.deployment.LeaseID().Owner != ownedBuilderOwner {
		return false
	}
	service := &b.group.Services[b.serviceIdx]
	if service.Name != ownedBuilderService || service.Image != image || service.Count != 1 ||
		len(service.Command) != 0 || len(service.Args) != 0 || service.Resources.GPU != nil ||
		service.Credentials != nil || service.InterconnectGroup != "" {
		return false
	}
	if service.Params != nil && (len(service.Params.Storage) != 0 || service.Params.Permissions != nil ||
		service.Params.Credentials != nil || service.Params.TEE != nil) {
		return false
	}
	if params := b.sparams[b.serviceIdx]; params != nil && params.RuntimeClass != "" {
		return false
	}
	// No command overrides, mounted payloads, workload permissions, extra
	// credentials or RootlessKit control variables enter the setup process.
	allowed := map[string]bool{
		"BUILDER_TLS_CA_B64": false, "BUILDER_TLS_CERT_B64": false,
		"BUILDER_TLS_KEY_B64": false, "BUILDER_TTL_SECONDS": false,
	}
	for _, env := range service.Env {
		parts := strings.SplitN(env, "=", 2)
		if len(parts) != 2 || parts[1] == "" {
			return false
		}
		seen, ok := allowed[parts[0]]
		if !ok || seen {
			return false
		}
		allowed[parts[0]] = true
	}
	return allowed["BUILDER_TLS_CA_B64"] && allowed["BUILDER_TLS_CERT_B64"] && allowed["BUILDER_TLS_KEY_B64"]
}

func ownedBuilderSecurityContext() *corev1.SecurityContext {
	uid, yes, no := int64(1000), true, false
	return &corev1.SecurityContext{
		RunAsUser: &uid, RunAsGroup: &uid, RunAsNonRoot: &yes,
		Privileged: &no, AllowPrivilegeEscalation: &yes,
		Capabilities: &corev1.Capabilities{
			Drop: []corev1.Capability{"ALL"},
			Add:  []corev1.Capability{"SETUID", "SETGID"},
		},
		SeccompProfile:  &corev1.SeccompProfile{Type: corev1.SeccompProfileTypeUnconfined},
		AppArmorProfile: &corev1.AppArmorProfile{Type: corev1.AppArmorProfileTypeUnconfined},
	}
}
