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
	"sync"
	"testing"
	"time"

	workflowv1alpha1 "github.com/kubevela/workflow/api/v1alpha1"
	"github.com/kubevela/workflow/pkg/executor"
	"github.com/stretchr/testify/require"
	kerrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/client/interceptor"
	ctrlEvent "sigs.k8s.io/controller-runtime/pkg/event"

	"github.com/oam-dev/kubevela/apis/core.oam.dev/common"
	"github.com/oam-dev/kubevela/apis/core.oam.dev/v1beta1"
	"github.com/oam-dev/kubevela/pkg/oam"
)

// countingReader counts the live reads a reconcile makes.
type countingReader struct {
	client.Reader
	gets int
}

func (c *countingReader) Get(ctx context.Context, key client.ObjectKey, obj client.Object, opts ...client.GetOption) error {
	c.gets++
	return c.Reader.Get(ctx, key, obj, opts...)
}

func runsOnceScheme(t *testing.T) *runtime.Scheme {
	scheme := runtime.NewScheme()
	require.NoError(t, v1beta1.AddToScheme(scheme))
	return scheme
}

// A deleted Application leaves nothing behind in the record of own writes.
func TestADeletedApplicationIsForgotten(t *testing.T) {
	cli := fake.NewClientBuilder().WithScheme(runsOnceScheme(t)).Build()
	r := &Reconciler{Client: cli, APIReader: cli, ownWrites: &sync.Map{}}
	key := client.ObjectKey{Namespace: "default", Name: "app"}
	r.ownWrites.Store(key, "7")

	_, err := r.Reconcile(context.Background(), ctrl.Request{NamespacedName: key})
	require.NoError(t, err)
	_, ok := r.ownWrites.Load(key)
	require.False(t, ok)
}

// A reconcile must not start from a cached Application older than the controller's own last
// status write: it would not see the steps that write recorded, and would run them again.
func TestReadOwnWritesUsesTheLiveApplicationWhenTheCacheIsBehind(t *testing.T) {
	live := &v1beta1.Application{ObjectMeta: metav1.ObjectMeta{Name: "shop", Namespace: "default"}}
	live.Status.Workflow = &common.WorkflowStatus{Steps: []workflowv1alpha1.WorkflowStepStatus{{StepStatus: workflowv1alpha1.StepStatus{Name: "notify", Phase: workflowv1alpha1.WorkflowStepPhaseSucceeded}}}}
	cli := fake.NewClientBuilder().WithScheme(runsOnceScheme(t)).WithObjects(live).WithStatusSubresource(live).Build()
	stored := &v1beta1.Application{}
	require.NoError(t, cli.Get(context.Background(), client.ObjectKeyFromObject(live), stored))

	reader := &countingReader{Reader: cli}
	r := &Reconciler{Client: cli, APIReader: reader, ownWrites: &sync.Map{}}
	r.recordOwnWrite(stored)

	cached := stored.DeepCopy()
	cached.ResourceVersion = "1"
	cached.Status.Workflow = nil
	got, err := r.readOwnWrites(context.Background(), cached)
	require.NoError(t, err)
	require.Equal(t, stored.ResourceVersion, got.ResourceVersion)
	require.Len(t, got.Status.Workflow.Steps, 1, "the live Application carries the recorded step")
	require.Equal(t, 1, reader.gets)

	got, err = r.readOwnWrites(context.Background(), stored.DeepCopy())
	require.NoError(t, err)
	require.Equal(t, stored.ResourceVersion, got.ResourceVersion)
	require.Equal(t, 1, reader.gets, "a cache that shows the last write needs no live read")
}

