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

	"github.com/AlecAivazis/survey/v2"
	"github.com/pkg/errors"
	"github.com/spf13/cobra"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/cli-runtime/pkg/genericclioptions"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/portforward"
	"k8s.io/client-go/transport/spdy"
	cmdpf "k8s.io/kubectl/pkg/cmd/portforward"
	k8scmdutil "k8s.io/kubectl/pkg/cmd/util"
	"k8s.io/utils/pointer"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/oam-dev/kubevela/apis/core.oam.dev/v1beta1"
	"github.com/oam-dev/kubevela/apis/types"
	"github.com/oam-dev/kubevela/pkg/multicluster"
	"github.com/oam-dev/kubevela/pkg/utils/common"
	"github.com/oam-dev/kubevela/pkg/utils/util"
	types2 "github.com/oam-dev/kubevela/pkg/velaql/providers/query/types"
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
	targetResource *types2.ServiceEndpoint

	f                    k8scmdutil.Factory
	kcPortForwardOptions *cmdpf.PortForwardOptions
	ClientSet            kubernetes.Interface
	Client               client.Client

	namespace string
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

	rawEndpoints, err := GetServiceEndpoints(o.Ctx, o.App.Name, o.namespace, o.VelaC, Filter{})
	if err != nil {
		return err
	}
	var endpoints []types2.ServiceEndpoint
	for _, ep := range rawEndpoints {
		if ep.Ref.Kind != "Service" {
			continue
		}
		endpoints = append(endpoints, ep)
	}
	if len(endpoints) == 0 {
		inSide := func(str string) bool {
			for _, s := range []string{"Deployment", "StatefulSet", "CloneSet", "Job"} {
				if str == s {
					return true
				}
			}
			return false
		}
		for _, ap := range app.Status.AppliedResources {
			if !inSide(ap.Kind) {
				continue
			}
			endpoints = append(endpoints, types2.ServiceEndpoint{
				Endpoint: types2.Endpoint{},
				Ref: corev1.ObjectReference{
					Namespace:  ap.Namespace,
					Name:       ap.Name,
					Kind:       ap.Kind,
					APIVersion: ap.APIVersion,
				},
				Cluster: ap.Cluster,
			})
		}
	}
	targetResource, err := AskToChooseOnePortForwardEndpoint(endpoints)
	if err != nil {
		return err
	}

	cf := genericclioptions.NewConfigFlags(true)
	cf.Namespace = pointer.String(o.namespace)
	cf.WrapConfigFn = func(cfg *rest.Config) *rest.Config {
		cfg.Wrap(multicluster.NewClusterGatewayRoundTripperWrapperGenerator(targetResource.Cluster))
		return cfg
	}
	o.f = k8scmdutil.NewFactory(k8scmdutil.NewMatchVersionFlags(cf))
	o.targetResource = &targetResource
	o.Ctx = multicluster.ContextWithClusterName(ctx, targetResource.Cluster)
	config, err := o.VelaC.GetConfig()
	if err != nil {
		return err
	}
	config.Wrap(multicluster.NewSecretModeMultiClusterRoundTripper)
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

// getPortsFromApp works for compatible
func getPortsFromApp(app *v1beta1.Application) int {
	if app == nil || len(app.Spec.Components) == 0 {
		return 0
	}
	_, configs := appfile.GetApplicationSettings(app, app.Spec.Components[0].Name)
	for k, v := range configs {
		portConv := func(v interface{}) int {
			switch pv := v.(type) {
			case int:
				return pv
			case string:
				data, err := strconv.ParseInt(pv, 10, 64)
				if err != nil {
					return 0
				}
				return int(data)
			case float64:
				return int(pv)
			}
			return 0
		}
		if k == "port" {
			return portConv(v)
		}
		if k == "ports" {
			portArray := v.([]interface{})
			for _, p := range portArray {
				return portConv(p.(map[string]interface{})["port"])
			}
		}
	}
	return 0
}

// Complete will complete the config of port-forward
func (o *VelaPortForwardOptions) Complete() error {

	var forwardTypeName string
	switch o.targetResource.Ref.Kind {
	case "Service":
		forwardTypeName = "svc/" + o.targetResource.Ref.Name
	case "Deployment", "StatefulSet", "CloneSet", "Job":
		var err error
		forwardTypeName, err = getPodNameForResource(o.Ctx, o.ClientSet, o.targetResource.Ref.Name, o.targetResource.Ref.Namespace)
		if err != nil {
			return err
		}
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
		pt := o.targetResource.Endpoint.Port
		if pt == 0 {
			pt = getPortsFromApp(o.App)
		}
		if pt == 0 {
			return errors.New("not port specified for port-forward")
		}
		o.Args = append(o.Args, formatPort(pt))
	}
	args := make([]string, len(o.Args))
	copy(args, o.Args)
	args[0] = forwardTypeName
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

// AskToChooseOnePortForwardEndpoint will ask user to select one applied resource as port forward endpoint
func AskToChooseOnePortForwardEndpoint(endpoints []types2.ServiceEndpoint) (types2.ServiceEndpoint, error) {
	if len(endpoints) == 0 {
		return types2.ServiceEndpoint{}, errors.New("no endpoint found in your application")
	}
	if len(endpoints) == 1 {
		return endpoints[0], nil
	}
	lines := formatEndpoints(endpoints)
	header := strings.Join(lines[0], " | ")
	var ops []string
	for i := 1; i < len(lines); i++ {
		ops = append(ops, strings.Join(lines[i], " | "))
	}
	prompt := &survey.Select{
		Message: fmt.Sprintf("You have %d endpoints in your app. Please choose one:\n%s", len(ops), header),
		Options: ops,
	}
	var selectedRsc string
	err := survey.AskOne(prompt, &selectedRsc)
	if err != nil {
		return types2.ServiceEndpoint{}, fmt.Errorf("choosing endpoint err %w", err)
	}
	for k, resource := range ops {
		if selectedRsc == resource {
			return endpoints[k], nil
		}
	}
	// it should never happen.
	return types2.ServiceEndpoint{}, errors.New("no endpoint match for your choice")
}
