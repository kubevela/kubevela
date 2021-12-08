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

	"github.com/spf13/cobra"
	k8stypes "k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/oam-dev/kubevela/apis/core.oam.dev/v1beta1"
	"github.com/oam-dev/kubevela/apis/types"
	"github.com/oam-dev/kubevela/pkg/utils/common"
	cmdutil "github.com/oam-dev/kubevela/pkg/utils/util"
	"github.com/oam-dev/kubevela/references/appfile"
)

// NewWorkflowCommand create `workflow` command
func NewWorkflowCommand(c common.Args, ioStreams cmdutil.IOStreams) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "workflow",
		Short: "Operate application workflow in KubeVela",
		Long:  "Operate application workflow in KubeVela",
		Annotations: map[string]string{
			types.TagCommandType: types.TypeSystem,
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
		Short:   "Suspend an application workflow",
		Long:    "Suspend an application workflow in cluster",
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
			if app.Spec.Workflow == nil {
				return fmt.Errorf("the application must have workflow")
			}
			if app.Status.Workflow == nil {
				return fmt.Errorf("the workflow in application is not running")
			}
			kubecli, err := c.GetClient()
			if err != nil {
				return err
			}

			err = suspendWorkflow(kubecli, app)
			if err != nil {
				return err
			}
			return nil
		},
	}
	addNamespaceArg(cmd)
	return cmd
}

// NewWorkflowResumeCommand create workflow resume command
func NewWorkflowResumeCommand(c common.Args, ioStream cmdutil.IOStreams) *cobra.Command {
	cmd := &cobra.Command{
		Use:     "resume",
		Short:   "Resume a suspend application workflow",
		Long:    "Resume a suspend application workflow in cluster",
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
			if !app.Status.Workflow.Suspend {
				_, err := ioStream.Out.Write([]byte("the workflow is not suspending\n"))
				if err != nil {
					return err
				}
				return nil
			}
			kubecli, err := c.GetClient()
			if err != nil {
				return err
			}

			err = resumeWorkflow(kubecli, app)
			if err != nil {
				return err
			}
			return nil
		},
	}
	addNamespaceArg(cmd)
	return cmd
}

// NewWorkflowTerminateCommand create workflow terminate command
func NewWorkflowTerminateCommand(c common.Args, ioStream cmdutil.IOStreams) *cobra.Command {
	cmd := &cobra.Command{
		Use:     "terminate",
		Short:   "Terminate an application workflow",
		Long:    "Terminate an application workflow in cluster",
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
			if app.Spec.Workflow == nil {
				return fmt.Errorf("the application must have workflow")
			}
			if app.Status.Workflow == nil {
				return fmt.Errorf("the workflow in application is not running")
			}
			kubecli, err := c.GetClient()
			if err != nil {
				return err
			}

			err = terminateWorkflow(kubecli, app)
			if err != nil {
				return err
			}
			return nil
		},
	}
	addNamespaceArg(cmd)
	return cmd
}

// NewWorkflowRestartCommand create workflow restart command
func NewWorkflowRestartCommand(c common.Args, ioStream cmdutil.IOStreams) *cobra.Command {
	cmd := &cobra.Command{
		Use:     "restart",
		Short:   "Restart an application workflow",
		Long:    "Restart an application workflow in cluster",
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
			if app.Spec.Workflow == nil {
				return fmt.Errorf("the application must have workflow")
			}
			if app.Status.Workflow == nil {
				return fmt.Errorf("the workflow in application is not running")
			}
			kubecli, err := c.GetClient()
			if err != nil {
				return err
			}

			err = restartWorkflow(kubecli, app)
			if err != nil {
				return err
			}
			return nil
		},
	}
	addNamespaceArg(cmd)
	return cmd
}

// NewWorkflowRollbackCommand create workflow rollback command
func NewWorkflowRollbackCommand(c common.Args, ioStream cmdutil.IOStreams) *cobra.Command {
	cmd := &cobra.Command{
		Use:     "rollback",
		Short:   "Rollback an application workflow to the latest revision",
		Long:    "Rollback an application workflow to the latest revision",
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
			if app.Spec.Workflow == nil {
				return fmt.Errorf("the application must have workflow")
			}
			if app.Status.Workflow != nil && !app.Status.Workflow.Terminated && !app.Status.Workflow.Suspend && !app.Status.Workflow.Finished {
				return fmt.Errorf("can not rollback a running workflow")
			}
			kubecli, err := c.GetClient()
			if err != nil {
				return err
			}

			err = rollbackWorkflow(kubecli, app)
			if err != nil {
				return err
			}
			return nil
		},
	}
	addNamespaceArg(cmd)
	return cmd
}

func suspendWorkflow(kubecli client.Client, app *v1beta1.Application) error {
	// set the workflow suspend to true
	app.Status.Workflow.Suspend = true

	if err := kubecli.Status().Patch(context.TODO(), app, client.Merge); err != nil {
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

func rollbackWorkflow(kubecli client.Client, app *v1beta1.Application) error {
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
