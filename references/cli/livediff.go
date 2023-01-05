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
	"strings"

	"github.com/pkg/errors"
	"github.com/spf13/cobra"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/oam-dev/kubevela/apis/core.oam.dev/v1beta1"
	"github.com/oam-dev/kubevela/apis/types"
	"github.com/oam-dev/kubevela/pkg/appfile/dryrun"
	"github.com/oam-dev/kubevela/pkg/oam"
	"github.com/oam-dev/kubevela/pkg/oam/discoverymapper"
	"github.com/oam-dev/kubevela/pkg/utils/common"
	cmdutil "github.com/oam-dev/kubevela/pkg/utils/util"
)

// LiveDiffCmdOptions contains the live-diff cmd options
type LiveDiffCmdOptions struct {
	cmdutil.IOStreams
	ApplicationFile   string
	DefinitionFile    string
	AppName           string
	Namespace         string
	Revision          string
	SecondaryRevision string
	Context           int
}

// NewLiveDiffCommand creates `live-diff` command
func NewLiveDiffCommand(c common.Args, order string, ioStreams cmdutil.IOStreams) *cobra.Command {
	o := &LiveDiffCmdOptions{IOStreams: ioStreams}

	cmd := &cobra.Command{
		Use:                   "live-diff",
		DisableFlagsInUseLine: true,
		Short:                 "Compare application and revisions",
		Long:                  "Compare application and revisions",
		Example: "# compare the current application and the running revision\n" +
			"> vela live-diff my-app\n" +
			"# compare the current application and the specified revision\n" +
			"> vela live-diff my-app --revision my-app-v1\n" +
			"# compare two application revisions\n" +
			"> vela live-diff --revision my-app-v1,my-app-v2\n" +
			"# compare the application file and the specified revision\n" +
			"> vela live-diff -f my-app.yaml -r my-app-v1 --context 10",
		Annotations: map[string]string{
			types.TagCommandOrder: order,
			types.TagCommandType:  types.TypeApp,
		},
		Args: cobra.RangeArgs(0, 1),
		RunE: func(cmd *cobra.Command, args []string) (err error) {
			o.Namespace, err = GetFlagNamespaceOrEnv(cmd, c)
			if err != nil {
				return err
			}
			if err = o.loadAndValidate(args); err != nil {
				return err
			}
			buff, err := LiveDiffApplication(o, c)
			if err != nil {
				return err
			}
			cmd.Println(buff.String())
			return nil
		},
	}

	cmd.Flags().StringVarP(&o.ApplicationFile, "file", "f", "", "application file name")
	cmd.Flags().StringVarP(&o.DefinitionFile, "definition", "d", "", "specify a file or directory containing capability definitions, they will only be used in dry-run rather than applied to K8s cluster")
	cmd.Flags().StringVarP(&o.Revision, "revision", "r", "", "specify one or two application revision name(s), by default, it will compare with the latest revision")
	cmd.Flags().IntVarP(&o.Context, "context", "c", -1, "output number lines of context around changes, by default show all unchanged lines")
	addNamespaceAndEnvArg(cmd)
	return cmd
}

// LiveDiffApplication can return user what would change if upgrade an application.
func LiveDiffApplication(cmdOption *LiveDiffCmdOptions, c common.Args) (bytes.Buffer, error) {
	var buff = bytes.Buffer{}

	newClient, err := c.GetClient()
	if err != nil {
		return buff, err
	}
	objs := []oam.Object{}
	if cmdOption.DefinitionFile != "" {
		objs, err = ReadDefinitionsFromFile(cmdOption.DefinitionFile)
		if err != nil {
			return buff, err
		}
	}
	pd, err := c.GetPackageDiscover()
	if err != nil {
		return buff, err
	}
	config, err := c.GetConfig()
	if err != nil {
		return buff, err
	}
	dm, err := discoverymapper.New(config)
	if err != nil {
		return buff, err
	}
	liveDiffOption := dryrun.NewLiveDiffOption(newClient, config, dm, pd, objs)
	if cmdOption.ApplicationFile == "" {
		return cmdOption.renderlessDiff(newClient, liveDiffOption)
	}

	app, err := readApplicationFromFile(cmdOption.ApplicationFile)
	if err != nil {
		return buff, errors.WithMessagef(err, "read application file: %s", cmdOption.ApplicationFile)
	}
	if app.Namespace == "" {
		app.SetNamespace(cmdOption.Namespace)
	}

	appRevision := &v1beta1.ApplicationRevision{}
	if cmdOption.Revision != "" {
		// get the Revision if user specifies
		if err := newClient.Get(context.Background(),
			client.ObjectKey{Name: cmdOption.Revision, Namespace: app.Namespace}, appRevision); err != nil {
			return buff, errors.Wrapf(err, "cannot get application Revision %q", cmdOption.Revision)
		}
	} else {
		// get the latest Revision of the application
		livingApp := &v1beta1.Application{}
		if err := newClient.Get(context.Background(),
			client.ObjectKey{Name: app.Name, Namespace: app.Namespace}, livingApp); err != nil {
			return buff, errors.Wrapf(err, "cannot get application %q", app.Name)
		}
		if livingApp.Status.LatestRevision != nil {
			latestRevName := livingApp.Status.LatestRevision.Name
			if err := newClient.Get(context.Background(),
				client.ObjectKey{Name: latestRevName, Namespace: app.Namespace}, appRevision); err != nil {
				return buff, errors.Wrapf(err, "cannot get application Revision %q", cmdOption.Revision)
			}
		} else {
			// .status.latestRevision is nil, that means the app has not
			// been rendered yet
			return buff, fmt.Errorf("the application %q has no Revision in the cluster", app.Name)
		}
	}

	diffResult, err := liveDiffOption.Diff(context.Background(), app, appRevision)
	if err != nil {
		return buff, errors.WithMessage(err, "cannot calculate diff")
	}

	reportDiffOpt := dryrun.NewReportDiffOption(cmdOption.Context, &buff)
	reportDiffOpt.PrintDiffReport(diffResult)

	return buff, nil
}

