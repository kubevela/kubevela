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
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	monitorContext "github.com/kubevela/pkg/monitor/context"
	"github.com/kubevela/pkg/util/singleton"

	"github.com/oam-dev/kubevela/apis/core.oam.dev/common"
	oamcore "github.com/oam-dev/kubevela/apis/core.oam.dev/v1beta1"
)

// setupVFRestartFakeClient seeds both the controller-runtime reconciler client
// (for Application Status updates) and the singleton KubeClient (for ConfigMap
// reads inside computeValuesFromContentFingerprint).
func setupVFRestartFakeClient(t *testing.T, app *oamcore.Application, cm *corev1.ConfigMap) *Reconciler {
	t.Helper()

	// Singleton client — only needs core types (ConfigMap/Secret reads).
	coreScheme := runtime.NewScheme()
	if err := clientgoscheme.AddToScheme(coreScheme); err != nil {
		t.Fatalf("add core scheme: %v", err)
	}
	singletonClient := fake.NewClientBuilder().WithScheme(coreScheme).WithObjects(cm).Build()
	t.Cleanup(func() { singleton.KubeClient.Set(nil) })
	singleton.KubeClient.Set(singletonClient)

	// Reconciler client — needs OAM types for Status().Update on Application.
	oamScheme := runtime.NewScheme()
	if err := corev1.AddToScheme(oamScheme); err != nil {
		t.Fatalf("add corev1 scheme: %v", err)
	}
	if err := oamcore.AddToScheme(oamScheme); err != nil {
		t.Fatalf("add oamcore scheme: %v", err)
	}
	reconcilerClient := fake.NewClientBuilder().
		WithScheme(oamScheme).
		WithStatusSubresource(&oamcore.Application{}).
		WithObjects(app).
		Build()

	return &Reconciler{Client: reconcilerClient, Scheme: oamScheme}
}

// TestCheckWorkflowRestart_ScheduledRestart_IncludesVFFingerprint is the
// regression test for the bug fixed in this PR: the scheduled-restart path in
// checkWorkflowRestart was setting AppRevision to the bare ApplicationRevision
// name (e.g. "demo-v1") without the "-vf-<fingerprint>" suffix.  On the next
// reconcile the revision-based gate would compute "demo-v1-vf-<fp>", see a
// mismatch, and fire a second spurious restart — causing every annotation-based
// restart to execute the workflow twice.
func TestCheckWorkflowRestart_ScheduledRestart_IncludesVFFingerprint(t *testing.T) {
	const ns = "default"
	cm := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{Name: "my-values", Namespace: ns},
		Data:       map[string]string{"values.yaml": "replicaCount: 2\n"},
	}

	app := newAppWithCMValuesFrom(t, "demo", ns, "my-values", "")
	app.Status = common.AppStatus{
		WorkflowRestartScheduledAt: &metav1.Time{Time: time.Now().Add(-1 * time.Minute)},
		Workflow: &common.WorkflowStatus{
			AppRevision: "demo-v1-vf-oldfingerprint12345678901234",
			Finished:    true,
			EndTime:     metav1.Time{Time: time.Now().Add(-10 * time.Minute)},
		},
	}

	r := setupVFRestartFakeClient(t, app, cm)

	handler := &AppHandler{
		currentAppRev: &oamcore.ApplicationRevision{
			ObjectMeta: metav1.ObjectMeta{Name: "demo-v1"},
		},
		latestAppRev: &oamcore.ApplicationRevision{
			ObjectMeta: metav1.ObjectMeta{Name: "demo-v1"},
			Status: oamcore.ApplicationRevisionStatus{
				Workflow: &common.WorkflowStatus{},
			},
		},
	}

	logCtx := monitorContext.NewTraceContext(context.Background(), "")
	r.checkWorkflowRestart(logCtx, app, handler)

	if app.Status.WorkflowRestartScheduledAt != nil {
		t.Fatal("expected WorkflowRestartScheduledAt to be cleared after scheduled restart fires")
	}
	got := app.Status.Workflow.AppRevision
	if !strings.HasPrefix(got, "demo-v1"+valuesFromSuffixSeparator) {
		t.Fatalf("AppRevision should start with %q, got %q — scheduled restart dropped the fingerprint suffix",
			"demo-v1"+valuesFromSuffixSeparator, got)
	}
	suffix := strings.TrimPrefix(got, "demo-v1"+valuesFromSuffixSeparator)
	if len(suffix) != valuesFromSuffixHexLen {
		t.Fatalf("fingerprint suffix should be %d hex chars, got %d (%q)",
			valuesFromSuffixHexLen, len(suffix), suffix)
	}
}

