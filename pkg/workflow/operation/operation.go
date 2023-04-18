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

package operation

import (
	"context"
	"fmt"
	"io"

	"github.com/pkg/errors"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/util/retry"
	"sigs.k8s.io/controller-runtime/pkg/client"

	workflowv1alpha1 "github.com/kubevela/workflow/api/v1alpha1"
	wfTypes "github.com/kubevela/workflow/pkg/types"
	wfUtils "github.com/kubevela/workflow/pkg/utils"

	"github.com/oam-dev/kubevela/apis/core.oam.dev/common"
	"github.com/oam-dev/kubevela/apis/core.oam.dev/v1beta1"
	"github.com/oam-dev/kubevela/pkg/appfile"
	"github.com/oam-dev/kubevela/pkg/controller/core.oam.dev/v1alpha2/application"
	"github.com/oam-dev/kubevela/pkg/controller/utils"
	"github.com/oam-dev/kubevela/pkg/oam"
	"github.com/oam-dev/kubevela/pkg/resourcetracker"
	"github.com/oam-dev/kubevela/pkg/rollout"
	errors3 "github.com/oam-dev/kubevela/pkg/utils/errors"
)

// NewApplicationWorkflowOperator get an workflow operator with k8sClient, ioWriter(optional, useful for cli) and application
func NewApplicationWorkflowOperator(cli client.Client, w io.Writer, app *v1beta1.Application) wfUtils.WorkflowOperator {
	return appWorkflowOperator{
		cli:          cli,
		outputWriter: w,
		application:  app,
	}
}

// NewApplicationWorkflowStepOperator get an workflow step operator with k8sClient, ioWriter(optional, useful for cli) and application
func NewApplicationWorkflowStepOperator(cli client.Client, w io.Writer, app *v1beta1.Application) wfUtils.WorkflowStepOperator {
	return appWorkflowStepOperator{
		cli:          cli,
		outputWriter: w,
		application:  app,
	}
}

type appWorkflowOperator struct {
	cli          client.Client
	outputWriter io.Writer
	application  *v1beta1.Application
}

type appWorkflowStepOperator struct {
	cli          client.Client
	outputWriter io.Writer
	application  *v1beta1.Application
}

// Suspend a running workflow
func (wo appWorkflowOperator) Suspend(ctx context.Context) error {
	app := wo.application
	if app.Status.Workflow == nil {
		return fmt.Errorf("the workflow in application is not running")
	}
	var err error
	if err = rollout.SuspendRollout(ctx, wo.cli, app, wo.outputWriter); err != nil {
		return err
	}
	if err := SuspendWorkflow(ctx, wo.cli, app, ""); err != nil {
		return err
	}
	return writeOutputF(wo.outputWriter, "Successfully suspend workflow: %s\n", app.Name)
}

// Suspend a suspending workflow
func (wo appWorkflowStepOperator) Suspend(ctx context.Context, step string) error {
	if step == "" {
		return fmt.Errorf("step can not be empty")
	}
	app := wo.application
	if app.Status.Workflow == nil {
		return fmt.Errorf("the workflow in application is not running")
	}
	if app.Status.Workflow.Terminated {
		return fmt.Errorf("can not suspend a terminated workflow")
	}

	if err := SuspendWorkflow(ctx, wo.cli, app, step); err != nil {
		return err
	}
	return writeOutputF(wo.outputWriter, "Successfully suspend workflow %s from step %s \n", app.Name, step)
}

