package op

import (
	"fmt"
	"github.com/oam-dev/kubevela/pkg/cue/model"
	"github.com/oam-dev/kubevela/pkg/workflow"
	wfContext "github.com/oam-dev/kubevela/pkg/workflow/context"
	"github.com/oam-dev/kubevela/pkg/workflow/providers"
	"strings"
)

const (
	ProviderName = "builtin"
)

type provider struct {
}

func (h *provider) Load(ctx wfContext.Context, v *model.Value, act workflow.Action) error {
	componentName, err := v.Field("#component")
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
	component, err := ctx.GetComponent(name, nil)
	if err != nil {
		return err
	}
	if err := v.FillRaw(component.Workload.String()); err != nil {
		return err
	}

	if len(component.Auxiliaries) > 0 {
		auxiliaries := []string{}
		for _, aux := range component.Auxiliaries {
			auxiliaries = append(auxiliaries, aux.String())
		}
		if err := v.FillRaw(fmt.Sprintf("[%s]", strings.Join(auxiliaries, ",")), "_auxiliaries"); err != nil {
			return err
		}
	}
	return nil
}

func (h *provider) Export(ctx wfContext.Context, v *model.Value, act workflow.Action) error {
	return nil
}

func (h *provider) Wait(ctx wfContext.Context, v *model.Value, act workflow.Action) error {
	ret, err := v.Field("return")
	if err != nil {
		return err
	}
	if !ret.Exists() {
		return nil
	}
	isReturn, err := ret.Bool()
	if err != nil {
		return err
	}
	if isReturn {
		act.Wait()
	}
	return nil
}

func (h *provider) Break(ctx wfContext.Context, v *model.Value, act workflow.Action) error {
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
