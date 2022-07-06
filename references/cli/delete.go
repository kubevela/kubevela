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
	"errors"
	"fmt"

	"github.com/spf13/cobra"

	"github.com/oam-dev/kubevela/apis/types"
	common2 "github.com/oam-dev/kubevela/pkg/utils/common"
	cmdutil "github.com/oam-dev/kubevela/pkg/utils/util"
	"github.com/oam-dev/kubevela/references/common"
)

// NewDeleteCommand Delete App
func NewDeleteCommand(c common2.Args, order string, ioStreams cmdutil.IOStreams) *cobra.Command {
	cmd := &cobra.Command{
		Use:                   "delete APP_NAME",
		DisableFlagsInUseLine: true,
		Short:                 "Delete an application",
		Long:                  "Delete an application.",
		Annotations: map[string]string{
			types.TagCommandOrder: order,
			types.TagCommandType:  types.TypeApp,
		},
		Example: "vela delete frontend",
	}
	cmd.SetOut(ioStreams.Out)

	o := &common.DeleteOptions{
		C: c,
	}
	cmd.RunE = func(cmd *cobra.Command, args []string) error {
		namespace, err := GetFlagNamespaceOrEnv(cmd, c)
		if err != nil {
			return err
		}
		o.Namespace = namespace
		newClient, err := c.GetClient()
		if err != nil {
			return err
		}
		o.Client = newClient

		if len(args) < 1 {
			return errors.New("must specify name for the app")
		}
		o.AppName = args[0]
		svcname, err := cmd.Flags().GetString(Service)
		if err != nil {
			return err
		}
		wait, err := cmd.Flags().GetBool("wait")
		if err != nil {
			return err
		}
		o.Wait = wait
		force, err := cmd.Flags().GetBool("force")
		if err != nil {
			return err
		}
		o.ForceDelete = force
		userInput := NewUserInput()
		if svcname == "" {
			if !assumeYes {
				userConfirmation := userInput.AskBool(fmt.Sprintf("Do you want to delete the application %s from namespace %s", o.AppName, o.Namespace), &UserInputOptions{assumeYes})
				if !userConfirmation {
					return fmt.Errorf("stopping Deleting")
				}
			}
			if err = o.DeleteApp(ioStreams); err != nil {
				return err
			}
			ioStreams.Info(green.Sprintf("app \"%s\" deleted from namespace \"%s\"", o.AppName, o.Namespace))
		} else {
			if !assumeYes {
				userConfirmation := userInput.AskBool(fmt.Sprintf("Do you want to delete the component %s from application %s in namespace %s", svcname, o.AppName, o.Namespace), &UserInputOptions{assumeYes})
				if !userConfirmation {
					return fmt.Errorf("stopping Deleting")
				}
			}
			o.CompName = svcname
			if err = o.DeleteComponent(ioStreams); err != nil {
				return err
			}
			ioStreams.Info(green.Sprintf("component \"%s\" deleted from \"%s\"", o.CompName, o.AppName))
		}
		return nil
	}

	cmd.PersistentFlags().StringP(Service, "", "", "delete only the specified service in this app")
	cmd.PersistentFlags().BoolVarP(&o.Wait, "wait", "w", false, "wait util the application is deleted completely")
	cmd.PersistentFlags().BoolVarP(&o.ForceDelete, "force", "f", false, "force to delete the application")
	addNamespaceAndEnvArg(cmd)
	return cmd
}
