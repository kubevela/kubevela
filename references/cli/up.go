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
	"os"
	"time"

	"github.com/kubevela/pkg/controller/sharding"
	"github.com/pkg/errors"
	"github.com/spf13/cobra"
	apitypes "k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/util/retry"
	"k8s.io/kubectl/pkg/util/i18n"
	"k8s.io/kubectl/pkg/util/templates"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/yaml"

	"github.com/oam-dev/kubevela/apis/core.oam.dev/v1beta1"
	"github.com/oam-dev/kubevela/apis/types"
	velacmd "github.com/oam-dev/kubevela/pkg/cmd"
	cmdutil "github.com/oam-dev/kubevela/pkg/cmd/util"
	"github.com/oam-dev/kubevela/pkg/oam"
	pkgUtils "github.com/oam-dev/kubevela/pkg/utils"
	utilapp "github.com/oam-dev/kubevela/pkg/utils/app"
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
	ShardID        string
	Debug          bool
	Wait           bool
	WaitTimeout    string
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
	if opt.AppName != "" && opt.PublishVersion == "" && opt.ShardID == "" {
		return errors.Errorf("publish-version must be set if you want to force existing application to re-run")
	}
	if opt.AppName == "" && opt.RevisionName != "" {
		return errors.Errorf("revision name must be used with application name")
	}
	if opt.RevisionName != "" && opt.ShardID != "" {
		return errors.Errorf("revision name must be used with shard id")
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
	_, _, err := utilapp.RollbackApplicationWithRevision(ctx, cli, opt.AppName, opt.Namespace, opt.RevisionName, opt.PublishVersion)
	if err != nil {
		return err
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
		if publishVersion := oam.GetPublishVersion(app); publishVersion == opt.PublishVersion && opt.ShardID == "" {
			return errors.Errorf("current PublishVersion is %s", publishVersion)
		}
		if opt.PublishVersion != "" {
			oam.SetPublishVersion(app, opt.PublishVersion)
		}
		if opt.Debug {
			addDebugPolicy(app)
		}
		if err := reschedule(ctx, cli, app, opt.ShardID); err != nil {
			return err
		}
		return cli.Update(ctx, app)
	}); err != nil {
		return err
	}
	if opt.PublishVersion != "" {
		cmd.Printf("Application updated with new PublishVersion %s\n", opt.PublishVersion)
	}
	if opt.ShardID != "" {
		cmd.Printf("Application scheduled to %s\n", opt.ShardID)
	}
	return nil
}

func addDebugPolicy(app *v1beta1.Application) {
	for _, policy := range app.Spec.Policies {
		if policy.Type == "debug" {
			return
		}
	}
	app.Spec.Policies = append(app.Spec.Policies, v1beta1.AppPolicy{
		Name: "debug",
		Type: "debug",
	})
}

func reschedule(ctx context.Context, cli client.Client, app *v1beta1.Application, shardID string) error {
	if shardID != "" {
		sharding.SetScheduledShardID(app, shardID)
		return utilapp.RescheduleAppRevAndRT(ctx, cli, app, shardID)
	}
	return nil
}

func (opt *UpCommandOptions) deployApplicationFromFile(f velacmd.Factory, cmd *cobra.Command) error {
	cli := f.Client()
	body, err := pkgUtils.ReadRemoteOrLocalPath(opt.File, true)
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
		if err = o.Run(opt.File, o.Namespace, utilcommon.Args{Schema: utilcommon.Scheme}); err != nil {
			return err
		}
		opt.AppName = o.Name
	} else {
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
		opt.AppName = app.Name
		if opt.Debug {
			addDebugPolicy(&app)
		}
		if err = reschedule(cmd.Context(), cli, &app, opt.ShardID); err != nil {
			return err
		}
		err = common.ApplyApplication(app, ioStream, cli)
		if err != nil {
			return err
		}
		cmd.Printf("Application %s applied.\n", green.Sprintf("%s/%s", app.Namespace, app.Name))
	}
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
		vela up example-app -n example-ns --publish-version beta --revision example-app-v2

		# Deploy an application with specified shard-id assigned. This can be used to manually re-schedule application.
		vela up example-app --shard-id shard-1

		# Deploy an application from stdin
		cat <<EOF | vela up -f -
        ... <app.yaml here> ...
        EOF
`))
)

// NewUpCommand will create command for applying an AppFile
func NewUpCommand(f velacmd.Factory, order string, c utilcommon.Args, ioStream util.IOStreams) *cobra.Command {
	o := &UpCommandOptions{
		WaitTimeout: "300s",
	}
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
			if o.Debug {
				dOpts := &debugOpts{}
				wargs := &WorkflowArgs{Args: c}
				ctx := context.Background()
				cmdutil.CheckErr(wargs.getWorkflowInstance(ctx, cmd, []string{o.AppName}))
				if wargs.Type == instanceTypeWorkflowRun {
					cmdutil.CheckErr(fmt.Errorf("please use `vela workflow debug <name>` instead"))
				}
				if wargs.App == nil {
					cmdutil.CheckErr(fmt.Errorf("application %s not found", args[0]))
				}
				cmdutil.CheckErr(dOpts.debugApplication(ctx, wargs, c, ioStream))
			}
			if o.Wait {
				dur, err := time.ParseDuration(o.WaitTimeout)
				if err != nil {
					cmdutil.CheckErr(fmt.Errorf("parse timeout duration err: %w", err))
				}
				status, err := printTrackingDeployStatus(c, ioStream, o.AppName, o.Namespace, dur)
				if err != nil {
					cmdutil.CheckErr(err)
				}
				if status != appDeployedHealthy {
					os.Exit(1)
				}
			}
		},
	}
	cmd.Flags().StringVarP(&o.File, "file", "f", o.File, "The file path for appfile or application. It could be a remote url.")
	cmd.Flags().StringVarP(&o.PublishVersion, "publish-version", "v", o.PublishVersion, "The publish version for deploying application.")
	cmd.Flags().StringVarP(&o.RevisionName, "revision", "r", o.RevisionName, "The revision to use for deploying the application, if empty, the current application configuration will be used.")
	cmd.Flags().StringVarP(&o.ShardID, "shard-id", "s", o.ShardID, "The shard id assigned to the application. If empty, it will not be used.")
	cmd.Flags().BoolVarP(&o.Debug, "debug", "", o.Debug, "Enable debug mode for application")
	cmd.Flags().BoolVarP(&o.Wait, "wait", "w", o.Wait, "Wait app to be healthy until timout, if no timeout specified, the default duration is 300s.")
	cmd.Flags().StringVarP(&o.WaitTimeout, "timeout", "", o.WaitTimeout, "Set the timout for wait app to be healthy, if not specified, the default duration is 300s.")
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
