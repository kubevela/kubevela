package context

import (
	"context"
	"cuelang.org/go/cue"
	"encoding/json"
	"github.com/oam-dev/kubevela/pkg/cue/model"
	"github.com/oam-dev/kubevela/pkg/oam/util"
	"github.com/pkg/errors"
	corev1 "k8s.io/api/core/v1"
	kerrors "k8s.io/apimachinery/pkg/api/errors"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

type workflowContext struct {
	cli        client.Client
	store      corev1.ConfigMap
	components map[string][]*componentManifest
	vars       *model.Value
	generate   string
}

func (wf *workflowContext) GetComponent(name string) (*componentManifest, error) {
	components, ok := wf.components[name]
	if !ok || len(components) == 0 {
		return nil, errors.Errorf("component %s not found in application", name)
	}
	return components[0], nil
}

func (wf *workflowContext) PatchComponent(name string, patchContent string) error {
	component, err := wf.GetComponent(name)
	if err != nil {
		return err
	}
	return component.Patch(patchContent)
}

func (wf *workflowContext) GetVar(paths ...string) (*model.Value, error) {
	return wf.vars.LookupValue(paths...)
}

func (wf *workflowContext) SetVar(v *model.Value, paths ...string) error {
	str, err := v.Fmt()
	if err != nil {
		return errors.WithMessage(err, "compile var")
	}
	return wf.vars.FillRaw(str, paths...)
}

func (wf *workflowContext) Commit() error {
	varFmt, err := wf.vars.Fmt()
	if err != nil {
		return err
	}
	cpsJsonObject := map[string][]string{}
	for name, cps := range wf.components {
		cpsJsonObject[name] = []string{}
		for _, cp := range cps {
			str, err := cp.string()
			if err != nil {
				return errors.WithMessagef(err, "encode component %s ", name)
			}
			cpsJsonObject[name] = append(cpsJsonObject[name], str)
		}
	}

	wf.store.Data = map[string]string{
		"components": string(util.MustJSONMarshal(cpsJsonObject)),
		"vars":       varFmt,
	}
	if err := wf.writeToStore(); err != nil {
		return errors.WithMessagef(err, "save context to configMap(%s/%s)", wf.store.Namespace, wf.store.Name)
	}
	return nil
}

func (wf *workflowContext) writeToStore() error {
	ctx := context.Background()
	if err := wf.cli.Update(ctx, &wf.store); err != nil {
		if kerrors.IsNotFound(err) {
			return wf.cli.Create(ctx, &wf.store)
		}
		return err
	}
	return nil
}

type componentManifest struct {
	Workload    model.Instance
	Auxiliaries []model.Instance
}

func (comp *componentManifest) Patch(pv string) error {
	var r cue.Runtime
	cueInst, err := r.Compile("-", pv)
	if err != nil {
		return err
	}
	pInst, err := model.NewOther(cueInst.Value())
	if err != nil {
		return err
	}
	return comp.Workload.Unify(pInst)
}

func (comp *componentManifest) string() (string, error) {
	comonentFmt := struct {
		Workload    string   `json:"workload"`
		Auxiliaries []string `json:"auxiliaries"`
	}{
		Workload: comp.Workload.String(),
	}
	for _, aux := range comp.Auxiliaries {
		comonentFmt.Auxiliaries = append(comonentFmt.Auxiliaries, aux.String())
	}
	js, err := json.Marshal(comonentFmt)
	return string(js), err
}