// Another writer changing the Application while a reconcile runs must not lose the reconcile's
// status write, which records the steps that ran.
func TestStatusWriteIsRetriedOnConflict(t *testing.T) {
	app := &v1beta1.Application{ObjectMeta: metav1.ObjectMeta{Name: "shop", Namespace: "default"}}
	conflicted := false
	base := fake.NewClientBuilder().WithScheme(runsOnceScheme(t)).WithObjects(app).WithStatusSubresource(app).
		WithInterceptorFuncs(interceptor.Funcs{
			SubResourcePatch: func(ctx context.Context, c client.Client, sub string, obj client.Object, patch client.Patch, opts ...client.SubResourcePatchOption) error {
				if !conflicted {
					conflicted = true
					// A GitOps sync lands between the reconcile's read and its write.
					other := &v1beta1.Application{}
					if err := c.Get(ctx, client.ObjectKeyFromObject(obj), other); err != nil {
						return err
					}
					other.Annotations = map[string]string{"synced-by": "gitops"}
					if err := c.Update(ctx, other); err != nil {
						return err
					}
					return kerrors.NewConflict(schema.GroupResource{Resource: "applications"}, obj.GetName(), nil)
				}
				return c.SubResource(sub).Patch(ctx, obj, patch, opts...)
			},
		}).Build()

	stale := &v1beta1.Application{}
	require.NoError(t, base.Get(context.Background(), client.ObjectKeyFromObject(app), stale))
	ctx := withOriginalApp(context.Background(), stale.DeepCopy())
	stale.Status.Workflow = &common.WorkflowStatus{Message: "notify sent"}
	r := &Reconciler{Client: base, APIReader: base}
	require.NoError(t, r.patchStatus(ctx, stale, common.ApplicationRunning))

	got := &v1beta1.Application{}
	require.NoError(t, base.Get(context.Background(), client.ObjectKeyFromObject(app), got))
	require.Equal(t, common.ApplicationRunning, got.Status.Phase)
	require.Equal(t, "notify sent", got.Status.Workflow.Message)
	require.Equal(t, "gitops", got.Annotations["synced-by"], "the other writer's change is kept")
}

// A status another writer changed (a workflow resume) is not overwritten by the reconcile's
// older copy: the write fails and the next reconcile starts from the new status.
func TestStatusWriteDoesNotOverwriteAnotherStatusWrite(t *testing.T) {
	app := &v1beta1.Application{ObjectMeta: metav1.ObjectMeta{Name: "shop", Namespace: "default"},
		Status: common.AppStatus{Workflow: &common.WorkflowStatus{Suspend: true}}}
	conflicted := false
	base := fake.NewClientBuilder().WithScheme(runsOnceScheme(t)).WithObjects(app).WithStatusSubresource(app).
		WithInterceptorFuncs(interceptor.Funcs{
			SubResourcePatch: func(ctx context.Context, c client.Client, sub string, obj client.Object, patch client.Patch, opts ...client.SubResourcePatchOption) error {
				if !conflicted {
					conflicted = true
					// The user resumes the workflow between the reconcile's read and its write.
					other := &v1beta1.Application{}
					if err := c.Get(ctx, client.ObjectKeyFromObject(obj), other); err != nil {
						return err
					}
					other.Status.Workflow.Suspend = false
					if err := c.Status().Update(ctx, other); err != nil {
						return err
					}
					return kerrors.NewConflict(schema.GroupResource{Resource: "applications"}, obj.GetName(), nil)
				}
				return c.SubResource(sub).Patch(ctx, obj, patch, opts...)
			},
		}).Build()

	stale := &v1beta1.Application{}
	require.NoError(t, base.Get(context.Background(), client.ObjectKeyFromObject(app), stale))
	ctx := withOriginalApp(context.Background(), stale.DeepCopy())
	stale.Status.Workflow.Message = "waiting for resume"
	r := &Reconciler{Client: base, APIReader: base}
	require.True(t, kerrors.IsConflict(r.patchStatus(ctx, stale, common.ApplicationWorkflowSuspending)))

	got := &v1beta1.Application{}
	require.NoError(t, base.Get(context.Background(), client.ObjectKeyFromObject(app), got))
	require.False(t, got.Status.Workflow.Suspend, "the resume is kept")
}

// A reconcile that has already written its status once still retries a later write that only
// conflicts with a write to the metadata: the status it is compared against is its own last write.
func TestStatusWriteIsRetriedAfterAnEarlierWriteOfTheSameReconcile(t *testing.T) {
	app := &v1beta1.Application{ObjectMeta: metav1.ObjectMeta{Name: "shop", Namespace: "default"}}
	writes := 0
	base := fake.NewClientBuilder().WithScheme(runsOnceScheme(t)).WithObjects(app).WithStatusSubresource(app).
		WithInterceptorFuncs(interceptor.Funcs{
			SubResourcePatch: func(ctx context.Context, c client.Client, sub string, obj client.Object, patch client.Patch, opts ...client.SubResourcePatchOption) error {
				writes++
				if writes == 2 {
					other := &v1beta1.Application{}
					if err := c.Get(ctx, client.ObjectKeyFromObject(obj), other); err != nil {
						return err
					}
					other.Annotations = map[string]string{"synced-by": "gitops"}
					if err := c.Update(ctx, other); err != nil {
						return err
					}
					return kerrors.NewConflict(schema.GroupResource{Resource: "applications"}, obj.GetName(), nil)
				}
				return c.SubResource(sub).Patch(ctx, obj, patch, opts...)
			},
		}).Build()

	current := &v1beta1.Application{}
	require.NoError(t, base.Get(context.Background(), client.ObjectKeyFromObject(app), current))
	ctx := withOriginalApp(context.Background(), current.DeepCopy())
	r := &Reconciler{Client: base, APIReader: base}
	current.Status.Workflow = &common.WorkflowStatus{Message: "step one sent"}
	require.NoError(t, r.patchStatus(ctx, current, common.ApplicationRunningWorkflow))
	current.Status.Workflow.Message = "step two sent"
	require.NoError(t, r.patchStatus(ctx, current, common.ApplicationRunning))

	got := &v1beta1.Application{}
	require.NoError(t, base.Get(context.Background(), client.ObjectKeyFromObject(app), got))
	require.Equal(t, "step two sent", got.Status.Workflow.Message)
	require.Equal(t, "gitops", got.Annotations["synced-by"])
}

