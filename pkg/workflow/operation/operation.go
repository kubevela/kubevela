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

	"k8s.io/client-go/util/retry"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/oam-dev/kubevela/apis/core.oam.dev/common"
	"github.com/oam-dev/kubevela/apis/core.oam.dev/v1beta1"
	"github.com/oam-dev/kubevela/pkg/apiserver/domain/service"
	"github.com/oam-dev/kubevela/pkg/controller/core.oam.dev/v1alpha2/application"
	"github.com/oam-dev/kubevela/pkg/controller/utils"
	"github.com/oam-dev/kubevela/pkg/oam"
	"github.com/oam-dev/kubevela/pkg/resourcetracker"
	"github.com/oam-dev/kubevela/pkg/rollout"
	errors3 "github.com/oam-dev/kubevela/pkg/utils/errors"

	"github.com/pkg/errors"
)

// WorkflowOperator is opratior handler for workflow's resume/rollback/restart
type WorkflowOperator interface {
	Suspend(ctx context.Context, app *v1beta1.Application) error
	Resume(ctx context.Context, app *v1beta1.Application) error
	Rollback(ctx context.Context, app *v1beta1.Application) error
	Restart(ctx context.Context, app *v1beta1.Application) error
	Terminate(ctx context.Context, app *v1beta1.Application) error
}

// NewWorkflowOperator get an workflow operator with k8sClient and ioWriter(optional, useful for cli)
func NewWorkflowOperator(cli client.Client, w io.Writer) WorkflowOperator {
	return wfOperator{cli: cli, outputWriter: w}
}

type wfOperator struct {
	cli          client.Client
	outputWriter io.Writer
}

// Suspend a running workflow
func (wo wfOperator) Suspend(ctx context.Context, app *v1beta1.Application) error {
	if app.Status.Workflow == nil {
		return fmt.Errorf("the workflow in application is not running")
	}
	var err error
	if err = rollout.SuspendRollout(context.Background(), wo.cli, app, wo.outputWriter); err != nil {
		return err
	}
	appKey := client.ObjectKeyFromObject(app)
	if err := retry.RetryOnConflict(retry.DefaultBackoff, func() error {
		if err := wo.cli.Get(ctx, appKey, app); err != nil {
			return err
		}
		// set the workflow suspend to true
		app.Status.Workflow.Suspend = true
		return wo.cli.Status().Patch(ctx, app, client.Merge)
	}); err != nil {
		return err
	}

	return wo.writeOutputF("Successfully suspend workflow: %s\n", app.Name)
}

// Resume a suspending workflow
func (wo wfOperator) Resume(ctx context.Context, app *v1beta1.Application) error {
	if app.Status.Workflow == nil {
		return fmt.Errorf("the workflow in application is not running")
	}
	if app.Status.Workflow.Terminated {
		return fmt.Errorf("can not resume a terminated workflow")
	}

	var rolloutResumed bool
	var err error

	if rolloutResumed, err = rollout.ResumeRollout(context.Background(), wo.cli, app, wo.outputWriter); err != nil {
		return err
	}
	if !rolloutResumed && !app.Status.Workflow.Suspend {
		return fmt.Errorf("the workflow is not suspending")
	}

	if app.Status.Workflow.Suspend {
		if err = service.ResumeWorkflow(ctx, wo.cli, app); err != nil {
			return err
		}
	}
	return nil
}

// Rollback a running in middle state workflow.
//nolint
func (wo wfOperator) Rollback(ctx context.Context, app *v1beta1.Application) error {
	if oam.GetPublishVersion(app) == "" {
		return fmt.Errorf("app without public version cannot rollback")
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
	err = wo.writeOutput("Application spec rollback successfully.\n")
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

	err = wo.writeOutput("Application status rollback successfully.\n")
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
		err = wo.writeOutput("Successfully rollback rollout")
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

	err = wo.writeOutput("Application outdated revision cleaned up.\n")
	if err != nil {
		return err
	}
	return nil
}

// Restart a terminated or finished workflow.
func (wo wfOperator) Restart(ctx context.Context, app *v1beta1.Application) error {
	if app.Status.Workflow == nil {
		return fmt.Errorf("the workflow in application is not running")
	}
	// reset the workflow status to restart the workflow
	app.Status.Workflow = nil

	if err := wo.cli.Status().Update(context.TODO(), app); err != nil {
		return err
	}

	return wo.writeOutputF("Successfully restart workflow: %s\n", app.Name)
}

func (wo wfOperator) Terminate(ctx context.Context, app *v1beta1.Application) error {
	if err := service.TerminateWorkflow(context.TODO(), wo.cli, app); err != nil {
		return err
	}

	return nil
}

func (wo wfOperator) writeOutput(str string) error {
	if wo.outputWriter == nil {
		return nil
	}
	_, err := wo.outputWriter.Write([]byte(str))
	return err
}

func (wo wfOperator) writeOutputF(format string, a ...interface{}) error {
	if wo.outputWriter == nil {
		return nil
	}
	_, err := wo.outputWriter.Write([]byte(fmt.Sprintf(format, a...)))
	return err
}