// SuspendWorkflow suspend workflow
func SuspendWorkflow(ctx context.Context, kubecli client.Client, app *v1beta1.Application, stepName string) error {
	app.Status.Workflow.Suspend = true
	steps := app.Status.Workflow.Steps
	found := stepName == ""

	for i, step := range steps {
		for j, sub := range step.SubStepsStatus {
			if sub.Phase != workflowv1alpha1.WorkflowStepPhaseRunning {
				continue
			}
			if stepName == "" {
				wfUtils.OperateSteps(steps, i, j, workflowv1alpha1.WorkflowStepPhaseSuspending)
			} else if stepName == sub.Name {
				wfUtils.OperateSteps(steps, i, j, workflowv1alpha1.WorkflowStepPhaseSuspending)
				found = true
				break
			}
		}
		if step.Phase != workflowv1alpha1.WorkflowStepPhaseRunning {
			continue
		}
		if stepName == "" {
			wfUtils.OperateSteps(steps, i, -1, workflowv1alpha1.WorkflowStepPhaseSuspending)
		} else if stepName == step.Name {
			wfUtils.OperateSteps(steps, i, -1, workflowv1alpha1.WorkflowStepPhaseSuspending)
			found = true
			break
		}
	}
	if !found {
		return fmt.Errorf("can not find step %s", stepName)
	}
	if err := retry.RetryOnConflict(retry.DefaultBackoff, func() error {
		return kubecli.Status().Patch(ctx, app, client.Merge)
	}); err != nil {
		return err
	}
	return nil
}

// Resume a suspending workflow
func (wo appWorkflowOperator) Resume(ctx context.Context) error {
	app := wo.application
	if app.Status.Workflow == nil {
		return fmt.Errorf("the workflow in application is not running")
	}
	if app.Status.Workflow.Terminated {
		return fmt.Errorf("can not resume a terminated workflow")
	}

	var rolloutResumed bool
	var err error

	if rolloutResumed, err = rollout.ResumeRollout(ctx, wo.cli, app, wo.outputWriter); err != nil {
		return err
	}
	if !rolloutResumed && !app.Status.Workflow.Suspend {
		return writeOutputF(wo.outputWriter, "workflow %s is not suspended.\n", app.Name)
	}

	if app.Status.Workflow.Suspend {
		if err = ResumeWorkflow(ctx, wo.cli, app, ""); err != nil {
			return err
		}
	}
	return writeOutputF(wo.outputWriter, "Successfully resume workflow: %s\n", app.Name)
}

// Resume a suspending workflow
func (wo appWorkflowStepOperator) Resume(ctx context.Context, step string) error {
	if step == "" {
		return fmt.Errorf("step can not be empty")
	}
	app := wo.application
	if app.Status.Workflow == nil {
		return fmt.Errorf("the workflow in application is not running")
	}
	if app.Status.Workflow.Terminated {
		return fmt.Errorf("can not resume a terminated workflow")
	}

	if !app.Status.Workflow.Suspend {
		return writeOutputF(wo.outputWriter, "workflow %s is not suspended.\n", app.Name)
	}

	if app.Status.Workflow.Suspend {
		if err := ResumeWorkflow(ctx, wo.cli, app, step); err != nil {
			return err
		}
	}
	return writeOutputF(wo.outputWriter, "Successfully resume workflow %s from step %s \n", app.Name, step)
}

// ResumeWorkflow resume workflow
func ResumeWorkflow(ctx context.Context, kubecli client.Client, app *v1beta1.Application, stepName string) error {
	app.Status.Workflow.Suspend = false
	steps := app.Status.Workflow.Steps
	found := stepName == ""

	for i, step := range steps {
		for j, sub := range step.SubStepsStatus {
			if sub.Phase != workflowv1alpha1.WorkflowStepPhaseSuspending {
				continue
			}
			if stepName == "" {
				wfUtils.OperateSteps(steps, i, j, workflowv1alpha1.WorkflowStepPhaseRunning)
			} else if stepName == sub.Name {
				wfUtils.OperateSteps(steps, i, j, workflowv1alpha1.WorkflowStepPhaseRunning)
				found = true
				break
			}
		}
		if step.Phase != workflowv1alpha1.WorkflowStepPhaseSuspending {
			continue
		}
		if stepName == "" {
			wfUtils.OperateSteps(steps, i, -1, workflowv1alpha1.WorkflowStepPhaseRunning)
		} else if stepName == step.Name {
			wfUtils.OperateSteps(steps, i, -1, workflowv1alpha1.WorkflowStepPhaseRunning)
			found = true
			break
		}
	}

	if !found {
		return fmt.Errorf("can not find step %s", stepName)
	}
	if err := kubecli.Status().Patch(ctx, app, client.Merge); err != nil {
		return err
	}
	return nil
}

