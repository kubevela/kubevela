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
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	"github.com/oam-dev/kubevela/apis/core.oam.dev/common"
	"github.com/oam-dev/kubevela/apis/core.oam.dev/condition"
	"github.com/oam-dev/kubevela/apis/core.oam.dev/v1beta1"
)

// Regression test for kubevela/kubevela#7164: when a reconcile path ends in a
// negative sub-condition (e.g. Parsed=False/ReconcileError), the rollup Ready
// condition must also flip to False. Previously Ready stayed True from the last
// successful reconcile, so health checkers polling Ready missed the failure.
func TestEndWithNegativeConditionFlipsReady(t *testing.T) {
	readyType := condition.ConditionType(common.ReadyCondition.String())

	app := &v1beta1.Application{
		ObjectMeta: metav1.ObjectMeta{Name: "app-7164", Namespace: "default"},
		Status: common.AppStatus{
			ConditionedStatus: condition.ConditionedStatus{
				Conditions: []condition.Condition{{
					Type:               readyType,
					Status:             corev1.ConditionTrue,
					Reason:             condition.ReasonReconcileSuccess,
					LastTransitionTime: metav1.Now(),
				}},
			},
		},
	}

	cli := fake.NewClientBuilder().
		WithScheme(newTestScheme(t)).
		WithStatusSubresource(&v1beta1.Application{}).
		WithObjects(app).
		Build()
	r := &Reconciler{Client: cli}

	parsedFailure := condition.ErrorCondition("Parsed", errors.New("ComponentDefinition not found"))
	_, _ = r.endWithNegativeCondition(context.Background(), app, parsedFailure, common.ApplicationRendering)

	persisted := &v1beta1.Application{}
	if err := cli.Get(context.Background(), types.NamespacedName{Name: app.Name, Namespace: app.Namespace}, persisted); err != nil {
		t.Fatalf("failed to read Application back from storage: %v", err)
	}

	gotReady := findCondition(persisted.Status.Conditions, readyType)
	assert.NotNil(t, gotReady, "Ready condition must be set after negative reconcile")
	assert.Equal(t, corev1.ConditionFalse, gotReady.Status, "Ready must be False on reconcile failure")
	assert.Equal(t, condition.ReasonReconcileError, gotReady.Reason, "Ready reason must be ReconcileError")
	assert.Equal(t, parsedFailure.Message, gotReady.Message, "Ready message must mirror the failing sub-condition's message")
}

func findCondition(conds []condition.Condition, t condition.ConditionType) *condition.Condition {
	for i := range conds {
		if conds[i].Type == t {
			return &conds[i]
		}
	}
	return nil
}
