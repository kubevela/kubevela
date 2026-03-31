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

package application

import (
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/oam-dev/kubevela/apis/core.oam.dev/v1beta1"
	common2 "github.com/oam-dev/kubevela/pkg/controller/common"
	"github.com/oam-dev/kubevela/pkg/oam"
)

func TestReconcileResultForApp(t *testing.T) {
	globalDefault := common2.ApplicationReSyncPeriod

	tests := []struct {
		name            string
		annotations     map[string]string
		explicitRequeue time.Duration
		err             error
		wantRequeue     time.Duration
		wantErr         bool
	}{
		{
			name:        "no annotation uses global default",
			annotations: nil,
			wantRequeue: globalDefault,
		},
		{
			name:        "valid annotation overrides global default",
			annotations: map[string]string{oam.AnnotationReconcileInterval: "1m"},
			wantRequeue: 1 * time.Minute,
		},
		{
			name:        "valid annotation with 15m",
			annotations: map[string]string{oam.AnnotationReconcileInterval: "15m"},
			wantRequeue: 15 * time.Minute,
		},
		{
			name:        "valid annotation with 30s",
			annotations: map[string]string{oam.AnnotationReconcileInterval: "30s"},
			wantRequeue: 30 * time.Second,
		},
		{
			name:        "annotation below minimum floor falls back to global default",
			annotations: map[string]string{oam.AnnotationReconcileInterval: "5s"},
			wantRequeue: globalDefault,
		},
		{
			name:        "annotation exactly at minimum floor is accepted",
			annotations: map[string]string{oam.AnnotationReconcileInterval: "10s"},
			wantRequeue: 10 * time.Second,
		},
		{
			name:        "invalid annotation value falls back to global default",
			annotations: map[string]string{oam.AnnotationReconcileInterval: "not-a-duration"},
			wantRequeue: globalDefault,
		},
		{
			name:        "empty annotation value falls back to global default",
			annotations: map[string]string{oam.AnnotationReconcileInterval: ""},
			wantRequeue: globalDefault,
		},
		{
			name:        "negative duration falls back to global default",
			annotations: map[string]string{oam.AnnotationReconcileInterval: "-5m"},
			wantRequeue: globalDefault,
		},
		{
			name:        "zero duration falls back to global default",
			annotations: map[string]string{oam.AnnotationReconcileInterval: "0s"},
			wantRequeue: globalDefault,
		},
		{
			name:            "explicit requeue takes precedence over annotation",
			annotations:     map[string]string{oam.AnnotationReconcileInterval: "1m"},
			explicitRequeue: 3 * time.Second,
			wantRequeue:     3 * time.Second,
		},
		{
			name:    "error path skips requeue duration",
			err:     fmt.Errorf("some error"),
			wantErr: true,
		},
		{
			name:        "other annotations do not interfere",
			annotations: map[string]string{"app.oam.dev/other": "value", oam.AnnotationReconcileInterval: "2m"},
			wantRequeue: 2 * time.Minute,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			app := &v1beta1.Application{
				ObjectMeta: metav1.ObjectMeta{
					Name:        "test-app",
					Namespace:   "default",
					Annotations: tt.annotations,
				},
			}

			r := &Reconciler{}
			res := r.result(tt.err)
			if tt.explicitRequeue > 0 {
				res.requeue(tt.explicitRequeue)
			}
			res.forApp(app)

			result, err := res.ret()

			if tt.wantErr {
				assert.Error(t, err)
				assert.Equal(t, time.Duration(0), result.RequeueAfter)
			} else {
				assert.NoError(t, err)
				assert.Equal(t, tt.wantRequeue, result.RequeueAfter)
			}
		})
	}
}

func TestReconcileResultForAppNilApp(t *testing.T) {
	r := &Reconciler{}
	res := r.result(nil).forApp(nil)
	result, err := res.ret()

	assert.NoError(t, err)
	assert.Equal(t, common2.ApplicationReSyncPeriod, result.RequeueAfter)
}

func TestReconcileResultWithoutForApp(t *testing.T) {
	r := &Reconciler{}
	result, err := r.result(nil).ret()

	assert.NoError(t, err)
	assert.Equal(t, common2.ApplicationReSyncPeriod, result.RequeueAfter)
}
