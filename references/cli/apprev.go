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
	"reflect"
	"strings"

	"github.com/moby/moby/pkg/namesgenerator"
	"github.com/pkg/errors"
	"github.com/spf13/cobra"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	apitypes "k8s.io/apimachinery/pkg/types"

	apicommon "github.com/oam-dev/kubevela/apis/core.oam.dev/common"
	"github.com/oam-dev/kubevela/apis/core.oam.dev/v1beta1"
	"github.com/oam-dev/kubevela/apis/types"
	"github.com/oam-dev/kubevela/pkg/controller/core.oam.dev/v1alpha2/application"
	"github.com/oam-dev/kubevela/pkg/oam"
	"github.com/oam-dev/kubevela/pkg/utils/common"
)

const (
	// FlagResult the result for application revision to filter
	FlagResult = "result"
	// FlagForce force the execution
	FlagForce = "force"
)

// AppRevCommandGroup the commands for managing application revisions
func AppRevCommandGroup(c common.Args) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "apprev",
		Short: "Manage Application Revisions",
		Long:  "Manage KubeVela Application Revisions",
		Annotations: map[string]string{
			types.TagCommandType: types.TypeApp,
		},
	}
	cmd.AddCommand(
		NewAppRevListCommand(c),
		NewDiffCommand(c),
		NewAppRevUnpublishCommand(c),
		NewAppRevPublishCommand(c),
	)
	return cmd
}

// NewAppRevListCommand list the revisions for application
func NewAppRevListCommand(c common.Args) *cobra.Command {
	cmd := &cobra.Command{
		Use:     "list",
		Aliases: []string{"ls"},
		Short:   "list application revisions",
		Long:    "list Kubevela application revisions",
		Args:    cobra.ExactValidArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			result, err := cmd.Flags().GetString(FlagResult)
			if err != nil {
				return err
			}
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
			publishVersion := oam.GetPublishVersion(app)
			if publishVersion == "" {
				cmd.Printf("Application is not running with PublishVersion, the workflow always runs with the latest spec.\n")
				return nil
			}
			revs, err := application.GetSortedAppRevisions(ctx, cli, name, namespace)
			if err != nil {
				return err
			}
			table := newUITable().AddRow("APPREV", "PUBLISH_VERSION", "RESULT", "BEGIN_TIME", "END_TIME")
			formatTime := func(t *metav1.Time) string {
				if t == nil {
					return ""
				}
				return t.Format("2006-01-02 15:04:05")
			}
			for _, rev := range revs {
				if result != "" && string(rev.Status.Workflow.Result) != result {
					continue
				}
				table.AddRow(rev.Name, rev.Status.Workflow.PublishVersion, rev.Status.Workflow.Result, formatTime(rev.Status.Workflow.StartTime), formatTime(rev.Status.Workflow.EndTime))
			}
			if len(table.Rows) == 0 {
				cmd.Printf("No matched application revisions found for application %s/%s.\n", namespace, name)
			} else {
				cmd.Println(table.String())
			}
			return nil
		},
	}
	addNamespaceAndEnvArg(cmd)
	cmd.Flags().StringP(FlagResult, "", "", "If set, only application revisions with the target result will be listed.")
	return cmd
}

