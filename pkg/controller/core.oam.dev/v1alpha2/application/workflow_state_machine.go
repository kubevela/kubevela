package application

import (
	"context"
	"fmt"

	"github.com/crossplane/crossplane-runtime/pkg/event"
	"k8s.io/klog/v2"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	"github.com/oam-dev/kubevela/apis/core.oam.dev/common"
	"github.com/oam-dev/kubevela/apis/core.oam.dev/condition"
	"github.com/oam-dev/kubevela/apis/core.oam.dev/v1beta1"
	velatypes "github.com/oam-dev/kubevela/apis/types"
	"github.com/oam-dev/kubevela/pkg/multicluster"
	"github.com/oam-dev/kubevela/pkg/workflow"
)

// workflowStateMachine run workflow state and always end the round of Reconcile loop.
// the only case is go to the normal status update
func (r *Reconciler) workflowStateMachine(ctx context.Context, app *v1beta1.Application, handler *AppHandler, workflowState common.WorkflowState, wf workflow.Workflow) (bool, ctrl.Result, error) {
	var endloop = true
	fmt.Println("XXXXXXXXXXXXX Getting into workflow state machine", app.Status.Phase, "=>", workflowState)
	handler.addServiceStatus(false, app.Status.Services...)
	fmt.Println("YYYYYYYYYYYYY old status", app.Status.Services)
	fmt.Println("YYYYYYYYYYYYY added status", handler.services)
	fmt.Println(handler.appliedResources)
	app.Status.AppliedResources = handler.appliedResources
	app.Status.Services = handler.services
	switch workflowState {
	case common.WorkflowStateSuspended:
		return endloop, ctrl.Result{}, r.patchStatusWithRetryOnConflict(ctx, app, common.ApplicationWorkflowSuspending)
	case common.WorkflowStateTerminated:
		if err := r.doWorkflowFinish(app, wf); err != nil {
			res, err := r.endWithNegativeConditionWithRetry(ctx, app, condition.ErrorCondition("DoWorkflowFinish", err), common.ApplicationRunningWorkflow)
			return endloop, res, err
		}
		return endloop, ctrl.Result{}, r.patchStatusWithRetryOnConflict(ctx, app, common.ApplicationWorkflowTerminated)
	case common.WorkflowStateExecuting:
		return endloop, reconcile.Result{RequeueAfter: baseWorkflowBackoffWaitTime}, r.patchStatusWithRetryOnConflict(ctx, app, common.ApplicationRunningWorkflow)
	case common.WorkflowStateSucceeded:
		wfStatus := app.Status.Workflow
		if wfStatus != nil {
			ref, err := handler.DispatchAndGC(ctx)
			if err == nil {
				err = multicluster.GarbageCollectionForOutdatedResourcesInSubClusters(ctx, app, func(c context.Context) error {
					_, e := handler.DispatchAndGC(c)
					return e
				})
			}
			if err != nil {
				klog.ErrorS(err, "Failed to gc after workflow",
					"application", klog.KObj(app))
				r.Recorder.Event(app, event.Warning(velatypes.ReasonFailedGC, err))
				res, err := r.endWithNegativeConditionWithRetry(ctx, app, condition.ErrorCondition("GCAfterWorkflow", err), common.ApplicationRunningWorkflow)
				return endloop, res, err
			}
			app.Status.ResourceTracker = ref
		}
		if err := r.doWorkflowFinish(app, wf); err != nil {
			res, err := r.endWithNegativeConditionWithRetry(ctx, app, condition.ErrorCondition("DoWorkflowFinish", err), common.ApplicationRunningWorkflow)
			return endloop, res, err
		}
		app.Status.SetConditions(condition.ReadyCondition("WorkflowFinished"))
		r.Recorder.Event(app, event.Normal(velatypes.ReasonApplied, velatypes.MessageWorkflowFinished))
		klog.Info("Application manifests has applied by workflow successfully", "application", klog.KObj(app))
		return endloop, ctrl.Result{}, r.patchStatusWithRetryOnConflict(ctx, app, common.ApplicationWorkflowFinished)
	case common.WorkflowStateFinished:
		if status := app.Status.Workflow; status != nil && status.Terminated {
			return endloop, ctrl.Result{}, nil
		}
	}

	fmt.Println("YYYYYYYYYY continue to another state", workflowState)

	return false, ctrl.Result{}, nil
}
