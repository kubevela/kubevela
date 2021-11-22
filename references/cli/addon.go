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
	"text/template"
	"time"

	yaml2 "k8s.io/apimachinery/pkg/util/yaml"

	"github.com/oam-dev/kubevela/pkg/oam/util"

	common2 "github.com/oam-dev/kubevela/apis/core.oam.dev/common"

	"github.com/Masterminds/sprig"
	"github.com/gosuri/uitable"
	"github.com/pkg/errors"
	"github.com/spf13/cobra"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	types2 "k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/wait"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/oam-dev/kubevela/apis/core.oam.dev/v1beta1"
	"github.com/oam-dev/kubevela/apis/types"
	"github.com/oam-dev/kubevela/pkg/oam"
	"github.com/oam-dev/kubevela/pkg/utils/apply"
	"github.com/oam-dev/kubevela/pkg/utils/common"
	cmdutil "github.com/oam-dev/kubevela/pkg/utils/util"
)

const (
	// DescAnnotation records the description of addon
	DescAnnotation = "addons.oam.dev/description"

	// DependsOnWorkFlowStepName is workflow step name which is used to check dependsOn app
	DependsOnWorkFlowStepName = "depends-on-app"
)

var statusUninstalled = "uninstalled"
var statusInstalled = "installed"
var clt client.Client
var clientArgs common.Args

var legacyAddonNamespace map[string]string

func init() {
	clientArgs, _ = common.InitBaseRestConfig()
	clt, _ = clientArgs.GetClient()
	legacyAddonNamespace = map[string]string{
		"fluxcd":              types.DefaultKubeVelaNS,
		"ns-flux-system":      types.DefaultKubeVelaNS,
		"kruise":              types.DefaultKubeVelaNS,
		"prometheus":          types.DefaultKubeVelaNS,
		"observability":       "observability",
		"observability-asset": types.DefaultKubeVelaNS,
		"istio":               "istio-system",
		"ns-istio-system":     types.DefaultKubeVelaNS,
		"keda":                types.DefaultKubeVelaNS,
		"ocm-cluster-manager": types.DefaultKubeVelaNS,
		"terraform":           types.DefaultKubeVelaNS,
		"terraform-alibaba":   "default",
		"terraform-azure":     "default",
	}
}

// NewAddonCommand create `addon` command
func NewAddonCommand(c common.Args, ioStreams cmdutil.IOStreams) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "addon",
		Short: "List and get addon in KubeVela",
		Long:  "List and get addon in KubeVela",
		Annotations: map[string]string{
			types.TagCommandType: types.TypeSystem,
		},
	}
	cmd.AddCommand(
		NewAddonListCommand(),
		NewAddonEnableCommand(ioStreams),
		NewAddonDisableCommand(ioStreams),
	)
	return cmd
}

// NewAddonListCommand create addon list command
func NewAddonListCommand() *cobra.Command {
	return &cobra.Command{
		Use:     "list",
		Aliases: []string{"ls"},
		Short:   "List addons",
		Long:    "List addons in KubeVela",
		RunE: func(cmd *cobra.Command, args []string) error {
			err := listAddons()
			if err != nil {
				return err
			}
			return nil
		},
	}
}

// NewAddonEnableCommand create addon enable command
func NewAddonEnableCommand(ioStream cmdutil.IOStreams) *cobra.Command {
	return &cobra.Command{
		Use:     "enable",
		Short:   "enable an addon",
		Long:    "enable an addon in cluster",
		Example: "vela addon enable <addon-name>",
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) < 1 {
				return fmt.Errorf("must specify addon name")
			}
			name := args[0]
			addonArgs, err := parseToMap(args[1:])
			if err != nil {
				return err
			}
			err = enableAddon(name, addonArgs)
			if err != nil {
				return err
			}
			fmt.Printf("Successfully enable addon:%s\n", name)
			return nil
		},
	}
}

func parseToMap(args []string) (map[string]string, error) {
	res := map[string]string{}
	for _, pair := range args {
		line := strings.Split(pair, "=")
		if len(line) != 2 {
			return nil, fmt.Errorf("parameter format should be foo=bar, %s not match", pair)
		}
		k := strings.TrimSpace(line[0])
		v := strings.TrimSpace(line[1])
		if k != "" && v != "" {
			res[k] = v
		}
	}
	return res, nil
}

