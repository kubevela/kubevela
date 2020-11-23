//nolint:golint
// TODO add lint back
package commands

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/crossplane/oam-kubernetes-runtime/pkg/oam"
	"github.com/spf13/cobra"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/cli-runtime/pkg/genericclioptions"
	"k8s.io/client-go/kubernetes"
	cmdexec "k8s.io/kubectl/pkg/cmd/exec"
	k8scmdutil "k8s.io/kubectl/pkg/cmd/util"

	"github.com/oam-dev/kubevela/api/types"
	"github.com/oam-dev/kubevela/pkg/application"
	"github.com/oam-dev/kubevela/pkg/commands/util"
	velacmdutil "github.com/oam-dev/kubevela/pkg/commands/util"
)

const (
	podRunningTimeoutFlag = "pod-running-timeout"
	defaultPodExecTimeout = 60 * time.Second
	defaultStdin          = true
	defaultTTY            = true
)

type VelaExecOptions struct {
	Cmd   *cobra.Command
	Args  []string
	Stdin bool
	TTY   bool

	context.Context
	VelaC types.Args
	Env   *types.EnvMeta
	App   *application.Application

	f             k8scmdutil.Factory
	kcExecOptions *cmdexec.ExecOptions
	ClientSet     kubernetes.Interface
}

func NewExecCommand(c types.Args, ioStreams velacmdutil.IOStreams) *cobra.Command {
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
		VelaC: c,
	}
	cmd := &cobra.Command{
		Use:   "exec [flags] APP_NAME -- COMMAND [args...]",
		Short: "Execute command in a container",
		Long:  "Execute command in a container",
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) < 1 {
				ioStreams.Error("Please specify an application name.")
				return nil
			}
			argsLenAtDash := cmd.ArgsLenAtDash()
			if argsLenAtDash != 1 {
				ioStreams.Error("Please specify at least one command for the container.")
				return nil
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
			types.TagCommandType: types.TypeApp,
		},
	}
	cmd.Flags().BoolVarP(&o.Stdin, "stdin", "i", defaultStdin, "Pass stdin to the container")
	cmd.Flags().BoolVarP(&o.TTY, "tty", "t", defaultTTY, "Stdin is a TTY")
	cmd.Flags().Duration(podRunningTimeoutFlag, defaultPodExecTimeout,
		"The length of time (like 5s, 2m, or 3h, higher than zero) to wait until at least one pod is running",
	)
	return cmd
}

func (o *VelaExecOptions) Init(ctx context.Context, c *cobra.Command, argsIn []string) error {
	o.Context = ctx
	o.Cmd = c
	o.Args = argsIn

	env, err := GetEnv(o.Cmd)
	if err != nil {
		return err
	}
	app, err := application.Load(env.Name, o.Args[0])
	if err != nil {
		return err
	}
	o.Env = env
	o.App = app

	cf := genericclioptions.NewConfigFlags(true)
	cf.Namespace = &o.Env.Namespace
	o.f = k8scmdutil.NewFactory(k8scmdutil.NewMatchVersionFlags(cf))

	if o.ClientSet == nil {
		c, err := kubernetes.NewForConfig(o.VelaC.Config)
		if err != nil {
			return err
		}
		o.ClientSet = c
	}
	return nil
}

func (o *VelaExecOptions) Complete() error {
	compName, err := util.AskToChooseOneService(o.App.GetComponents())
	if err != nil {
		return err
	}
	podName, err := o.getPodName(compName)
	if err != nil {
		return err
	}
	o.kcExecOptions.StreamOptions.Stdin = o.Stdin
	o.kcExecOptions.StreamOptions.TTY = o.TTY

	args := make([]string, len(o.Args))
	copy(args, o.Args)
	// args for kcExecOptions MUST be in such formart:
	// [podName, COMMAND...]
	args[0] = podName
	return o.kcExecOptions.Complete(o.f, o.Cmd, args, 1)
}

func (o *VelaExecOptions) getPodName(compName string) (string, error) {
	podList, err := o.ClientSet.CoreV1().Pods(o.Env.Namespace).List(o.Context, v1.ListOptions{
		LabelSelector: labels.Set(map[string]string{
			//TODO(roywang) except core workloads, not any workloads will pass these label to pod
			// find a rigorous way to get pod by compname
			oam.LabelAppComponent: compName,
		}).String(),
	})
	if err != nil {
		return "", nil
	}
	if podList != nil && len(podList.Items) == 0 {
		return "", fmt.Errorf("cannot get pods")
	}
	for _, p := range podList.Items {
		if strings.HasPrefix(p.Name, compName+"-") {
			return p.Name, nil
		}
	}
	// if no pod with name matched prefix as component name
	// just return the first one
	return podList.Items[0].Name, nil
}

func (o *VelaExecOptions) Run() error {
	return o.kcExecOptions.Run()
}
