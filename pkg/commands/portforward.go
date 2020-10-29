package commands

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"strings"

	"github.com/crossplane/oam-kubernetes-runtime/pkg/oam"
	"github.com/oam-dev/kubevela/api/types"
	"github.com/spf13/cobra"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/cli-runtime/pkg/genericclioptions"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/portforward"
	"k8s.io/client-go/transport/spdy"

	"github.com/oam-dev/kubevela/pkg/application"
	"github.com/oam-dev/kubevela/pkg/commands/util"
	velacmdutil "github.com/oam-dev/kubevela/pkg/commands/util"
	cmdpf "k8s.io/kubectl/pkg/cmd/portforward"
	k8scmdutil "k8s.io/kubectl/pkg/cmd/util"
)

type VelaPortForwardOptions struct {
	Cmd       *cobra.Command
	Args      []string
	ioStreams velacmdutil.IOStreams

	context.Context
	VelaC types.Args
	Env   *types.EnvMeta
	App   *application.Application

	f                    k8scmdutil.Factory
	kcPortForwardOptions *cmdpf.PortForwardOptions
	ClientSet            kubernetes.Interface
}

func NewPortForwardCommand(c types.Args, ioStreams velacmdutil.IOStreams) *cobra.Command {
	o := &VelaPortForwardOptions{
		VelaC:     c,
		ioStreams: ioStreams,
		kcPortForwardOptions: &cmdpf.PortForwardOptions{
			PortForwarder: &defaultPortForwarder{ioStreams},
		},
	}
	cmd := &cobra.Command{
		Use:   "port-forward APP_NAME [options] [LOCAL_PORT:]REMOTE_PORT [...[LOCAL_PORT_N:]REMOTE_PORT_N]",
		Short: "Forward one or more local ports to a Pod of a service in an application",
		Long:  "Forward one or more local ports to a Pod of a service in an application",
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) < 2 {
				ioStreams.Error("Please specify application name and list of ports.")
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
	cmd.Flags().StringSliceVar(&o.kcPortForwardOptions.Address, "address", []string{"localhost"}, "Addresses to listen on (comma separated). Only accepts IP addresses or localhost as a value. When localhost is supplied, vela will try to bind on both 127.0.0.1 and ::1 and will fail if neither of these addresses are available to bind.")
	cmd.Flags().Duration(podRunningTimeoutFlag, defaultPodExecTimeout,
		"The length of time (like 5s, 2m, or 3h, higher than zero) to wait until at least one pod is running",
	)
	return cmd
}

func (o *VelaPortForwardOptions) Init(ctx context.Context, cmd *cobra.Command, argsIn []string) error {
	o.Context = ctx
	o.Cmd = cmd
	o.Args = argsIn

	env, err := GetEnv(o.Cmd)
	if err != nil {
		return err
	}
	o.Env = env

	app, err := application.Load(env.Name, o.Args[0])
	if err != nil {
		return err
	}
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

func (o *VelaPortForwardOptions) Complete() error {
	svcName, err := util.AskToChooseOneService(o.App.GetComponents())
	if err != nil {
		return err
	}
	podName, err := o.getPodName(svcName)
	if err != nil {
		return err
	}

	args := make([]string, len(o.Args))
	copy(args, o.Args)
	args[0] = podName
	return o.kcPortForwardOptions.Complete(o.f, o.Cmd, args)
}

func (o *VelaPortForwardOptions) getPodName(svcName string) (string, error) {
	podList, err := o.ClientSet.CoreV1().Pods(o.Env.Namespace).List(o.Context, v1.ListOptions{
		LabelSelector: labels.Set(map[string]string{
			oam.LabelAppComponent: svcName,
		}).String(),
	})
	if err != nil {
		return "", err
	}
	if podList != nil && len(podList.Items) == 0 {
		return "", fmt.Errorf("cannot get pods")
	}
	for _, p := range podList.Items {
		if strings.HasPrefix(p.Name, svcName+"-") {
			return p.Name, nil
		}
	}
	return podList.Items[0].Name, nil
}

func (o *VelaPortForwardOptions) Run() error {
	go func() {
		<-o.kcPortForwardOptions.ReadyChannel
		o.ioStreams.Info("\nForward successfully! Opening browser ...")
		local, _ := splitPort(o.Args[1])
		var url = "http://127.0.0.1:" + local
		if err := OpenBrowser(url); err != nil {
			o.ioStreams.Errorf("\nFailed to open browser: %v", err)
		}
	}()

	return o.kcPortForwardOptions.RunPortForward()
}

func splitPort(port string) (local, remote string) {
	parts := strings.Split(port, ":")
	if len(parts) == 2 {
		return parts[0], parts[1]
	}
	return parts[0], parts[0]
}

type defaultPortForwarder struct {
	velacmdutil.IOStreams
}

func (f *defaultPortForwarder) ForwardPorts(method string, url *url.URL, opts cmdpf.PortForwardOptions) error {
	transport, upgrader, err := spdy.RoundTripperFor(opts.Config)
	if err != nil {
		return err
	}
	dialer := spdy.NewDialer(upgrader, &http.Client{Transport: transport}, method, url)
	fw, err := portforward.NewOnAddresses(dialer, opts.Address, opts.Ports, opts.StopChannel, opts.ReadyChannel, f.Out, f.ErrOut)
	if err != nil {
		return err
	}
	return fw.ForwardPorts()
}
