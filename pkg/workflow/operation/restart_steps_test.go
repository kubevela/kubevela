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

package operation

import (
	"testing"

	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime"
	utilfeature "k8s.io/apiserver/pkg/util/feature"
	featuregatetesting "k8s.io/component-base/featuregate/testing"

	wfv1alpha1 "github.com/kubevela/pkg/apis/oam/v1alpha1"
	workflowv1alpha1 "github.com/kubevela/workflow/api/v1alpha1"
	wfUtils "github.com/kubevela/workflow/pkg/utils"

	"github.com/oam-dev/kubevela/apis/core.oam.dev/common"
	"github.com/oam-dev/kubevela/pkg/appfile"
	"github.com/oam-dev/kubevela/pkg/features"
	"github.com/oam-dev/kubevela/pkg/oam"
	"github.com/oam-dev/kubevela/pkg/sources"
)

// Restarting from a step restarts every step that waits on it, the ones a run
// orders after it by their reads included.
func TestRestartingAStepRestartsTheStepsThatReadIt(t *testing.T) {
	featuregatetesting.SetFeatureGateDuringTest(t, utilfeature.DefaultFeatureGate, features.EnableCelExpressions, true)
	featuregatetesting.SetFeatureGateDuringTest(t, utilfeature.DefaultFeatureGate, features.RequireCelExpressionOptIn, true)
	step := func(name, typ, props string, outputs ...string) wfv1alpha1.WorkflowStep {
		s := wfv1alpha1.WorkflowStep{WorkflowStepBase: wfv1alpha1.WorkflowStepBase{Name: name, Type: typ,
			Properties: &runtime.RawExtension{Raw: []byte(props)}}}
		for _, o := range outputs {
			s.Outputs = append(s.Outputs, wfv1alpha1.OutputItem{Name: o, ValueFrom: "value"})
		}
		return s
	}
	db := common.ApplicationComponent{Name: "db", Type: "webservice", Properties: &runtime.RawExtension{Raw: []byte(`{}`)}}
	api := common.ApplicationComponent{Name: "api", Type: "webservice",
		Properties: &runtime.RawExtension{Raw: []byte(`{"host":"$(component.db.output.metadata.name)"}`)}}
	af := &appfile.Appfile{
		AppAnnotations: map[string]string{oam.AnnotationCelExpressions: "true"},
		Components:     []common.ApplicationComponent{db, api},
		WorkflowMode:   &wfv1alpha1.WorkflowExecuteMode{Steps: workflowv1alpha1.WorkflowModeDAG, SubSteps: workflowv1alpha1.WorkflowModeDAG},
		WorkflowSteps: []wfv1alpha1.WorkflowStep{
			step("db", "apply-component", `{"component":"db"}`),
			step("api", "apply-component", `{"component":"api"}`),
			step("build", "echo", `{}`, "image"),
			step("notify", "echo", `{"msg":"$(steps.build.outputs.image)"}`),
			step("lint", "echo", `{}`),
		},
	}
	af.Dependencies = sources.Dependencies(af.Components, af.AppAnnotations)
	// failed failed, and reader was skipped for it; the rest succeeded.
	kept := func(failed, reader string) []string {
		var status []workflowv1alpha1.WorkflowStepStatus
		for _, n := range []string{"db", "api", "build", "notify", "lint"} {
			phase := workflowv1alpha1.WorkflowStepPhaseSucceeded
			switch n {
			case failed:
				phase = workflowv1alpha1.WorkflowStepPhaseFailed
			case reader:
				phase = workflowv1alpha1.WorkflowStepPhaseSkipped
			}
			status = append(status, workflowv1alpha1.WorkflowStepStatus{StepStatus: workflowv1alpha1.StepStatus{Name: n, Phase: phase}})
		}
		got, _, err := wfUtils.CleanStatusFromStep(stepsAsRun(af), status, *af.WorkflowMode, &corev1.ConfigMap{}, failed)
		require.NoError(t, err)
		var names []string
		for _, s := range got {
			names = append(names, s.Name)
		}
		return names
	}
	require.Equal(t, []string{"db", "api", "lint"}, kept("build", "notify"), "notify reads build's output")
	require.Equal(t, []string{"build", "notify", "lint"}, kept("db", "api"), "api reads db's output")
}