// Rollback a running in middle state workflow.
// nolint
func (wo appWorkflowOperator) Rollback(ctx context.Context) error {
	app := wo.application
	if app.Status.Workflow != nil && !app.Status.Workflow.Terminated && !app.Status.Workflow.Suspend && !app.Status.Workflow.Finished {
		return fmt.Errorf("can not rollback a running workflow")
	}
	if oam.GetPublishVersion(app) == "" {
		if app.Status.LatestRevision == nil || app.Status.LatestRevision.Name == "" {
			return fmt.Errorf("the latest revision is not set: %s", app.Name)
		}
		// get the last revision
		revision := &v1beta1.ApplicationRevision{}
		if err := wo.cli.Get(ctx, types.NamespacedName{Name: app.Status.LatestRevision.Name, Namespace: app.Namespace}, revision); err != nil {
			return fmt.Errorf("failed to get the latest revision: %w", err)
		}

		app.Spec = revision.Spec.Application.Spec
		if err := wo.cli.Status().Update(ctx, app); err != nil {
			return err
		}

		fmt.Printf("Successfully rollback workflow to the latest revision: %s\n", app.Name)
		return nil
	}

	appRevs, err := application.GetSortedAppRevisions(ctx, wo.cli, app.Name, app.Namespace)
	if err != nil {
		return errors.Wrapf(err, "failed to list revisions for application %s/%s", app.Namespace, app.Name)
	}

	// find succeeded revision to rollback
	var rev *v1beta1.ApplicationRevision
	var outdatedRev []*v1beta1.ApplicationRevision
	for i := range appRevs {
		candidate := appRevs[len(appRevs)-i-1]
		_rev := candidate.DeepCopy()
		if !candidate.Status.Succeeded || oam.GetPublishVersion(_rev) == "" {
			outdatedRev = append(outdatedRev, _rev)
			continue
		}
		rev = _rev
		break
	}
	if rev == nil {
		return errors.Errorf("failed to find previous succeeded revision for application %s/%s", app.Namespace, app.Name)
	}
	publishVersion := oam.GetPublishVersion(rev)
	revisionNumber, err := utils.ExtractRevision(rev.Name)
	if err != nil {
		return errors.Wrapf(err, "failed to extract revision number from revision %s", rev.Name)
	}
	_, currentRT, historyRTs, _, err := resourcetracker.ListApplicationResourceTrackers(ctx, wo.cli, app)
	if err != nil {
		return errors.Wrapf(err, "failed to list resource trackers for application %s/%s", app.Namespace, app.Name)
	}
	var matchRT *v1beta1.ResourceTracker
	for _, rt := range append(historyRTs, currentRT) {
		if rt == nil {
			continue
		}
		labels := rt.GetLabels()
		if labels != nil && labels[oam.LabelAppRevision] == rev.Name {
			matchRT = rt.DeepCopy()
		}
	}
	if matchRT == nil {
		return errors.Errorf("cannot find resource tracker for previous revision %s, unable to rollback", rev.Name)
	}
	if matchRT.DeletionTimestamp != nil {
		return errors.Errorf("previous revision %s is being recycled, unable to rollback", rev.Name)
	}
	err = wo.writeOutput("Find succeeded application revision %s (PublishVersion: %s) to rollback.\n")
	if err != nil {
		return err
	}
	appKey := client.ObjectKeyFromObject(app)
	// rollback application spec and freeze
	controllerRequirement, err := utils.FreezeApplication(ctx, wo.cli, app, func() {
		app.Spec = rev.Spec.Application.Spec
		oam.SetPublishVersion(app, publishVersion)
	})
	if err != nil {
		return errors.Wrapf(err, "failed to rollback application spec to revision %s (PublishVersion: %s)", rev.Name, publishVersion)
	}
	err = writeOutputF(wo.outputWriter, "Application spec rollback successfully.\n")
	if err != nil {
		return err
	}
	// rollback application status
	if err = retry.RetryOnConflict(retry.DefaultBackoff, func() error {
		if err = wo.cli.Get(ctx, appKey, app); err != nil {
			return err
		}
		app.Status.Workflow = rev.Status.Workflow
		app.Status.Services = []common.ApplicationComponentStatus{}
		app.Status.AppliedResources = []common.ClusterObjectReference{}
		for _, rsc := range matchRT.Spec.ManagedResources {
			app.Status.AppliedResources = append(app.Status.AppliedResources, rsc.ClusterObjectReference)
		}
		app.Status.LatestRevision = &common.Revision{
			Name:         rev.Name,
			Revision:     int64(revisionNumber),
			RevisionHash: rev.GetLabels()[oam.LabelAppRevisionHash],
		}
		return wo.cli.Status().Update(ctx, app)
	}); err != nil {
		return errors.Wrapf(err, "failed to rollback application status to revision %s (PublishVersion: %s)", rev.Name, publishVersion)
	}

	err = writeOutputF(wo.outputWriter, "Application status rollback successfully.\n")
	if err != nil {
		return err
	}
	// update resource tracker generation
	matchRTKey := client.ObjectKeyFromObject(matchRT)
	if err = retry.RetryOnConflict(retry.DefaultBackoff, func() error {
		if err = wo.cli.Get(ctx, matchRTKey, matchRT); err != nil {
			return err
		}
		matchRT.Spec.ApplicationGeneration = app.Generation
		return wo.cli.Update(ctx, matchRT)
	}); err != nil {
		return errors.Wrapf(err, "failed to update application generation in resource tracker")
	}

	// unfreeze application
	if err = utils.UnfreezeApplication(ctx, wo.cli, app, nil, controllerRequirement); err != nil {
		return errors.Wrapf(err, "failed to resume application to restart")
	}

	rollback, err := rollout.RollbackRollout(ctx, wo.cli, app, wo.outputWriter)
	if err != nil {
		return err
	}

	if rollback {
		err = writeOutputF(wo.outputWriter, "Successfully rollback app.\n")
		if err != nil {
			return err
		}
	}

	// clean up outdated revisions
	var errs errors3.ErrorList
	for _, _rev := range outdatedRev {
		if err = wo.cli.Delete(ctx, _rev); err != nil {
			errs = append(errs, err)
		}
	}
	if errs.HasError() {
		return errors.Wrapf(errs, "failed to clean up outdated revisions")
	}

	err = writeOutputF(wo.outputWriter, "Application outdated revision cleaned up.\n")
	if err != nil {
		return err
	}
	return nil
}

