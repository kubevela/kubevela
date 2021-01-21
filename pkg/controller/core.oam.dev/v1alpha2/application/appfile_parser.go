package application

import (
	"github.com/crossplane/crossplane-runtime/apis/core/v1alpha1"
	"k8s.io/apimachinery/pkg/runtime"

	"github.com/oam-dev/kubevela/apis/core.oam.dev/v1alpha2"
	"github.com/oam-dev/kubevela/pkg/appfile/config"
	"github.com/oam-dev/kubevela/pkg/dsl/process"
	"github.com/oam-dev/kubevela/pkg/oam"
)

const (
	// OAMApplicationLabel is application's metadata label
	OAMApplicationLabel = "application.oam.dev"
)

// GenerateApplicationConfiguration converts an appFile to applicationConfig & Components
func (p *Parser) GenerateApplicationConfiguration(app *Appfile, ns string) (*v1alpha2.ApplicationConfiguration,
	[]*v1alpha2.Component, error) {
	appconfig := &v1alpha2.ApplicationConfiguration{}
	appconfig.SetGroupVersionKind(v1alpha2.ApplicationConfigurationGroupVersionKind)
	appconfig.Name = app.Name
	appconfig.Namespace = ns
	appconfig.Spec.Components = []v1alpha2.ApplicationConfigurationComponent{}

	if appconfig.Labels == nil {
		appconfig.Labels = map[string]string{}
	}
	appconfig.Labels[OAMApplicationLabel] = app.Name

	var components []*v1alpha2.Component
	for _, wl := range app.Workloads {

		pCtx := process.NewContext(wl.Name)
		userConfig := wl.GetUserConfigName()
		if userConfig != "" {
			cg := config.Configmap{Client: p.client}

			// TODO(wonderflow): envName should not be namespace when we have serverside env
			var envName = ns

			data, err := cg.GetConfigData(config.GenConfigMapName(app.Name, wl.Name, userConfig), envName)
			if err != nil {
				return nil, nil, err
			}
			pCtx.SetConfigs(data)
		}

		if err := wl.EvalContext(pCtx); err != nil {
			return nil, nil, err
		}
		for _, tr := range wl.Traits {
			if err := tr.EvalContext(pCtx); err != nil {
				return nil, nil, err
			}
		}
		comp, acComp, err := evalWorkloadWithContext(pCtx, wl)
		if err != nil {
			return nil, nil, err
		}
		comp.Name = wl.Name
		acComp.ComponentName = comp.Name

		for _, sc := range wl.Scopes {
			acComp.Scopes = append(acComp.Scopes, v1alpha2.ComponentScope{ScopeReference: v1alpha1.TypedReference{
				APIVersion: sc.GVK.GroupVersion().String(),
				Kind:       sc.GVK.Kind,
				Name:       sc.Name,
			}})
		}

		comp.Namespace = ns
		if comp.Labels == nil {
			comp.Labels = map[string]string{}
		}
		comp.Labels[OAMApplicationLabel] = app.Name
		comp.SetGroupVersionKind(v1alpha2.ComponentGroupVersionKind)

		components = append(components, comp)
		appconfig.Spec.Components = append(appconfig.Spec.Components, *acComp)
	}
	return appconfig, components, nil
}

// evalWorkloadWithContext evaluate the workload's template to generate component and ACComponent
func evalWorkloadWithContext(pCtx process.Context, wl *Workload) (*v1alpha2.Component, *v1alpha2.ApplicationConfigurationComponent, error) {
	base, assists := pCtx.Output()
	componentWorkload, err := base.Unstructured()
	if err != nil {
		return nil, nil, err
	}
	workloadType := wl.Type
	labels := componentWorkload.GetLabels()
	if labels == nil {
		labels = map[string]string{oam.WorkloadTypeLabel: workloadType}
	} else {
		labels[oam.WorkloadTypeLabel] = workloadType
	}
	componentWorkload.SetLabels(labels)

	component := &v1alpha2.Component{}
	component.Spec.Workload.Object = componentWorkload

	acComponent := &v1alpha2.ApplicationConfigurationComponent{}
	acComponent.Traits = []v1alpha2.ComponentTrait{}
	for _, assist := range assists {
		tr, err := assist.Ins.Unstructured()
		if err != nil {
			return nil, nil, err
		}
		tr.SetLabels(map[string]string{oam.TraitTypeLabel: assist.Type})
		acComponent.Traits = append(acComponent.Traits, v1alpha2.ComponentTrait{
			Trait: runtime.RawExtension{
				Object: tr,
			},
		})
	}
	return component, acComponent, nil
}
