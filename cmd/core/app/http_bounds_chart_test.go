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

package app

import (
	"testing"

	"github.com/stretchr/testify/require"
	"helm.sh/helm/v3/pkg/chart/loader"
	"helm.sh/helm/v3/pkg/chartutil"
	"helm.sh/helm/v3/pkg/engine"
)

func renderWorkflowHTTP(t *testing.T, http map[string]interface{}) error {
	t.Helper()
	chart, err := loader.Load("../../../charts/vela-core")
	require.NoError(t, err)
	vals, err := chartutil.ToRenderValues(chart, map[string]interface{}{"workflow": map[string]interface{}{"http": http}},
		chartutil.ReleaseOptions{Name: "kubevela", Namespace: "vela-system", IsInstall: true}, chartutil.DefaultCapabilities)
	require.NoError(t, err)
	_, err = engine.Render(chart, vals)
	return err
}

// The chart refuses HTTP bounds the controller would reject at start, so a bad value fails the
// install or upgrade instead of leaving vela-core crash-looping.
func TestChartRefusesHTTPBoundsTheControllerRejects(t *testing.T) {
	for _, timeout := range []interface{}{"60s", "1m30s", "1.5s"} {
		require.NoError(t, renderWorkflowHTTP(t, map[string]interface{}{"maxTimeout": timeout}), "maxTimeout %v", timeout)
	}
	for _, timeout := range []interface{}{60, "60", "0s", "0"} {
		err := renderWorkflowHTTP(t, map[string]interface{}{"maxTimeout": timeout})
		require.ErrorContains(t, err, "workflow.http.maxTimeout must be a positive Go duration", "maxTimeout %v", timeout)
	}
	for _, size := range []interface{}{2048, "4096", float64(1048576)} {
		require.NoError(t, renderWorkflowHTTP(t, map[string]interface{}{"maxResponseBytes": size}), "maxResponseBytes %v", size)
	}
	for _, size := range []interface{}{"1Mi", 0, 1.5, "1.5"} {
		err := renderWorkflowHTTP(t, map[string]interface{}{"maxResponseBytes": size})
		require.ErrorContains(t, err, "workflow.http.maxResponseBytes must be a positive whole number", "maxResponseBytes %v", size)
	}
}
