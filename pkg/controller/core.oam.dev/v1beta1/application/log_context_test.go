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
	"testing"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"

	"github.com/oam-dev/kubevela/apis/core.oam.dev/v1beta1"
	"github.com/oam-dev/kubevela/pkg/logging"
	"github.com/oam-dev/kubevela/pkg/oam"
)

func TestApplicationLogContext_UsesAnnotationWhenPresent(t *testing.T) {
	app := &v1beta1.Application{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "my-app",
			Namespace: "default",
			UID:       "app-uid-1",
			Annotations: map[string]string{
				oam.AnnotationTraceID: "fixed-trace-id",
			},
		},
	}
	req := ctrl.Request{NamespacedName: types.NamespacedName{Name: app.Name, Namespace: app.Namespace}}

	logCtx, traceID := ApplicationLogContext(context.Background(), app, req)
	if logCtx == nil {
		t.Fatal("expected non-nil log context")
	}
	if traceID != "fixed-trace-id" {
		t.Fatalf("expected trace ID to be the annotation value, got %q", traceID)
	}
	if got, ok := logging.RequestIDFrom(logCtx); !ok || got != "fixed-trace-id" {
		t.Fatalf("expected request ID to be set on std ctx; got %q ok=%v", got, ok)
	}
}

func TestApplicationLogContext_MintsWhenAnnotationMissing(t *testing.T) {
	app := &v1beta1.Application{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "my-app",
			Namespace: "default",
			UID:       "app-uid-2",
		},
	}
	req := ctrl.Request{NamespacedName: types.NamespacedName{Name: app.Name, Namespace: app.Namespace}}

	logCtx, traceID := ApplicationLogContext(context.Background(), app, req)
	if logCtx == nil {
		t.Fatal("expected non-nil log context")
	}
	if traceID == "" {
		t.Fatalf("expected a freshly minted trace ID, got empty string")
	}
	got, ok := logging.RequestIDFrom(logCtx)
	if !ok || got != traceID {
		t.Fatalf("expected std ctx requestID to equal returned trace ID; got %q ok=%v want %q", got, ok, traceID)
	}
}

func TestApplicationLogContext_DifferentReconcilesMintDifferentTraceIDs(t *testing.T) {
	app := &v1beta1.Application{
		ObjectMeta: metav1.ObjectMeta{Name: "a", Namespace: "ns"},
	}
	req := ctrl.Request{NamespacedName: types.NamespacedName{Name: "a", Namespace: "ns"}}

	_, id1 := ApplicationLogContext(context.Background(), app, req)
	_, id2 := ApplicationLogContext(context.Background(), app, req)
	if id1 == id2 {
		t.Fatalf("expected distinct trace IDs across reconciles without annotation, got %q twice", id1)
	}
}

func TestApplicationLogContext_FreshSpanIDPerReconcile(t *testing.T) {
	// When the trace ID annotation is set, the trace ID should persist
	// across reconciles, but each reconcile gets its own root span ID.
	app := &v1beta1.Application{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "stable-app",
			Namespace: "ns",
			Annotations: map[string]string{
				oam.AnnotationTraceID: "fixed-trace-id",
			},
		},
	}
	req := ctrl.Request{NamespacedName: types.NamespacedName{Name: app.Name, Namespace: app.Namespace}}

	logCtx1, traceID1 := ApplicationLogContext(context.Background(), app, req)
	logCtx2, traceID2 := ApplicationLogContext(context.Background(), app, req)

	if traceID1 != "fixed-trace-id" || traceID2 != "fixed-trace-id" {
		t.Fatalf("trace IDs should both be the annotation value, got %q and %q", traceID1, traceID2)
	}
	if logCtx1.GetID() == logCtx2.GetID() {
		t.Fatalf("each reconcile should get a unique span ID, got %q twice", logCtx1.GetID())
	}
	if logCtx1.GetID() == traceID1 {
		t.Fatalf("reconcile span ID must not equal trace ID; got %q", logCtx1.GetID())
	}
}

func TestDetermineApplicationReconcileReason(t *testing.T) {
	now := metav1.Now()
	tests := []struct {
		name string
		app  *v1beta1.Application
		want string
	}{
		{
			name: "nil app",
			app:  nil,
			want: "unknown",
		},
		{
			name: "delete in progress",
			app:  &v1beta1.Application{ObjectMeta: metav1.ObjectMeta{DeletionTimestamp: &now}},
			want: "delete",
		},
		{
			name: "workflow restart annotation present",
			app: &v1beta1.Application{ObjectMeta: metav1.ObjectMeta{
				Annotations: map[string]string{oam.AnnotationWorkflowRestart: "true"},
			}},
			want: "workflow_restart",
		},
		{
			name: "workflow restart annotation empty value",
			app: &v1beta1.Application{ObjectMeta: metav1.ObjectMeta{
				Annotations: map[string]string{oam.AnnotationWorkflowRestart: ""},
			}},
			want: "unknown",
		},
		{
			name: "plain app",
			app:  &v1beta1.Application{ObjectMeta: metav1.ObjectMeta{Name: "x"}},
			want: "unknown",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := determineApplicationReconcileReason(tc.app); got != tc.want {
				t.Fatalf("reason: want %q, got %q", tc.want, got)
			}
		})
	}
}
