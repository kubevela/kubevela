/*
Copyright 2026 The KubeVela Authors.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package addon

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/kubevela/pkg/util/singleton"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/rest"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	veltypes "github.com/oam-dev/kubevela/apis/types"
	pkgaddon "github.com/oam-dev/kubevela/pkg/addon"
)

// TestVersionMismatchIsDistinguishable is the contract the webhook relies on:
// checkAddonVersionMeetRequired reports both "requirement evaluated and not met"
// and "requirement could not be evaluated" through a plain error, and only the
// first is a reason to deny. This webhook runs with failurePolicy: Fail, so
// denying on a transient discovery-API or vela-core lookup failure would block
// Application applies.
func TestVersionMismatchIsDistinguishable(t *testing.T) {
	cases := map[string]struct {
		err        error
		wantDenial bool
	}{
		"a real mismatch is denied": {
			err:        fmt.Errorf("%w: the kubernetes version v1.31.5 require: <=v1.3.0", pkgaddon.ErrVersionMismatch),
			wantDenial: true,
		},
		"a wrapped mismatch is still denied": {
			err:        fmt.Errorf("checking addon foo: %w", fmt.Errorf("%w: mismatch", pkgaddon.ErrVersionMismatch)),
			wantDenial: true,
		},
		"a discovery-API failure fails open": {
			err:        errors.New("Get \"https://10.0.0.1:443/version\": dial tcp: i/o timeout"),
			wantDenial: false,
		},
		"a vela-core image lookup failure fails open": {
			err:        errors.New("cannot get vela core deployment: etcdserver: request timed out"),
			wantDenial: false,
		},
		"a malformed constraint fails open rather than denying": {
			err:        errors.New("improper constraint: <= 1.3.0"),
			wantDenial: false,
		},
	}

	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			assert.Equal(t, tc.wantDenial, errors.Is(tc.err, pkgaddon.ErrVersionMismatch))
		})
	}
}

// TestDefaultCompatCheckerFailsOpenWithoutRegistry covers the production checker
// when the addon cannot be resolved: the result must be nil (allow) rather than a
// denial.
//
// The client is injected rather than left to the singleton's lazy loader. That
// loader builds a client from whatever kubeconfig happens to be on the machine,
// so the outcome would otherwise depend on ambient state -- and on a machine with
// a reachable cluster the checker would take a different path than the one under
// test.
func TestDefaultCompatCheckerFailsOpenWithoutRegistry(t *testing.T) {
	singleton.KubeClient.Set(fake.NewClientBuilder().WithScheme(scheme.Scheme).Build())
	defer singleton.KubeClient.Reload()

	assert.Nil(t, defaultCompatChecker(context.Background(), "some-addon", "", ""),
		"an addon that cannot be resolved must never produce a denial")
}

// TestDefaultCompatCheckerRegistryNameFiltersRegistries covers the `registry
// != ""` branch: a caller-supplied registry name is still passed through to
// FindAddonPackagesDetailFromRegistry, and a name that matches nothing still
// fails open exactly like the unscoped ("") case.
func TestDefaultCompatCheckerRegistryNameFiltersRegistries(t *testing.T) {
	// Snapshot and restore rather than KubeClient.Reload: Reload rebuilds the
	// client from singleton.KubeConfig, used elsewhere in this package, panics
	// on a nil *rest.Config.
	originalKubeClient := singleton.KubeClient.Get()
	defer singleton.KubeClient.Set(originalKubeClient)
	singleton.KubeClient.Set(fake.NewClientBuilder().WithScheme(scheme.Scheme).Build())

	originalKubeConfig := singleton.KubeConfig.Get()
	singleton.KubeConfig.Set(nil)
	defer singleton.KubeConfig.Set(originalKubeConfig)

	assert.Nil(t, defaultCompatChecker(context.Background(), "some-addon", "", "a-registry-that-does-not-exist"),
		"a named registry that cannot be found must never produce a denial")
}

// fluxRegistryFixtureDir reuses the pkg/addon package's own multiversion-helm-repo
// test fixture (read-only): a "fluxcd" addon with two versions carrying
// different SystemRequirements (1.0.0 requires vela >=1.3.0 / k8s >=1.10.0,
// 2.0.0 requires vela >=1.4.0 / k8s >=1.20.0), plus a "fluxcd-no-requirements"
// entry with none. Serving it locally lets defaultCompatChecker resolve a real
// addon package -- exercising every branch downstream of a successful
// registry lookup -- without any dependency beyond a loopback httptest server.
const fluxRegistryFixtureDir = "../../../../../addon/testdata/multiversion-helm-repo"

// newFluxRegistryServer serves fluxRegistryFixtureDir, rewriting the index's
// fixed chart host (the fixture hard-codes http://127.0.0.1:18083/multi) to
// wherever this server actually listens.
func newFluxRegistryServer(t *testing.T) string {
	t.Helper()
	var server *httptest.Server
	server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.Contains(r.URL.Path, "index.yaml"):
			content, err := os.ReadFile(filepath.Join(fluxRegistryFixtureDir, "index.yaml"))
			require.NoError(t, err)
			_, _ = w.Write([]byte(strings.ReplaceAll(string(content), "http://127.0.0.1:18083/multi", server.URL)))
		case strings.Contains(r.URL.Path, "fluxcd-1.0.0.tgz"):
			b, err := os.ReadFile(filepath.Join(fluxRegistryFixtureDir, "fluxcd-1.0.0.tgz"))
			require.NoError(t, err)
			_, _ = w.Write(b)
		case strings.Contains(r.URL.Path, "fluxcd-2.0.0.tgz"):
			b, err := os.ReadFile(filepath.Join(fluxRegistryFixtureDir, "fluxcd-2.0.0.tgz"))
			require.NoError(t, err)
			_, _ = w.Write(b)
		}
	}))
	t.Cleanup(server.Close)
	return server.URL
}

// newCompatCheckerFakeClient builds a fake client with a "flux" Helm registry
// pointed at registryURL and, when velaCoreImage is non-empty, a vela-core
// Deployment carrying that image tag -- the shape fetchVelaCoreImageTag reads
// to learn the version of the running vela-core controller.
func newCompatCheckerFakeClient(t *testing.T, registryURL, velaCoreImage string) client.Client {
	t.Helper()
	cli := fake.NewClientBuilder().WithScheme(scheme.Scheme).Build()
	err := pkgaddon.NewRegistryDataStore(cli).AddRegistry(context.Background(), pkgaddon.Registry{
		Name: "flux",
		Helm: &pkgaddon.HelmSource{URL: registryURL},
	})
	require.NoError(t, err)

	if velaCoreImage != "" {
		deploy := &appsv1.Deployment{
			ObjectMeta: metav1.ObjectMeta{
				Name:      veltypes.KubeVelaControllerDeployment,
				Namespace: veltypes.DefaultKubeVelaNS,
			},
			Spec: appsv1.DeploymentSpec{
				Selector: &metav1.LabelSelector{MatchLabels: map[string]string{"app": "vela-core"}},
				Template: corev1.PodTemplateSpec{
					ObjectMeta: metav1.ObjectMeta{Labels: map[string]string{"app": "vela-core"}},
					Spec: corev1.PodSpec{
						Containers: []corev1.Container{{Name: veltypes.DefaultKubeVelaReleaseName, Image: velaCoreImage}},
					},
				},
			},
		}
		require.NoError(t, cli.Create(context.Background(), deploy))
	}
	return cli
}

// TestDefaultCompatCheckerAgainstResolvedAddon exercises defaultCompatChecker
// once a real addon package resolves successfully, covering: version
// pinning (both the override and the resolution-failure path), the
// require == nil short-circuit, both outcomes of constructing the discovery
// client, and both the allow and deny outcomes of the compatibility check
// itself.
func TestDefaultCompatCheckerAgainstResolvedAddon(t *testing.T) {
	registryURL := newFluxRegistryServer(t)

	// Snapshot whatever KubeConfig already resolved to (real value or nil) so
	// each subtest can restore it afterward. KubeClient.Reload, used by other
	// tests in this package, calls client.New(KubeConfig.Get(), ...), which
	// panics on a nil config -- so leaving a subtest's nil or broken
	// *rest.Config behind would break whichever test runs next.
	originalKubeConfig := singleton.KubeConfig.Get()

	// Likewise snapshot KubeClient: each subtest sets its own fake client, and
	// the last one set stays until this whole test's newFluxRegistryServer(t)
	// cleanup closes its backing server -- leaving that fake behind would hand
	// a later test in this package a stale client pointed at a closed server
	// instead of the lazy default.
	originalKubeClient := singleton.KubeClient.Get()
	t.Cleanup(func() { singleton.KubeClient.Set(originalKubeClient) })

	// Neither config is ever actually dialed: every case below that supplies
	// one also denies on the vela-core version check first, which returns
	// before checkAddonVersionMeetRequired ever looks at the discovery
	// client. They only need to be well-formed enough for
	// discovery.NewDiscoveryClientForConfig to accept or reject at
	// construction time.
	validDiscoveryConfig := &rest.Config{Host: "https://127.0.0.1:1"}
	brokenDiscoveryConfig := &rest.Config{
		Host: "https://127.0.0.1:1",
		TLSClientConfig: rest.TLSClientConfig{
			CertFile: "/nonexistent/client.crt",
			KeyFile:  "/nonexistent/client.key",
		},
	}

	cases := map[string]struct {
		addonName     string
		version       string
		velaCoreImage string
		kubeConfig    *rest.Config
		wantDeny      bool
	}{
		"the latest version denies when the running vela-core does not meet it": {
			addonName:     "fluxcd",
			velaCoreImage: "oamdev/vela-core:v1.0.0",
			wantDeny:      true,
		},
		"pinning an older, looser version allows what the latest version would deny": {
			addonName:     "fluxcd",
			version:       "1.0.0",
			velaCoreImage: "oamdev/vela-core:v1.3.5",
			wantDeny:      false,
		},
		"the same environment denies against the latest version's stricter requirement": {
			addonName:     "fluxcd",
			velaCoreImage: "oamdev/vela-core:v1.3.5",
			wantDeny:      true,
		},
		"an unresolvable pinned version fails open": {
			addonName: "fluxcd",
			version:   "9.9.9",
			wantDeny:  false,
		},
		"a fully compatible environment allows": {
			addonName:     "fluxcd",
			velaCoreImage: "oamdev/vela-core:v2.0.0",
			wantDeny:      false,
		},
		"a constructible discovery client does not stop a vela-core mismatch from denying": {
			addonName:     "fluxcd",
			velaCoreImage: "oamdev/vela-core:v1.0.0",
			kubeConfig:    validDiscoveryConfig,
			wantDeny:      true,
		},
		"a discovery client that fails to construct still lets a vela-core mismatch deny": {
			addonName:     "fluxcd",
			velaCoreImage: "oamdev/vela-core:v1.0.0",
			kubeConfig:    brokenDiscoveryConfig,
			wantDeny:      true,
		},
		"an addon version with no system requirements allows without any environment check": {
			addonName: "fluxcd-no-requirements",
			wantDeny:  false,
		},
		"a missing vela-core deployment fails open rather than denying": {
			addonName: "fluxcd",
			wantDeny:  false,
		},
	}

	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			t.Cleanup(func() { singleton.KubeConfig.Set(originalKubeConfig) })

			singleton.KubeClient.Set(newCompatCheckerFakeClient(t, registryURL, tc.velaCoreImage))
			singleton.KubeConfig.Set(tc.kubeConfig)

			err := defaultCompatChecker(context.Background(), tc.addonName, tc.version, "flux")
			if !tc.wantDeny {
				assert.Nil(t, err)
				return
			}
			require.NotNil(t, err)
			assert.Equal(t, "spec.components", err.Field)
			assert.Equal(t, tc.addonName, err.BadValue)
			assert.Contains(t, err.Detail, "incompatible")
		})
	}
}
