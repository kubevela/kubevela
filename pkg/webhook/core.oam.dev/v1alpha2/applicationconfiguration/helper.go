package applicationconfiguration

import (
	"context"
	"encoding/json"
	"strings"

	"github.com/pkg/errors"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/crossplane/oam-kubernetes-runtime/apis/core/v1alpha2"
	"github.com/crossplane/oam-kubernetes-runtime/pkg/oam/discoverymapper"
	"github.com/crossplane/oam-kubernetes-runtime/pkg/oam/util"
)

const (
	errFmtGetComponent          = "cannot get component %q"
	errFmtGetTraitDefinition    = "cannot get trait definition in component %q"
	errFmtUnmarshalWorkload     = "cannot unmarshal workload of component %q"
	errFmtUnmarshalTrait        = "cannot unmarshal trait of component %q"
	errFmtGetWorkloadDefinition = "cannot get workload definition of component %q"
)

// ValidatingAppConfig is used for validating ApplicationConfiguration
type ValidatingAppConfig struct {
	appConfig       v1alpha2.ApplicationConfiguration
	validatingComps []ValidatingComponent
}

// ValidatingComponent is used for validatiing ApplicationConfigurationComponent
type ValidatingComponent struct {
	appConfigComponent v1alpha2.ApplicationConfigurationComponent

	// below data is convenient for validation
	compName           string
	component          v1alpha2.Component
	workloadDefinition v1alpha2.WorkloadDefinition
	workloadContent    unstructured.Unstructured
	validatingTraits   []ValidatingTrait
}

// ValidatingTrait is used for validating Trait
type ValidatingTrait struct {
	componentTrait v1alpha2.ComponentTrait

	// below data is convenient for validation
	traitDefinition v1alpha2.TraitDefinition
	traitContent    unstructured.Unstructured
}

// PrepareForValidation prepares data for validations to avoiding repetitive GET/unmarshal operations
func (v *ValidatingAppConfig) PrepareForValidation(ctx context.Context, c client.Reader, dm discoverymapper.DiscoveryMapper, ac *v1alpha2.ApplicationConfiguration) error {
	v.appConfig = *ac
	v.validatingComps = make([]ValidatingComponent, 0, len(ac.Spec.Components))
	for _, acc := range ac.Spec.Components {
		tmp := ValidatingComponent{}
		tmp.appConfigComponent = acc

		if acc.ComponentName != "" {
			tmp.compName = acc.ComponentName
		} else {
			tmp.compName = acc.RevisionName
		}
		comp, _, err := util.GetComponent(ctx, c, acc, ac.Namespace)
		if err != nil {
			return errors.Wrapf(err, errFmtGetComponent, tmp.compName)
		}
		tmp.component = *comp

		// get worload content from raw
		var wlContentObject map[string]interface{}
		if err := json.Unmarshal(comp.Spec.Workload.Raw, &wlContentObject); err != nil {
			return errors.Wrapf(err, errFmtUnmarshalWorkload, tmp.compName)
		}
		wl := unstructured.Unstructured{
			Object: wlContentObject,
		}
		tmp.workloadContent = wl

		// get workload definition
		wlDef, err := util.FetchWorkloadDefinition(ctx, c, dm, &wl)
		if err != nil {
			return errors.Wrapf(err, errFmtGetWorkloadDefinition, tmp.compName)
		}
		tmp.workloadDefinition = *wlDef

		tmp.validatingTraits = make([]ValidatingTrait, 0, len(acc.Traits))
		for _, t := range acc.Traits {
			tmpT := ValidatingTrait{}
			tmpT.componentTrait = t
			// get trait content from raw
			var tContentObject map[string]interface{}
			if err := json.Unmarshal(t.Trait.Raw, &tContentObject); err != nil {
				return errors.Wrapf(err, errFmtUnmarshalTrait, tmp.compName)
			}
			tContent := unstructured.Unstructured{
				Object: tContentObject,
			}
			// get trait definition
			tDef, err := util.FetchTraitDefinition(ctx, c, dm, &tContent)
			if err != nil {
				return errors.Wrapf(err, errFmtGetTraitDefinition, tmp.compName)
			}
			tmpT.traitContent = tContent
			tmpT.traitDefinition = *tDef
			tmp.validatingTraits = append(tmp.validatingTraits, tmpT)
		}
		v.validatingComps = append(v.validatingComps, tmp)
	}
	return nil
}

// checkParams will check whether exist parameter assigning value to workload name
func checkParams(cp []v1alpha2.ComponentParameter, cpv []v1alpha2.ComponentParameterValue) (bool, string) {
	targetParams := make(map[string]bool)
	for _, v := range cp {
		for _, fp := range v.FieldPaths {
			// only check metadata.name field parameter
			if strings.Contains(fp, WorkloadNamePath) {
				targetParams[v.Name] = true
			}
		}
	}
	for _, v := range cpv {
		if targetParams[v.Name] {
			// check fails if get parameter to overwrite workload name
			return false, v.Value.StrVal
		}
	}
	return true, ""
}
