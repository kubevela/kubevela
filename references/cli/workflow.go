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

package cli

import (
	"context"
	"fmt"

	"github.com/pkg/errors"
	"github.com/spf13/cobra"
	k8stypes "k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/util/retry"
	"sigs.k8s.io/controller-runtime/pkg/client"

	oamcommon "github.com/oam-dev/kubevela/apis/core.oam.dev/common"
	"github.com/oam-dev/kubevela/apis/core.oam.dev/v1beta1"
	"github.com/oam-dev/kubevela/apis/types"
	"github.com/oam-dev/kubevela/pkg/controller/core.oam.dev/v1alpha2/application"
	"github.com/oam-dev/kubevela/pkg/controller/utils"
	"github.com/oam-dev/kubevela/pkg/multicluster"
	"github.com/oam-dev/kubevela/pkg/oam"
	"github.com/oam-dev/kubevela/pkg/resourcetracker"
	"github.com/oam-dev/kubevela/pkg/rollout"
	"github.com/oam-dev/kubevela/pkg/utils/common"
	velaerrors "github.com/oam-dev/kubevela/pkg/utils/errors"
	cmdutil "github.com/oam-dev/kubevela/pkg/utils/util"
	"github.com/oam-dev/kubevela/references/appfile"
)

// NewWorkflowCommand create `workflow` command
func NewWorkflowCommand(c common.Args, ioStreams cmdutil.IOStreams) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "workflow",
		Short: "Operate application delivery workflow.",
		Long:  "Operate the Workflow during Application Delivery.",
		Annotations: map[string]string{
			types.TagCommandType: types.TypeCD,
		},
	}
	cmd.AddCommand(
		NewWorkflowSuspendCommand(c, ioStreams),
		NewWorkflowResumeCommand(c, ioStreams),
		NewWorkflowTerminateCommand(c, ioStreams),
		NewWorkflowRestartCommand(c, ioStreams),
		NewWorkflowRollbackCommand(c, ioStreams),
	)
	return cmd
}

// NewWorkflowSuspendCommand create workflow suspend command
func NewWorkflowSuspendCommand(c common.Args, ioStream cmdutil.IOStreams) *cobra.Command {
	cmd := &cobra.Command{
		Use:     "suspend",
		Short:   "Suspend an application workflow.",
		Long:    "Suspend an application workflow in cluster.",
		Example: "vela workflow suspend <application-name>",
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) < 1 {
				return fmt.Errorf("must specify application name")
			}
			namespace, err := GetFlagNamespaceOrEnv(cmd, c)
			if err != nil {
				return err
			}
			app, err := appfile.LoadApplication(namespace, args[0], c)
			if err != nil {
				return err
			}
			if app.Status.Workflow == nil {
				return fmt.Errorf("the workflow in application is not running")
			}
			config, err := c.GetConfig()
			if err != nil {
				return err
			}
			config.Wrap(multicluster.NewSecretModeMultiClusterRoundTripper)
			client, err := c.GetClient()
			if err != nil {
				return err
			}
			if err = rollout.SuspendRollout(context.Background(), client, app, cmd.OutOrStdout()); err != nil {
				return err
			}
			if err = suspendWorkflow(client, app); err != nil {
				return err
			}
			return nil
		},
	}
	addNamespaceAndEnvArg(cmd)
	return cmd
}

// NewWorkflowResumeCommand create workflow resume command
func NewWorkflowResumeCommand(c common.Args, ioStream cmdutil.IOStreams) *cobra.Command {
	cmd := &cobra.Command{
		Use:     "resume",
		Short:   "Resume a suspend application workflow.",
		Long:    "Resume a suspend application workflow in cluster.",
		Example: "vela workflow resume <application-name>",
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) < 1 {
				return fmt.Errorf("must specify application name")
			}
			namespace, err := GetFlagNamespaceOrEnv(cmd, c)
			if err != nil {
				return err
			}
			app, err := appfile.LoadApplication(namespace, args[0], c)
			if err != nil {
				return err
			}
			if app.Status.Workflow == nil {
				return fmt.Errorf("the workflow in application is not running")
			}
			if app.Status.Workflow.Terminated {
				return fmt.Errorf("can not resume a terminated workflow")
			}
			config, err := c.GetConfig()
			if err != nil {
				return err
			}
			config.Wrap(multicluster.NewSecretModeMultiClusterRoundTripper)
			client, err := c.GetClient()
			if err != nil {
				return err
			}

			var rolloutResumed bool
			if rolloutResumed, err = rollout.ResumeRollout(context.Background(), client, app, cmd.OutOrStdout()); err != nil {
				return err
			}
			if !rolloutResumed && !app.Status.Workflow.Suspend {
				_, err := ioStream.Out.Write([]byte("the workflow is not suspending\n"))
				if err != nil {
					return err
				}
				return nil
			}
			if app.Status.Workflow.Suspend {
				if err = resumeWorkflow(client, app); err != nil {
					return err
				}
			}
			return nil
		},
	}
	addNamespaceAndEnvArg(cmd)
	return cmd
}

