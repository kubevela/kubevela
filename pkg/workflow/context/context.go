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

package context

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"sync"
	"time"

	"cuelang.org/go/cue"
	"github.com/pkg/errors"
	corev1 "k8s.io/api/core/v1"
	kerrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/utils/pointer"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/oam-dev/kubevela/apis/core.oam.dev/v1beta1"
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

var (
	workflowMemoryCache sync.Map
)

// WorkflowContext is workflow context.
type WorkflowContext struct {
	cli         client.Client
	store       *corev1.ConfigMap
	memoryStore *sync.Map
	components  map[string]*ComponentManifest
	vars        *value.Value
	modified    bool
}

// GetComponent Get ComponentManifest from workflow context.
func (wf *WorkflowContext) GetComponent(name string) (*ComponentManifest, error) {
	component, ok := wf.components[name]
	if !ok {
		return nil, errors.Errorf("component %s not found in application", name)
	}
	return component, nil
}

// GetComponents Get All ComponentManifest from workflow context.
func (wf *WorkflowContext) GetComponents() map[string]*ComponentManifest {
	return wf.components
}

// PatchComponent patch component with value.
func (wf *WorkflowContext) PatchComponent(name string, patchValue *value.Value) error {
	component, err := wf.GetComponent(name)
	if err != nil {
		return err
	}
	if err := component.Patch(patchValue); err != nil {
		return err
	}
	wf.modified = true
	return nil
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
	if err := wf.vars.FillRaw(str, paths...); err != nil {
		return err
	}
	if err := wf.vars.Error(); err != nil {
		return err
	}
	wf.modified = true
	return nil
}

// GetStore get store of workflow context.
func (wf *WorkflowContext) GetStore() *corev1.ConfigMap {
	return wf.store
}

// GetMutableValue get mutable data from workflow context.
func (wf *WorkflowContext) GetMutableValue(paths ...string) string {
	return wf.store.Data[strings.Join(paths, ".")]
}

// SetMutableValue set mutable data in workflow context config map.
func (wf *WorkflowContext) SetMutableValue(data string, paths ...string) {
	wf.store.Data[strings.Join(paths, ".")] = data
	wf.modified = true
}

// DeleteMutableValue delete mutable data in workflow context.
func (wf *WorkflowContext) DeleteMutableValue(paths ...string) {
	key := strings.Join(paths, ".")
	if _, ok := wf.store.Data[key]; ok {
		delete(wf.store.Data, strings.Join(paths, "."))
		wf.modified = true
	}
}

// IncreaseCountValueInMemory increase count in workflow context memory store.
func (wf *WorkflowContext) IncreaseCountValueInMemory(paths ...string) int {
	key := strings.Join(paths, ".")
	c, ok := wf.memoryStore.Load(key)
	if !ok {
		wf.memoryStore.Store(key, 0)
		return 0
	}
	count, ok := c.(int)
	if !ok {
		wf.memoryStore.Store(key, 0)
		return 0
	}
	count++
	wf.memoryStore.Store(key, count)
	return count
}

// SetValueInMemory set data in workflow context memory store.
func (wf *WorkflowContext) SetValueInMemory(data interface{}, paths ...string) {
	wf.memoryStore.Store(strings.Join(paths, "."), data)
}

// GetValueInMemory get data in workflow context memory store.
func (wf *WorkflowContext) GetValueInMemory(paths ...string) (interface{}, bool) {
	return wf.memoryStore.Load(strings.Join(paths, "."))
}

// DeleteValueInMemory delete data in workflow context memory store.
func (wf *WorkflowContext) DeleteValueInMemory(paths ...string) {
	wf.memoryStore.Delete(strings.Join(paths, "."))
}

// MakeParameter make 'value' with interface{}
func (wf *WorkflowContext) MakeParameter(parameter interface{}) (*value.Value, error) {
	var s = "{}"
	if parameter != nil {
		bt, err := json.Marshal(parameter)
		if err != nil {
			return nil, err
		}
		s = string(bt)
	}

	return wf.vars.MakeValue(s)
}

// Commit the workflow context and persist it's content.
func (wf *WorkflowContext) Commit() error {
	if !wf.modified {
		return nil
	}
	if err := wf.writeToStore(); err != nil {
		return err
	}
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

	if wf.store.Data == nil {
		wf.store.Data = make(map[string]string)
	}
	wf.store.Data[ConfigMapKeyComponents] = string(util.MustJSONMarshal(jsonObject))
	wf.store.Data[ConfigMapKeyVars] = varStr
	return nil
}

