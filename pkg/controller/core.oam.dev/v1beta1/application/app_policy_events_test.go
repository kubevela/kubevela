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

	"github.com/crossplane/crossplane-runtime/pkg/event"
	"k8s.io/apimachinery/pkg/runtime"

	"github.com/oam-dev/kubevela/apis/core.oam.dev/common"
	"github.com/oam-dev/kubevela/apis/core.oam.dev/v1beta1"
)

// recordingRecorder captures emitted events for assertions.
type recordingRecorder struct {
	events []event.Event
}

func (r *recordingRecorder) Event(_ runtime.Object, e event.Event) {
	r.events = append(r.events, e)
}

func (r *recordingRecorder) WithAnnotations(_ ...string) event.Recorder { return r }

func TestEmitPolicyEvents(t *testing.T) {
	tests := []struct {
		name           string
		policies       []common.AppliedApplicationPolicy
		wantReasons    []string
		wantEventTypes []event.Type
	}{
		{
			name: "applied explicit policy emits PolicyApplied",
			policies: []common.AppliedApplicationPolicy{
				{Name: "my-policy", Applied: true, Source: PolicySourceExplicit},
			},
			wantReasons:    []string{"PolicyApplied"},
			wantEventTypes: []event.Type{event.TypeNormal},
		},
		{
			name: "applied global policy emits GlobalPolicyApplied",
			policies: []common.AppliedApplicationPolicy{
				{Name: "global-policy", Applied: true, Source: PolicySourceGlobal},
			},
			wantReasons:    []string{"GlobalPolicyApplied"},
			wantEventTypes: []event.Type{event.TypeNormal},
		},
		{
			name: "failed explicit policy emits PolicyFailed (not GlobalPolicyFailed)",
			policies: []common.AppliedApplicationPolicy{
				{Name: "bad-policy", Applied: false, Error: true, Message: "render error", Source: PolicySourceExplicit},
			},
			wantReasons:    []string{"PolicyFailed"},
			wantEventTypes: []event.Type{event.TypeWarning},
		},
		{
			name: "failed global policy emits GlobalPolicyFailed",
			policies: []common.AppliedApplicationPolicy{
				{Name: "bad-global", Applied: false, Error: true, Message: "render error", Source: PolicySourceGlobal},
			},
			wantReasons:    []string{"GlobalPolicyFailed"},
			wantEventTypes: []event.Type{event.TypeWarning},
		},
		{
			name: "skipped explicit policy emits PolicySkipped",
			policies: []common.AppliedApplicationPolicy{
				{Name: "skipped-policy", Applied: false, Error: false, Message: "disabled", Source: PolicySourceExplicit},
			},
			wantReasons:    []string{"PolicySkipped"},
			wantEventTypes: []event.Type{event.TypeNormal},
		},
		{
			name: "skipped global policy emits GlobalPolicySkipped",
			policies: []common.AppliedApplicationPolicy{
				{Name: "skipped-global", Applied: false, Error: false, Message: "disabled", Source: PolicySourceGlobal},
			},
			wantReasons:    []string{"GlobalPolicySkipped"},
			wantEventTypes: []event.Type{event.TypeNormal},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rec := &recordingRecorder{}
			r := &Reconciler{Recorder: rec}

			app := &v1beta1.Application{}
			app.Status.AppliedApplicationPolicies = tt.policies

			r.emitPolicyEvents(app)

			if len(rec.events) != len(tt.wantReasons) {
				t.Fatalf("got %d events, want %d", len(rec.events), len(tt.wantReasons))
			}
			for i, e := range rec.events {
				if string(e.Reason) != tt.wantReasons[i] {
					t.Errorf("event[%d] reason: got %q, want %q", i, e.Reason, tt.wantReasons[i])
				}
				if e.Type != tt.wantEventTypes[i] {
					t.Errorf("event[%d] type: got %q, want %q", i, e.Type, tt.wantEventTypes[i])
				}
			}
		})
	}
}