// NewWorkflowTerminateCommand create workflow terminate command
func NewWorkflowTerminateCommand(c common.Args, ioStream cmdutil.IOStreams) *cobra.Command {
	cmd := &cobra.Command{
		Use:     "terminate",
		Short:   "Terminate an application workflow.",
		Long:    "Terminate an application workflow in cluster.",
		Example: "vela workflow terminate <application-name>",
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) < 1 {
				return fmt.Errorf("must specify application name")
			}
			namespace, err := GetFlagNamespaceOrEnv(cmd, c)
			if err != nil {
				return err
			}
			app, err := appfile.LoadApplication(namespace, args[0], c)
			if err != nil {
				return err
			}
			if app.Status.Workflow == nil {
				return fmt.Errorf("the workflow in application is not running")
			}
			client, err := c.GetClient()
			if err != nil {
				return err
			}
			err = terminateWorkflow(client, app)
			if err != nil {
				return err
			}
			return nil
		},
	}
	addNamespaceAndEnvArg(cmd)
	return cmd
}

// NewWorkflowRestartCommand create workflow restart command
func NewWorkflowRestartCommand(c common.Args, ioStream cmdutil.IOStreams) *cobra.Command {
	cmd := &cobra.Command{
		Use:     "restart",
		Short:   "Restart an application workflow.",
		Long:    "Restart an application workflow in cluster.",
		Example: "vela workflow restart <application-name>",
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) < 1 {
				return fmt.Errorf("must specify application name")
			}
			namespace, err := GetFlagNamespaceOrEnv(cmd, c)
			if err != nil {
				return err
			}
			app, err := appfile.LoadApplication(namespace, args[0], c)
			if err != nil {
				return err
			}
			if app.Status.Workflow == nil {
				return fmt.Errorf("the workflow in application is not running")
			}
			config, err := c.GetConfig()
			if err != nil {
				return err
			}
			config.Wrap(multicluster.NewSecretModeMultiClusterRoundTripper)
			client, err := c.GetClient()
			if err != nil {
				return err
			}
			_, _ = rollout.ResumeRollout(context.Background(), client, app, cmd.OutOrStdout())

			err = restartWorkflow(client, app)
			if err != nil {
				return err
			}
			return nil
		},
	}
	addNamespaceAndEnvArg(cmd)
	return cmd
}

// NewWorkflowRollbackCommand create workflow rollback command
func NewWorkflowRollbackCommand(c common.Args, ioStream cmdutil.IOStreams) *cobra.Command {
	cmd := &cobra.Command{
		Use:     "rollback",
		Short:   "Rollback an application workflow to the latest revision.",
		Long:    "Rollback an application workflow to the latest revision.",
		Example: "vela workflow rollback <application-name>",
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) < 1 {
				return fmt.Errorf("must specify application name")
			}
			namespace, err := GetFlagNamespaceOrEnv(cmd, c)
			if err != nil {
				return err
			}
			app, err := appfile.LoadApplication(namespace, args[0], c)
			if err != nil {
				return err
			}
			if app.Status.Workflow != nil && !app.Status.Workflow.Terminated && !app.Status.Workflow.Suspend && !app.Status.Workflow.Finished {
				return fmt.Errorf("can not rollback a running workflow")
			}
			config, err := c.GetConfig()
			if err != nil {
				return err
			}
			config.Wrap(multicluster.NewSecretModeMultiClusterRoundTripper)
			client, err := c.GetClient()
			if err != nil {
				return err
			}
			_, _ = rollout.ResumeRollout(context.Background(), client, app, cmd.OutOrStdout())

			err = rollbackWorkflow(cmd, client, app)
			if err != nil {
				return err
			}
			return nil
		},
	}
	addNamespaceAndEnvArg(cmd)
	return cmd
}

func suspendWorkflow(kubecli client.Client, app *v1beta1.Application) error {
	appKey := client.ObjectKeyFromObject(app)
	ctx := context.Background()
	if err := retry.RetryOnConflict(retry.DefaultBackoff, func() error {
		if err := kubecli.Get(ctx, appKey, app); err != nil {
			return err
		}
		// set the workflow suspend to true
		app.Status.Workflow.Suspend = true
		return kubecli.Status().Patch(ctx, app, client.Merge)
	}); err != nil {
		return err
	}

	fmt.Printf("Successfully suspend workflow: %s\n", app.Name)
	return nil
}

func resumeWorkflow(kubecli client.Client, app *v1beta1.Application) error {
	// set the workflow suspend to false
	app.Status.Workflow.Suspend = false

	if err := kubecli.Status().Patch(context.TODO(), app, client.Merge); err != nil {
		return err
	}

	fmt.Printf("Successfully resume workflow: %s\n", app.Name)
	return nil
}

