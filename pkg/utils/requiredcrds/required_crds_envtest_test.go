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

package requiredcrds

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/envtest"
	metricsserver "sigs.k8s.io/controller-runtime/pkg/metrics/server"
)

// The unit tests above use a hand-built RESTMapper, which proves the reporting
// but not the premise: that a CRD which was never applied produces a
// NoKindMatchError from a real API server, through the mapper a real manager
// hands out. That premise is the whole check, so it is worth an API server.
//
// It is also the only place the "everything installed" case is honest. Passing
// against a mapper built from Required is close to tautological; passing against
// a cluster loaded from charts/vela-core/crds is not.
func TestVerifyAgainstARealAPIServer(t *testing.T) {
	if os.Getenv("KUBEBUILDER_ASSETS") == "" {
		t.Skip("KUBEBUILDER_ASSETS is unset; skipping the envtest CRD checks")
	}

	const chartDir = "../../../charts/vela-core/crds"
	files, err := filepath.Glob(filepath.Join(chartDir, "*.yaml"))
	require.NoError(t, err)
	require.NotEmpty(t, files)

	// Everything except the SourceDefinition CRD, which is the shape a cluster
	// takes after `helm upgrade` onto a version that added one.
	withoutSourceDefinitions := t.TempDir()
	var omitted string
	for _, f := range files {
		if strings.Contains(filepath.Base(f), "sourcedefinitions") {
			omitted = filepath.Base(f)
			continue
		}
		raw, err := os.ReadFile(f)
		require.NoError(t, err)
		require.NoError(t, os.WriteFile(filepath.Join(withoutSourceDefinitions, filepath.Base(f)), raw, 0o600))
	}
	require.NotEmpty(t, omitted, "expected a sourcedefinitions CRD in the chart")

	for _, tc := range []struct {
		name    string
		crdDir  string
		wantErr string
	}{
		{name: "every CRD installed", crdDir: chartDir},
		{
			name:    "upgraded onto a version that added one",
			crdDir:  withoutSourceDefinitions,
			wantErr: "sourcedefinitions.core.oam.dev",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			useExisting := false
			env := &envtest.Environment{
				ControlPlaneStartTimeout: time.Minute,
				ControlPlaneStopTimeout:  time.Minute,
				CRDDirectoryPaths:        []string{tc.crdDir},
				UseExistingCluster:       &useExisting,
			}
			cfg, err := env.Start()
			require.NoError(t, err)
			t.Cleanup(func() { require.NoError(t, env.Stop()) })

			mgr, err := ctrl.NewManager(cfg, ctrl.Options{
				Metrics: metricsserver.Options{BindAddress: "0"},
			})
			require.NoError(t, err)

			err = Verify(mgr.GetRESTMapper())
			if tc.wantErr == "" {
				require.NoError(t, err)
				return
			}
			require.Error(t, err)
			require.Contains(t, err.Error(), tc.wantErr)
			require.Contains(t, err.Error(), "1 required CustomResourceDefinition(s)")
		})
	}
}