// NewAppRevPublishCommand publish a new version for application
func NewAppRevPublishCommand(c common.Args) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "publish APP_NAME [VERSION_NAME]",
		Short: "Publish a new application revision",
		Long:  "Publish a new application revision. If version name is not specified, a docker container-like name will be auto-generated.",
		Args:  cobra.RangeArgs(1, 2),
		RunE: func(cmd *cobra.Command, args []string) error {
			namespace, err := GetFlagNamespaceOrEnv(cmd, c)
			if err != nil {
				return err
			}
			cli, err := c.GetClient()
			if err != nil {
				return err
			}
			force, err := cmd.Flags().GetBool(FlagForce)
			if err != nil {
				return err
			}
			ctx := context.Background()
			name := args[0]
			app := &v1beta1.Application{}
			if err = cli.Get(ctx, apitypes.NamespacedName{Namespace: namespace, Name: name}, app); err != nil {
				return errors.Wrapf(err, "failed to get application %s/%s", namespace, name)
			}
			if !force && app.Status.LatestRevision != nil {
				rev := &v1beta1.ApplicationRevision{}
				if err = cli.Get(ctx, apitypes.NamespacedName{Namespace: namespace, Name: app.Status.LatestRevision.Name}, rev); err != nil {
					return errors.Wrapf(err, "failed to get application revision %s", app.Status.LatestRevision.Name)
				}
				appParser, err := getAppParser(c, cli)
				if err != nil {
					return errors.Wrapf(err, "failed to get app parser")
				}
				app.Status.LatestRevision = nil
				af1, err := appParser.GenerateAppFile(ctx, app)
				if err != nil {
					return errors.Wrapf(err, "failed generating appfile for current application spec")
				}
				af2, err := appParser.GenerateAppFileFromRevision(rev)
				if err != nil {
					return errors.Wrapf(err, "failed generating appfile for latest application revision %s", rev.Name)
				}
				if reflect.DeepEqual(app.Spec, rev.Spec.Application.Spec) &&
					reflect.DeepEqual(af1.WorkflowSteps, af2.WorkflowSteps) &&
					reflect.DeepEqual(af1.Policies, af2.Policies) {
					return errors.Errorf("application is not changed, please use --force to the a non-change publish")
				}
			}
			metav1.SetMetaDataAnnotation(&app.ObjectMeta, oam.AnnotationPublishVersion, strings.ReplaceAll(namesgenerator.GetRandomName(0), "_", "-"))
			if err = cli.Update(ctx, app); err != nil {
				return errors.Wrapf(err, "failed to update application")
			}
			return nil
		},
	}
	addNamespaceAndEnvArg(cmd)
	cmd.Flags().BoolP(FlagForce, "", false, "If set, a new application revision will be published even if application is unchanged")
	return cmd
}

// NewAppRevUnpublishCommand list the revisions for application
func NewAppRevUnpublishCommand(c common.Args) *cobra.Command {
	// TODO: fix! not working!
	cmd := &cobra.Command{
		Use:   "unpublish",
		Short: "Unpublish current application revision",
		Long:  "Unpublish current application revision",
		Args:  cobra.ExactValidArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			namespace, err := GetFlagNamespaceOrEnv(cmd, c)
			if err != nil {
				return err
			}
			cli, err := c.GetClient()
			if err != nil {
				return err
			}
			ctx := context.Background()
			name := args[0]
			app := &v1beta1.Application{}
			if err = cli.Get(ctx, apitypes.NamespacedName{Namespace: namespace, Name: name}, app); err != nil {
				return errors.Wrapf(err, "failed to get application %s/%s", namespace, name)
			}
			var currentRev string
			if app.Status.LatestRevision != nil {
				currentRev = app.Status.LatestRevision.Name
			}
			publishVersion := oam.GetPublishVersion(app)
			if publishVersion == "" {
				cmd.Printf("Application is not running with PublishVersion, the workflow always runs with the latest spec.\n")
				return nil
			}
			revs, err := application.GetSortedAppRevisions(ctx, cli, name, namespace)
			if err != nil {
				return err
			}
			var rollbackRev *v1beta1.ApplicationRevision
			for _, rev := range revs {
				if rev.Name != currentRev && rev.Status.Workflow.Result == types.WorkflowSucceed {
					rollbackRev = rev.DeepCopy()
					break
				}
			}
			app.Spec = rollbackRev.Spec.Application.Spec
			metav1.SetMetaDataAnnotation(&app.ObjectMeta, oam.AnnotationPublishVersion, rollbackRev.Status.Workflow.PublishVersion)
			app.Status.LatestRevision = &apicommon.Revision{Name: rollbackRev.Name}
			app.Status.Workflow = rollbackRev.Status.Workflow.Status
			if err = cli.Update(ctx, app); err != nil {
				return errors.Wrapf(err, "failed to update application")
			}
			return nil
		},
	}
	addNamespaceAndEnvArg(cmd)
	return cmd
}