func terminateWorkflow(kubecli client.Client, app *v1beta1.Application) error {
	// set the workflow terminated to true
	app.Status.Workflow.Terminated = true

	if err := kubecli.Status().Patch(context.TODO(), app, client.Merge); err != nil {
		return err
	}

	fmt.Printf("Successfully terminate workflow: %s\n", app.Name)
	return nil
}

func restartWorkflow(kubecli client.Client, app *v1beta1.Application) error {
	// reset the workflow status to restart the workflow
	app.Status.Workflow = nil

	if err := kubecli.Status().Update(context.TODO(), app); err != nil {
		return err
	}

	fmt.Printf("Successfully restart workflow: %s\n", app.Name)
	return nil
}

func rollbackWorkflow(cmd *cobra.Command, kubecli client.Client, app *v1beta1.Application) error {
	if oam.GetPublishVersion(app) != "" {
		return rollbackApplicationWithPublishVersion(cmd, kubecli, app)
	}
	if app.Status.LatestRevision == nil || app.Status.LatestRevision.Name == "" {
		return fmt.Errorf("the latest revision is not set: %s", app.Name)
	}
	// get the last revision
	revision := &v1beta1.ApplicationRevision{}
	if err := kubecli.Get(context.TODO(), k8stypes.NamespacedName{Name: app.Status.LatestRevision.Name, Namespace: app.Namespace}, revision); err != nil {
		return fmt.Errorf("failed to get the latest revision: %w", err)
	}

	app.Spec = revision.Spec.Application.Spec
	if err := kubecli.Status().Update(context.TODO(), app); err != nil {
		return err
	}

	fmt.Printf("Successfully rollback workflow to the latest revision: %s\n", app.Name)
	return nil
}

func rollbackApplicationWithPublishVersion(cmd *cobra.Command, cli client.Client, app *v1beta1.Application) error {
	ctx := context.Background()
	appRevs, err := application.GetSortedAppRevisions(ctx, cli, app.Name, app.Namespace)
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
	_, currentRT, historyRTs, _, err := resourcetracker.ListApplicationResourceTrackers(ctx, cli, app)
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
	cmd.Printf("Find succeeded application revision %s (PublishVersion: %s) to rollback.\n", rev.Name, publishVersion)

	appKey := client.ObjectKeyFromObject(app)
	// rollback application spec and freeze
	controllerRequirement, err := utils.FreezeApplication(ctx, cli, app, func() {
		app.Spec = rev.Spec.Application.Spec
		oam.SetPublishVersion(app, publishVersion)
	})
	if err != nil {
		return errors.Wrapf(err, "failed to rollback application spec to revision %s (PublishVersion: %s)", rev.Name, publishVersion)
	}
	cmd.Printf("Application spec rollback successfully.\n")

	// rollback application status
	if err = retry.RetryOnConflict(retry.DefaultBackoff, func() error {
		if err = cli.Get(ctx, appKey, app); err != nil {
			return err
		}
		app.Status.Workflow = rev.Status.Workflow
		app.Status.Services = []oamcommon.ApplicationComponentStatus{}
		app.Status.AppliedResources = []oamcommon.ClusterObjectReference{}
		for _, rsc := range matchRT.Spec.ManagedResources {
			app.Status.AppliedResources = append(app.Status.AppliedResources, rsc.ClusterObjectReference)
		}
		app.Status.LatestRevision = &oamcommon.Revision{
			Name:         rev.Name,
			Revision:     int64(revisionNumber),
			RevisionHash: rev.GetLabels()[oam.LabelAppRevisionHash],
		}
		return cli.Status().Update(ctx, app)
	}); err != nil {
		return errors.Wrapf(err, "failed to rollback application status to revision %s (PublishVersion: %s)", rev.Name, publishVersion)
	}
	cmd.Printf("Application status rollback successfully.\n")

	// update resource tracker generation
	matchRTKey := client.ObjectKeyFromObject(matchRT)
	if err = retry.RetryOnConflict(retry.DefaultBackoff, func() error {
		if err = cli.Get(ctx, matchRTKey, matchRT); err != nil {
			return err
		}
		matchRT.Spec.ApplicationGeneration = app.Generation
		return cli.Update(ctx, matchRT)
	}); err != nil {
		return errors.Wrapf(err, "failed to update application generation in resource tracker")
	}

	// unfreeze application
	if err = utils.UnfreezeApplication(ctx, cli, app, nil, controllerRequirement); err != nil {
		return errors.Wrapf(err, "failed to resume application to restart")
	}
	cmd.Printf("Application rollback completed.\n")

	// clean up outdated revisions
	var errs velaerrors.ErrorList
	for _, _rev := range outdatedRev {
		if err = cli.Delete(ctx, _rev); err != nil {
			errs = append(errs, err)
		}
	}
	if errs.HasError() {
		return errors.Wrapf(errs, "failed to clean up outdated revisions")
	}
	cmd.Printf("Application outdated revision cleaned up.\n")
	return nil
}
