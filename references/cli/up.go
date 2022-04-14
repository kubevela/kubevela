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
	"github.com/pkg/errors"
	"github.com/spf13/cobra"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	apitypes "k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/util/retry"
	cmdutil "k8s.io/kubectl/pkg/cmd/util"
	"k8s.io/kubectl/pkg/util/i18n"
	"k8s.io/kubectl/pkg/util/templates"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/yaml"

	apicommon "github.com/oam-dev/kubevela/apis/core.oam.dev/common"
	"github.com/oam-dev/kubevela/apis/core.oam.dev/v1beta1"
	"github.com/oam-dev/kubevela/apis/types"
	velacmd "github.com/oam-dev/kubevela/pkg/cmd"
	"github.com/oam-dev/kubevela/pkg/component"
	"github.com/oam-dev/kubevela/pkg/controller/core.oam.dev/v1alpha2/application"
	"github.com/oam-dev/kubevela/pkg/controller/utils"
	"github.com/oam-dev/kubevela/pkg/oam"
	utilcommon "github.com/oam-dev/kubevela/pkg/utils/common"
	"github.com/oam-dev/kubevela/pkg/utils/util"
	"github.com/oam-dev/kubevela/references/common"
)

// UpCommandOptions command args for vela up
type UpCommandOptions struct {
	AppName        string
	Namespace      string
	File           string
	PublishVersion string
	RevisionName   string
}

// Complete fill the args for vela up
func (opt *UpCommandOptions) Complete(f velacmd.Factory, cmd *cobra.Command, args []string) {
	if len(args) > 0 {
		opt.AppName = args[0]
	}
	opt.Namespace = velacmd.GetNamespace(f, cmd)
}

// Validate if vela up args is valid, interrupt the command
func (opt *UpCommandOptions) Validate() error {
	if opt.AppName != "" && opt.File != "" {
		return errors.Errorf("cannot use app name and file at the same time")
	}
	if opt.AppName == "" && opt.File == "" {
		return errors.Errorf("either app name or file should be set")
	}
	if opt.AppName != "" && opt.PublishVersion == "" {
		return errors.Errorf("publish-version must be set if you want to force existing application to re-run")
	}
	if opt.AppName == "" && opt.RevisionName != "" {
		return errors.Errorf("revision name must be used with application name")
	}
	return nil
}

// Run execute the vela up command
func (opt *UpCommandOptions) Run(f velacmd.Factory, cmd *cobra.Command) error {
	if opt.File != "" {
		return opt.deployApplicationFromFile(f, cmd)
	}
	if opt.RevisionName == "" {
		return opt.deployExistingApp(f, cmd)
	}
	return opt.deployExistingAppUsingRevision(f, cmd)
}

func (opt *UpCommandOptions) deployExistingAppUsingRevision(f velacmd.Factory, cmd *cobra.Command) error {
	ctx, cli := cmd.Context(), f.Client()
	app := &v1beta1.Application{}
	if err := cli.Get(ctx, apitypes.NamespacedName{Name: opt.AppName, Namespace: opt.Namespace}, app); err != nil {
		return err
	}
	if publishVersion := oam.GetPublishVersion(app); publishVersion == opt.PublishVersion {
		return errors.Errorf("current PublishVersion is %s", publishVersion)
	}
	// check revision
	revs, err := application.GetSortedAppRevisions(ctx, cli, opt.AppName, opt.Namespace)
	if err != nil {
		return err
	}
	var matchedRev *v1beta1.ApplicationRevision
	for _, rev := range revs {
		if rev.Name == opt.RevisionName {
			matchedRev = rev.DeepCopy()
		}
	}
	if matchedRev == nil {
		return errors.Errorf("failed to find revision %s matching application %s", opt.RevisionName, opt.AppName)
	}
	if app.Status.LatestRevision != nil && app.Status.LatestRevision.Name == opt.RevisionName {
		return nil
	}

	// freeze the application
	appKey := client.ObjectKeyFromObject(app)
	controllerRequirement, err := utils.FreezeApplication(ctx, cli, app, func() {
		app.Spec = matchedRev.Spec.Application.Spec
		oam.SetPublishVersion(app, opt.PublishVersion)
	})
	if err != nil {
		return errors.Wrapf(err, "failed to freeze application %s before update", appKey)
	}

	// create new revision based on the matched revision
	revName, revisionNum := utils.GetAppNextRevision(app)
	matchedRev.Name = revName
	oam.SetPublishVersion(matchedRev, opt.PublishVersion)
	obj, err := runtime.DefaultUnstructuredConverter.ToUnstructured(matchedRev)
	if err != nil {
		return err
	}
	un := &unstructured.Unstructured{Object: obj}
	component.ClearRefObjectForDispatch(un)
	if err = cli.Create(ctx, un); err != nil {
		return errors.Wrapf(err, "failed to update application %s to create new revision %s", appKey, revName)
	}

	// update application status to point to the new revision
	if err = retry.RetryOnConflict(retry.DefaultBackoff, func() error {
		if err = cli.Get(ctx, appKey, app); err != nil {
			return err
		}
		app.Status = apicommon.AppStatus{
			LatestRevision: &apicommon.Revision{Name: revName, Revision: revisionNum, RevisionHash: matchedRev.GetLabels()[oam.LabelAppRevisionHash]},
		}
		return cli.Status().Update(ctx, app)
	}); err != nil {
		return errors.Wrapf(err, "failed to update application %s to use new revision %s", appKey, revName)
	}

	// unfreeze application
	if err = utils.UnfreezeApplication(ctx, cli, app, nil, controllerRequirement); err != nil {
		return errors.Wrapf(err, "failed to unfreeze application %s after update", appKey)
	}

	cmd.Printf("Application updated with new PublishVersion %s using revision %s\n", opt.PublishVersion, opt.RevisionName)
	return nil
}

