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
	"net/http"
	"net/url"
	"strconv"
	"strings"

	"github.com/pkg/errors"
	"github.com/spf13/cobra"
	"k8s.io/cli-runtime/pkg/genericclioptions"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/portforward"
	"k8s.io/client-go/transport/spdy"
	cmdpf "k8s.io/kubectl/pkg/cmd/portforward"
	k8scmdutil "k8s.io/kubectl/pkg/cmd/util"
	"k8s.io/utils/pointer"
	"sigs.k8s.io/controller-runtime/pkg/client"

	pkgmulticluster "github.com/kubevela/pkg/multicluster"

	"github.com/oam-dev/kubevela/apis/core.oam.dev/v1beta1"
	"github.com/oam-dev/kubevela/apis/types"
	"github.com/oam-dev/kubevela/pkg/multicluster"
	"github.com/oam-dev/kubevela/pkg/utils/common"
	"github.com/oam-dev/kubevela/pkg/utils/util"
	querytypes "github.com/oam-dev/kubevela/pkg/velaql/providers/query/types"
	"github.com/oam-dev/kubevela/references/appfile"
)

// VelaPortForwardOptions for vela port-forward
type VelaPortForwardOptions struct {
	Cmd           *cobra.Command
	Args          []string
	ioStreams     util.IOStreams
	ClusterName   string
	ComponentName string
	ResourceName  string
	ResourceType  string

	Ctx   context.Context
	VelaC common.Args

	namespace      string
	App            *v1beta1.Application
	targetResource struct {
		kind      string
		name      string
		cluster   string
		namespace string
	}
	targetPort int

	f                    k8scmdutil.Factory
	kcPortForwardOptions *cmdpf.PortForwardOptions
	ClientSet            kubernetes.Interface
	Client               client.Client
}

// NewPortForwardCommand is vela port-forward command
func NewPortForwardCommand(c common.Args, order string, ioStreams util.IOStreams) *cobra.Command {
	o := &VelaPortForwardOptions{
		ioStreams: ioStreams,
		kcPortForwardOptions: &cmdpf.PortForwardOptions{
			PortForwarder: &defaultPortForwarder{ioStreams},
		},
	}
	cmd := &cobra.Command{
		Use:     "port-forward APP_NAME",
		Short:   "Forward local ports to container/service port of vela application.",
		Long:    "Forward local ports to container/service port of vela application.",
		Example: "port-forward APP_NAME [options] [LOCAL_PORT:]REMOTE_PORT [...[LOCAL_PORT_N:]REMOTE_PORT_N]",
		PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
			o.VelaC = c
			return nil
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) < 1 {
				ioStreams.Error("Please specify application name.")
				return nil
			}
			if o.ResourceType != "pod" && o.ResourceType != "service" {
				o.ResourceType = "service"
			}
			if o.ResourceType == "pod" && len(args) < 2 {
				return errors.New("not port specified for port-forward")
			}
			var err error
			o.namespace, err = GetFlagNamespaceOrEnv(cmd, c)
			if err != nil {
				return err
			}

			newClient, err := o.VelaC.GetClient()
			if err != nil {
				return err
			}
			o.Client = newClient
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
	}

	cmd.Flags().StringSliceVar(&o.kcPortForwardOptions.Address, "address", []string{"localhost"}, "Addresses to listen on (comma separated). Only accepts IP addresses or localhost as a value. When localhost is supplied, vela will try to bind on both 127.0.0.1 and ::1 and will fail if neither of these addresses are available to bind.")
	cmd.Flags().Duration(podRunningTimeoutFlag, defaultPodExecTimeout,
		"The length of time (like 5s, 2m, or 3h, higher than zero) to wait until at least one pod is running",
	)
	cmd.Flags().StringVarP(&o.ComponentName, "component", "c", "", "filter the pod by the component name")
	cmd.Flags().StringVarP(&o.ClusterName, "cluster", "", "", "filter the pod by the cluster name")
	cmd.Flags().StringVarP(&o.ResourceName, "resource-name", "", "", "specify the resource name")
	cmd.Flags().StringVarP(&o.ResourceType, "resource-type", "t", "", "specify the resource type, support the service, and pod")
	addNamespaceAndEnvArg(cmd)
	return cmd
}

