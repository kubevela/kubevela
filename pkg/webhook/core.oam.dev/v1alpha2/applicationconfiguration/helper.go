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

package applicationconfiguration

import (
	"context"
	"encoding/json"
	"strings"

	"github.com/pkg/errors"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/client-go/util/retry"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/oam-dev/kubevela/apis/core.oam.dev/v1alpha2"
	"github.com/oam-dev/kubevela/pkg/oam/discoverymapper"
	"github.com/oam-dev/kubevela/pkg/oam/util"
)

const (
	errFmtGetComponent          = "cannot get component %q"
	errFmtGetTraitDefinition    = "cannot get trait definition in component %q"
	errFmtUnmarshalWorkload     = "cannot unmarshal workload of component %q"
	errFmtUnmarshalTrait        = "cannot unmarshal trait of component %q"
	errFmtGetWorkloadDefinition = "cannot get workload definition of component %q"
	errFmtCheckTrait            = "failed checking trait of component %q"
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
func (v *ValidatingAppConfig) PrepareForValidation(ctx context.Context, c client.Reader,
	dm discoverymapper.DiscoveryMapper, ac *v1alpha2.ApplicationConfiguration) error {
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
		var comp *v1alpha2.Component
		accCopy := *acc.DeepCopy()
		err := wait.ExponentialBackoff(retry.DefaultBackoff, func() (bool, error) {
			var err error
			comp, _, err = util.GetComponent(ctx, c, accCopy, ac.Namespace)
			if err != nil && !k8serrors.IsNotFound(err) {
				return false, err
			}
			return true, nil
		})
		if err != nil {
			return errors.Wrapf(err, errFmtGetComponent, tmp.compName)
		}
		tmp.component = *comp

		// get workload content from raw
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
			tContent := unstructured.Unstructured{}
			if err := json.Unmarshal(t.Trait.Raw, &tContent.Object); err != nil {
				return errors.Wrapf(err, errFmtUnmarshalTrait, tmp.compName)
			}

			if err := checkTraitObj(&tContent); err != nil {
				return errors.Wrapf(err, errFmtCheckTrait, tmp.compName)
			}

			// get trait definition
			tDef, err := util.FetchTraitDefinition(ctx, c, dm, &tContent)
			if err != nil {
				if !k8serrors.IsNotFound(err) {
					return errors.Wrapf(err, errFmtGetTraitDefinition, tmp.compName)
				}
				tDef = util.GetDummyTraitDefinition(&tContent)
			}
			tmpT.traitContent = tContent
			tmpT.traitDefinition = *tDef
			tmp.validatingTraits = append(tmp.validatingTraits, tmpT)
		}
		v.validatingComps = append(v.validatingComps, tmp)
	}
	return nil
}

// checkTraitObj checks trait whether it's muated correctly and has GVK.
// Further validation on traits should provieded by validators but not here.
func checkTraitObj(t *unstructured.Unstructured) error {
	if t.Object[TraitTypeField] != nil {
		return errors.New("the trait contains 'name' info that should be mutated to GVK")
	}
	if t.Object[TraitSpecField] != nil {
		return errors.New("the trait contains 'properties' info that should be mutated to spec")
	}
	if len(t.GetAPIVersion()) == 0 || len(t.GetKind()) == 0 {
		return errors.New("the trait data missing GVK")
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
