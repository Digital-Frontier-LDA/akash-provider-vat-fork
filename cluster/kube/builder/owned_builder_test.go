package builder

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	mani "pkg.akt.dev/go/manifest/v2beta3"
	mtypes "pkg.akt.dev/go/node/market/v1"
	attrtypes "pkg.akt.dev/go/node/types/attributes/v1"
	resources "pkg.akt.dev/go/node/types/resources/v1beta4"
	"pkg.akt.dev/go/testutil"

	crd "github.com/akash-network/provider/pkg/apis/akash.network/v2beta2"
)

const ownedBuilderTestPullSecret = "owned-builder-readonly"

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
		"gpu-attributes-without-units": func(b *Workload) {
			b.group.Services[0].Resources.GPU = &resources.GPU{
				Units: resources.NewResourceValue(0), Attributes: attrtypes.Attributes{{Key: "vendor", Value: "nvidia"}},
			}
		},
		"scheduler-gpu-without-units": func(b *Workload) {
			b.sparams[0] = &crd.SchedulerParams{Resources: &crd.SchedulerResources{GPU: &crd.SchedulerResourceGPU{Vendor: GPUVendorNvidia}}}
		},
		"scheduler-interconnect": func(b *Workload) {
			b.sparams[0] = &crd.SchedulerParams{Resources: &crd.SchedulerResources{Interconnect: &crd.SchedulerResourceInterconnect{}}}
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
	b.settings.OwnedBuilderImagePullSecretName = ownedBuilderTestPullSecret
	b.settings.DockerImagePullSecretsName = "legacy-global"
	require.Equal(t, []corev1.LocalObjectReference{{Name: ownedBuilderTestPullSecret}}, b.imagePullSecrets())
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
		b.settings.OwnedBuilderImagePullSecretName = ownedBuilderTestPullSecret
		b.settings.DockerImagePullSecretsName = "legacy-global"
		change(b)
		require.Equal(t, []corev1.LocalObjectReference{{Name: "legacy-global"}}, b.imagePullSecrets())
		require.False(t, *b.container().SecurityContext.AllowPrivilegeEscalation)
	}
	b.group.Services[0].Credentials = &mani.ImageCredentials{Host: "ghcr.io", Username: "inert", Password: "inert"}
	require.NotEqual(t, []corev1.LocalObjectReference{{Name: ownedBuilderTestPullSecret}}, b.imagePullSecrets())
	require.False(t, *b.container().SecurityContext.AllowPrivilegeEscalation)
}

func TestOwnedBuilderPullSecretValidation(t *testing.T) {
	valid := ownedBuilderFixture().settings
	valid.OwnedBuilderImagePullSecretName = ownedBuilderTestPullSecret
	require.NoError(t, ValidateSettings(valid))
	for _, name := range []string{"UPPER", "../other", "space value", "-prefix", "suffix-", strings.Repeat("a", 64)} {
		changed := valid
		changed.OwnedBuilderImagePullSecretName = name
		require.ErrorIs(t, ValidateSettings(changed), ErrSettingsValidation)
	}
	valid.OwnedBuilderImage = ""
	require.ErrorIs(t, ValidateSettings(valid), ErrSettingsValidation)
}

func TestOwnedBuilderLifetimeMatchesPublishedEntrypoint(t *testing.T) {
	for _, tc := range []struct {
		value string
		admit bool
	}{
		{"60", true}, {"120", true}, {"3599", true}, {"3600", true},
		{"0060", true}, {"060", true},
		{"", false}, {"0", false}, {"59", false}, {"3601", false},
		{"00060", false}, {"99999999999999999999999999", false},
		{"-60", false}, {"+60", false}, {"60.0", false}, {"6e2", false},
		{" 60", false}, {"60 ", false}, {"60\n", false}, {"６０", false},
	} {
		t.Run(tc.value, func(t *testing.T) {
			b := ownedBuilderFixture()
			b.settings.OwnedBuilderImagePullSecretName = ownedBuilderTestPullSecret
			b.group.Services[0].Env = append(b.group.Services[0].Env, "BUILDER_TTL_SECONDS="+tc.value)
			require.Equal(t, tc.admit, *b.container().SecurityContext.AllowPrivilegeEscalation)
			if tc.admit {
				require.Equal(t, []corev1.LocalObjectReference{{Name: ownedBuilderTestPullSecret}}, b.imagePullSecrets())
			} else {
				require.Empty(t, b.imagePullSecrets())
			}
		})
	}
	b := ownedBuilderFixture()
	b.group.Services[0].Env = append(b.group.Services[0].Env, "BUILDER_TTL_SECONDS=60", "BUILDER_TTL_SECONDS=60")
	require.False(t, *b.container().SecurityContext.AllowPrivilegeEscalation)
}

func TestOwnedBuilderGeneratedPodContract(t *testing.T) {
	for _, admitted := range []bool{true, false} {
		name := "invalid-lifetime"
		if admitted {
			name = "default-lifetime"
		}
		t.Run(name, func(t *testing.T) {
			fixture := ownedBuilderFixture()
			fixture.settings.OwnedBuilderImagePullSecretName = ownedBuilderTestPullSecret
			if !admitted {
				fixture.group.Services[0].Env = append(fixture.group.Services[0].Env, "BUILDER_TTL_SECONDS=3601")
			}
			dep := fixture.deployment.(*ClusterDeployment)
			dep.Lid = testutil.LeaseID(t)
			dep.Lid.Owner = ownedBuilderOwner
			params := crd.ClusterSettings{SchedulerParams: []*crd.SchedulerParams{nil}}
			manifest, err := crd.NewManifest("lease", dep.Lid, &fixture.group, params)
			require.NoError(t, err)
			b, err := NewWorkloadBuilder(testutil.Logger(t), fixture.settings, dep, manifest, 0)
			require.NoError(t, err)

			dbuilder := NewDeployment(b)
			d, err := dbuilder.Create()
			require.NoError(t, err)
			updatedDeployment, err := dbuilder.Update(d)
			require.NoError(t, err)
			sbuilder := BuildStatefulSet(b)
			s, err := sbuilder.Create()
			require.NoError(t, err)
			updatedStatefulSet, err := sbuilder.Update(s)
			require.NoError(t, err)
			for _, pod := range []corev1.PodSpec{d.Spec.Template.Spec, updatedDeployment.Spec.Template.Spec,
				s.Spec.Template.Spec, updatedStatefulSet.Spec.Template.Spec} {
				require.Len(t, pod.Containers, 1)
				require.Empty(t, pod.InitContainers)
				require.Empty(t, pod.Volumes)
				require.Empty(t, pod.ServiceAccountName)
				require.False(t, *pod.AutomountServiceAccountToken)
				require.False(t, pod.HostNetwork)
				require.False(t, pod.HostPID)
				require.False(t, pod.HostIPC)
				require.Nil(t, pod.ShareProcessNamespace)
				require.Nil(t, pod.RuntimeClassName)
				container := pod.Containers[0]
				require.Empty(t, container.VolumeMounts)
				require.False(t, *container.SecurityContext.Privileged)
				require.Equal(t, admitted, *container.SecurityContext.AllowPrivilegeEscalation)
				if admitted {
					require.Equal(t, ownedBuilderSecurityContext(), container.SecurityContext)
					require.Equal(t, []corev1.LocalObjectReference{{Name: ownedBuilderTestPullSecret}}, pod.ImagePullSecrets)
				} else {
					require.Nil(t, container.SecurityContext.Capabilities)
					require.Empty(t, pod.ImagePullSecrets)
				}
			}
		})
	}
}
