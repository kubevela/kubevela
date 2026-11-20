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
	"testing"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/util/validation/field"

	"github.com/stretchr/testify/require"

	"github.com/oam-dev/kubevela/apis/core.oam.dev/v1beta1"
)

// A restricted definition an Application already uses is reported, not enforced: the
// Application keeps rendering, and the condition clears once the use is gone.
func TestRecordRestrictionViolations(t *testing.T) {
	rec := &recordingRecorder{}
	r := &Reconciler{Recorder: rec}
	app := &v1beta1.Application{}

	r.recordRestrictionViolations(app, field.ErrorList{
		field.Forbidden(field.NewPath("spec", "workflow", "ref"), `WorkflowStepDefinition "admin-step" may not be used in namespace "tenant-a"`),
	})
	cond := app.Status.GetCondition(restrictionViolationCondition)
	require.Equal(t, corev1.ConditionTrue, cond.Status)
	require.Contains(t, cond.Message, "spec.workflow.ref")
	require.Contains(t, cond.Message, "admin-step")
	require.Len(t, rec.events, 1)

	r.recordRestrictionViolations(app, nil)
	require.Equal(t, corev1.ConditionFalse, app.Status.GetCondition(restrictionViolationCondition).Status)
	require.Len(t, rec.events, 1, "no event once the use is gone")

	clean := &v1beta1.Application{}
	r.recordRestrictionViolations(clean, nil)
	require.Empty(t, clean.Status.Conditions, "an Application that never had one gets no condition")
}