func (opt *UpCommandOptions) deployExistingApp(f velacmd.Factory, cmd *cobra.Command) error {
	ctx, cli := cmd.Context(), f.Client()
	app := &v1beta1.Application{}
	if err := retry.RetryOnConflict(retry.DefaultBackoff, func() error {
		if err := cli.Get(ctx, apitypes.NamespacedName{Name: opt.AppName, Namespace: opt.Namespace}, app); err != nil {
			return err
		}
		if publishVersion := oam.GetPublishVersion(app); publishVersion == opt.PublishVersion {
			return errors.Errorf("current PublishVersion is %s", publishVersion)
		}
		oam.SetPublishVersion(app, opt.PublishVersion)
		return cli.Update(ctx, app)
	}); err != nil {
		return err
	}
	cmd.Printf("Application updated with new PublishVersion %s\n", opt.PublishVersion)
	return nil
}

func (opt *UpCommandOptions) deployApplicationFromFile(f velacmd.Factory, cmd *cobra.Command) error {
	cli := f.Client()
	body, err := common.ReadRemoteOrLocalPath(opt.File)
	if err != nil {
		return err
	}
	ioStream := util.IOStreams{
		In:     cmd.InOrStdin(),
		Out:    cmd.OutOrStdout(),
		ErrOut: cmd.ErrOrStderr(),
	}
	if common.IsAppfile(body) { // legacy compatibility
		o := &common.AppfileOptions{Kubecli: cli, IO: ioStream, Namespace: opt.Namespace}
		return o.Run(opt.File, o.Namespace, utilcommon.Args{Schema: utilcommon.Scheme})
	}
	var app v1beta1.Application
	err = yaml.Unmarshal(body, &app)
	if err != nil {
		return errors.Wrap(err, "File format is illegal, only support vela appfile format or OAM Application object yaml")
	}

	// Override namespace if namespace flag is set. We should check if namespace is `default` or not
	// since GetFlagNamespaceOrEnv returns default namespace when failed to get current env.
	if opt.Namespace != "" && opt.Namespace != types.DefaultAppNamespace {
		app.SetNamespace(opt.Namespace)
	}
	if opt.PublishVersion != "" {
		oam.SetPublishVersion(&app, opt.PublishVersion)
	}
	err = common.ApplyApplication(app, ioStream, cli)
	if err != nil {
		return err
	}
	cmd.Printf("Application %s/%s applied.\n", app.Namespace, app.Name)
	return nil
}

var (
	upLong = templates.LongDesc(i18n.T(`
		Deploy one application

		Deploy one application based on local files or re-deploy an existing application.
		With the -n/--namespace flag, you can choose the location of the target application.

		To apply application from file, use the -f/--file flag to specify the application 
		file location.

		To give a particular version to this deploy, use the -v/--publish-version flag. When
		you are deploying an existing application, the version name must be different from
		the current name. You can also use a history revision for the deploy and override the
		current application by using the -r/--revision flag.`))

	upExample = templates.Examples(i18n.T(`
		# Deploy an application from file
		vela up -f ./app.yaml

		# Deploy an application with a version name
		vela up example-app -n example-ns --publish-version beta

		# Deploy an application using existing revision
		vela up example-app -n example-ns --publish-version beta --revision example-app-v2`))
)

// NewUpCommand will create command for applying an AppFile
func NewUpCommand(f velacmd.Factory, order string) *cobra.Command {
	o := &UpCommandOptions{}
	cmd := &cobra.Command{
		Use:                   "up",
		DisableFlagsInUseLine: true,
		Short:                 i18n.T("Deploy one application"),
		Long:                  upLong,
		Example:               upExample,
		Annotations: map[string]string{
			types.TagCommandOrder: order,
			types.TagCommandType:  types.TypeStart,
		},
		Args: cobra.RangeArgs(0, 1),
		ValidArgsFunction: func(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
			o.Complete(f, cmd, args)
			if o.File == "" {
				return velacmd.GetApplicationsForCompletion(cmd.Context(), f, o.Namespace, toComplete)
			}
			return nil, cobra.ShellCompDirectiveDefault
		},
		Run: func(cmd *cobra.Command, args []string) {
			o.Complete(f, cmd, args)
			cmdutil.CheckErr(o.Validate())
			cmdutil.CheckErr(o.Run(f, cmd))
		},
	}
	cmd.Flags().StringVarP(&o.File, "file", "f", o.File, "The file path for appfile or application. It could be a remote url.")
	cmd.Flags().StringVarP(&o.PublishVersion, "publish-version", "v", o.PublishVersion, "The publish version for deploying application.")
	cmd.Flags().StringVarP(&o.RevisionName, "revision", "r", o.RevisionName, "The revision to use for deploying the application, if empty, the current application configuration will be used.")
	cmdutil.CheckErr(cmd.RegisterFlagCompletionFunc(
		"revision",
		func(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
			var appName string
			if len(args) > 0 {
				appName = args[0]
			}
			namespace := velacmd.GetNamespace(f, cmd)
			return velacmd.GetRevisionForCompletion(cmd.Context(), f, appName, namespace, toComplete)
		}))

	return velacmd.NewCommandBuilder(f, cmd).
		WithNamespaceFlag().
		WithResponsiveWriter().
		Build()
}
