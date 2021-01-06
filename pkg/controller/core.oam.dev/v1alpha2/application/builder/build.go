package builder

import (
	"github.com/crossplane/crossplane-runtime/apis/core/v1alpha1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/oam-dev/kubevela/apis/core.oam.dev/v1alpha2"
	"github.com/oam-dev/kubevela/pkg/appfile/config"
	"github.com/oam-dev/kubevela/pkg/controller/core.oam.dev/v1alpha2/application/parser"
	"github.com/oam-dev/kubevela/pkg/dsl/process"
	"github.com/oam-dev/kubevela/pkg/oam"
)

type builder struct {
	app *parser.Appfile
	c   client.Client
}

const (
	// OAMApplicationLabel is application's metadata label
	OAMApplicationLabel = "application.oam.dev"
)

// Build template to applicationConfig & Component
func Build(ns string, app *parser.Appfile, c client.Client) (*v1alpha2.ApplicationConfiguration, []*v1alpha2.Component, error) {
	b := &builder{app: app, c: c}
	return b.CompleteWithContext(ns)
}

func (b *builder) CompleteWithContext(ns string) (*v1alpha2.ApplicationConfiguration, []*v1alpha2.Component, error) {
	appconfig := &v1alpha2.ApplicationConfiguration{}
	appconfig.SetGroupVersionKind(v1alpha2.ApplicationConfigurationGroupVersionKind)
	appconfig.Name = b.app.Name()
	appconfig.Namespace = ns
	appconfig.Spec.Components = []v1alpha2.ApplicationConfigurationComponent{}

	if appconfig.Labels == nil {
		appconfig.Labels = map[string]string{}
	}
	appconfig.Labels[OAMApplicationLabel] = b.app.Name()

	var components []*v1alpha2.Component
	for _, wl := range b.app.Services() {

		pCtx := process.NewContext(wl.Name())
		userConfig := wl.GetUserConfigName()
		if userConfig != "" {
			cg := config.Configmap{Client: b.c}

			// TODO(wonderflow): envName should not be namespace when we have serverside env
			var envName = ns

			data, err := cg.GetConfigData(config.GenConfigMapName(b.app.Name(), wl.Name(), userConfig), envName)
			if err != nil {
				return nil, nil, err
			}
			pCtx.SetConfigs(data)
		}

		if err := wl.EvalContext(pCtx); err != nil {
			return nil, nil, err
		}
		for _, tr := range wl.Traits() {
			if err := tr.EvalContext(pCtx); err != nil {
				return nil, nil, err
			}
		}
		comp, acComp, err := generateOAM(pCtx)
		if err != nil {
			return nil, nil, err
		}
		comp.Name = wl.Name()
		acComp.ComponentName = comp.Name

		for _, sc := range wl.Scopes() {
			acComp.Scopes = append(acComp.Scopes, v1alpha2.ComponentScope{ScopeReference: v1alpha1.TypedReference{
				APIVersion: sc.GVK.GroupVersion().String(),
				Kind:       sc.GVK.Kind,
				Name:       sc.Name,
			}})
		}

		workloadType := wl.Type()
		workloadObject := comp.Spec.Workload.Object.(*unstructured.Unstructured)
		labels := workloadObject.GetLabels()
		if labels == nil {
			labels = map[string]string{oam.WorkloadTypeLabel: workloadType}
		} else {
			labels[oam.WorkloadTypeLabel] = workloadType
		}
		workloadObject.SetLabels(labels)

		comp.Namespace = ns
		if comp.Labels == nil {
			comp.Labels = map[string]string{}
		}
		comp.Labels[OAMApplicationLabel] = b.app.Name()
		comp.SetGroupVersionKind(v1alpha2.ComponentGroupVersionKind)

		components = append(components, comp)
		appconfig.Spec.Components = append(appconfig.Spec.Components, *acComp)
	}

	return appconfig, components, nil
}

func generateOAM(pCtx process.Context) (*v1alpha2.Component, *v1alpha2.ApplicationConfigurationComponent, error) {
	base, assists := pCtx.Output()
	componentWorkload, err := base.Object(nil)
	if err != nil {
		return nil, nil, err
	}
	component := &v1alpha2.Component{}
	component.Spec.Workload.Object = componentWorkload

	acComponent := &v1alpha2.ApplicationConfigurationComponent{}
	acComponent.Traits = []v1alpha2.ComponentTrait{}
	for _, assist := range assists {
		traitRef, err := assist.Ins.Object(nil)
		if err != nil {
			return nil, nil, err
		}
		tr := traitRef.(*unstructured.Unstructured)
		tr.SetLabels(map[string]string{oam.TraitTypeLabel: assist.Type})
		acComponent.Traits = append(acComponent.Traits, v1alpha2.ComponentTrait{
			Trait: runtime.RawExtension{
				Object: tr,
			},
		})
	}
	return component, acComponent, nil
}