// Restart a terminated or finished workflow.
func (wo appWorkflowOperator) Restart(ctx context.Context) error {
	app := wo.application
	status := app.Status.Workflow
	if status == nil {
		return fmt.Errorf("the workflow in application is not running")
	}
	// reset the workflow status to restart the workflow
	app.Status.Workflow = nil

	if err := wo.cli.Status().Update(ctx, app); err != nil {
		return err
	}

	return writeOutputF(wo.outputWriter, "Successfully restart workflow: %s\n", app.Name)
}

// Restart a terminated or finished workflow.
func (wo appWorkflowStepOperator) Restart(ctx context.Context, step string) error {
	if step == "" {
		return fmt.Errorf("step can not be empty")
	}
	app := wo.application
	status := app.Status.Workflow
	if status == nil {
		return fmt.Errorf("the workflow in application is not running")
	}
	status.Terminated = false
	status.Suspend = false
	status.Finished = false
	if !status.EndTime.IsZero() {
		status.EndTime = metav1.Time{}
	}
	var cm *corev1.ConfigMap
	if status.ContextBackend != nil {
		if err := wo.cli.Get(ctx, client.ObjectKey{Namespace: app.Namespace, Name: status.ContextBackend.Name}, cm); err != nil {
			return err
		}
	}
	appParser := appfile.NewApplicationParser(wo.cli, nil, nil)
	appFile, err := appParser.GenerateAppFile(ctx, app)
	if err != nil {
		return fmt.Errorf("failed to parse appfile: %w", err)
	}
	stepStatus, cm, err := wfUtils.CleanStatusFromStep(appFile.WorkflowSteps, status.Steps, *appFile.WorkflowMode, cm, step)
	if err != nil {
		return err
	}
	status.Steps = stepStatus
	if err := retry.RetryOnConflict(retry.DefaultBackoff, func() error {
		return wo.cli.Status().Update(ctx, app)
	}); err != nil {
		return err
	}
	if cm != nil {
		if err := retry.RetryOnConflict(retry.DefaultBackoff, func() error {
			return wo.cli.Update(ctx, cm)
		}); err != nil {
			return err
		}
	}
	return writeOutputF(wo.outputWriter, "Successfully restart workflow %s from step %s\n", app.Name, step)
}