// NewAddonDisableCommand create addon disable command
func NewAddonDisableCommand(ioStream cmdutil.IOStreams) *cobra.Command {
	return &cobra.Command{
		Use:     "disable",
		Short:   "disable an addon",
		Long:    "disable an addon in cluster",
		Example: "vela addon disable <addon-name>",
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) < 1 {
				return fmt.Errorf("must specify addon name")
			}
			name := args[0]
			err := disableAddon(name)
			if err != nil {
				return err
			}
			fmt.Printf("Successfully disable addon:%s\n", name)
			return nil
		},
	}
}

func listAddons() error {
	repo, err := NewAddonRepo()
	if err != nil {
		return err
	}
	addons := repo.listAddons()
	table := uitable.New()
	table.AddRow("NAME", "DESCRIPTION", "STATUS")
	for _, addon := range addons {
		// Addon terraform should be invisible to end-users. It will be installed by other addons like `terraform-alibaba`
		if addon.name == "terraform" {
			continue
		}
		table.AddRow(addon.name, addon.description, addon.getStatus())
	}
	fmt.Println(table.String())
	return nil
}

func enableAddon(name string, args map[string]string) error {
	repo, err := NewAddonRepo()
	if err != nil {
		return err
	}
	addon, err := repo.getAddon(name)
	if err != nil {
		return err
	}
	addon.setArgs(args)
	err = addon.enable()
	return err
}

func disableAddon(name string) error {
	if isLegacyAddonExist(name) {
		return tryDisableInitializerAddon(name)
	}
	repo, err := NewAddonRepo()
	if err != nil {
		return err
	}
	addon, err := repo.getAddon(name)
	if err != nil {
		return errors.Wrap(err, "get addon err")
	}
	if addon.getStatus() == statusUninstalled {
		fmt.Printf("Addon %s is not installed\n", addon.name)
		return nil
	}
	return addon.disable()

}

func isLegacyAddonExist(name string) bool {
	if namespace, ok := legacyAddonNamespace[name]; ok {
		convertedAddonName := TransAddonName(name)
		init := unstructured.Unstructured{
			Object: map[string]interface{}{
				"apiVersion": "core.oam.dev/v1beta1",
				"kind":       "Initializer",
			},
		}
		err := clt.Get(context.TODO(), client.ObjectKey{
			Namespace: namespace,
			Name:      convertedAddonName,
		}, &init)
		return err == nil
	}
	return false
}

func tryDisableInitializerAddon(addonName string) error {
	fmt.Printf("Trying to disable addon in initializer implementation...\n")
	init := unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "core.oam.dev/v1beta1",
			"kind":       "Initializer",
			"metadata": map[string]interface{}{
				"name":      TransAddonName(addonName),
				"namespace": legacyAddonNamespace[addonName],
			},
		},
	}
	return clt.Delete(context.TODO(), &init)

}
func newAddon(data *v1.ConfigMap) *Addon {
	description := data.ObjectMeta.Annotations[DescAnnotation]
	a := Addon{name: data.Annotations[oam.AnnotationAddonsName], description: description, data: data.Data["application"]}
	return &a
}

// AddonRepo is a place to store addon info
type AddonRepo interface {
	getAddon(name string) (Addon, error)
	listAddons() []Addon
}

// NewAddonRepo create new addon repo,now only support ConfigMap
func NewAddonRepo() (AddonRepo, error) {
	list := v1.ConfigMapList{}
	matchLabels := metav1.LabelSelector{
		MatchExpressions: []metav1.LabelSelectorRequirement{{
			Key:      oam.LabelAddonsName,
			Operator: metav1.LabelSelectorOpExists,
		}},
	}
	selector, err := metav1.LabelSelectorAsSelector(&matchLabels)
	if err != nil {
		return nil, err
	}
	err = clt.List(context.Background(), &list, &client.ListOptions{LabelSelector: selector})
	if err != nil {
		return nil, errors.Wrap(err, "Get addon list failed")
	}
	return configMapAddonRepo{maps: list.Items}, nil
}

type configMapAddonRepo struct {
	maps []v1.ConfigMap
}

// AddonNotFoundErr means addon not found
type AddonNotFoundErr struct {
	addonName string
}

func (e AddonNotFoundErr) Error() string {
	return fmt.Sprintf("addon %s not found", e.addonName)
}

func (c configMapAddonRepo) getAddon(name string) (Addon, error) {
	for i := range c.maps {
		if addonName, ok := c.maps[i].Annotations[oam.AnnotationAddonsName]; ok && name == addonName {
			return *newAddon(&c.maps[i]), nil
		}
	}
	return Addon{}, AddonNotFoundErr{addonName: name}
}

