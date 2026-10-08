package builder

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	mani "pkg.akt.dev/go/manifest/v2beta3"
	mtypes "pkg.akt.dev/go/node/market/v1"
	resources "pkg.akt.dev/go/node/types/resources/v1beta4"

	crd "github.com/akash-network/provider/pkg/apis/akash.network/v2beta2"
)

func ownedBuilderFixture() *Workload {
	image := "ghcr.io/digital-frontier-lda/df-akash-builder@sha256:" + strings.Repeat("a", 64)
	group := &mani.Group{Services: []mani.Service{{
		Name: ownedBuilderService, Image: image, Count: 1,
		Env: []string{"BUILDER_TLS_CA_B64=inert", "BUILDER_TLS_CERT_B64=inert", "BUILDER_TLS_KEY_B64=inert"},
	}}}
	return &Workload{builder: builder{
		settings:   Settings{OwnedBuilderImage: image},
		deployment: &ClusterDeployment{Lid: mtypes.LeaseID{Owner: ownedBuilderOwner}, Group: group},
		group:      *group, sparams: []*crd.SchedulerParams{nil},
	}}
}

func TestOwnedBuilderActualContainerSecurityContext(t *testing.T) {
	b := ownedBuilderFixture()
	sc := b.container().SecurityContext
	require.True(t, *sc.AllowPrivilegeEscalation)
	require.False(t, *sc.Privileged)
	require.True(t, *sc.RunAsNonRoot)
	require.Equal(t, int64(1000), *sc.RunAsUser)
	require.Equal(t, int64(1000), *sc.RunAsGroup)
	require.Equal(t, []corev1.Capability{"ALL"}, sc.Capabilities.Drop)
	require.Equal(t, []corev1.Capability{"SETUID", "SETGID"}, sc.Capabilities.Add)
	require.Equal(t, corev1.SeccompProfileTypeUnconfined, sc.SeccompProfile.Type)
	require.Equal(t, corev1.AppArmorProfileTypeUnconfined, sc.AppArmorProfile.Type)
	require.Nil(t, sc.ProcMount)
	require.False(t, *b.automountServiceAccountToken())
	require.Empty(t, b.container().VolumeMounts)
}

func TestOwnedBuilderCannotSelectItsOwnException(t *testing.T) {
	cases := map[string]func(*Workload){
		"default-off": func(b *Workload) { b.settings.OwnedBuilderImage = "" },
		"mutable-operator-image": func(b *Workload) {
			b.settings.OwnedBuilderImage = "ghcr.io/digital-frontier-lda/df-akash-builder:latest"
		},
		"foreign-owner":          func(b *Workload) { b.deployment.(*ClusterDeployment).Lid.Owner = "foreign-owner" },
		"different-service":      func(b *Workload) { b.group.Services[0].Name = "runner" },
		"different-image":        func(b *Workload) { b.group.Services[0].Image += "b" },
		"replicas":               func(b *Workload) { b.group.Services[0].Count = 2 },
		"command-override":       func(b *Workload) { b.group.Services[0].Command = []string{"sh"} },
		"arguments":              func(b *Workload) { b.group.Services[0].Args = []string{"-c", "inert"} },
		"credential-environment": func(b *Workload) { b.group.Services[0].Env = append(b.group.Services[0].Env, "GITHUB_TOKEN=inert") },
		"rootlesskit-control": func(b *Workload) {
			b.group.Services[0].Env = append(b.group.Services[0].Env, "_ROOTLESSKIT_STATE_DIR=/tmp/foreign")
		},
		"duplicate-tls": func(b *Workload) {
			b.group.Services[0].Env = append(b.group.Services[0].Env, "BUILDER_TLS_CA_B64=other")
		},
		"missing-tls":   func(b *Workload) { b.group.Services[0].Env = b.group.Services[0].Env[1:] },
		"empty-tls":     func(b *Workload) { b.group.Services[0].Env[0] = "BUILDER_TLS_CA_B64=" },
		"malformed-env": func(b *Workload) { b.group.Services[0].Env[0] = "BUILDER_TLS_CA_B64" },
		"mounted-payload": func(b *Workload) {
			b.group.Services[0].Params = &mani.ServiceParams{Storage: []mani.StorageParams{{Name: "payload", Mount: "/opt"}}}
		},
		"workload-permissions": func(b *Workload) {
			b.group.Services[0].Params = &mani.ServiceParams{Permissions: &mani.ServicePermissions{Read: []string{"pods"}}}
		},
		"image-credentials": func(b *Workload) { b.group.Services[0].Credentials = &mani.ImageCredentials{} },
		"params-credentials": func(b *Workload) {
			b.group.Services[0].Params = &mani.ServiceParams{Credentials: &mani.ImageCredentials{}}
		},
		"tee":           func(b *Workload) { b.group.Services[0].Params = &mani.ServiceParams{TEE: &mani.TEEParams{}} },
		"runtime-class": func(b *Workload) { b.sparams[0] = &crd.SchedulerParams{RuntimeClass: RuntimeClassKataQemuTDX} },
		"interconnect":  func(b *Workload) { b.group.Services[0].InterconnectGroup = "foreign-group" },
		"gpu": func(b *Workload) {
			b.group.Services[0].Resources.GPU = &resources.GPU{Units: resources.NewResourceValue(1)}
			b.sparams[0] = &crd.SchedulerParams{Resources: &crd.SchedulerResources{GPU: &crd.SchedulerResourceGPU{Vendor: GPUVendorNvidia}}}
		},
	}
	for name, mutate := range cases {
		t.Run(name, func(t *testing.T) {
			b := ownedBuilderFixture()
			mutate(b)
			sc := b.container().SecurityContext
			require.False(t, *sc.AllowPrivilegeEscalation)
			require.False(t, *sc.Privileged)
			require.Nil(t, sc.Capabilities)
		})
	}
}

