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
	"reflect"
	"strconv"
	"strings"

	"github.com/pkg/errors"
	"github.com/spf13/cobra"
	corev1 "k8s.io/api/core/v1"
	types2 "k8s.io/apimachinery/pkg/types"
	"k8s.io/cli-runtime/pkg/genericclioptions"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/portforward"
	"k8s.io/client-go/transport/spdy"
	cmdpf "k8s.io/kubectl/pkg/cmd/portforward"
	k8scmdutil "k8s.io/kubectl/pkg/cmd/util"
	"k8s.io/utils/pointer"
	"sigs.k8s.io/controller-runtime/pkg/client"

	common2 "github.com/oam-dev/kubevela/apis/core.oam.dev/common"
	"github.com/oam-dev/kubevela/apis/core.oam.dev/v1alpha2"
	"github.com/oam-dev/kubevela/apis/core.oam.dev/v1beta1"
	"github.com/oam-dev/kubevela/apis/types"
	"github.com/oam-dev/kubevela/pkg/multicluster"
	"github.com/oam-dev/kubevela/pkg/utils/common"
	"github.com/oam-dev/kubevela/pkg/utils/util"
	"github.com/oam-dev/kubevela/references/appfile"
)

// VelaPortForwardOptions for vela port-forward
type VelaPortForwardOptions struct {
	Cmd       *cobra.Command
	Args      []string
	ioStreams util.IOStreams

	Ctx            context.Context
	VelaC          common.Args
	Env            *types.EnvMeta
	App            *v1beta1.Application
	targetResource *common2.ClusterObjectReference

	f                    k8scmdutil.Factory
	kcPortForwardOptions *cmdpf.PortForwardOptions
	ClientSet            kubernetes.Interface
	Client               client.Client
	routeTrait           bool
}

// NewPortForwardCommand is vela port-forward command
func NewPortForwardCommand(c common.Args, ioStreams util.IOStreams) *cobra.Command {
	o := &VelaPortForwardOptions{
		ioStreams: ioStreams,
		kcPortForwardOptions: &cmdpf.PortForwardOptions{
			PortForwarder: &defaultPortForwarder{ioStreams},
		},
	}
	cmd := &cobra.Command{
		Use:     "port-forward APP_NAME",
		Short:   "Forward local ports to services in an application",
		Long:    "Forward local ports to services in an application",
		Example: "port-forward APP_NAME [options] [LOCAL_PORT:]REMOTE_PORT [...[LOCAL_PORT_N:]REMOTE_PORT_N]",
		PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
			if err := c.SetConfig(); err != nil {
				return err
			}
			o.VelaC = c
			return nil
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) < 1 {
				ioStreams.Error("Please specify application name.")
				return nil
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
			types.TagCommandType: types.TypeApp,
		},
	}
	cmd.Flags().StringSliceVar(&o.kcPortForwardOptions.Address, "address", []string{"localhost"}, "Addresses to listen on (comma separated). Only accepts IP addresses or localhost as a value. When localhost is supplied, vela will try to bind on both 127.0.0.1 and ::1 and will fail if neither of these addresses are available to bind.")
	cmd.Flags().Duration(podRunningTimeoutFlag, defaultPodExecTimeout,
		"The length of time (like 5s, 2m, or 3h, higher than zero) to wait until at least one pod is running",
	)
	cmd.Flags().BoolVar(&o.routeTrait, "route", false, "forward ports from route trait service")
	cmd.Flags().StringP(FlagNamespace, "n", "", "Specify which namespace to get. If empty, uses namespace in env.")
	return cmd
}

