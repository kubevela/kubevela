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

	"github.com/spf13/cobra"
	apierrors "k8s.io/apimachinery/pkg/api/errors"

	"github.com/oam-dev/kubevela/apis/types"
	common2 "github.com/oam-dev/kubevela/pkg/utils/common"
	cmdutil "github.com/oam-dev/kubevela/pkg/utils/util"
	"github.com/oam-dev/kubevela/references/common"
)

// NewDeleteCommand Delete App
func NewDeleteCommand(c common2.Args, ioStreams cmdutil.IOStreams) *cobra.Command {
	cmd := &cobra.Command{
		Use:                   "delete APP_NAME",
		DisableFlagsInUseLine: true,
		Short:                 "Delete an application",
		Long:                  "Delete an application",
		PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
			return c.SetConfig()
		},
		Annotations: map[string]string{
			types.TagCommandType: types.TypeApp,
		},
		Example: "vela delete frontend",
	}
	cmd.SetOut(ioStreams.Out)

	cmd.RunE = func(cmd *cobra.Command, args []string) error {
		namespace, err := GetFlagNamespaceOrEnv(cmd, c)
		if err != nil {
			return err
		}
		newClient, err := c.GetClient()
		if err != nil {
			return err
		}
		o := &common.DeleteOptions{
			C:         c,
			Namespace: namespace,
			Client:    newClient,
		}
		if len(args) < 1 {
			return errors.New("must specify name for the app")
		}
		o.AppName = args[0]
		svcname, err := cmd.Flags().GetString(Service)
		if err != nil {
			return err
		}
		if svcname == "" {
			ioStreams.Infof("Deleting Application \"%s\"\n", o.AppName)
			info, err := o.DeleteApp()
			if err != nil {
				if apierrors.IsNotFound(err) {
					ioStreams.Info("Already deleted")
					return nil
				}
				return err
			}
			ioStreams.Info(info)
		} else {
			ioStreams.Infof("Deleting Service %s from Application \"%s\"\n", svcname, o.AppName)
			o.CompName = svcname
			message, err := o.DeleteComponent(ioStreams)
			if err != nil {
				return err
			}
			ioStreams.Info(message)
		}
		return nil
	}
	cmd.PersistentFlags().StringP(Service, "", "", "delete only the specified service in this app")
	addNamespaceArg(cmd)
	return cmd
}