// Init will initialize
func (o *VelaPortForwardOptions) Init(ctx context.Context, cmd *cobra.Command, argsIn []string) error {
	o.Ctx = ctx
	o.Cmd = cmd
	o.Args = argsIn

	app, err := appfile.LoadApplication(o.namespace, o.Args[0], o.VelaC)
	if err != nil {
		return err
	}
	o.App = app

	if o.ResourceType == "service" {
		var selectService *querytypes.ResourceItem
		services, err := GetApplicationServices(o.Ctx, o.App.Name, o.namespace, o.VelaC, Filter{
			Component: o.ComponentName,
			Cluster:   o.ClusterName,
		})
		if err != nil {
			return fmt.Errorf("failed to load the application services: %w", err)
		}

		if o.ResourceName != "" {
			for i, service := range services {
				if service.Object.GetName() == o.ResourceName {
					selectService = &services[i]
					break
				}
			}
			if selectService == nil {
				fmt.Println("The Service you specified does not exist, please select it from the list.")
			}
		}
		if len(services) > 0 {
			if selectService == nil {
				selectService, o.targetPort, err = AskToChooseOneService(services, len(o.Args) < 2)
				if err != nil {
					return err
				}
			}
			if selectService != nil {
				o.targetResource.cluster = selectService.Cluster
				o.targetResource.name = selectService.Object.GetName()
				o.targetResource.namespace = selectService.Object.GetNamespace()
				o.targetResource.kind = selectService.Object.GetKind()
			}
		} else if o.ResourceName == "" {
			// If users do not specify the resource name and there is no service, switch to query the pod
			o.ResourceType = "pod"
		}
	}

	if o.ResourceType == "pod" {
		var selectPod *querytypes.PodBase
		pods, err := GetApplicationPods(o.Ctx, o.App.Name, o.namespace, o.VelaC, Filter{
			Component: o.ComponentName,
			Cluster:   o.ClusterName,
		})
		if err != nil {
			return fmt.Errorf("failed to load the application services: %w", err)
		}

		if o.ResourceName != "" {
			for i, pod := range pods {
				if pod.Metadata.Name == o.ResourceName {
					selectPod = &pods[i]
					break
				}
			}
			if selectPod == nil {
				fmt.Println("The Service you specified does not exist, please select it from the list.")
			}
		}
		if selectPod == nil {
			selectPod, err = AskToChooseOnePod(pods)
			if err != nil {
				return err
			}
		}
		if selectPod != nil {
			o.targetResource.cluster = selectPod.Cluster
			o.targetResource.name = selectPod.Metadata.Name
			o.targetResource.namespace = selectPod.Metadata.Namespace
			o.targetResource.kind = "Pod"
		}
	}

	cf := genericclioptions.NewConfigFlags(true)
	cf.Namespace = pointer.String(o.targetResource.namespace)
	cf.WrapConfigFn = func(cfg *rest.Config) *rest.Config {
		cfg.Wrap(pkgmulticluster.NewTransportWrapper(pkgmulticluster.ForCluster(o.targetResource.cluster)))
		return cfg
	}
	o.f = k8scmdutil.NewFactory(k8scmdutil.NewMatchVersionFlags(cf))
	o.Ctx = multicluster.ContextWithClusterName(ctx, o.targetResource.cluster)
	config, err := o.VelaC.GetConfig()
	if err != nil {
		return err
	}
	config.Wrap(pkgmulticluster.NewTransportWrapper())
	forwardClient, err := client.New(config, client.Options{Scheme: common.Scheme})
	if err != nil {
		return err
	}
	o.VelaC.SetClient(forwardClient)
	if o.ClientSet == nil {
		c, err := kubernetes.NewForConfig(config)
		if err != nil {
			return err
		}
		o.ClientSet = c
	}
	return nil
}

// Complete will complete the config of port-forward
func (o *VelaPortForwardOptions) Complete() error {
	var forwardTypeName string
	switch o.targetResource.kind {
	case "Service":
		forwardTypeName = "svc/" + o.targetResource.name
	case "Pod":
		forwardTypeName = "pod/" + o.targetResource.name
	}

	if len(o.Args) < 2 {
		formatPort := func(p int) string {
			val := strconv.Itoa(p)
			if val == "80" {
				val = "8080:80"
			} else if val == "443" {
				val = "8443:443"
			}
			return val
		}
		pt := o.targetPort
		if pt == 0 {
			return errors.New("not port specified for port-forward")
		}
		o.Args = append(o.Args, formatPort(pt))
	}
	args := make([]string, len(o.Args))
	copy(args, o.Args)
	args[0] = forwardTypeName
	o.kcPortForwardOptions.Namespace = o.targetResource.namespace
	o.ioStreams.Infof("trying to connect the remote endpoint %s ..", strings.Join(args, " "))
	return o.kcPortForwardOptions.Complete(o.f, o.Cmd, args)
}

// Run will execute port-forward
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
	util.IOStreams
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
