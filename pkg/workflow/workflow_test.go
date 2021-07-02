/*
Copyright 2021 The KubeVela Authors.

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

package workflow

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"

	oamcore "github.com/oam-dev/kubevela/apis/core.oam.dev/v1beta1"
	"github.com/oam-dev/kubevela/pkg/utils/apply"
)

func TestExecuteSteps(t *testing.T) {

	zerostepApp := &oamcore.Application{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test",
			Namespace: "test",
		},
		Spec: oamcore.ApplicationSpec{
			Workflow: &oamcore.Workflow{
				Steps: []oamcore.WorkflowStep{},
			},
		},
	}

	onestepApp := zerostepApp.DeepCopy()
	onestepApp.Spec.Workflow.Steps = []oamcore.WorkflowStep{{
		Name: "test",
		Type: "test",
	}}

	twostepsApp := onestepApp.DeepCopy()
	twostepsApp.Spec.Workflow.Steps = append(twostepsApp.Spec.Workflow.Steps, oamcore.WorkflowStep{
		Name: "test2",
		Type: "test2",
	})

	succeededMessage, err := json.Marshal(&SucceededMessage{ObservedGeneration: 1})
	if err != nil {
		panic(err)
	}

	succeededStep := &unstructured.Unstructured{
		Object: map[string]interface{}{
			"metadata": map[string]interface{}{
				"generation": int64(1),
			},
			"status": map[string]interface{}{
				"conditions": []interface{}{map[string]interface{}{
					"type":    CondTypeWorkflowFinish,
					"reason":  CondReasonSucceeded,
					"message": string(succeededMessage),
					"status":  CondStatusTrue,
				}},
			},
		},
	}
	succeededStepUnmatchedGen := &unstructured.Unstructured{
		Object: map[string]interface{}{
			"metadata": map[string]interface{}{
				"generation": int64(2),
			},
			"status": map[string]interface{}{
				"conditions": []interface{}{map[string]interface{}{
					"type":    CondTypeWorkflowFinish,
					"reason":  CondReasonSucceeded,
					"message": string(succeededMessage),
					"status":  CondStatusTrue,
				}},
			},
		},
	}

	runningStep := &unstructured.Unstructured{
		Object: map[string]interface{}{
			"status": map[string]interface{}{
				"conditions": []interface{}{map[string]interface{}{
					"type":   CondTypeWorkflowFinish,
					"status": CondStatusTrue,
				}},
			},
		},
	}
	stoppedStep := &unstructured.Unstructured{
		Object: map[string]interface{}{
			"status": map[string]interface{}{
				"conditions": []interface{}{map[string]interface{}{
					"type":   CondTypeWorkflowFinish,
					"reason": CondReasonStopped,
					"status": CondStatusTrue,
				}},
			},
		},
	}

	type want struct {
		done bool
		err  error
	}

	testcases := []struct {
		desc  string
		app   *oamcore.Application
		steps []*unstructured.Unstructured
		want  want
	}{{
		desc: "zero steps should return true",
		app:  zerostepApp.DeepCopy(),
		want: want{
			done: true,
		},
	}, {
		desc:  "one succeeded step should return true",
		app:   onestepApp.DeepCopy(),
		steps: []*unstructured.Unstructured{succeededStep.DeepCopy()},
		want: want{
			done: true,
		},
	}, {
		desc:  "one succeeded step with unmatched generation should return false",
		app:   onestepApp.DeepCopy(),
		steps: []*unstructured.Unstructured{succeededStepUnmatchedGen.DeepCopy()},
		want: want{
			done: false,
		},
	}, {
		desc:  "one running step should return false",
		app:   onestepApp.DeepCopy(),
		steps: []*unstructured.Unstructured{runningStep.DeepCopy()},
		want: want{
			done: false,
		},
	}, {
		desc:  "one stopped step should return true",
		app:   onestepApp.DeepCopy(),
		steps: []*unstructured.Unstructured{stoppedStep.DeepCopy()},
		want: want{
			done: true,
		},
	}, {
		desc:  "one succeeded step and one running step should return false",
		app:   twostepsApp.DeepCopy(),
		steps: []*unstructured.Unstructured{succeededStep.DeepCopy(), runningStep.DeepCopy()},
		want: want{
			done: false,
		},
	}}
	for _, tc := range testcases {
		t.Logf("%s", tc.desc)
		done, err := NewWorkflow(tc.app, mockApplicator()).ExecuteSteps(context.Background(), "app-v1", tc.steps)
		if err != nil {
			assert.Equal(t, tc.want.err, err)
			continue
		}
		assert.Equal(t, tc.want.done, done)
	}
}

type testmockApplicator struct {
}

func (t *testmockApplicator) Apply(ctx context.Context, object runtime.Object, option ...apply.ApplyOption) error {
	return nil
}

func mockApplicator() apply.Applicator {
	return &testmockApplicator{}
}
