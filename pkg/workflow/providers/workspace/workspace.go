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
	componentName, _ := v.Field("component")
	if !componentName.Exists() {
		componets := ctx.GetComponents()
		for name, c := range componets {
			if err := fillComponent(v, c, "value", name); err != nil {
				return err
			}
		}
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
	return fillComponent(v, component, "value")
}

func fillComponent(v *value.Value, component *wfContext.ComponentManifest, paths ...string) error {
	workload, err := component.Workload.String()
	if err != nil {
		return err
	}
	if err := v.FillRaw(workload, append(paths, "workload")...); err != nil {
		return err
	}
	if len(component.Auxiliaries) > 0 {
		var auxiliaries []string
		for _, aux := range component.Auxiliaries {
			auxiliary, err := aux.String()
			if err != nil {
				return err
			}
			auxiliaries = append(auxiliaries, "{"+auxiliary+"}")
		}
		if err := v.FillRaw(fmt.Sprintf("[%s]", strings.Join(auxiliaries, ",")), append(paths, "auxiliaries")...); err != nil {
			return err
		}
	}
	return nil
}

// DoVar get & put variable from context.
func (h *provider) DoVar(ctx wfContext.Context, v *value.Value, act types.Action) error {
	methodV, err := v.Field("method")
	if err != nil {
		return err
	}
	method, err := methodV.String()
	if err != nil {
		return err
	}

	pathV, err := v.Field("path")
	if err != nil {
		return err
	}
	path, err := pathV.String()
	if err != nil {
		return err
	}

	switch method {
	case "Get":
		value, err := ctx.GetVar(strings.Split(path, ".")...)
		if err != nil {
			return err
		}
		raw, err := value.String()
		if err != nil {
			return err
		}
		return v.FillRaw(raw, "value")
	case "Put":
		value, err := v.LookupValue("value")
		if err != nil {
			return err
		}
		return ctx.SetVar(value, strings.Split(path, ".")...)
	}
	return nil
}

// Export put data into context.
func (h *provider) Export(ctx wfContext.Context, v *value.Value, act types.Action) error {
	val, err := v.LookupValue("value")
	if err != nil {
		return err
	}

	nameValue, err := v.Field("component")
	if err != nil {
		return err
	}

	name, err := nameValue.String()
	if err != nil {
		return err
	}
	return ctx.PatchComponent(name, val)
}

// Wait let workflow wait.
func (h *provider) Wait(ctx wfContext.Context, v *value.Value, act types.Action) error {

	cv := v.CueValue()
	if cv.Exists() {
		ret := cv.LookupPath(value.FieldPath("continue"))
		if ret.Exists() {
			isContinue, err := ret.Bool()
			if err == nil && isContinue {
				return nil
			}
		}
	}
	msg, _ := v.GetString("message")
	act.Wait(msg)
	return nil
}

// Break let workflow terminate.
func (h *provider) Break(ctx wfContext.Context, v *value.Value, act types.Action) error {
	var msg string
	if v != nil {
		msg, _ = v.GetString("message")
	}
	act.Terminate(msg)
	return nil
}

// Fail let the step fail, its status is failed and reason is Action
func (h *provider) Fail(ctx wfContext.Context, v *value.Value, act types.Action) error {
	var msg string
	if v != nil {
		msg, _ = v.GetString("message")
	}
	act.Fail(msg)
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
		"fail":   prd.Fail,
		"var":    prd.DoVar,
	})
}
