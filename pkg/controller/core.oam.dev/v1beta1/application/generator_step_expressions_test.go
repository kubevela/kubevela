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

package application

import (
	"context"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	"k8s.io/apimachinery/pkg/runtime"
	utilfeature "k8s.io/apiserver/pkg/util/feature"
	featuregatetesting "k8s.io/component-base/featuregate/testing"

	wfContext "github.com/kubevela/workflow/pkg/context"
	wfprocess "github.com/kubevela/workflow/pkg/cue/process"
	"github.com/kubevela/workflow/pkg/expressions"
	wfTypes "github.com/kubevela/workflow/pkg/types"

	"github.com/oam-dev/kubevela/pkg/appfile"
	"github.com/oam-dev/kubevela/pkg/features"
	"github.com/oam-dev/kubevela/pkg/oam"
	"github.com/oam-dev/kubevela/pkg/sources"
)

// stepRunContext is a workflow context with only its mutable values.
type stepRunContext struct {
	wfContext.Context
	data map[string]string
}

func (c *stepRunContext) GetMutableValue(paths ...string) string {
	return c.data[strings.Join(paths, ".")]
}
func (c *stepRunContext) SetMutableValue(v string, paths ...string) {
	c.data[strings.Join(paths, ".")] = v
}

func TestWorkflowStepExpressionsAreOnlyForAnApplicationThatOptsIn(t *testing.T) {
	featuregatetesting.SetFeatureGateDuringTest(t, utilfeature.DefaultFeatureGate, features.EnableCelExpressions, true)
	featuregatetesting.SetFeatureGateDuringTest(t, utilfeature.DefaultFeatureGate, features.RequireCelExpressionOptIn, true)
	require.Nil(t, workflowStepExpressions(&appfile.Appfile{Name: "web"}, nil))
	require.NotNil(t, workflowStepExpressions(&appfile.Appfile{Name: "web",
		AppAnnotations: map[string]string{oam.AnnotationCelExpressions: "true"}}, nil))
}

func TestAWorkflowStepReadsItsOwnRenderContext(t *testing.T) {
	featuregatetesting.SetFeatureGateDuringTest(t, utilfeature.DefaultFeatureGate, features.EnableCelExpressions, true)
	featuregatetesting.SetFeatureGateDuringTest(t, utilfeature.DefaultFeatureGate, features.RequireCelExpressionOptIn, false)
	opts := workflowStepExpressions(&appfile.Appfile{Name: "web", Namespace: "prod"},
		func(string, string, map[string]sources.SourceResolutionStatus) {})
	require.NotNil(t, opts)
	ev, err := expressions.NewEvaluator(*opts, nil, "run-1")
	require.NoError(t, err)

	for props, want := range map[string]string{
		`{"component":"$(context.appName)","ns":"$(context.namespace)"}`: `{"component":"web","ns":"prod"}`,
		`{"step":"$(context.stepName) is $(context.stepType)"}`:          `{"step":"scale is scale-component"}`,
		`{"items":"$([context.appName, \"west\"])"}`:                     `{"items":["web","west"]}`,
	} {
		out, err := ev.Properties(context.Background(), wfTypes.ExpressionStep{Name: "scale", Type: "scale-component"},
			&runtime.RawExtension{Raw: []byte(props)}, &stepRunContext{data: map[string]string{}},
			wfprocess.NewContext(wfprocess.ContextData{}), nil)
		require.NoError(t, err, props)
		require.JSONEq(t, want, string(out.Raw), props)
	}
}
