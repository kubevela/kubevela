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

package appfile

import (
	"fmt"

	"github.com/pkg/errors"

	"github.com/kubevela/workflow/pkg/cue/process"

	"github.com/oam-dev/kubevela/apis/core.oam.dev/v1alpha1"
	"github.com/oam-dev/kubevela/apis/types"
	velaprocess "github.com/oam-dev/kubevela/pkg/cue/process"
)

// ValidateCUESchematicAppfile validates CUE schematic workloads in an Appfile
func (p *Parser) ValidateCUESchematicAppfile(a *Appfile) error {
	for _, wl := range a.Workloads {
		// because helm & kube schematic has no CUE template
		// it only validates CUE schematic workload
		if wl.CapabilityCategory != types.CUECategory || wl.Type == v1alpha1.RefObjectsComponentType {
			continue
		}
		ctxData := GenerateContextDataFromAppFile(a, wl.Name)
		pCtx, err := newValidationProcessContext(wl, ctxData)
		if err != nil {
			return errors.WithMessagef(err, "cannot create the validation process context of app=%s in namespace=%s", a.Name, a.Namespace)
		}
		for _, tr := range wl.Traits {
			if tr.CapabilityCategory != types.CUECategory {
				continue
			}
			if err := tr.EvalContext(pCtx); err != nil {
				return errors.WithMessagef(err, "cannot evaluate trait %q", tr.Name)
			}
		}
	}
	return nil
}

func newValidationProcessContext(wl *Workload, ctxData velaprocess.ContextData) (process.Context, error) {
	baseHooks := []process.BaseHook{
		// add more hook funcs here to validate CUE base
	}
	auxiliaryHooks := []process.AuxiliaryHook{
		// add more hook funcs here to validate CUE auxiliaries
		validateAuxiliaryNameUnique(),
	}

	ctxData.BaseHooks = baseHooks
	ctxData.AuxiliaryHooks = auxiliaryHooks
	pCtx := velaprocess.NewContext(ctxData)
	if err := wl.EvalContext(pCtx); err != nil {
		return nil, errors.Wrapf(err, "evaluate base template app=%s in namespace=%s", ctxData.AppName, ctxData.Namespace)
	}
	return pCtx, nil
}

// validateAuxiliaryNameUnique validates the name of each outputs item which is
// called auxiliary in vela CUE-based DSL.
// Each capability definition can have arbitrary number of outputs and each
// outputs can have more than one auxiliaries.
// Auxiliaries can be referenced by other cap's template to pass data
// within a workload, so their names must be unique.
func validateAuxiliaryNameUnique() process.AuxiliaryHook {
	return process.AuxiliaryHookFn(func(c process.Context, a []process.Auxiliary) error {
		_, existingAuxs := c.Output()
		for _, newAux := range a {
			for _, existingAux := range existingAuxs {
				if existingAux.Name == newAux.Name {
					return errors.Wrap(fmt.Errorf("auxiliary %q already exits", newAux.Name),
						"outputs item name must be unique")
				}
			}
		}
		return nil
	})
}
