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

package client

import (
	"context"

	sdkcommon "github.com/kubevela/vela-go-sdk/pkg/apis/common"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/rest"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/config"

	"github.com/oam-dev/kubevela-core-api/apis/core.oam.dev/v1beta1"

	"github.com/kubevela/vela-go-sdk/pkg/apis"
)

var schema *runtime.Scheme

type Client interface {
	Get(ctx context.Context, key client.ObjectKey) (apis.TypedApplication, error)
	List(ctx context.Context, opts client.ListOption) ([]apis.TypedApplication, error)
	Create(ctx context.Context, app apis.TypedApplication) error
	Delete(ctx context.Context, app apis.TypedApplication) error
	Update(ctx context.Context, app apis.TypedApplication) error
	Patch(ctx context.Context, app apis.TypedApplication, patch client.Patch) error
}

type clientImpl struct {
	clt client.Client
}

func init() {
	schema = runtime.NewScheme()
	_ = v1beta1.AddToScheme(schema)
}
func NewFromClient(clt client.Client) Client {
	return &clientImpl{clt: clt}
}

func New(config *rest.Config) (Client, error) {
	clt, err := client.New(config, client.Options{Scheme: schema})
	if err != nil {
		return nil, err
	}
	return NewFromClient(clt), nil
}

func NewDefault() (Client, error) {
	restConf, err := config.GetConfig()
	if err != nil {
		return nil, err
	}

	return New(restConf)
}

func NewDefaultOrDie() Client {
	restConf, err := config.GetConfig()
	if err != nil {
		panic(err)
	}

	clt, err := New(restConf)
	if err != nil {
		panic(err)
	}
	return clt
}

func NewFromConfigWithOptions(config *rest.Config, options client.Options) (Client, error) {
	clt, err := client.New(config, options)
	if err != nil {
		return nil, err
	}
	return NewFromClient(clt), nil
}

func (c clientImpl) Get(ctx context.Context, key client.ObjectKey) (apis.TypedApplication, error) {
	_app := v1beta1.Application{}
	err := c.clt.Get(ctx, key, &_app)
	if err != nil {
		return nil, err
	}
	return sdkcommon.FromK8sObject(_app)
}

func (c clientImpl) List(ctx context.Context, opts client.ListOption) ([]apis.TypedApplication, error) {
	_appList := &v1beta1.ApplicationList{}
	err := c.clt.List(ctx, _appList, opts)
	if err != nil {
		return nil, err
	}
	var apps []apis.TypedApplication
	for _, app := range _appList.Items {
		_app, err := sdkcommon.FromK8sObject(app)
		if err != nil {
			return nil, err
		}
		apps = append(apps, _app)
	}
	return apps, nil
}

func (c clientImpl) Create(ctx context.Context, app apis.TypedApplication) error {
	appObj := app.Build()
	return c.clt.Create(ctx, &appObj)
}

func (c clientImpl) Delete(ctx context.Context, app apis.TypedApplication) error {
	appObj := app.Build()
	return c.clt.Delete(ctx, &appObj)
}

func (c clientImpl) Update(ctx context.Context, app apis.TypedApplication) error {
	appObj := app.Build()
	return c.clt.Update(ctx, &appObj)
}

func (c clientImpl) Patch(ctx context.Context, app apis.TypedApplication, patch client.Patch) error {
	appObj := app.Build()
	return c.clt.Patch(ctx, &appObj, patch)
}
