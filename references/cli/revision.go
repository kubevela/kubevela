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
	"io"

	"github.com/pkg/errors"
	"github.com/spf13/cobra"
	apitypes "k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/yaml"

	"github.com/oam-dev/kubevela/pkg/apiserver/utils/log"
	"github.com/oam-dev/kubevela/pkg/velaql"

	"github.com/oam-dev/kubevela/apis/core.oam.dev/v1beta1"
	"github.com/oam-dev/kubevela/apis/types"
	"github.com/oam-dev/kubevela/pkg/controller/core.oam.dev/v1alpha2/application"
	"github.com/oam-dev/kubevela/pkg/oam"
	"github.com/oam-dev/kubevela/pkg/utils"
	"github.com/oam-dev/kubevela/pkg/utils/common"
)

const (
	revisionView = "application-revision-view"
)

// RevisionCommandGroup the commands for managing application revisions
func RevisionCommandGroup(c common.Args) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "revision",
		Short: "Manage Application Revisions",
		Long:  "Manage KubeVela Application Revisions",
		Annotations: map[string]string{
			types.TagCommandType: types.TypeApp,
		},
	}
	cmd.AddCommand(
		NewRevisionListCommand(c),
		NewRevisionGetCommand(c),
	)
	return cmd
}

// NewRevisionListCommand list the revisions for application
func NewRevisionListCommand(c common.Args) *cobra.Command {
	cmd := &cobra.Command{
		Use:     "list",
		Aliases: []string{"ls"},
		Short:   "list application revisions",
		Long:    "list Kubevela application revisions",
		Args:    cobra.ExactValidArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			namespace, err := GetFlagNamespaceOrEnv(cmd, c)
			if err != nil {
				return err
			}
			cli, err := c.GetClient()
			if err != nil {
				return err
			}
			name := args[0]
			app := &v1beta1.Application{}
			ctx := context.Background()
			if err = cli.Get(ctx, apitypes.NamespacedName{Namespace: namespace, Name: name}, app); err != nil {
				return errors.Wrapf(err, "failed to get application %s/%s", namespace, name)
			}
			revs, err := application.GetSortedAppRevisions(ctx, cli, name, namespace)
			if err != nil {
				return err
			}
			table := newUITable().AddRow("NAME", "PUBLISH_VERSION", "SUCCEEDED", "HASH", "BEGIN_TIME", "STATUS", "SIZE")
			for _, rev := range revs {
				var begin, status, hash, size string
				status = "NotStart"
				if rev.Status.Workflow != nil {
					begin = rev.Status.Workflow.StartTime.Format("2006-01-02 15:04:05")
					// aggregate workflow result
					switch {
					case rev.Status.Succeeded:
						status = "Succeeded"
					case rev.Status.Workflow.Terminated || rev.Status.Workflow.Suspend || rev.Status.Workflow.Finished:
						status = "Failed"
					case app.Status.LatestRevision != nil && app.Status.LatestRevision.Name == rev.Name:
						status = "Executing"
					default:
						status = "Failed"
					}
				}
				if labels := rev.GetLabels(); labels != nil {
					hash = rev.GetLabels()[oam.LabelAppRevisionHash]
				}
				if bs, err := yaml.Marshal(rev.Spec); err == nil {
					size = utils.ByteCountIEC(int64(len(bs)))
				}
				table.AddRow(rev.Name, oam.GetPublishVersion(rev.DeepCopy()), rev.Status.Succeeded, hash, begin, status, size)
			}
			if len(table.Rows) == 0 {
				cmd.Printf("No revisions found for application %s/%s.\n", namespace, name)
			} else {
				cmd.Println(table.String())
			}
			return nil
		},
	}
	addNamespaceAndEnvArg(cmd)
	return cmd
}

