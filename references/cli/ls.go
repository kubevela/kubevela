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

// AllNamespace list app in all namespaces
var AllNamespace bool

// NewListCommand creates `ls` command and its nested children command
func NewListCommand(c common.Args, order string, ioStreams cmdutil.IOStreams) *cobra.Command {
	ctx := context.Background()
	cmd := &cobra.Command{
		Use:                   "ls",
		Aliases:               []string{"list"},
		DisableFlagsInUseLine: true,
		Short:                 "List applications",
		Long:                  "List all vela applications.",
		Example:               `vela ls`,
		RunE: func(cmd *cobra.Command, args []string) error {
			newClient, err := c.GetClient()
			if err != nil {
				return err
			}
			namespace, err := GetFlagNamespaceOrEnv(cmd, c)
			if err != nil {
				return err
			}
			if AllNamespace {
				namespace = ""
			}
			return printApplicationList(ctx, newClient, namespace, ioStreams)
		},
		Annotations: map[string]string{
			types.TagCommandOrder: order,
			types.TagCommandType:  types.TypeApp,
		},
	}
	addNamespaceAndEnvArg(cmd)
	cmd.Flags().BoolVarP(&AllNamespace, "all-namespaces", "A", false, "If true, check the specified action in all namespaces.")
	return cmd
}

func printApplicationList(ctx context.Context, c client.Reader, namespace string, ioStreams cmdutil.IOStreams) error {
	table := newUITable()
	header := []interface{}{"APP", "COMPONENT", "TYPE", "TRAITS", "PHASE", "HEALTHY", "STATUS", "CREATED-TIME"}
	if AllNamespace {
		header = append([]interface{}{"NAMESPACE"}, header...)
	}
	table.AddRow(header...)
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
			if AllNamespace {
				table.AddRow(a.Namespace, appName, cmp.Name, cmp.Type, strings.Join(traits, ","), a.Status.Phase, healthy, status, a.CreationTimestamp)
			} else {
				table.AddRow(appName, cmp.Name, cmp.Type, strings.Join(traits, ","), a.Status.Phase, healthy, status, a.CreationTimestamp)
			}
		}
	}
	ioStreams.Info(table.String())
	return nil
}
