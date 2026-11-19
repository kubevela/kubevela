/*
 Copyright 2026. The KubeVela Authors.

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

// Package operation reconciles core.oam.dev/v2alpha1 Operation, part of the
// Operations KEP implementation (KEP 2.15).
//
// TODO(KEP 2.15): this controller performs no admission checks yet. Any
// RBAC principal able to create an Operation can invoke any
// OperationTemplate against any source in its namespace -- the
// two SubjectAccessReviews the KEP requires are not implemented. Do not
// run outside a disposable namespace against non-destructive templates.
package operation

import (
	"context"
	"fmt"
	"time"

	"github.com/crossplane/crossplane-runtime/v2/pkg/event"
	kerrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"

	monitorContext "github.com/kubevela/pkg/monitor/context"
	workflowv1alpha1 "github.com/kubevela/workflow/api/v1alpha1"
	"github.com/kubevela/workflow/pkg/executor"
	wfgenerator "github.com/kubevela/workflow/pkg/generator"
	"github.com/kubevela/workflow/pkg/providers"
	"github.com/kubevela/workflow/pkg/tasks/template"
	wfTypes "github.com/kubevela/workflow/pkg/types"

	"github.com/oam-dev/kubevela/apis/core.oam.dev/v2alpha1"
	core "github.com/oam-dev/kubevela/pkg/controller/core.oam.dev"
)

// Reconciler reconciles an Operation object.
type Reconciler struct {
	client.Client
	// APIReader bypasses the manager's cache, so Reconcile's initial Get
	// can't read a stale op.Status right after its own status write.
	APIReader client.Reader
	Scheme    *runtime.Scheme
	Recorder  event.Recorder
	// DefaultOperationTTL is how long a terminal Operation is kept before
	// being deleted, when the Operation itself sets no
	// spec.ttlSecondsAfterFinished. Zero disables the default (no automatic
	// deletion), threaded from core.Args.DefaultOperationTTLSeconds.
	DefaultOperationTTL time.Duration
}

// +kubebuilder:rbac:groups=core.oam.dev,resources=operations,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=core.oam.dev,resources=operations/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=core.oam.dev,resources=operationtemplates,verbs=get;list;watch

// Reconcile runs an Operation's workflow to completion. A terminal Operation
// is only reprocessed to enforce its TTL (see sweepTTL) -- restart is
// triggered by the CLI patching .status directly via
// pkg/workflow/operation, which the next Reconcile just observes like any
// other status change.
func (r *Reconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logCtx := monitorContext.NewTraceContext(ctx, "").AddTag("operation", req.String(), "controller", "operation")
	logCtx.Info("start reconcile operation")
	defer logCtx.Commit("end reconcile operation")

	op := new(v2alpha1.Operation)
	if err := r.APIReader.Get(logCtx, req.NamespacedName, op); err != nil {
		if !kerrors.IsNotFound(err) {
			logCtx.Error(err, "get operation")
		}
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	if op.IsTerminal() {
		return r.sweepTTL(logCtx, op)
	}

	if len(op.Spec.Clusters) > 1 || (len(op.Spec.Clusters) == 1 && op.Spec.Clusters[0] != localCluster) {
		return r.fail(logCtx, op, fmt.Errorf("spec.clusters only supports %q so far, got %v", localCluster, op.Spec.Clusters))
	}

	// Template is resolved before source: "is a source required" is a fact
	// about the template's scope, and the template isn't known yet on a
	// fresh Operation, so source resolution can't tell a legitimate nil
	// (None scope) from a genuine error (Component/Application scope)
	// without it. On a resumed reconcile (restart/resume), Status.Template
	// is already snapshotted and resolveTemplate is skipped entirely --
	// scope comes from that snapshot instead.
	if op.Status.Template == nil {
		tmpl, err := r.resolveTemplate(logCtx, op)
		if err != nil {
			return r.fail(logCtx, op, err)
		}
		// Persisted below, with Workflows/Phase, in a single write.
		op.Status.Template = &tmpl.Spec
		op.Status.Phase = v2alpha1.OperationPhaseRunning
		op.Status.StartTime = metav1.Now()
		op.Status.Attempts = 1
	}

	source, err := r.resolveSource(logCtx, op, op.Status.Template)
	if err != nil {
		return r.fail(logCtx, op, err)
	}

	pCtx, err := buildProcessContext(logCtx, op, source)
	if err != nil {
		return r.fail(logCtx, op, err)
	}

	instance := buildWorkflowInstance(op)
	executor.InitializeWorkflowInstance(instance)

	runners, err := wfgenerator.GenerateRunners(logCtx, instance, wfTypes.StepGeneratorOptions{
		Compiler:       providers.DefaultCompiler.Get(),
		ProcessCtx:     pCtx,
		TemplateLoader: template.NewWorkflowStepTemplateLoader(),
	})
	if err != nil {
		return r.fail(logCtx, op, err)
	}

	phase, execErr := executor.New(instance).ExecuteRunners(logCtx, runners)
	if execErr != nil {
		logCtx.Error(execErr, "execute operation workflow")
	}

	var previousWorkflow v2alpha1.OperationWorkflowStatus
	if len(op.Status.Workflows) > 0 {
		previousWorkflow = op.Status.Workflows[0]
	}
	op.Status.Workflows = []v2alpha1.OperationWorkflowStatus{
		operationWorkflowStatusFromEngine(localCluster, instance.Status, previousWorkflow),
	}

	switch phase {
	case workflowv1alpha1.WorkflowStateSucceeded:
		op.Status.Phase = v2alpha1.OperationPhaseSucceeded
		now := metav1.Now()
		op.Status.CompletionTime = &now
	case workflowv1alpha1.WorkflowStateFailed, workflowv1alpha1.WorkflowStateTerminated:
		op.Status.Phase = v2alpha1.OperationPhaseFailed
		now := metav1.Now()
		op.Status.CompletionTime = &now
		if execErr != nil {
			op.Status.Message = execErr.Error()
		}
	case workflowv1alpha1.WorkflowStateSuspending:
		op.Status.Phase = v2alpha1.OperationPhaseSuspended
	default:
		op.Status.Phase = v2alpha1.OperationPhaseRunning
	}

	if op.IsTerminal() {
		return r.finish(logCtx, op)
	}
	if err := r.Status().Update(logCtx, op); err != nil {
		return ctrl.Result{}, err
	}
	if op.Status.Phase == v2alpha1.OperationPhaseSuspended {
		// Non-terminal: the concurrency lease keeps renewing, but there's no
		// point re-running ExecuteRunners every 5s while nothing can
		// progress -- a `vela operation resume` is what moves this forward.
		return ctrl.Result{RequeueAfter: suspendedRequeueInterval}, nil
	}
	return ctrl.Result{RequeueAfter: 5 * time.Second}, nil
}

// suspendedRequeueInterval is how often a Suspended Operation is
// reconciled purely to renew its target lock. Backoff strategy (fixed vs.
// exponential) is an open question.
const suspendedRequeueInterval = 30 * time.Second

// finish persists op's already-set terminal status, then releases its
// target lock -- only once the write succeeds, so a failed write can't
// drop the lock before the terminal state is durable. The returned
// ctrl.Result comes from sweepTTL, so a terminal Operation still gets
// requeued to enforce its TTL rather than going fully idle.
func (r *Reconciler) finish(ctx context.Context, op *v2alpha1.Operation) (ctrl.Result, error) {
	if err := r.Status().Update(ctx, op); err != nil {
		return ctrl.Result{}, err
	}
	return r.sweepTTL(ctx, op)
}

// fail marks the Operation Failed before any workflow step ran (template or
// source resolution failure) and persists the status.
func (r *Reconciler) fail(ctx context.Context, op *v2alpha1.Operation, cause error) (ctrl.Result, error) {
	if lg, ok := ctx.(monitorContext.Context); ok {
		lg.Error(cause, "operation failed before workflow execution")
	}
	op.Status.Phase = v2alpha1.OperationPhaseFailed
	op.Status.Message = cause.Error()
	if op.Status.StartTime.IsZero() {
		op.Status.StartTime = metav1.Now()
	}
	now := metav1.Now()
	op.Status.CompletionTime = &now
	return r.finish(ctx, op)
}

// SetupWithManager sets up the controller with the Manager.
func (r *Reconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&v2alpha1.Operation{}).
		Complete(r)
}

// Setup adds the Operation Reconciler to Manager.
func Setup(mgr ctrl.Manager, args core.Args) error {
	if args.DefaultOperationTTLSeconds < 0 {
		return fmt.Errorf("--default-operation-ttl-seconds must be >= 0, got %d", args.DefaultOperationTTLSeconds)
	}
	r := Reconciler{
		Client:              mgr.GetClient(),
		APIReader:           mgr.GetAPIReader(),
		Scheme:              mgr.GetScheme(),
		Recorder:            event.NewAPIRecorder(mgr.GetEventRecorderFor("Operation")),
		DefaultOperationTTL: time.Duration(args.DefaultOperationTTLSeconds) * time.Second,
	}
	return r.SetupWithManager(mgr)
}