func (wf *WorkflowContext) sync() error {
	ctx := context.Background()
	if EnableInMemoryContext {
		MemStore.UpdateInMemoryContext(wf.store)
	} else if err := wf.cli.Update(ctx, wf.store); err != nil {
		if kerrors.IsNotFound(err) {
			return wf.cli.Create(ctx, wf.store)
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
	wf.components = map[string]*ComponentManifest{}
	for name, compJs := range componentsJs {
		cm := new(ComponentManifest)
		if err := cm.unmarshal(compJs); err != nil {
			return errors.WithMessagef(err, "unmarshal component(%s) manifest", name)
		}
		wf.components[name] = cm
	}
	var err error
	wf.vars, err = value.NewValue(data[ConfigMapKeyVars], nil, "")
	if err != nil {
		return errors.WithMessage(err, "decode vars")
	}
	return nil
}

// StoreRef return the store reference of workflow context.
func (wf *WorkflowContext) StoreRef() *corev1.ObjectReference {
	return &corev1.ObjectReference{
		APIVersion: wf.store.APIVersion,
		Kind:       wf.store.Kind,
		Name:       wf.store.Name,
		UID:        wf.store.UID,
	}
}

// ComponentManifest contains resources rendered from an application component.
type ComponentManifest struct {
	Workload    model.Instance
	Auxiliaries []model.Instance
}

// Patch the ComponentManifest with value
func (comp *ComponentManifest) Patch(patchValue *value.Value) error {
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

func (comp *ComponentManifest) string() (string, error) {
	cm := componentMould{
		StandardWorkload: comp.Workload.String(),
	}
	for _, aux := range comp.Auxiliaries {
		cm.Traits = append(cm.Traits, aux.String())
	}
	js, err := json.Marshal(cm)
	return string(js), err
}

func (comp *ComponentManifest) unmarshal(v string) error {

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

// NewContext new workflow context without initialize data.
func NewContext(cli client.Client, ns, app string, appUID types.UID) (Context, error) {
	wfCtx, err := newContext(cli, ns, app, appUID)
	if err != nil {
		return nil, err
	}

	return wfCtx, wfCtx.Commit()
}

// CleanupMemoryStore cleans up memory store.
func CleanupMemoryStore(app, ns string) {
	workflowMemoryCache.Delete(fmt.Sprintf("%s-%s", app, ns))
}

func newContext(cli client.Client, ns, app string, appUID types.UID) (*WorkflowContext, error) {
	var (
		ctx   = context.Background()
		store corev1.ConfigMap
	)
	store.Name = generateStoreName(app)
	store.Namespace = ns
	store.SetOwnerReferences([]metav1.OwnerReference{
		{
			APIVersion: v1beta1.SchemeGroupVersion.String(),
			Kind:       v1beta1.ApplicationKind,
			Name:       app,
			UID:        appUID,
			Controller: pointer.BoolPtr(true),
		},
	})
	if EnableInMemoryContext {
		MemStore.GetOrCreateInMemoryContext(&store)
	} else if err := cli.Get(ctx, client.ObjectKey{Name: store.Name, Namespace: store.Namespace}, &store); err != nil {
		if kerrors.IsNotFound(err) {
			if err := cli.Create(ctx, &store); err != nil {
				return nil, err
			}
		} else {
			return nil, err
		}
	}
	store.Annotations = map[string]string{
		AnnotationStartTimestamp: time.Now().String(),
	}
	memCache := getMemoryStore(fmt.Sprintf("%s-%s", app, ns))
	wfCtx := &WorkflowContext{
		cli:         cli,
		store:       &store,
		memoryStore: memCache,
		components:  map[string]*ComponentManifest{},
		modified:    true,
	}
	var err error
	wfCtx.vars, err = value.NewValue("", nil, "")

	return wfCtx, err
}

func getMemoryStore(key string) *sync.Map {
	memCache := &sync.Map{}
	mc, ok := workflowMemoryCache.Load(key)
	if !ok {
		workflowMemoryCache.Store(key, memCache)
	} else {
		memCache, ok = mc.(*sync.Map)
		if !ok {
			workflowMemoryCache.Store(key, memCache)
		}
	}
	return memCache
}

// LoadContext load workflow context from store.
func LoadContext(cli client.Client, ns, app string) (Context, error) {
	var store corev1.ConfigMap
	store.Name = generateStoreName(app)
	store.Namespace = ns
	if EnableInMemoryContext {
		MemStore.GetOrCreateInMemoryContext(&store)
	} else if err := cli.Get(context.Background(), client.ObjectKey{
		Namespace: ns,
		Name:      generateStoreName(app),
	}, &store); err != nil {
		return nil, err
	}
	memCache := getMemoryStore(fmt.Sprintf("%s-%s", app, ns))
	ctx := &WorkflowContext{
		cli:         cli,
		store:       &store,
		memoryStore: memCache,
	}
	if err := ctx.LoadFromConfigMap(store); err != nil {
		return nil, err
	}
	return ctx, nil
}

// generateStoreName generates the config map name of workflow context.
func generateStoreName(app string) string {
	return fmt.Sprintf("workflow-%s-context", app)
}
