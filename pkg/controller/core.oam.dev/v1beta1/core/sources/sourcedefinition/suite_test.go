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

package sourcedefinition

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	crdv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/rest"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/envtest"
	metricsserver "sigs.k8s.io/controller-runtime/pkg/metrics/server"

	oamCore "github.com/oam-dev/kubevela/apis/core.oam.dev"
	oamcommon "github.com/oam-dev/kubevela/apis/core.oam.dev/common"
	"github.com/oam-dev/kubevela/apis/core.oam.dev/v1beta1"
	oamctrl "github.com/oam-dev/kubevela/pkg/controller/core.oam.dev"
)

// startEnv brings up an API server for the wiring tests. Skips rather than fails
// when the assets are absent, so a machine without them still runs the rest of
// the package.
func startEnv(t *testing.T) *rest.Config {
	t.Helper()
	if os.Getenv("KUBEBUILDER_ASSETS") == "" {
		t.Skip("KUBEBUILDER_ASSETS is unset; skipping the envtest wiring specs")
	}
	useExisting := false
	env := &envtest.Environment{
		ControlPlaneStartTimeout: time.Minute,
		ControlPlaneStopTimeout:  time.Minute,
		CRDDirectoryPaths: []string{
			filepath.Join("../../../../../../..", "charts/vela-core/crds"),
		},
		UseExistingCluster: &useExisting,
	}
	cfg, err := env.Start()
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, env.Stop()) })

	require.NoError(t, oamCore.AddToScheme(scheme.Scheme))
	require.NoError(t, crdv1.AddToScheme(scheme.Scheme))
	return cfg
}

func newManager(t *testing.T, cfg *rest.Config) ctrl.Manager {
	t.Helper()
	mgr, err := ctrl.NewManager(cfg, ctrl.Options{
		Scheme:  scheme.Scheme,
		Metrics: metricsserver.Options{BindAddress: "0"},
	})
	require.NoError(t, err)
	return mgr
}

// Setup is the whole wiring path: parseOptions, the reconciler, the controller
// registration and the GC runnable. Nothing else calls it, so a failure here is
// a controller that silently never starts.
//
// One test rather than several, because controller-runtime registers a
// controller's name globally for its metrics: a second Setup in the same binary
// fails with "already exists" no matter what it is testing.
func TestControllerWiring(t *testing.T) {
	cfg := startEnv(t)
	mgr := newManager(t, cfg)

	require.NoError(t, Setup(mgr, oamctrl.Args{
		ConcurrentReconciles: 2,
		DefRevisionLimit:     5,
	}), "the controller could not be registered with the manager")

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	stopped := make(chan error, 1)
	go func() { stopped <- mgr.Start(ctx) }()

	require.True(t, mgr.GetCache().WaitForCacheSync(ctx),
		"the cache never synced, so the controller is not watching SourceDefinitions")

	cli, err := client.New(cfg, client.Options{Scheme: scheme.Scheme})
	require.NoError(t, err)
	_ = cli.Create(ctx, &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "vela-system"}})

	// The fake-client tests cover the branches; this covers that the controller
	// is wired to the events at all, and reconciles a real object through a real
	// API server.
	t.Run("reconciles a real definition", func(t *testing.T) {
		def := &v1beta1.SourceDefinition{
			ObjectMeta: metav1.ObjectMeta{Name: "probe-source", Namespace: "vela-system"},
			Spec: v1beta1.SourceDefinitionSpec{
				Schematic: &oamcommon.Schematic{CUE: &oamcommon.CUE{Template: `
$internal: {key: "probe-source", keyInputs: []}
schema: {host: string}
output: {host: "example.com"}
`}},
			},
		}
		require.NoError(t, cli.Create(ctx, def))
		t.Cleanup(func() { _ = cli.Delete(context.Background(), def) })

		require.Eventually(t, func() bool {
			got := &v1beta1.SourceDefinition{}
			if err := cli.Get(ctx, client.ObjectKeyFromObject(def), got); err != nil {
				return false
			}
			return got.Status.ConfigTemplateRef != nil && got.Status.ConfigTemplateRef.Name != ""
		}, 90*time.Second, time.Second,
			"no ConfigTemplate was registered, so nothing validates this source's reads")
	})

	// Every Runnable the manager holds, the sweep included, has to honour the
	// context or shutdown hangs.
	t.Run("stops cleanly", func(t *testing.T) {
		cancel()
		select {
		case err := <-stopped:
			require.NoError(t, err)
		case <-time.After(60 * time.Second):
			t.Fatal("the manager did not stop; a Runnable is ignoring its context")
		}
	})
}
