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
	"bytes"
	"context"
	"fmt"

	"github.com/pkg/errors"
	"github.com/spf13/cobra"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/oam-dev/kubevela/apis/core.oam.dev/v1beta1"
	"github.com/oam-dev/kubevela/pkg/oam"
	"github.com/oam-dev/kubevela/pkg/oam/discoverymapper"
	"github.com/oam-dev/kubevela/pkg/utils/common"
	cmdutil "github.com/oam-dev/kubevela/pkg/utils/util"
	"github.com/oam-dev/kubevela/references/appfile/dryrun"
)

type livediffCmdOptions struct {
	dryRunCmdOptions
	revision string
	context  int
}

// NewLiveDiffCommand creates `live-diff` command
func NewLiveDiffCommand(c common.Args, ioStreams cmdutil.IOStreams) *cobra.Command {
	o := &livediffCmdOptions{
		dryRunCmdOptions: dryRunCmdOptions{
			IOStreams: ioStreams,
		}}
	cmd := &cobra.Command{
		Use:                   "live-diff",
		DisableFlagsInUseLine: true,
		Short:                 "Dry-run an application, and do diff on a specific app revison",
		Long:                  "Dry-run an application, and do diff on a specific app revison. The provided capability definitions will be used during Dry-run. If any capabilities used in the app are not found in the provided ones, it will try to find from cluster.",
		Example:               "vela live-diff -f app-v2.yaml -r app-v1 --context 10",
		PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
			return c.SetConfig()
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			newClient, err := c.GetClient()
			if err != nil {
				return err
			}

			velaEnv, err := GetEnv(cmd)
			if err != nil {
				return err
			}
			objs := []oam.Object{}
			if o.definitionFile != "" {
				objs, err = ReadObjectsFromFile(o.definitionFile)
				if err != nil {
					return err
				}
			}
			pd, err := c.GetPackageDiscover()
			if err != nil {
				return err
			}

			dm, err := discoverymapper.New(c.Config)
			if err != nil {
				return err
			}

			app, err := readApplicationFromFile(o.applicationFile)
			if err != nil {
				return errors.WithMessagef(err, "read application file: %s", o.applicationFile)
			}
			if app.Namespace == "" {
				app.SetNamespace(velaEnv.Namespace)
			}

			appRevision := &v1beta1.ApplicationRevision{}
			if o.revision != "" {
				// get the revision if user specifies
				if err := newClient.Get(context.Background(),
					client.ObjectKey{Name: o.revision, Namespace: app.Namespace}, appRevision); err != nil {
					return errors.Wrapf(err, "cannot get application revision %q", o.revision)
				}
			} else {
				// get the latest revision of the application
				livingApp := &v1beta1.Application{}
				if err := newClient.Get(context.Background(),
					client.ObjectKey{Name: app.Name, Namespace: app.Namespace}, livingApp); err != nil {
					return errors.Wrapf(err, "cannot get application %q", app.Name)
				}
				if livingApp.Status.LatestRevision != nil {
					latestRevName := livingApp.Status.LatestRevision.Name
					if err := newClient.Get(context.Background(),
						client.ObjectKey{Name: latestRevName, Namespace: app.Namespace}, appRevision); err != nil {
						return errors.Wrapf(err, "cannot get application revision %q", o.revision)
					}
				} else {
					// .status.latestRevision is nil, that means the app has not
					// been rendered yet
					return fmt.Errorf("the application %q has no revision in the cluster", app.Name)
				}
			}

			liveDiffOption := dryrun.NewLiveDiffOption(newClient, dm, pd, objs)
			diffResult, err := liveDiffOption.Diff(context.Background(), app, appRevision)
			if err != nil {
				return errors.WithMessage(err, "cannot calculate diff")
			}

			var buff = bytes.Buffer{}
			reportDiffOpt := dryrun.NewReportDiffOption(o.context, &buff)
			reportDiffOpt.PrintDiffReport(diffResult)
			o.Info(buff.String())
			return nil
		},
	}

	cmd.Flags().StringVarP(&o.applicationFile, "file", "f", "./app.yaml", "application file name")
	cmd.Flags().StringVarP(&o.definitionFile, "definition", "d", "", "specify a file or directory containing capability definitions, they will only be used in dry-run rather than applied to K8s cluster")
	cmd.Flags().StringVarP(&o.revision, "revision", "r", "", "specify an application revision name, by default, it will compare with the latest revision")
	cmd.Flags().IntVarP(&o.context, "context", "c", -1, "output number lines of context around changes, by default show all unchanged lines")
	cmd.SetOut(ioStreams.Out)
	return cmd
}
