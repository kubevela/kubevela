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
	"encoding/json"
	"errors"
	"fmt"

	"k8s.io/apimachinery/pkg/runtime"

	"github.com/oam-dev/kubevela/apis/core.oam.dev/v1beta1"
	"github.com/oam-dev/kubevela/references/appfile/api"
)

var errorAppNilPointer = errors.New("app is nil pointer")

// SetWorkload will set user workload for Appfile
func SetWorkload(app *api.Application, componentName, workloadType string, workloadData map[string]interface{}) error {
	if app == nil {
		return errorAppNilPointer
	}

	s, ok := app.Services[componentName]
	if !ok {
		s = api.Service{}
	}
	s["type"] = workloadType
	for k, v := range workloadData {
		s[k] = v
	}
	app.Services[componentName] = s
	return Validate(app)
}

// SetTrait will set user trait for Appfile
func SetTrait(app *v1beta1.Application, componentName, traitType string, traitData map[string]interface{}) error {
	if app == nil {
		return errorAppNilPointer
	}
	if traitData == nil {
		traitData = make(map[string]interface{})
	}
	data, err := json.Marshal(traitData)
	if err != nil {
		return fmt.Errorf("fail to marshal trait data %w", err)
	}
	var foundComp bool
	for idx, comp := range app.Spec.Components {
		if comp.Name != componentName {
			continue
		}
		foundComp = true
		var added bool
		for j, tr := range app.Spec.Components[idx].Traits {
			if tr.Type != traitType {
				continue
			}
			added = true
			app.Spec.Components[idx].Traits[j].Properties.Raw = data
		}
		if !added {
			app.Spec.Components[idx].Traits = append(app.Spec.Components[idx].Traits, v1beta1.ApplicationTrait{Type: traitType, Properties: runtime.RawExtension{Raw: data}})
		}
	}
	if !foundComp {
		return errors.New(componentName + " not found in app " + app.Name)
	}
	return nil
}

// RemoveTrait will remove a trait from Appfile
func RemoveTrait(app *api.Application, componentName, traitType string) error {
	if app == nil {
		return errorAppNilPointer
	}

	s, ok := app.Services[componentName]
	if !ok {
		return nil
	}
	delete(s, traitType)
	return nil
}

// RemoveComponent will remove component from Application
func RemoveComponent(app *v1beta1.Application, componentName string) error {
	if app == nil {
		return errorAppNilPointer
	}
	var newComps []v1beta1.ApplicationComponent
	for _, comp := range app.Spec.Components {
		if comp.Name == componentName {
			continue
		}
		newComps = append(newComps, comp)
	}
	app.Spec.Components = newComps
	return nil
}