// NewRevisionGetCommand gets specific revision of application
func NewRevisionGetCommand(c common.Args) *cobra.Command {
	var outputFormat string
	ctx := context.Background()
	cmd := &cobra.Command{
		Use:     "get",
		Aliases: []string{"get"},
		Short:   "get specific revision of application",
		Long:    "get specific revision of application",
		Args:    cobra.ExactValidArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			namespace, err := GetFlagNamespaceOrEnv(cmd, c)
			if err != nil {
				return err
			}
			name := args[0]
			def, err := cmd.Flags().GetString("definition")
			if err != nil {
				return err
			}

			return getRevision(ctx, c, outputFormat, cmd.OutOrStdout(), name, namespace, def)
		},
	}
	addNamespaceAndEnvArg(cmd)
	cmd.Flags().StringP("definition", "d", "", "component definition")
	cmd.Flags().StringVarP(&outputFormat, "output", "o", "", "raw Application output format. One of: (json, yaml, jsonpath)")
	return cmd
}

func getRevision(ctx context.Context, c common.Args, format string, out io.Writer, name string, namespace string, def string) error {

	kubeConfig, err := c.GetConfig()
	if err != nil {
		return err
	}
	cli, err := c.GetClient()
	if err != nil {
		return err
	}

	dm, err := c.GetDiscoveryMapper()
	if err != nil {
		return err
	}

	pd, err := c.GetPackageDiscover()
	if err != nil {
		return err
	}

	params := map[string]string{
		"name":      name,
		"namespace": namespace,
	}
	query, err := velaql.ParseVelaQL(MakeVelaQL(revisionView, params, "status"))
	if err != nil {
		log.Logger.Errorf("fail to parse ql string %s", err.Error())
		return fmt.Errorf(fmt.Sprintf("Unable to get application revision %s in namespace %s", name, namespace))
	}

	queryValue, err := velaql.NewViewHandler(cli, kubeConfig, dm, pd).QueryView(ctx, query)
	if err != nil {
		log.Logger.Errorf("fail to query the view %s", err.Error())
		return fmt.Errorf(fmt.Sprintf("Unable to get application revision %s in namespace %s", name, namespace))
	}

	apprev := v1beta1.ApplicationRevision{}
	err = queryValue.UnmarshalTo(&apprev)
	if err != nil {
		return err
	}
	if apprev.CreationTimestamp.IsZero() {
		fmt.Fprintf(out, "No such application revision %s in namespace %s", name, namespace)
		return nil
	}

	if def != "" {
		if cd, exist := apprev.Spec.ComponentDefinitions[def]; exist {
			ba, err := yaml.Marshal(&cd)
			if err != nil {
				return err
			}
			fmt.Fprint(out, string(ba))
		} else {
			fmt.Fprintf(out, "No such definition %s", def)
		}
	} else {
		if format == "" {
			printApprev(out, apprev)
		} else {
			output, err := convertApplicationRevisionTo(format, &apprev)
			if err != nil {
				return err
			}
			fmt.Fprint(out, output)

		}
	}

	return nil
}

func printApprev(out io.Writer, apprev v1beta1.ApplicationRevision) {
	table := newUITable().AddRow("NAME", "PUBLISH_VERSION", "SUCCEEDED", "HASH", "BEGIN_TIME", "STATUS", "SIZE")
	var begin, status, hash, size string
	status = "NotStart"
	if apprev.Status.Workflow != nil {
		begin = apprev.Status.Workflow.StartTime.Format("2006-01-02 15:04:05")
		// aggregate workflow result
		switch {
		case apprev.Status.Succeeded:
			status = "Succeeded"
		case apprev.Status.Workflow.Terminated || apprev.Status.Workflow.Suspend || apprev.Status.Workflow.Finished:
			status = "Failed"
		default:
			status = "Executing or Failed"
		}
	}
	if labels := apprev.GetLabels(); labels != nil {
		hash = apprev.GetLabels()[oam.LabelAppRevisionHash]
	}
	if bs, err := yaml.Marshal(apprev.Spec); err == nil {
		size = utils.ByteCountIEC(int64(len(bs)))
	}
	table.AddRow(apprev.Name, oam.GetPublishVersion(apprev.DeepCopy()), apprev.Status.Succeeded, hash, begin, status, size)
	fmt.Fprint(out, table.String())
}
