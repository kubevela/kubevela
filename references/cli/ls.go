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
	"strings"

	"github.com/spf13/cobra"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/oam-dev/kubevela/apis/core.oam.dev/v1beta1"
	"github.com/oam-dev/kubevela/apis/types"
	"github.com/oam-dev/kubevela/pkg/utils/common"
	cmdutil "github.com/oam-dev/kubevela/pkg/utils/util"
)

// NewListCommand creates `ls` command and its nested children command
func NewListCommand(c common.Args, ioStreams cmdutil.IOStreams) *cobra.Command {
	ctx := context.Background()
	cmd := &cobra.Command{
		Use:                   "ls",
		Aliases:               []string{"list"},
		DisableFlagsInUseLine: true,
		Short:                 "List applications",
		Long:                  "List all applications in cluster",
		Example:               `vela ls`,
		PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
			return c.SetConfig()
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			env, err := GetEnv(cmd)
			if err != nil {
				return err
			}
			newClient, err := c.GetClient()
			if err != nil {
				return err
			}
			namespace, err := cmd.Flags().GetString(Namespace)
			if err != nil {
				return err
			}
			if namespace == "" {
				namespace = env.Namespace
			}
			return printApplicationList(ctx, newClient, namespace, ioStreams)
		},
		Annotations: map[string]string{
			types.TagCommandType: types.TypeApp,
		},
	}
	cmd.PersistentFlags().StringP(Namespace, "n", "", "specify the namespace the application want to list, default is the current env namespace")
	return cmd
}

func printApplicationList(ctx context.Context, c client.Reader, namespace string, ioStreams cmdutil.IOStreams) error {
	table := newUITable()
	table.AddRow("APP", "COMPONENT", "TYPE", "TRAITS", "PHASE", "HEALTHY", "STATUS", "CREATED-TIME")
	applist := v1beta1.ApplicationList{}
	if err := c.List(ctx, &applist, client.InNamespace(namespace)); err != nil {
		if apierrors.IsNotFound(err) {
			ioStreams.Info(table.String())
			return nil
		}
		return err
	}

	for _, a := range applist.Items {
		for idx, cmp := range a.Spec.Components {
			var appName = a.Name
			if idx > 0 {
				appName = "├─"
				if idx == len(a.Spec.Components)-1 {
					appName = "└─"
				}
			}
			var healthy, status string
			if len(a.Status.Services) > idx {
				if a.Status.Services[idx].Healthy {
					healthy = "healthy"
				} else {
					healthy = "unhealthy"
				}
				status = a.Status.Services[idx].Message
			}
			var traits []string
			for _, tr := range cmp.Traits {
				traits = append(traits, tr.Type)
			}
			table.AddRow(appName, cmp.Name, cmp.Type, strings.Join(traits, ","), a.Status.Phase, healthy, status, a.CreationTimestamp)
		}
	}
	ioStreams.Info(table.String())
	return nil
}
