package context

import (
	"context"
	"encoding/json"
	"time"

	"cuelang.org/go/cue"
	runtimev1alpha1 "github.com/crossplane/crossplane-runtime/apis/core/v1alpha1"
	"github.com/pkg/errors"
	corev1 "k8s.io/api/core/v1"
	kerrors "k8s.io/apimachinery/pkg/api/errors"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/oam-dev/kubevela/pkg/cue/model"
	"github.com/oam-dev/kubevela/pkg/cue/model/value"
	"github.com/oam-dev/kubevela/pkg/oam/util"
)

const (
	// ConfigMapKeyComponents is the key in ConfigMap Data field for containing data of components
	ConfigMapKeyComponents = "components"
	// ConfigMapKeyVars is the key in ConfigMap Data field for containing data of variable
	ConfigMapKeyVars = "vars"
	// AnnotationStartTimestamp is the annotation key of the workflow start  timestamp
	AnnotationStartTimestamp = "vela.io/startTime"
)

// WorkflowContext is workflow context.
type WorkflowContext struct {
	cli        client.Client
	store      corev1.ConfigMap
	components map[string]*componentManifest
	vars       *value.Value
}

// GetComponent Get ComponentManifest from workflow context.
func (wf *WorkflowContext) GetComponent(name string) (*componentManifest, error) {
	component, ok := wf.components[name]
	if !ok {
		return nil, errors.Errorf("component %s not found in application", name)
	}
	return component, nil
}

// PatchComponent patch component with value.
func (wf *WorkflowContext) PatchComponent(name string, patchValue *value.Value) error {
	component, err := wf.GetComponent(name)
	if err != nil {
		return err
	}
	return component.Patch(patchValue)
}

// GetVar get variable from workflow context.
func (wf *WorkflowContext) GetVar(paths ...string) (*value.Value, error) {
	return wf.vars.LookupValue(paths...)
}

// SetVar set variable to workflow context.
func (wf *WorkflowContext) SetVar(v *value.Value, paths ...string) error {
	str, err := v.String()
	if err != nil {
		return errors.WithMessage(err, "compile var")
	}
	return wf.vars.FillRaw(str, paths...)
}

// MakeParameter make 'value' with map[string]interface{}
func (wf *WorkflowContext) MakeParameter(parameter map[string]interface{}) (*value.Value, error) {
	var s = "{}"
	if parameter != nil {
		s = string(util.MustJSONMarshal(parameter))
	}

	return wf.vars.MakeValue(s)
}

// Commit the workflow context and persist it's content.
func (wf *WorkflowContext) Commit() error {
	wf.writeToStore()
	if err := wf.sync(); err != nil {
		return errors.WithMessagef(err, "save context to configMap(%s/%s)", wf.store.Namespace, wf.store.Name)
	}
	return nil
}

func (wf *WorkflowContext) writeToStore() error {
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
	return nil
}

func (wf *WorkflowContext) sync() error {
	ctx := context.Background()
	if err := wf.cli.Update(ctx, &wf.store); err != nil {
		if kerrors.IsNotFound(err) {
			return wf.cli.Create(ctx, &wf.store)
		}
		return err
	}
	return nil
}

// LoadFromConfigMap recover workflow context from configMap.
func (wf *WorkflowContext) LoadFromConfigMap(cm corev1.ConfigMap) error {
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

// StoreRef return the store reference of workflow context.
func (wf *WorkflowContext) StoreRef() *runtimev1alpha1.TypedReference {
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

// Patch the componentManifest with value
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

// NewContext new workflow context.
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
	if err := wfCtx.LoadFromConfigMap(manifestCm); err != nil {
		return nil, errors.WithMessagef(err, "load from ConfigMap  %s/%s", ns, rev)
	}

	return wfCtx, wfCtx.Commit()
}

func newContext(cli client.Client, ns, rev string) (*WorkflowContext, error) {
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
	store.Annotations = map[string]string{
		AnnotationStartTimestamp: time.Now().String(),
	}
	wfCtx := &WorkflowContext{
		cli:        cli,
		store:      store,
		components: map[string]*componentManifest{},
	}
	var err error
	wfCtx.vars, err = value.NewValue("", nil)

	return wfCtx, err
}

// LoadContext load workflow context from store.
func LoadContext(cli client.Client, ns, rev string) (Context, error) {
	var store corev1.ConfigMap
	if err := cli.Get(context.Background(), client.ObjectKey{
		Namespace: ns,
		Name:      generateStoreName(rev),
	}, &store); err != nil {
		return nil, err
	}
	ctx := &WorkflowContext{
		cli:   cli,
		store: store,
	}
	if err := ctx.LoadFromConfigMap(store); err != nil {
		return nil, err
	}
	return ctx, nil
}

func generateStoreName(rev string) string {
	return "workflow-" + rev
}
