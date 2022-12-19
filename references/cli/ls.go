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

	"github.com/oam-dev/kubevela/pkg/utils"

	"github.com/gosuri/uitable"
	"github.com/spf13/cobra"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/fields"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"

	commontypes "github.com/oam-dev/kubevela/apis/core.oam.dev/common"
	"github.com/oam-dev/kubevela/apis/core.oam.dev/v1beta1"
	"github.com/oam-dev/kubevela/apis/types"
	"github.com/oam-dev/kubevela/pkg/utils/common"
	cmdutil "github.com/oam-dev/kubevela/pkg/utils/util"
)

// AllNamespace list app in all namespaces
var AllNamespace bool

// LabelSelector list app using label selector
var LabelSelector string

// FieldSelector list app using field selector
var FieldSelector string

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
	cmd.Flags().StringVarP(&LabelSelector, "selector", "l", LabelSelector, "Selector (label query) to filter on, supports '=', '==', and '!='.(e.g. -l key1=value1,key2=value2).")
	cmd.Flags().StringVar(&FieldSelector, "field-selector", FieldSelector, "Selector (field query) to filter on, supports '=', '==', and '!='.(e.g. --field-selector key1=value1,key2=value2).")
	return cmd
}

func printApplicationList(ctx context.Context, c client.Reader, namespace string, ioStreams cmdutil.IOStreams) error {
	table, err := buildApplicationListTable(ctx, c, namespace)
	if err != nil {
		return err
	}
	ioStreams.Info(table.String())
	return nil
}

func buildApplicationListTable(ctx context.Context, c client.Reader, namespace string) (*uitable.Table, error) {
	table := newUITable()
	header := []interface{}{"APP", "COMPONENT", "TYPE", "TRAITS", "PHASE", "HEALTHY", "STATUS", "CREATED-TIME"}
	if AllNamespace {
		header = append([]interface{}{"NAMESPACE"}, header...)
	}
	table.AddRow(header...)

	labelSelector := labels.NewSelector()
	if len(LabelSelector) > 0 {
		selector, err := labels.Parse(LabelSelector)
		if err != nil {
			return nil, err
		}
		labelSelector = selector
	}

	applist := v1beta1.ApplicationList{}
	if err := c.List(ctx, &applist, client.InNamespace(namespace), &client.ListOptions{LabelSelector: labelSelector}); err != nil {
		if apierrors.IsNotFound(err) {
			return table, nil
		}
		return nil, err
	}

	if len(FieldSelector) > 0 {
		fieldSelector, err := fields.ParseSelector(FieldSelector)
		if err != nil {
			return nil, err
		}
		var objects []runtime.Object
		for i := range applist.Items {
			objects = append(objects, &applist.Items[i])
		}
		applist.Items = objectsToApps(utils.FilterObjectsByFieldSelector(objects, fieldSelector))
	}

	for _, a := range applist.Items {
		service := map[string]commontypes.ApplicationComponentStatus{}
		for _, s := range a.Status.Services {
			service[s.Name] = s
		}

		if len(a.Spec.Components) == 0 {
			if AllNamespace {
				table.AddRow(a.Namespace, a.Name, "", "", "", a.Status.Phase, "", "", a.CreationTimestamp)
			} else {
				table.AddRow(a.Name, "", "", "", a.Status.Phase, "", "", a.CreationTimestamp)
			}
			continue
		}

		for idx, cmp := range a.Spec.Components {
			var appName = a.Name
			if idx > 0 {
				appName = "├─"
				if idx == len(a.Spec.Components)-1 {
					appName = "└─"
				}
			}

			var healthy, status string
			if s, ok := service[cmp.Name]; ok {
				healthy = getHealthString(s.Healthy)
				status = s.Message
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
	return table, nil
}

func getHealthString(healthy bool) string {
	if healthy {
		return "healthy"
	}
	return "unhealthy"
}

// objectsToApps objects to apps
func objectsToApps(objs []runtime.Object) []v1beta1.Application {
	res := make([]v1beta1.Application, 0)
	for _, obj := range objs {
		obj, ok := obj.(*v1beta1.Application)
		if ok {
			res = append(res, *obj)
		}
	}
	return res
}
