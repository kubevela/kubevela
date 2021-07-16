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
	ProviderName = "builtin"
)

type provider struct {
}

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
		auxiliaries := []string{}
		for _, aux := range component.Auxiliaries {
			auxiliaries = append(auxiliaries, aux.String())
		}
		if err := v.FillRaw(fmt.Sprintf("[%s]", strings.Join(auxiliaries, ",")), "auxiliaries"); err != nil {
			return err
		}
	}
	return nil
}

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
	}
	return nil
}

func (h *provider) Wait(ctx wfContext.Context, v *value.Value, act types.Action) error {
	ret, err := v.Field("continue")
	if err != nil {
		return err
	}
	if !ret.Exists() {
		return nil
	}
	isContinue, err := ret.Bool()
	if err != nil {
		return err
	}
	if !isContinue {
		act.Wait()
	}
	return nil
}

func (h *provider) Break(ctx wfContext.Context, v *value.Value, act types.Action) error {
	act.Terminated()
	return nil
}

func Install(p providers.Providers) {
	prd := &provider{}
	p.Register(ProviderName, map[string]providers.Handler{
		"load":   prd.Load,
		"export": prd.Export,
		"wait":   prd.Wait,
		"break":  prd.Break,
	})
}