func (wo appWorkflowOperator) Terminate(ctx context.Context) error {
	app := wo.application
	if err := TerminateWorkflow(ctx, wo.cli, app); err != nil {
		return err
	}

	return writeOutputF(wo.outputWriter, "Successfully terminate workflow: %s\n", app.Name)
}

// TerminateWorkflow terminate workflow
func TerminateWorkflow(ctx context.Context, kubecli client.Client, app *v1beta1.Application) error {
	// set the workflow terminated to true
	app.Status.Workflow.Terminated = true
	// set the workflow suspend to false
	app.Status.Workflow.Suspend = false
	steps := app.Status.Workflow.Steps
	for i, step := range steps {
		switch step.Phase {
		case workflowv1alpha1.WorkflowStepPhaseFailed:
			if step.Reason != wfTypes.StatusReasonFailedAfterRetries && step.Reason != wfTypes.StatusReasonTimeout {
				steps[i].Reason = wfTypes.StatusReasonTerminate
			}
		case workflowv1alpha1.WorkflowStepPhaseRunning, workflowv1alpha1.WorkflowStepPhaseSuspending:
			steps[i].Phase = workflowv1alpha1.WorkflowStepPhaseFailed
			steps[i].Reason = wfTypes.StatusReasonTerminate
		default:
		}
		for j, sub := range step.SubStepsStatus {
			switch sub.Phase {
			case workflowv1alpha1.WorkflowStepPhaseFailed:
				if sub.Reason != wfTypes.StatusReasonFailedAfterRetries && sub.Reason != wfTypes.StatusReasonTimeout {
					steps[i].SubStepsStatus[j].Reason = wfTypes.StatusReasonTerminate
				}
			case workflowv1alpha1.WorkflowStepPhaseRunning, workflowv1alpha1.WorkflowStepPhaseSuspending:
				steps[i].SubStepsStatus[j].Phase = workflowv1alpha1.WorkflowStepPhaseFailed
				steps[i].SubStepsStatus[j].Reason = wfTypes.StatusReasonTerminate
			default:
			}
		}
	}

	if err := kubecli.Status().Patch(ctx, app, client.Merge); err != nil {
		return err
	}
	return nil
}

func (wo appWorkflowOperator) writeOutput(str string) error {
	if wo.outputWriter == nil {
		return nil
	}
	_, err := wo.outputWriter.Write([]byte(str))
	return err
}

func writeOutputF(outputWriter io.Writer, format string, a ...interface{}) error {
	if outputWriter == nil {
		return nil
	}
	_, err := fmt.Fprintf(outputWriter, format, a...)
	return err
}