// TestCheckWorkflowRestart_ScheduledRestart_NoVF_NoSuffix ensures that apps
// without helmchart+valuesFrom components are unaffected: the AppRevision token
// remains the bare revision name with no "-vf-" suffix.
func TestCheckWorkflowRestart_ScheduledRestart_NoVF_NoSuffix(t *testing.T) {
	const ns = "default"

	// App has no helmchart components — fingerprint returns "".
	app := &oamcore.Application{
		ObjectMeta: metav1.ObjectMeta{Name: "plain-app", Namespace: ns},
		Spec: oamcore.ApplicationSpec{
			Components: []common.ApplicationComponent{
				{Name: "web", Type: "webservice"},
			},
		},
		Status: common.AppStatus{
			WorkflowRestartScheduledAt: &metav1.Time{Time: time.Now().Add(-1 * time.Minute)},
			Workflow: &common.WorkflowStatus{
				AppRevision: "plain-app-v1",
				Finished:    true,
				EndTime:     metav1.Time{Time: time.Now().Add(-10 * time.Minute)},
			},
		},
	}

	// Singleton client is unused for non-helmchart apps but must be set to a
	// non-nil value so singleton.KubeClient.Get() doesn't panic.
	coreScheme := runtime.NewScheme()
	if err := clientgoscheme.AddToScheme(coreScheme); err != nil {
		t.Fatalf("add core scheme: %v", err)
	}
	singleton.KubeClient.Set(fake.NewClientBuilder().WithScheme(coreScheme).Build())
	t.Cleanup(func() { singleton.KubeClient.Set(nil) })

	oamScheme := runtime.NewScheme()
	if err := corev1.AddToScheme(oamScheme); err != nil {
		t.Fatalf("add corev1 scheme: %v", err)
	}
	if err := oamcore.AddToScheme(oamScheme); err != nil {
		t.Fatalf("add oamcore scheme: %v", err)
	}
	reconcilerClient := fake.NewClientBuilder().
		WithScheme(oamScheme).
		WithStatusSubresource(&oamcore.Application{}).
		WithObjects(app).
		Build()
	r := &Reconciler{Client: reconcilerClient, Scheme: oamScheme}

	handler := &AppHandler{
		currentAppRev: &oamcore.ApplicationRevision{
			ObjectMeta: metav1.ObjectMeta{Name: "plain-app-v1"},
		},
		latestAppRev: &oamcore.ApplicationRevision{
			ObjectMeta: metav1.ObjectMeta{Name: "plain-app-v1"},
			Status:     oamcore.ApplicationRevisionStatus{Workflow: &common.WorkflowStatus{}},
		},
	}

	logCtx := monitorContext.NewTraceContext(context.Background(), "")
	r.checkWorkflowRestart(logCtx, app, handler)

	got := app.Status.Workflow.AppRevision
	if strings.Contains(got, valuesFromSuffixSeparator) {
		t.Fatalf("non-helmchart app should have no fingerprint suffix in AppRevision, got %q", got)
	}
	if got != "plain-app-v1" {
		t.Fatalf("expected AppRevision %q, got %q", "plain-app-v1", got)
	}
}
