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
	"errors"
	"fmt"
	"time"

	"github.com/fatih/color"
	"github.com/gosuri/uilive"
	"github.com/spf13/cobra"
	"k8s.io/apimachinery/pkg/util/wait"

	"github.com/oam-dev/kubevela/apis/types"
	common2 "github.com/oam-dev/kubevela/pkg/utils/common"
	cmdutil "github.com/oam-dev/kubevela/pkg/utils/util"
	"github.com/oam-dev/kubevela/references/common"
)

// NewScaleCommand Scale App
func NewScaleCommand(c common2.Args, order string, ioStreams cmdutil.IOStreams) *cobra.Command {
	cmd := &cobra.Command{
		Use:                   "scale",
		DisableFlagsInUseLine: true,
		Short:                 "Scale a component",
		Long:                  "Scale a component.",
		Annotations: map[string]string{
			types.TagCommandOrder: order,
			types.TagCommandType:  types.TypeApp,
		},
		Example: "scale app -c comp --replicas 2",
	}
	cmd.SetOut(ioStreams.Out)

	o := &common.ScaleOptions{
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
		err = o.ScaleComponent(ioStreams)
		if err != nil {
			return err
		}

		if o.Wait {
			ctx := context.Background()
			tryCnt, startTime := 0, time.Now()
			writer := uilive.New()
			writer.Start()
			defer writer.Stop()

			ioStreams.Infof(color.New(color.FgYellow).Sprintf("waiting for scale the application \"%s\"...\n", o.AppName))
			err := wait.PollImmediate(2*time.Second, 2*time.Minute, func() (done bool, err error) {
				tryCnt++
				f := Filter{
					Component: o.CompName,
				}

				pods, err := GetApplicationPods(ctx, o.AppName, o.Namespace, c, f)
				if err != nil {
					return false, err
				}
				fmt.Fprintf(writer, "try to scale the application for the %d time, wait a total of %f s, expect: %d, current: %d \n", tryCnt, time.Since(startTime).Seconds(), o.Replicas, len(pods))
				if len(pods) == int(o.Replicas) {
					return true, nil
				}
				return false, nil

			})
			if err != nil {
				ioStreams.Info("waiting for the application to be scale timed out, please try again")
				return err
			}
			return nil
		}

		ioStreams.Info(green.Sprintf("app \"%s\" scale %s to %d from namespace \"%s\"", o.AppName, o.CompName, o.Replicas, o.Namespace))
		return nil
	}

	cmd.PersistentFlags().StringVarP(&o.CompName, "component", "c", "", "filter the endpoints or pods by component name")
	cmd.PersistentFlags().Int64VarP(&o.Replicas, "replicas", "r", 1, "filter the endpoints or pods by component name")
	cmd.PersistentFlags().BoolVarP(&o.Wait, "wait", "w", false, "wait util the application is scaled completely")
	addNamespaceAndEnvArg(cmd)
	return cmd
}
