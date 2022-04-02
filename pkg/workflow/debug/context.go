/*
Copyright 2022 The KubeVela Authors.

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

package debug

import (
	"context"
	"fmt"
	"reflect"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/oam-dev/kubevela/apis/core.oam.dev/common"
	"github.com/oam-dev/kubevela/apis/core.oam.dev/v1beta1"
	"github.com/oam-dev/kubevela/pkg/cue/model/value"
	"github.com/oam-dev/kubevela/pkg/oam/util"
	"github.com/oam-dev/kubevela/pkg/resourcekeeper"
	"github.com/oam-dev/kubevela/pkg/utils/apply"
)

// ContextImpl is workflow debug context interface
type ContextImpl interface {
	Set(v *value.Value) error
}

// Context is debug context.
type Context struct {
	cli  client.Client
	app  *v1beta1.Application
	step string
}

// Set sets debug content into context
func (d *Context) Set(v *value.Value) error {
	data, err := v.String()
	if err != nil {
		return err
	}
	err = setStore(context.Background(), d.cli, d.app, d.step, data)
	if err != nil {
		return err
	}

	return nil
}

func setStore(ctx context.Context, cli client.Client, app *v1beta1.Application, step, data string) error {
	cm := &corev1.ConfigMap{}
	if err := cli.Get(ctx, types.NamespacedName{
		Namespace: app.Namespace,
		Name:      GenerateContextName(app.Name, step),
	}, cm); err != nil {
		if errors.IsNotFound(err) {
			rk, err := resourcekeeper.NewResourceKeeper(ctx, cli, app)
			if err != nil {
				return err
			}
			cm.Name = GenerateContextName(app.Name, step)
			cm.Namespace = app.Namespace
			cm.Data = map[string]string{
				"debug": data,
			}
			u, err := util.Object2Unstructured(cm)
			if err != nil {
				return err
			}
			u.SetGroupVersionKind(
				corev1.SchemeGroupVersion.WithKind(
					reflect.TypeOf(corev1.ConfigMap{}).Name(),
				),
			)
			if err := rk.Dispatch(ctx, []*unstructured.Unstructured{u}, []apply.ApplyOption{apply.DisableUpdateAnnotation()}, resourcekeeper.MetaOnlyOption{}, resourcekeeper.CreatorOption{Creator: common.DebugResourceCreator}); err != nil {
				return err
			}
			return nil
		}
		return err
	}
	cm.Data = map[string]string{
		"debug": data,
	}
	if err := cli.Update(ctx, cm); err != nil {
		return err
	}

	return nil
}

// NewContext new workflow context without initialize data.
func NewContext(cli client.Client, app *v1beta1.Application, step string) ContextImpl {
	return &Context{
		cli:  cli,
		app:  app,
		step: step,
	}
}

// GenerateContextName generate context name
func GenerateContextName(app, step string) string {
	return fmt.Sprintf("%s-%s-debug", app, step)
}