func TestOwnedBuilderSettingsRequireFixedImmutablePackage(t *testing.T) {
	require.NoError(t, ValidateSettings(NewDefaultSettings()))
	require.NoError(t, ValidateSettings(ownedBuilderFixture().settings))
	for _, image := range []string{
		"ghcr.io/digital-frontier-lda/df-akash-builder:latest",
		"ghcr.io/foreign/df-akash-builder@sha256:" + strings.Repeat("a", 64),
		"ghcr.io/digital-frontier-lda/df-akash-runner@sha256:" + strings.Repeat("a", 64),
		"ghcr.io/digital-frontier-lda/df-akash-builder@sha256:" + strings.Repeat("A", 64),
	} {
		require.ErrorIs(t, ValidateSettings(Settings{OwnedBuilderImage: image}), ErrSettingsValidation)
	}
}

func TestOwnedBuilderPullSecretIsLimitedToExactContract(t *testing.T) {
	b := ownedBuilderFixture()
	b.settings.OwnedBuilderImagePullSecretName = "owned-builder-readonly"
	b.settings.DockerImagePullSecretsName = "legacy-global"
	require.Equal(t, []corev1.LocalObjectReference{{Name: "owned-builder-readonly"}}, b.imagePullSecrets())
	require.Empty(t, b.container().VolumeMounts)
	require.False(t, *b.automountServiceAccountToken())
	for _, change := range []func(*Workload){
		func(b *Workload) { b.deployment.(*ClusterDeployment).Lid.Owner = "foreign-owner" },
		func(b *Workload) { b.group.Services[0].Name = "runner" },
		func(b *Workload) { b.group.Services[0].Image += "b" },
		func(b *Workload) { b.group.Services[0].Command = []string{"sh"} },
		func(b *Workload) { b.group.Services[0].Env = append(b.group.Services[0].Env, "GITHUB_TOKEN=inert") },
	} {
		b := ownedBuilderFixture()
		b.settings.OwnedBuilderImagePullSecretName = "owned-builder-readonly"
		b.settings.DockerImagePullSecretsName = "legacy-global"
		change(b)
		require.Equal(t, []corev1.LocalObjectReference{{Name: "legacy-global"}}, b.imagePullSecrets())
		require.False(t, *b.container().SecurityContext.AllowPrivilegeEscalation)
	}
	b.group.Services[0].Credentials = &mani.ImageCredentials{Host: "ghcr.io", Username: "inert", Password: "inert"}
	require.NotEqual(t, []corev1.LocalObjectReference{{Name: "owned-builder-readonly"}}, b.imagePullSecrets())
	require.False(t, *b.container().SecurityContext.AllowPrivilegeEscalation)
}

func TestOwnedBuilderPullSecretValidation(t *testing.T) {
	valid := ownedBuilderFixture().settings
	valid.OwnedBuilderImagePullSecretName = "owned-builder-readonly"
	require.NoError(t, ValidateSettings(valid))
	for _, name := range []string{"UPPER", "../other", "space value", "-prefix", "suffix-", strings.Repeat("a", 64)} {
		changed := valid
		changed.OwnedBuilderImagePullSecretName = name
		require.ErrorIs(t, ValidateSettings(changed), ErrSettingsValidation)
	}
	valid.OwnedBuilderImage = ""
	require.ErrorIs(t, ValidateSettings(valid), ErrSettingsValidation)
}