func (c configMapAddonRepo) listAddons() []Addon {
	var addons []Addon
	for i := range c.maps {
		addon := newAddon(&c.maps[i])
		addons = append(addons, *addon)
	}
	return addons
}

// Addon consist of a Initializer resource to enable an addon
type Addon struct {
	name        string
	description string
	data        string
	// Args is map for renderInitializer
	Args        map[string]string
	application *v1beta1.Application
}

func (a *Addon) renderApplication() (*v1beta1.Application, error) {
	if a.Args == nil {
		a.Args = map[string]string{}
	}
	t, err := template.New("addon-template").Delims("[[", "]]").Funcs(sprig.TxtFuncMap()).Parse(a.data)
	if err != nil {
		return nil, errors.Wrap(err, "parsing addon initializer template error")
	}
	buf := bytes.Buffer{}
	err = t.Execute(&buf, a)
	if err != nil {
		return nil, errors.Wrap(err, "application template render fail")
	}
	err = yaml2.NewYAMLOrJSONDecoder(&buf, buf.Len()).Decode(&a.application)
	if err != nil {
		return nil, err
	}
	return a.application, nil
}

func (a *Addon) enable() error {
	applicator := apply.NewAPIApplicator(clt)
	ctx := context.Background()
	obj, err := a.renderApplication()
	if err != nil {
		return err
	}
	err = a.installDependsOn()
	if err != nil {
		return errors.Wrap(err, "Error occurs when install dependent addon")
	}
	err = applicator.Apply(ctx, obj)
	if err != nil {
		return errors.Wrapf(err, "Error occurs when apply addon application: %s\n", a.name)
	}
	err = waitApplicationRunning(a.application)
	if err != nil {
		return errors.Wrap(err, "Error occurs when waiting addon applicatoin running")
	}
	return nil
}

func waitApplicationRunning(obj *v1beta1.Application) error {
	ctx := context.Background()
	period := 20 * time.Second
	timeout := 10 * time.Minute
	var app v1beta1.Application
	return wait.PollImmediate(period, timeout, func() (done bool, err error) {
		err = clt.Get(ctx, types2.NamespacedName{Name: obj.GetName(), Namespace: obj.GetNamespace()}, &app)
		if err != nil {
			return false, client.IgnoreNotFound(err)
		}
		phase := app.Status.Phase
		if phase == common2.ApplicationRunning {
			return true, nil
		}
		fmt.Printf("Application %s is in phase:%s...\n", obj.GetName(), phase)
		return false, nil
	})
}
func (a *Addon) disable() error {
	obj, err := a.renderApplication()
	if err != nil {
		return err
	}
	fmt.Println("Deleting all resources...")
	err = clt.Delete(context.TODO(), obj, client.PropagationPolicy(metav1.DeletePropagationForeground))
	if err != nil {
		return err
	}
	return nil
}

func (a *Addon) getStatus() string {
	var application v1beta1.Application
	err := clt.Get(context.Background(), client.ObjectKey{
		Namespace: types.DefaultKubeVelaNS,
		Name:      TransAddonName(a.name),
	}, &application)
	if err != nil {
		return statusUninstalled
	}
	return statusInstalled
}

func (a *Addon) setArgs(args map[string]string) {
	a.Args = args
}

func (a *Addon) installDependsOn() error {
	if a.application.Spec.Workflow == nil || a.application.Spec.Workflow.Steps == nil {
		return nil
	}
	repo, err := NewAddonRepo()
	if err != nil {
		return err
	}
	for _, step := range a.application.Spec.Workflow.Steps {
		if step.Type == DependsOnWorkFlowStepName {
			props, err := util.RawExtension2Map(step.Properties)
			if err != nil {
				return err
			}
			dependsOnAddonName, _ := props["name"].(string)
			fmt.Printf("Installing dependent addon: %s\n", dependsOnAddonName)
			addon, err := repo.getAddon(dependsOnAddonName)
			if err != nil {
				return err
			}
			if addon.getStatus() != statusInstalled {
				err = addon.enable()
				if err != nil {
					return err
				}
			}
		}
	}
	return nil
}

// TransAddonName will turn addon's name from xxx/yyy to xxx-yyy
func TransAddonName(name string) string {
	return strings.ReplaceAll(name, "/", "-")
}
