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
	)
	return cmd
}

// NewWorkflowSuspendCommand create workflow suspend command
func NewWorkflowSuspendCommand(c common.Args, ioStream cmdutil.IOStreams) *cobra.Command {
	return &cobra.Command{
		Use:     "suspend",
		Short:   "Suspend an application workflow",
		Long:    "Suspend an application workflow in cluster",
		Example: "vela workflow suspend <application-name>",
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) < 1 {
				return fmt.Errorf("must specify application name")
			}
			env, err := GetEnv(cmd)
			if err != nil {
				return err
			}
			app, err := appfile.LoadApplication(env.Namespace, args[0], c)
			if err != nil {
				return err
			}
			if app.Spec.Workflow == nil {
				return fmt.Errorf("the application must have workflow")
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
}

func suspendWorkflow(kubecli client.Client, app *v1beta1.Application) error {
	// set the workflow suspend to true
	if app.Status.Workflow != nil {
		app.Status.Workflow.Suspend = true
	} else {
		return fmt.Errorf("the workflow in application is not running")
	}

	if err := kubecli.Status().Patch(context.TODO(), app, client.Merge); err != nil {
		return err
	}

	fmt.Printf("Successfully suspend workflow: %s\n", app.Name)
	return nil
}
