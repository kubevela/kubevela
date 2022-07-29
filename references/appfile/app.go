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

package appfile

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"reflect"
	"sort"

	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/oam-dev/kubevela/apis/core.oam.dev/v1alpha2"
	"github.com/oam-dev/kubevela/apis/core.oam.dev/v1beta1"
	"github.com/oam-dev/kubevela/apis/types"
	"github.com/oam-dev/kubevela/pkg/utils/common"
	"github.com/oam-dev/kubevela/references/appfile/api"
	"github.com/oam-dev/kubevela/references/appfile/template"
)

// NewEmptyApplication new empty application, only set tm
func NewEmptyApplication(namespace string, c common.Args) (*api.Application, error) {
	tm, err := template.Load(namespace, c)
	if err != nil {
		return nil, err
	}
	return NewApplication(nil, tm), nil
}

// NewApplication will create application object
func NewApplication(f *api.AppFile, tm template.Manager) *api.Application {
	if f == nil {
		f = api.NewAppFile()
	}
	return &api.Application{AppFile: f, Tm: tm}
}

// Validate will validate whether an Appfile is valid.
func Validate(app *api.Application) error {
	if app.Name == "" {
		return errors.New("name is required")
	}
	if len(app.Services) == 0 {
		return errors.New("at least one service is required")
	}
	for name, svc := range app.Services {
		for traitName, traitData := range svc.GetApplicationConfig() {
			if app.Tm.IsTrait(traitName) {
				if _, ok := traitData.(map[string]interface{}); !ok {
					return fmt.Errorf("trait %s in '%s' must be map", traitName, name)
				}
			}
		}
	}
	return nil
}

// LoadApplication will load application from cluster.
func LoadApplication(namespace, appName string, c common.Args) (*v1beta1.Application, error) {
	newClient, err := c.GetClient()
	if err != nil {
		return nil, err
	}
	app := &v1beta1.Application{}
	if err := newClient.Get(context.TODO(), client.ObjectKey{Namespace: namespace, Name: appName}, app); err != nil {
		return nil, fmt.Errorf("failed to load application %s from namespace %s: %w", appName, namespace, err)
	}
	return app, nil
}

// GetComponents will get oam components from v1beta1.Application.
func GetComponents(app *v1beta1.Application) []string {
	var components []string
	for _, cmp := range app.Spec.Components {
		components = append(components, cmp.Name)
	}
	sort.Strings(components)
	return components
}

// GetServiceConfig will get service type and it's configuration
func GetServiceConfig(app *api.Application, componentName string) (string, map[string]interface{}) {
	svc, ok := app.Services[componentName]
	if !ok {
		return "", make(map[string]interface{})
	}
	return svc.GetType(), svc.GetApplicationConfig()
}

// GetApplicationSettings will get service type and it's configuration
func GetApplicationSettings(app *v1beta1.Application, componentName string) (string, map[string]interface{}) {
	for _, comp := range app.Spec.Components {
		if comp.Name == componentName {
			data := map[string]interface{}{}
			if comp.Properties != nil {
				_ = json.Unmarshal(comp.Properties.Raw, &data)
			}
			return comp.Type, data
		}
	}
	return "", make(map[string]interface{})
}

// GetWorkload will get workload type and it's configuration
func GetWorkload(app *api.Application, componentName string) (string, map[string]interface{}) {
	svcType, config := GetServiceConfig(app, componentName)
	if svcType == "" {
		return "", make(map[string]interface{})
	}
	workloadData := make(map[string]interface{})
	for k, v := range config {
		if app.Tm.IsTrait(k) {
			continue
		}
		workloadData[k] = v
	}
	return svcType, workloadData
}

// GetTraits will list all traits and it's configurations attached to the specified component.
func GetTraits(app *api.Application, componentName string) (map[string]map[string]interface{}, error) {
	_, config := GetServiceConfig(app, componentName)
	traitsData := make(map[string]map[string]interface{})
	for k, v := range config {
		if !app.Tm.IsTrait(k) {
			continue
		}
		newV, ok := v.(map[string]interface{})
		if !ok {
			return nil, fmt.Errorf("%s is trait, but with invalid format %s, should be map[string]interface{}", k, reflect.TypeOf(v))
		}
		traitsData[k] = newV
	}
	return traitsData, nil
}

// GetAppConfig will get AppConfig from K8s cluster.
func GetAppConfig(ctx context.Context, c client.Client, app *v1beta1.Application, env *types.EnvMeta) (*v1alpha2.ApplicationConfiguration, error) {
	appConfig := &v1alpha2.ApplicationConfiguration{}
	if err := c.Get(ctx, client.ObjectKey{Namespace: env.Namespace, Name: app.Name}, appConfig); err != nil {
		return nil, err
	}
	return appConfig, nil
}
