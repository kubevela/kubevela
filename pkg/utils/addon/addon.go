package addon

import (
	"bytes"
	"context"
	"fmt"
	"strings"
	"text/template"
	"time"

	"github.com/Masterminds/sprig"
	"github.com/pkg/errors"

	"github.com/oam-dev/kubevela/pkg/utils/common"

	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/runtime/serializer/yaml"
	types2 "k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/client-go/dynamic"
	"sigs.k8s.io/controller-runtime/pkg/client"

	common2 "github.com/oam-dev/kubevela/apis/core.oam.dev/common"
	"github.com/oam-dev/kubevela/apis/core.oam.dev/v1beta1"
	"github.com/oam-dev/kubevela/apis/types"
	"github.com/oam-dev/kubevela/pkg/oam"
	"github.com/oam-dev/kubevela/pkg/oam/discoverymapper"
	"github.com/oam-dev/kubevela/pkg/utils/apply"
)

const (
	// DescAnnotation records the Description of addon
	DescAnnotation = "addons.oam.dev/description"
)

var (
	// StatusUninstalled means addon not installed
	StatusUninstalled = "uninstalled"
	// StatusInstalled means addon installed
	StatusInstalled = "installed"
	clt             client.Client
	clientArgs      common.Args
)

func init() {
	clientArgs, _ = common.InitBaseRestConfig()
	clt, _ = clientArgs.GetClient()

}

func newAddon(data *v1.ConfigMap) *Addon {
	description := data.ObjectMeta.Annotations[DescAnnotation]
	a := Addon{
		Name:        data.Annotations[oam.AnnotationAddonsName],
		Description: description,
		Detail:      data.Data["detail"],
		Data:        data.Data["application"],
	}
	return &a
}

// Repo is a place to store addon info
type Repo interface {
	GetAddon(name string) (Addon, error)
	ListAddons() []Addon
}

// NewAddonRepo create new addon repo,now only support ConfigMap
func NewAddonRepo() (Repo, error) {
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

// NotFoundErr means addon not found
type NotFoundErr struct {
	addonName string
}

func (e NotFoundErr) Error() string {
	return fmt.Sprintf("addon %s not found", e.addonName)
}

// GetAddon will get addon from ConfigMap
func (c configMapAddonRepo) GetAddon(name string) (Addon, error) {
	for i := range c.maps {
		if addonName, ok := c.maps[i].Annotations[oam.AnnotationAddonsName]; ok && name == addonName {
			return *newAddon(&c.maps[i]), nil
		}
	}
	return Addon{}, NotFoundErr{addonName: name}
}

// ListAddons will list addons from ConfigMap
func (c configMapAddonRepo) ListAddons() []Addon {
	var addons []Addon
	for i := range c.maps {
		addon := newAddon(&c.maps[i])
		addons = append(addons, *addon)
	}
	return addons
}

// Addon consist of a Initializer resource to Enable an addon
type Addon struct {
	Name        string
	Description string
	Data        string
	// Args is map for renderInitializer
	Args        map[string]string
	application *unstructured.Unstructured
	gvk         *schema.GroupVersionKind

	// Detail is doc for addon
	Detail string
}

// GetGVK will return addon's application's GVK
func (a *Addon) GetGVK() (*schema.GroupVersionKind, error) {
	if a.gvk == nil {
		if a.application == nil {
			_, err := a.RenderApplication()
			if err != nil {
				return nil, err
			}
		}
		gvk := schema.FromAPIVersionAndKind(a.application.GetAPIVersion(), a.application.GetKind())
		a.gvk = &gvk
	}
	return a.gvk, nil
}

// RenderApplication will render addon application
// this will use addon's Data and Args
func (a *Addon) RenderApplication() (*unstructured.Unstructured, error) {
	if a.Args == nil {
		a.Args = map[string]string{}
	}
	t, err := template.New("addon-template").Delims("[[", "]]").Funcs(sprig.TxtFuncMap()).Parse(a.Data)
	if err != nil {
		return nil, errors.Wrap(err, "parsing addon initializer template error")
	}
	buf := bytes.Buffer{}
	err = t.Execute(&buf, a)
	if err != nil {
		return nil, errors.Wrap(err, "application template render fail")
	}
	dec := yaml.NewDecodingSerializer(unstructured.UnstructuredJSONScheme)
	obj := &unstructured.Unstructured{}
	_, gvk, err := dec.Decode(buf.Bytes(), nil, obj)
	if err != nil {
		return nil, err
	}
	a.application = obj
	a.gvk = gvk
	return a.application, nil
}

// Enable will enable an addon by apply application
func (a *Addon) Enable() error {
	applicator := apply.NewAPIApplicator(clt)
	ctx := context.Background()
	obj, err := a.RenderApplication()
	if err != nil {
		return err
	}
	err = applicator.Apply(ctx, obj)
	if err != nil {
		return errors.Wrapf(err, "Error occurs when apply addon application: %s\n", a.Name)
	}
	err = waitApplicationRunning(a.application)
	if err != nil {
		return errors.Wrap(err, "Error occurs when waiting addon applicatoin running")
	}
	return nil
}

func waitApplicationRunning(obj *unstructured.Unstructured) error {
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

// Disable will delete addon's application
func (a *Addon) Disable() error {
	dynamicClient, err := dynamic.NewForConfig(clientArgs.Config)
	if err != nil {
		return err
	}
	mapper, err := discoverymapper.New(clientArgs.Config)
	if err != nil {
		return err
	}
	obj, err := a.RenderApplication()
	if err != nil {
		return err
	}
	gvk, err := a.GetGVK()
	if err != nil {
		return err
	}
	mapping, err := mapper.RESTMapping(gvk.GroupKind(), gvk.Version)
	if err != nil {
		return err
	}
	var resourceREST dynamic.ResourceInterface
	if mapping.Scope.Name() == meta.RESTScopeNameNamespace {
		// namespaced resources should specify the namespace
		resourceREST = dynamicClient.Resource(mapping.Resource).Namespace(obj.GetNamespace())
	} else {
		// for cluster-wide resources
		resourceREST = dynamicClient.Resource(mapping.Resource)
	}
	deletePolicy := metav1.DeletePropagationForeground
	deleteOptions := metav1.DeleteOptions{
		PropagationPolicy: &deletePolicy,
	}
	fmt.Println("Deleting all resources...")
	err = resourceREST.Delete(context.TODO(), obj.GetName(), deleteOptions)
	if err != nil {
		return err
	}
	return nil
}

// GetStatus will return if an Addon is enabled
func (a *Addon) GetStatus() string {
	var application v1beta1.Application
	err := clt.Get(context.Background(), client.ObjectKey{
		Namespace: types.DefaultKubeVelaNS,
		Name:      TransAddonName(a.Name),
	}, &application)
	if err != nil {
		return StatusUninstalled
	}
	return StatusInstalled
}

// SetArgs will set Args for application render
func (a *Addon) SetArgs(args map[string]string) {
	a.Args = args
}

// TransAddonName will turn addon's name from xxx/yyy to xxx-yyy
func TransAddonName(name string) string {
	return strings.ReplaceAll(name, "/", "-")
}
