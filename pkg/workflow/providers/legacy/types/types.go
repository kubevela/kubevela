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

package types

import (
	"context"
	"encoding/json"

	"cuelang.org/go/cue"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"

	"github.com/kubevela/workflow/pkg/types"
	"github.com/oam-dev/kubevela/apis/core.oam.dev/common"
	"github.com/oam-dev/kubevela/apis/core.oam.dev/v1beta1"
	"github.com/oam-dev/kubevela/pkg/appfile"
	"github.com/oam-dev/kubevela/pkg/controller/core.oam.dev/v1alpha2/application"
)

// ComponentApply apply oam component.
type ComponentApply func(ctx context.Context, comp common.ApplicationComponent, patcher cue.Value, clusterName string, overrideNamespace string, env string) (*unstructured.Unstructured, []*unstructured.Unstructured, bool, error)

// ComponentRender render oam component.
type ComponentRender func(ctx context.Context, comp common.ApplicationComponent, patcher cue.Value, clusterName string, overrideNamespace string, env string) (*unstructured.Unstructured, []*unstructured.Unstructured, error)

type contextKey string

const (
	componentApplyKey  contextKey = "componentApply"
	componentRenderKey contextKey = "componentRender"
	appKey             contextKey = "app"
	appfileKey         contextKey = "appfile"
)

// OAMParams is the params for oam
type RuntimeParams struct {
	ComponentApply  ComponentApply
	ComponentRender ComponentRender
	AppHandler      *application.AppHandler
	App             *v1beta1.Application
	Appfile         *appfile.Appfile
	AppParser       *appfile.Parser
	AppRev          *v1beta1.ApplicationRevision
	Action          types.Action
}

// LegacyParams is the legacy input parameters of a provider.
type OAMParams[T any] struct {
	Params T
	RuntimeParams
}

// LegacyGenericProviderFn is the legacy provider function
type OAMGenericProviderFn[T any, U any] func(context.Context, *OAMParams[T]) (*U, error)

// Call marshal value into json and decode into underlying function input
// parameters, then fill back the returned output value
func (fn OAMGenericProviderFn[T, U]) Call(ctx context.Context, value cue.Value) (cue.Value, error) {
	params := new(T)
	bs, err := value.MarshalJSON()
	if err != nil {
		return value, err
	}
	if err = json.Unmarshal(bs, params); err != nil {
		return value, err
	}
	runtimeParams := RuntimeParamsFrom(ctx)
	ret, err := fn(ctx, &OAMParams[T]{Params: *params, RuntimeParams: runtimeParams})
	if err != nil {
		return value, err
	}
	return value.FillPath(cue.ParsePath(""), ret), nil
}

// LegacyNativeProviderFn is the legacy native provider function
type OAMNativeProviderFn func(context.Context, *OAMParams[cue.Value]) (cue.Value, error)

// Call marshal value into json and decode into underlying function input
// parameters, then fill back the returned output value
func (fn OAMNativeProviderFn) Call(ctx context.Context, value cue.Value) (cue.Value, error) {
	runtimeParams := RuntimeParamsFrom(ctx)
	return fn(ctx, &OAMParams[cue.Value]{Params: value, RuntimeParams: runtimeParams})
}

// WithRuntimeParams returns a copy of parent in which the runtime params value is set
func WithRuntimeParams(parent context.Context, params RuntimeParams) context.Context {
	ctx := context.WithValue(parent, componentApplyKey, params.ComponentApply)
	ctx = context.WithValue(ctx, componentRenderKey, params.ComponentRender)
	ctx = context.WithValue(ctx, appKey, params.App)
	ctx = context.WithValue(ctx, appfileKey, params.Appfile)
	return ctx
}

// RuntimeParamsFrom returns the runtime params value stored in ctx, if any.
func RuntimeParamsFrom(ctx context.Context) RuntimeParams {
	params := RuntimeParams{}
	if apply, ok := ctx.Value(componentApplyKey).(ComponentApply); ok {
		params.ComponentApply = apply
	}
	if render, ok := ctx.Value(componentRenderKey).(ComponentRender); ok {
		params.ComponentRender = render
	}
	if app, ok := ctx.Value(appKey).(*v1beta1.Application); ok {
		params.App = app
	}
	if appfile, ok := ctx.Value(appfileKey).(*appfile.Appfile); ok {
		params.Appfile = appfile
	}
	return params
}
