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

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/oam-dev/kubevela/apis/core.oam.dev/v1beta1"
	"github.com/oam-dev/kubevela/pkg/oam"
)

func TestDetermineApplicationReconcileReason(t *testing.T) {
	now := metav1.Now()

	tests := map[string]struct {
		app  *v1beta1.Application
		want string
	}{
		"unknown for nil application": {
			want: reconcileReasonUnknown,
		},
		"delete for deleting application": {
			app: &v1beta1.Application{
				ObjectMeta: metav1.ObjectMeta{
					DeletionTimestamp: &now,
				},
			},
			want: reconcileReasonDelete,
		},
		"workflow restart annotation": {
			app: &v1beta1.Application{
				ObjectMeta: metav1.ObjectMeta{
					Annotations: map[string]string{
						oam.AnnotationWorkflowRestart: "true",
					},
				},
			},
			want: reconcileReasonWorkflowRestart,
		},
		"unknown by default": {
			app:  &v1beta1.Application{},
			want: reconcileReasonUnknown,
		},
	}

	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			if got := determineApplicationReconcileReason(tt.app); got != tt.want {
				t.Fatalf("got reconcile reason %q, want %q", got, tt.want)
			}
		})
	}
}
