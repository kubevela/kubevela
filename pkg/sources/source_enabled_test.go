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

package sources

import (
	"testing"

	"github.com/stretchr/testify/require"
	utilfeature "k8s.io/apiserver/pkg/util/feature"
	featuregatetesting "k8s.io/component-base/featuregate/testing"

	"github.com/oam-dev/kubevela/pkg/cue/process"
	"github.com/oam-dev/kubevela/pkg/features"
	"github.com/oam-dev/kubevela/pkg/oam"
)

func TestExpressionsEnabledFor(t *testing.T) {
	optedIn := map[string]string{oam.AnnotationCelExpressions: "true"}

	t.Run("off for everyone", func(t *testing.T) {
		featuregatetesting.SetFeatureGateDuringTest(t, utilfeature.DefaultMutableFeatureGate,
			features.EnableCelExpressions, false)
		require.False(t, ExpressionsEnabledFor(nil))
		require.False(t, ExpressionsEnabledFor(optedIn),
			"the annotation cannot switch on a feature the operator has not enabled")
	})

	t.Run("opt-in: annotated Applications only", func(t *testing.T) {
		featuregatetesting.SetFeatureGateDuringTest(t, utilfeature.DefaultMutableFeatureGate,
			features.EnableCelExpressions, true)
		featuregatetesting.SetFeatureGateDuringTest(t, utilfeature.DefaultMutableFeatureGate,
			features.RequireCelExpressionOptIn, true)
		require.True(t, ExpressionsEnabledFor(optedIn))
		require.False(t, ExpressionsEnabledFor(nil))
		require.False(t, ExpressionsEnabledFor(map[string]string{
			oam.AnnotationCelExpressions: "yes"}),
			"only \"true\" opts in; anything else leaves the Application as it was")
	})

	t.Run("on: every Application", func(t *testing.T) {
		featuregatetesting.SetFeatureGateDuringTest(t, utilfeature.DefaultMutableFeatureGate,
			features.EnableCelExpressions, true)
		featuregatetesting.SetFeatureGateDuringTest(t, utilfeature.DefaultMutableFeatureGate,
			features.RequireCelExpressionOptIn, false)
		require.True(t, ExpressionsEnabledFor(nil))
		require.True(t, ExpressionsEnabledFor(optedIn))
	})
}

// The bug this gate exists for: $(VAR_NAME) is Kubernetes' own syntax for a
// dependent environment variable. With the pass running it is read as an
// expression and refused; with the feature off it must reach the container
// exactly as written.
func TestKubernetesEnvVarSurvivesWithTheFeatureOff(t *testing.T) {
	featuregatetesting.SetFeatureGateDuringTest(t, utilfeature.DefaultMutableFeatureGate,
		features.EnableCelExpressions, false)

	pCtx := process.NewContext(process.ContextData{
		Namespace: "default", CompName: "web", AppName: "app",
	})
	props := map[string]interface{}{
		"image": "nginx",
		"env": []interface{}{
			map[string]interface{}{"name": "SVC_HOST", "value": "example.internal"},
			map[string]interface{}{"name": "URL", "value": "http://$(SVC_HOST):8080"},
		},
		"cmd": []interface{}{"sh", "-c", "echo $(hostname)"},
	}

	out, err := ResolveSourceExpressions(pCtx, props, SurfaceComponent)
	require.NoError(t, err, "an ordinary Application must not be refused by a feature it does not use")

	got, ok := out.(map[string]interface{})
	require.True(t, ok)
	env := got["env"].([]interface{})
	require.Equal(t, "http://$(SVC_HOST):8080",
		env[1].(map[string]interface{})["value"],
		"the value must reach the workload verbatim, for kubelet to expand")
	require.Equal(t, []interface{}{"sh", "-c", "echo $(hostname)"}, got["cmd"])
}