func (o *LiveDiffCmdOptions) loadAndValidate(args []string) error {
	if len(args) > 0 {
		o.AppName = args[0]
	}
	revisions := strings.Split(o.Revision, ",")
	if len(revisions) > 2 {
		return errors.Errorf("cannot use more than 2 revisions")
	}
	o.Revision = revisions[0]
	if len(revisions) == 2 {
		o.SecondaryRevision = revisions[1]
	}
	if (o.AppName == "" && len(revisions) == 1) && o.ApplicationFile == "" {
		return errors.Errorf("either application name or application file must be set")
	}
	if (o.AppName != "" || len(revisions) > 1) && o.ApplicationFile != "" {
		return errors.Errorf("cannot set application name and application file at the same time")
	}
	if o.AppName != "" && o.SecondaryRevision != "" {
		return errors.Errorf("cannot use application name and two revisions at the same time")
	}
	if o.SecondaryRevision != "" && o.ApplicationFile != "" {
		return errors.Errorf("cannot use application file and two revisions at the same time")
	}
	return nil
}

func (o *LiveDiffCmdOptions) renderlessDiff(cli client.Client, option *dryrun.LiveDiffOption) (bytes.Buffer, error) {
	var base, comparor dryrun.LiveDiffObject
	ctx := context.Background()
	var buf bytes.Buffer
	if o.AppName != "" {
		app := &v1beta1.Application{}
		if err := cli.Get(ctx, client.ObjectKey{Name: o.AppName, Namespace: o.Namespace}, app); err != nil {
			return buf, errors.Wrapf(err, "cannot get application %s/%s", o.Namespace, o.AppName)
		}
		base = dryrun.LiveDiffObject{Application: app}
		if o.Revision == "" {
			if app.Status.LatestRevision == nil {
				return buf, errors.Errorf("no latest application revision available for application %s/%s", o.Namespace, o.AppName)
			}
			o.Revision = app.Status.LatestRevision.Name
		}
	}
	rev, secondaryRev := &v1beta1.ApplicationRevision{}, &v1beta1.ApplicationRevision{}
	if err := cli.Get(ctx, client.ObjectKey{Name: o.Revision, Namespace: o.Namespace}, rev); err != nil {
		return buf, errors.Wrapf(err, "cannot get application revision %s/%s", o.Namespace, o.Revision)
	}
	if o.SecondaryRevision == "" {
		comparor = dryrun.LiveDiffObject{ApplicationRevision: rev}
	} else {
		if err := cli.Get(ctx, client.ObjectKey{Name: o.SecondaryRevision, Namespace: o.Namespace}, secondaryRev); err != nil {
			return buf, errors.Wrapf(err, "cannot get application revision %s/%s", o.Namespace, o.SecondaryRevision)
		}
		base = dryrun.LiveDiffObject{ApplicationRevision: rev}
		comparor = dryrun.LiveDiffObject{ApplicationRevision: secondaryRev}
	}
	diffResult, err := option.RenderlessDiff(ctx, base, comparor)
	if err != nil {
		return buf, errors.WithMessage(err, "cannot calculate diff")
	}
	reportDiffOpt := dryrun.NewReportDiffOption(o.Context, &buf)
	reportDiffOpt.PrintDiffReport(diffResult)
	return buf, nil
}
