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
	"time"

	"github.com/spf13/cobra"
	"k8s.io/cli-runtime/pkg/genericclioptions"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	cmdexec "k8s.io/kubectl/pkg/cmd/exec"
	k8scmdutil "k8s.io/kubectl/pkg/cmd/util"

	"github.com/oam-dev/kubevela/apis/core.oam.dev/v1beta1"
	"github.com/oam-dev/kubevela/apis/types"
	"github.com/oam-dev/kubevela/pkg/multicluster"
	"github.com/oam-dev/kubevela/pkg/utils/common"
	"github.com/oam-dev/kubevela/pkg/utils/util"
	"github.com/oam-dev/kubevela/references/appfile"
)

const (
	podRunningTimeoutFlag = "pod-running-timeout"
	defaultPodExecTimeout = 60 * time.Second
	defaultStdin          = true
	defaultTTY            = true
)

// VelaExecOptions creates options for `exec` command
type VelaExecOptions struct {
	Cmd   *cobra.Command
	Args  []string
	Stdin bool
	TTY   bool

	Ctx   context.Context
	VelaC common.Args
	Env   *types.EnvMeta
	App   *v1beta1.Application

	namespace         string
	resourceName      string
	resourceNamespace string
	f                 k8scmdutil.Factory
	kcExecOptions     *cmdexec.ExecOptions
	ClientSet         kubernetes.Interface
}

// NewExecCommand creates `exec` command
func NewExecCommand(c common.Args, order string, ioStreams util.IOStreams) *cobra.Command {
	o := &VelaExecOptions{
		kcExecOptions: &cmdexec.ExecOptions{
			StreamOptions: cmdexec.StreamOptions{
				IOStreams: genericclioptions.IOStreams{
					In:     ioStreams.In,
					Out:    ioStreams.Out,
					ErrOut: ioStreams.ErrOut,
				},
			},
			Executor: &cmdexec.DefaultRemoteExecutor{},
		},
	}
	cmd := &cobra.Command{
		Use:   "exec [flags] APP_NAME -- COMMAND [args...]",
		Short: "Execute command in a container",
		Long:  "Execute command inside container based vela application.",
		PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
			o.VelaC = c
			return nil
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) < 1 {
				ioStreams.Error("Please specify an application name.")
				return nil
			}
			if len(args) == 1 {
				ioStreams.Error("Please specify at least one command for the container.")
				return nil
			}
			argsLenAtDash := cmd.ArgsLenAtDash()
			if argsLenAtDash != 1 {
				ioStreams.Error("vela exec APP_NAME COMMAND is not supported. Use vela exec APP_NAME -- COMMAND instead.")
				return nil
			}
			var err error
			o.namespace, err = GetFlagNamespaceOrEnv(cmd, c)
			if err != nil {
				return err
			}
			if err := o.Init(context.Background(), cmd, args); err != nil {
				return err
			}
			if err := o.Complete(); err != nil {
				return err
			}
			if err := o.Run(); err != nil {
				return err
			}
			return nil
		},
		Annotations: map[string]string{
			types.TagCommandOrder: order,
			types.TagCommandType:  types.TypeApp,
		},
		Example: `
		# Get output from running 'date' command from app pod, using the first container by default
		vela exec my-app -- date

		# Switch to raw terminal mode, sends stdin to 'bash' in containers of application my-app
		# and sends stdout/stderr from 'bash' back to the client
		vela exec my-app -i -t -- bash -il
		`,
	}
	cmd.Flags().BoolVarP(&o.Stdin, "stdin", "i", defaultStdin, "Pass stdin to the container")
	cmd.Flags().BoolVarP(&o.TTY, "tty", "t", defaultTTY, "Stdin is a TTY")
	cmd.Flags().Duration(podRunningTimeoutFlag, defaultPodExecTimeout,
		"The length of time (like 5s, 2m, or 3h, higher than zero) to wait until at least one pod is running",
	)
	addNamespaceAndEnvArg(cmd)

	return cmd
}

// Init prepares the arguments accepted by the Exec command
func (o *VelaExecOptions) Init(ctx context.Context, c *cobra.Command, argsIn []string) error {
	o.Cmd = c
	o.Args = argsIn

	app, err := appfile.LoadApplication(o.namespace, o.Args[0], o.VelaC)
	if err != nil {
		return err
	}
	o.App = app

	targetResource, err := common.AskToChooseOneEnvResource(o.App)
	if err != nil {
		return err
	}

	cf := genericclioptions.NewConfigFlags(true)
	cf.Namespace = &targetResource.Namespace
	cf.WrapConfigFn = func(cfg *rest.Config) *rest.Config {
		cfg.Wrap(multicluster.NewClusterGatewayRoundTripperWrapperGenerator(targetResource.Cluster))
		return cfg
	}
	o.f = k8scmdutil.NewFactory(k8scmdutil.NewMatchVersionFlags(cf))
	o.resourceName = targetResource.Name
	o.Ctx = multicluster.ContextWithClusterName(ctx, targetResource.Cluster)
	o.resourceNamespace = targetResource.Namespace
	config, err := o.VelaC.GetConfig()
	if err != nil {
		return err
	}
	config.Wrap(multicluster.NewSecretModeMultiClusterRoundTripper)
	k8sClient, err := kubernetes.NewForConfig(config)
	if err != nil {
		return err
	}
	o.ClientSet = k8sClient

	o.kcExecOptions.In = c.InOrStdin()
	o.kcExecOptions.Out = c.OutOrStdout()
	o.kcExecOptions.ErrOut = c.OutOrStderr()
	return nil
}

// Complete loads data from the command environment
func (o *VelaExecOptions) Complete() error {
	podName, err := o.getPodName(o.resourceName)
	if err != nil {
		return err
	}
	o.kcExecOptions.StreamOptions.Stdin = o.Stdin
	o.kcExecOptions.StreamOptions.TTY = o.TTY

	args := make([]string, len(o.Args))
	copy(args, o.Args)
	// args for kcExecOptions MUST be in such format:
	// [podName, COMMAND...]
	args[0] = podName
	return o.kcExecOptions.Complete(o.f, o.Cmd, args, 1)
}

func (o *VelaExecOptions) getPodName(resourceName string) (string, error) {
	return getPodNameForResource(o.Ctx, o.ClientSet, resourceName, o.resourceNamespace)
}

// Run executes a validated remote execution against a pod
func (o *VelaExecOptions) Run() error {
	return o.kcExecOptions.Run()
}
