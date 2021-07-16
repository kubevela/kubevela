package context

import (
	"context"
	"encoding/json"
	"time"

	"github.com/oam-dev/kubevela/pkg/cue/model/value"

	"cuelang.org/go/cue"
	runtimev1alpha1 "github.com/crossplane/crossplane-runtime/apis/core/v1alpha1"
	"github.com/pkg/errors"
	corev1 "k8s.io/api/core/v1"
	kerrors "k8s.io/apimachinery/pkg/api/errors"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/oam-dev/kubevela/pkg/cue/model"
	"github.com/oam-dev/kubevela/pkg/oam/util"
)

const (
	// ConfigMapKeyComponents is the key in ConfigMap Data field for containing data of components
	ConfigMapKeyComponents = "components"
	// ConfigMapKeyVars is the key in ConfigMap Data field for containing data of variable
	ConfigMapKeyVars = "vars"
	// AnnotationStartTimestamp is the workflow start  timestamp
	AnnotationStartTimestamp = "vela.io/startTime"
)

type workflowContext struct {
	cli        client.Client
	store      corev1.ConfigMap
	components map[string]*componentManifest
	vars       *value.Value
	generate   string
}

func (wf *workflowContext) GetComponent(name string) (*componentManifest, error) {
	component, ok := wf.components[name]
	if !ok {
		return nil, errors.Errorf("component %s not found in application", name)
	}
	return component, nil
}

func (wf *workflowContext) PatchComponent(name string, patchValue *value.Value) error {
	component, err := wf.GetComponent(name)
	if err != nil {
		return err
	}
	return component.Patch(patchValue)
}

func (wf *workflowContext) GetVar(paths ...string) (*value.Value, error) {
	return wf.vars.LookupValue(paths...)
}

func (wf *workflowContext) SetVar(v *value.Value, paths ...string) error {
	str, err := v.String()
	if err != nil {
		return errors.WithMessage(err, "compile var")
	}
	return wf.vars.FillRaw(str, paths...)
}

func (wf *workflowContext) MakeParameter(parameter map[string]interface{}) (*value.Value, error) {
	var s = "{}"
	if parameter != nil {
		s = string(util.MustJSONMarshal(parameter))
	}

	return wf.vars.MakeValue(s)
}

func (wf *workflowContext) Commit() error {
	varStr, err := wf.vars.String()
	if err != nil {
		return err
	}
	jsonObject := map[string]string{}
	for name, comp := range wf.components {
		s, err := comp.string()
		if err != nil {
			return errors.WithMessagef(err, "encode component %s ", name)
		}
		jsonObject[name] = s
	}

	wf.store.Data = map[string]string{
		ConfigMapKeyComponents: string(util.MustJSONMarshal(jsonObject)),
		ConfigMapKeyVars:       varStr,
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

func (wf *workflowContext) loadFromConfigMap(cm corev1.ConfigMap) error {
	data := cm.Data
	componentsJs := map[string]string{}

	if err := json.Unmarshal([]byte(data[ConfigMapKeyComponents]), &componentsJs); err != nil {
		return errors.WithMessage(err, "decode components")
	}

	wf.components = map[string]*componentManifest{}
	for name, compJs := range componentsJs {
		cm := new(componentManifest)
		if err := cm.unmarshal(compJs); err != nil {
			return errors.WithMessagef(err, "unmarshal component(%s) manifest", name)
		}
		wf.components[name] = cm
	}
	var err error
	wf.vars, err = value.NewValue(data[ConfigMapKeyVars], nil)
	if err != nil {
		return errors.WithMessage(err, "decode vars")
	}
	return nil
}

func (wf *workflowContext) StoreRef() *runtimev1alpha1.TypedReference {
	return &runtimev1alpha1.TypedReference{
		APIVersion: wf.store.APIVersion,
		Kind:       wf.store.Kind,
		Name:       wf.store.Name,
		UID:        wf.store.UID,
	}
}

type componentManifest struct {
	Workload    model.Instance
	Auxiliaries []model.Instance
}

func (comp *componentManifest) Patch(patchValue *value.Value) error {
	pInst, err := model.NewOther(patchValue.CueValue())
	if err != nil {
		return err
	}
	return comp.Workload.Unify(pInst)
}

type componentMould struct {
	StandardWorkload string
	Traits           []string
}

func (comp *componentManifest) string() (string, error) {
	cm := componentMould{
		StandardWorkload: comp.Workload.String(),
	}
	for _, aux := range comp.Auxiliaries {
		cm.Traits = append(cm.Traits, aux.String())
	}
	js, err := json.Marshal(cm)
	return string(js), err
}

func (comp *componentManifest) unmarshal(v string) error {

	cm := componentMould{}
	if err := json.Unmarshal([]byte(v), &cm); err != nil {
		return err
	}

	var r cue.Runtime
	wlInst, err := r.Compile("workload", cm.StandardWorkload)
	if err != nil {
		return err
	}
	wl, err := model.NewBase(wlInst.Value())
	if err != nil {
		return err
	}

	comp.Workload = wl
	for _, s := range cm.Traits {
		auxInst, err := r.Compile("-", s)
		if err != nil {
			return err
		}
		aux, err := model.NewOther(auxInst.Value())
		if err != nil {
			return err
		}
		comp.Auxiliaries = append(comp.Auxiliaries, aux)
	}

	return nil
}

func NewContext(cli client.Client, ns, rev string) (Context, error) {

	var (
		ctx        = context.Background()
		manifestCm corev1.ConfigMap
	)

	if err := cli.Get(ctx, client.ObjectKey{
		Namespace: ns,
		Name:      rev,
	}, &manifestCm); err != nil {
		return nil, errors.WithMessagef(err, "Get manifest ConfigMap %s/%s ", ns, rev)
	}

	wfCtx, err := newContext(cli, ns, rev)
	if err != nil {
		return nil, err
	}
	if err := wfCtx.loadFromConfigMap(manifestCm); err != nil {
		return nil, errors.WithMessagef(err, "load from ConfigMap  %s/%s", ns, rev)
	}

	return wfCtx, wfCtx.Commit()
}

func newContext(cli client.Client, ns, rev string) (*workflowContext, error) {
	var (
		ctx   = context.Background()
		store corev1.ConfigMap
	)
	store.Name = generateStoreName(rev)
	store.Namespace = ns
	if err := cli.Get(ctx, client.ObjectKey{Name: store.Name, Namespace: store.Namespace}, &store); err != nil {
		if kerrors.IsNotFound(err) {
			if err := cli.Create(ctx, &store); err != nil {
				return nil, err
			}
		}
		return nil, err
	}
	store.Annotations[AnnotationStartTimestamp] = time.Now().String()
	wfCtx := &workflowContext{
		cli:        cli,
		store:      store,
		components: map[string]*componentManifest{},
	}
	var err error
	wfCtx.vars, err = value.NewValue("", nil)

	return wfCtx, err
}

func LoadContext(cli client.Client, ns, rev string) (Context, error) {
	var store corev1.ConfigMap
	if err := cli.Get(context.Background(), client.ObjectKey{
		Namespace: ns,
		Name:      generateStoreName(rev),
	}, &store); err != nil {
		return nil, err
	}
	ctx := &workflowContext{
		cli:   cli,
		store: store,
	}
	if err := ctx.loadFromConfigMap(store); err != nil {
		return nil, err
	}
	return ctx, nil
}

func generateStoreName(rev string) string {
	return "workflow-" + rev
}
