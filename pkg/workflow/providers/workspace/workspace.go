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

package workspace

import (
	"fmt"
	"strings"

	"github.com/pkg/errors"

	"github.com/oam-dev/kubevela/pkg/cue/model/value"
	wfContext "github.com/oam-dev/kubevela/pkg/workflow/context"
	"github.com/oam-dev/kubevela/pkg/workflow/providers"
	"github.com/oam-dev/kubevela/pkg/workflow/types"
)

const (
	// ProviderName is provider name.
	ProviderName = "builtin"
)

type provider struct {
}

// Load get component from context.
func (h *provider) Load(ctx wfContext.Context, v *value.Value, act types.Action) error {
	componentName, err := v.Field("component")
	if err != nil {
		return err
	}
	if !componentName.Exists() {
		return nil
	}
	name, err := componentName.String()
	if err != nil {
		return err
	}
	component, err := ctx.GetComponent(name)
	if err != nil {
		return err
	}
	if err := v.FillRaw(component.Workload.String(), "workload"); err != nil {
		return err
	}

	if len(component.Auxiliaries) > 0 {
		var auxiliaries []string
		for _, aux := range component.Auxiliaries {
			auxiliaries = append(auxiliaries, "{"+aux.String()+"}")
		}
		if err := v.FillRaw(fmt.Sprintf("[%s]", strings.Join(auxiliaries, ",")), "auxiliaries"); err != nil {
			return err
		}
	}
	return nil
}

// Export put data into context.
func (h *provider) Export(ctx wfContext.Context, v *value.Value, act types.Action) error {
	tpyValue, err := v.Field("type")
	if err != nil {
		return err
	}
	tpy, err := tpyValue.String()
	if err != nil {
		return err
	}

	val, err := v.LookupValue("value")
	if err != nil {
		return err
	}

	switch tpy {
	case "patch":
		nameValue, err := v.Field("component")
		if err != nil {
			return err
		}

		name, err := nameValue.String()
		if err != nil {
			return err
		}
		return ctx.PatchComponent(name, val)
	case "var":
		pathValue, err := v.Field("path")
		if err != nil {
			return err
		}

		path, err := pathValue.String()
		if err != nil {
			return err
		}

		return ctx.SetVar(val, strings.Split(path, ".")...)
	default:
		return errors.Errorf("export type=%s not supported", tpy)
	}
}

// Wait let workflow wait.
func (h *provider) Wait(ctx wfContext.Context, v *value.Value, act types.Action) error {

	cv := v.CueValue()
	if cv.Exists() {
		ret := cv.Lookup("continue")
		if ret.Exists() {
			isContinue, err := ret.Bool()
			if err == nil && isContinue {
				return nil
			}
		}
	}

	act.Wait("")
	return nil
}

// Break let workflow terminate.
func (h *provider) Break(ctx wfContext.Context, v *value.Value, act types.Action) error {
	act.Terminate("")
	return nil
}

// Install register handler to provider discover.
func Install(p providers.Providers) {
	prd := &provider{}
	p.Register(ProviderName, map[string]providers.Handler{
		"load":   prd.Load,
		"export": prd.Export,
		"wait":   prd.Wait,
		"break":  prd.Break,
	})
}