// A status the controller writes directly, as scheduling a workflow restart does, becomes the
// status later writes in the same reconcile are compared against.
func TestStatusWriteIsRetriedAfterADirectStatusWrite(t *testing.T) {
	app := &v1beta1.Application{ObjectMeta: metav1.ObjectMeta{Name: "shop", Namespace: "default",
		Annotations: map[string]string{oam.AnnotationWorkflowRestart: "5m"}}}
	patches := 0
	base := fake.NewClientBuilder().WithScheme(runsOnceScheme(t)).WithObjects(app).WithStatusSubresource(app).
		WithInterceptorFuncs(interceptor.Funcs{
			SubResourcePatch: func(ctx context.Context, c client.Client, sub string, obj client.Object, patch client.Patch, opts ...client.SubResourcePatchOption) error {
				patches++
				if patches == 1 {
					other := &v1beta1.Application{}
					if err := c.Get(ctx, client.ObjectKeyFromObject(obj), other); err != nil {
						return err
					}
					other.Labels = map[string]string{"synced-by": "gitops"}
					if err := c.Update(ctx, other); err != nil {
						return err
					}
					return kerrors.NewConflict(schema.GroupResource{Resource: "applications"}, obj.GetName(), nil)
				}
				return c.SubResource(sub).Patch(ctx, obj, patch, opts...)
			},
		}).Build()

	current := &v1beta1.Application{}
	require.NoError(t, base.Get(context.Background(), client.ObjectKeyFromObject(app), current))
	ctx := withOriginalApp(context.Background(), current.DeepCopy())
	r := &Reconciler{Client: base, APIReader: base}
	r.handleWorkflowRestartAnnotation(ctx, current)
	require.NotNil(t, current.Status.WorkflowRestartScheduledAt)

	current.Status.Workflow = &common.WorkflowStatus{Message: "step sent"}
	require.NoError(t, r.patchStatus(ctx, current, common.ApplicationRunning))

	got := &v1beta1.Application{}
	require.NoError(t, base.Get(context.Background(), client.ObjectKeyFromObject(app), got))
	require.Equal(t, "step sent", got.Status.Workflow.Message)
	require.NotNil(t, got.Status.WorkflowRestartScheduledAt)
}

// A conflict whose live object is another Application of the same name, deleted and created
// again since the reconcile read it, is not written over: the write fails and the next
// reconcile starts from the new one.
func TestStatusWriteIsNotRetriedOntoAnotherApplication(t *testing.T) {
	app := &v1beta1.Application{ObjectMeta: metav1.ObjectMeta{Name: "shop", Namespace: "default", UID: "old-uid"}}
	conflicted := false
	base := fake.NewClientBuilder().WithScheme(runsOnceScheme(t)).WithObjects(app).WithStatusSubresource(app).
		WithInterceptorFuncs(interceptor.Funcs{
			SubResourcePatch: func(ctx context.Context, c client.Client, sub string, obj client.Object, patch client.Patch, opts ...client.SubResourcePatchOption) error {
				if !conflicted {
					conflicted = true
					other := &v1beta1.Application{}
					if err := c.Get(ctx, client.ObjectKeyFromObject(obj), other); err != nil {
						return err
					}
					if err := c.Delete(ctx, other); err != nil {
						return err
					}
					if err := c.Create(ctx, &v1beta1.Application{ObjectMeta: metav1.ObjectMeta{Name: "shop", Namespace: "default", UID: "new-uid"}}); err != nil {
						return err
					}
					return kerrors.NewConflict(schema.GroupResource{Resource: "applications"}, obj.GetName(), nil)
				}
				return c.SubResource(sub).Patch(ctx, obj, patch, opts...)
			},
		}).Build()

	stale := &v1beta1.Application{}
	require.NoError(t, base.Get(context.Background(), client.ObjectKeyFromObject(app), stale))
	ctx := withOriginalApp(context.Background(), stale.DeepCopy())
	stale.Status.Workflow = &common.WorkflowStatus{Message: "step sent"}
	r := &Reconciler{Client: base, APIReader: base}
	require.True(t, kerrors.IsConflict(r.patchStatus(ctx, stale, common.ApplicationRunning)))

	got := &v1beta1.Application{}
	require.NoError(t, base.Get(context.Background(), client.ObjectKeyFromObject(app), got))
	require.Equal(t, types.UID("new-uid"), got.UID)
	require.Nil(t, got.Status.Workflow, "the new Application's status is not the old one's")
}

