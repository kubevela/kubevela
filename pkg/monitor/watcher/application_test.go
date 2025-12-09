/*
Copyright 2022 The KubeVela Authors.

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

package watcher

import (
	"testing"

	"github.com/stretchr/testify/assert"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	workflowv1alpha1 "github.com/kubevela/workflow/api/v1alpha1"

	"github.com/oam-dev/kubevela/apis/core.oam.dev/common"
	"github.com/oam-dev/kubevela/apis/core.oam.dev/v1beta1"
)

func TestApplicationMetricsWatcher(t *testing.T) {
	t.Parallel()

	appRunning := &v1beta1.Application{
		ObjectMeta: metav1.ObjectMeta{Name: "app-running"},
		Status: common.AppStatus{
			Phase: common.ApplicationRunning,
		},
	}
	appRendering := &v1beta1.Application{
		ObjectMeta: metav1.ObjectMeta{Name: "app-rendering"},
		Status: common.AppStatus{
			Phase: common.ApplicationRendering,
		},
	}
	appWithWorkflow := &v1beta1.Application{
		ObjectMeta: metav1.ObjectMeta{Name: "app-with-workflow"},
		Status: common.AppStatus{
			Phase: common.ApplicationRunning,
			Workflow: &common.WorkflowStatus{
				Steps: []workflowv1alpha1.WorkflowStepStatus{
					{
						StepStatus: workflowv1alpha1.StepStatus{
							Name:  "step1",
							Type:  "apply-component",
							Phase: workflowv1alpha1.WorkflowStepPhaseSucceeded,
						},
					},
				},
			},
		},
	}
	appWithMixedWorkflow := &v1beta1.Application{
		ObjectMeta: metav1.ObjectMeta{Name: "app-with-mixed-workflow"},
		Status: common.AppStatus{
			Phase: common.ApplicationRunning,
			Workflow: &common.WorkflowStatus{
				Steps: []workflowv1alpha1.WorkflowStepStatus{
					{
						StepStatus: workflowv1alpha1.StepStatus{
							Name:  "step1",
							Type:  "apply-component",
							Phase: workflowv1alpha1.WorkflowStepPhaseSucceeded,
						},
					},
					{
						StepStatus: workflowv1alpha1.StepStatus{
							Name:  "step2",
							Type:  "apply-component",
							Phase: workflowv1alpha1.WorkflowStepPhaseFailed,
						},
					},
					{
						StepStatus: workflowv1alpha1.StepStatus{
							Name:  "step3",
							Type:  "suspend",
							Phase: workflowv1alpha1.WorkflowStepPhaseRunning,
						},
					},
				},
			},
		},
	}

	testCases := map[string]struct {
		app    *v1beta1.Application
		op     int
		wantPC map[string]int
		wantSC map[string]int
		wantPD map[string]struct{}
		wantSD map[string]struct{}
	}{
		"Add an application": {
			app:    appRunning,
			op:     1,
			wantPC: map[string]int{"running": 1},
			wantSC: map[string]int{},
			wantPD: map[string]struct{}{"running": {}},
			wantSD: map[string]struct{}{},
		},
		"Add an application with workflow": {
			app:    appWithWorkflow,
			op:     1,
			wantPC: map[string]int{"running": 1},
			wantSC: map[string]int{"apply-component/succeeded#": 1},
			wantPD: map[string]struct{}{"running": {}},
			wantSD: map[string]struct{}{"apply-component/succeeded#": {}},
		},
		"Delete an application": {
			app:    appRunning,
			op:     -1,
			wantPC: map[string]int{"running": -1},
			wantSC: map[string]int{},
			wantPD: map[string]struct{}{"running": {}},
			wantSD: map[string]struct{}{},
		},
		"Update an application": {
			app:    appRendering,
			op:     -1,
			wantPC: map[string]int{"rendering": -1},
			wantSC: map[string]int{},
			wantPD: map[string]struct{}{"rendering": {}},
			wantSD: map[string]struct{}{},
		},
		"Nil app status": {
			app:    &v1beta1.Application{},
			op:     1,
			wantPC: map[string]int{"-": 1},
			wantSC: map[string]int{},
			wantPD: map[string]struct{}{"-": {}},
			wantSD: map[string]struct{}{},
		},
		"Add an application with mixed workflow": {
			app:    appWithMixedWorkflow,
			op:     1,
			wantPC: map[string]int{"running": 1},
			wantSC: map[string]int{
				"apply-component/succeeded#": 1,
				"apply-component/failed#":    1,
				"suspend/running#":           1,
			},
			wantPD: map[string]struct{}{"running": {}},
			wantSD: map[string]struct{}{
				"apply-component/succeeded#": {},
				"apply-component/failed#":    {},
				"suspend/running#":           {},
			},
		},
		"Empty workflow steps": {
			app: &v1beta1.Application{
				ObjectMeta: metav1.ObjectMeta{Name: "app-empty-workflow"},
				Status: common.AppStatus{
					Phase: common.ApplicationRunning,
					Workflow: &common.WorkflowStatus{
						Steps: []workflowv1alpha1.WorkflowStepStatus{},
					},
				},
			},
			op:     1,
			wantPC: map[string]int{"running": 1},
			wantSC: map[string]int{},
			wantPD: map[string]struct{}{"running": {}},
			wantSD: map[string]struct{}{},
		},
		"Unknown phase": {
			app: &v1beta1.Application{
				ObjectMeta: metav1.ObjectMeta{Name: "app-unknown-phase"},
				Status: common.AppStatus{
					Phase: "unknown",
				},
			},
			op:     1,
			wantPC: map[string]int{"unknown": 1},
			wantSC: map[string]int{},
			wantPD: map[string]struct{}{"unknown": {}},
			wantSD: map[string]struct{}{},
		},
	}

	for name, tc := range testCases {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			watcher := &applicationMetricsWatcher{
				phaseCounter:     map[string]int{},
				stepPhaseCounter: map[string]int{},
				phaseDirty:       map[string]struct{}{},
				stepPhaseDirty:   map[string]struct{}{},
			}
			watcher.inc(tc.app, tc.op)
			assert.Equal(t, tc.wantPC, watcher.phaseCounter)
			assert.Equal(t, tc.wantSC, watcher.stepPhaseCounter)
			assert.Equal(t, tc.wantPD, watcher.phaseDirty)
			assert.Equal(t, tc.wantSD, watcher.stepPhaseDirty)
		})
	}

	t.Run("Idempotence", func(t *testing.T) {
		t.Parallel()
		watcher := &applicationMetricsWatcher{
			phaseCounter:     map[string]int{},
			stepPhaseCounter: map[string]int{},
			phaseDirty:       map[string]struct{}{},
			stepPhaseDirty:   map[string]struct{}{},
		}
		watcher.inc(appRunning, 1)
		watcher.inc(appRunning, 1)
		assert.Equal(t, map[string]int{"running": 2}, watcher.phaseCounter)
		assert.Equal(t, map[string]struct{}{"running": {}}, watcher.phaseDirty)
	})

	t.Run("Report should clear dirty flags", func(t *testing.T) {
		t.Parallel()
		watcher := &applicationMetricsWatcher{
			phaseCounter:     map[string]int{"running": 1},
			stepPhaseCounter: map[string]int{"apply-component/succeeded#": 1},
			phaseDirty:       map[string]struct{}{"running": {}},
			stepPhaseDirty:   map[string]struct{}{"apply-component/succeeded#": {}},
		}
		watcher.report()
		assert.Empty(t, watcher.phaseDirty)
		assert.Empty(t, watcher.stepPhaseDirty)
		assert.Equal(t, map[string]int{"running": 1}, watcher.phaseCounter)
		assert.Equal(t, map[string]int{"apply-component/succeeded#": 1}, watcher.stepPhaseCounter)
	})

	t.Run("getPhase helper function", func(t *testing.T) {
		t.Parallel()
		watcher := &applicationMetricsWatcher{}
		assert.Equal(t, "-", watcher.getPhase(""))
		assert.Equal(t, "running", watcher.getPhase("running"))
	})

	t.Run("getApp helper function", func(t *testing.T) {
		t.Parallel()
		watcher := &applicationMetricsWatcher{}
		inputApp := &v1beta1.Application{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test-app",
				Namespace: "test-ns",
			},
			Status: common.AppStatus{
				Phase: common.ApplicationRunning,
			},
		}
		resultApp := watcher.getApp(inputApp)
		assert.NotNil(t, resultApp)
		assert.Equal(t, "test-app", resultApp.Name)
		assert.Equal(t, "test-ns", resultApp.Namespace)
		assert.Equal(t, common.ApplicationRunning, resultApp.Status.Phase)
	})
}