// Init will initialize
func (o *VelaPortForwardOptions) Init(ctx context.Context, cmd *cobra.Command, argsIn []string) error {
	o.Ctx = ctx
	o.Cmd = cmd
	o.Args = argsIn

	namespace, err := cmd.Flags().GetString(FlagNamespace)
	if err != nil {
		return errors.Wrapf(err, "failed to get `%s`", FlagNamespace)
	}
	if namespace == "" {
		env, err := GetFlagEnvOrCurrent(o.Cmd, o.VelaC)
		if err != nil {
			return err
		}
		namespace = env.Namespace
	}
	app, err := appfile.LoadApplication(namespace, o.Args[0], o.VelaC)
	if err != nil {
		return err
	}
	o.App = app

	targetResource, err := common.AskToChooseOnePortForwardEndpoint(o.App)
	if err != nil {
		return err
	}

	cf := genericclioptions.NewConfigFlags(true)
	cf.Namespace = pointer.String(targetResource.Namespace)
	cf.WrapConfigFn = func(cfg *rest.Config) *rest.Config {
		cfg.Wrap(multicluster.NewClusterGatewayRoundTripperWrapperGenerator(targetResource.Cluster))
		return cfg
	}
	o.f = k8scmdutil.NewFactory(k8scmdutil.NewMatchVersionFlags(cf))
	o.targetResource = targetResource
	o.Ctx = multicluster.ContextWithClusterName(ctx, targetResource.Cluster)
	o.VelaC.Config.Wrap(multicluster.NewSecretModeMultiClusterRoundTripper)
	o.VelaC.Client, err = client.New(o.VelaC.Config, client.Options{Scheme: common.Scheme})
	if err != nil {
		return err
	}
	if o.ClientSet == nil {
		c, err := kubernetes.NewForConfig(o.VelaC.Config)
		if err != nil {
			return err
		}
		o.ClientSet = c
	}
	return nil
}

func getRouteServiceName(appconfig *v1alpha2.ApplicationConfiguration, svcName string) string {
	for _, comp := range appconfig.Status.Workloads {
		if comp.ComponentName != svcName {
			continue
		}
		for _, tr := range comp.Traits {
			// TODO check from Capability
			if tr.Reference.Kind == "Route" && tr.Reference.APIVersion == "standard.oam.dev/v1alpha1" {
				return tr.Reference.Name
			}
		}
	}
	return ""
}

// Complete will complete the config of port-forward
func (o *VelaPortForwardOptions) Complete() error {
	compName, err := getCompNameFromClusterObjectReference(o.Ctx, o.VelaC.Client, o.targetResource)
	if err != nil {
		return err
	}
	if compName == "" {
		return fmt.Errorf("failed to get component name")
	}
	if o.routeTrait {
		appconfig, err := appfile.GetAppConfig(o.Ctx, o.VelaC.Client, o.App, o.Env)
		if err != nil {
			return err
		}
		routeSvc := getRouteServiceName(appconfig, compName)
		if routeSvc == "" {
			return fmt.Errorf("no route trait found in %s %s", o.App.Name, compName)
		}
		var svc = corev1.Service{}
		err = o.VelaC.Client.Get(o.Ctx, types2.NamespacedName{Name: routeSvc, Namespace: o.Env.Namespace}, &svc)
		if err != nil {
			return err
		}
		if len(svc.Spec.Ports) == 0 {
			return fmt.Errorf("no port found in service %s", routeSvc)
		}
		val := strconv.Itoa(int(svc.Spec.Ports[0].Port))
		if val == "80" {
			val = "8080:80"
		} else if val == "443" {
			val = "8443:443"
		}
		o.Args = append(o.Args, val)
		args := make([]string, len(o.Args))
		copy(args, o.Args)
		args[0] = "svc/" + routeSvc
		return o.kcPortForwardOptions.Complete(o.f, o.Cmd, args)
	}

	var podName string
	if o.targetResource.Kind == "Service" {
		podName = "svc/" + o.targetResource.Name
	} else {
		podName, err = getPodNameForResource(o.Ctx, o.ClientSet, o.targetResource.Name, o.targetResource.Namespace)
		if err != nil {
			return err
		}
	}
	if len(o.Args) < 2 {
		var found bool
		_, configs := appfile.GetApplicationSettings(o.App, compName)
		for k, v := range configs {
			if k == "port" {
				var val string
				switch pv := v.(type) {
				case int:
					val = strconv.Itoa(pv)
				case string:
					val = pv
				case float64:
					val = strconv.Itoa(int(pv))
				default:
					return fmt.Errorf("invalid type '%s' of port %v", reflect.TypeOf(v), k)
				}
				if val == "80" {
					val = "8080:80"
				} else if val == "443" {
					val = "8443:443"
				}
				o.Args = append(o.Args, val)
				found = true
			}
		}
		if !found {
			return fmt.Errorf("no port found in app or arguments")
		}
	}
	args := make([]string, len(o.Args))
	copy(args, o.Args)
	args[0] = podName
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