// vela workflow rollback freezes the Application, restores an earlier spec and releases it. A
// reconcile cut short by the freeze can leave the phase at rendering, and the release only changes
// an annotation, so the release must still be reconciled or the Application waits for the resync.
func TestReleasingAFrozenApplicationIsReconciled(t *testing.T) {
	frozen := &v1beta1.Application{ObjectMeta: metav1.ObjectMeta{Name: "busybox", Namespace: "default", Generation: 4},
		Status: common.AppStatus{Phase: common.ApplicationRendering}}
	oam.SetControllerRequirement(frozen, "Disabled")
	released := frozen.DeepCopy()
	oam.SetControllerRequirement(released, "")
	require.True(t, applicationUpdateNeedsReconcile(ctrlEvent.UpdateEvent{ObjectOld: frozen, ObjectNew: released}))

	statusOnly := frozen.DeepCopy()
	statusOnly.Status.Services = []common.ApplicationComponentStatus{{Name: "busybox"}}
	require.False(t, applicationUpdateNeedsReconcile(ctrlEvent.UpdateEvent{ObjectOld: frozen, ObjectNew: statusOnly}),
		"a status change while rendering is still left to the reconcile that made it")
}

// A status write that fails for a reason other than a conflict sets the step guard back to the
// steps the stored status records, so the next reconcile carries on from it instead of skipping
// until the guard expires.
func TestAFailedStatusWriteSetsTheStepGuardBack(t *testing.T) {
	executor.InitStepStatusCache(context.Background())
	key := "shop-default"
	stored := []workflowv1alpha1.WorkflowStepStatus{{StepStatus: workflowv1alpha1.StepStatus{Name: "warm-up", Phase: workflowv1alpha1.WorkflowStepPhaseSucceeded}}}
	app := &v1beta1.Application{ObjectMeta: metav1.ObjectMeta{Name: "shop", Namespace: "default"},
		Status: common.AppStatus{Workflow: &common.WorkflowStatus{Steps: stored}}}
	denied := fake.NewClientBuilder().WithScheme(runsOnceScheme(t)).WithObjects(app).WithStatusSubresource(app).
		WithInterceptorFuncs(interceptor.Funcs{
			SubResourcePatch: func(ctx context.Context, c client.Client, sub string, obj client.Object, patch client.Patch, opts ...client.SubResourcePatchOption) error {
				return kerrors.NewForbidden(schema.GroupResource{Resource: "applications"}, obj.GetName(), nil)
			},
		}).Build()

	read := &v1beta1.Application{}
	require.NoError(t, denied.Get(context.Background(), client.ObjectKeyFromObject(app), read))
	ctx := withOriginalApp(context.Background(), read.DeepCopy())
	// The engine ran the next step and set the guard to the steps it now holds in memory.
	read.Status.Workflow.Steps = append(read.Status.Workflow.Steps, workflowv1alpha1.WorkflowStepStatus{StepStatus: workflowv1alpha1.StepStatus{Name: "notify", Phase: workflowv1alpha1.WorkflowStepPhaseSucceeded}})
	executor.StepStatusCache.Put(key, len(read.Status.Workflow.Steps), time.Minute)

	r := &Reconciler{Client: denied, APIReader: denied}
	require.True(t, kerrors.IsForbidden(r.patchStatus(ctx, read, common.ApplicationRunningWorkflow)))
	guard, ok := executor.StepStatusCache.Get(key)
	require.True(t, ok)
	require.Equal(t, len(stored), guard, "the guard records the steps the stored status holds")
}
