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

package api

import (
	"encoding/json"

	"k8s.io/apimachinery/pkg/runtime"

	"github.com/oam-dev/kubevela/apis/core.oam.dev/common"
	"github.com/oam-dev/kubevela/references/appfile/template"
)

// Service defines the service spec for AppFile, it will contain all related information including OAM component, traits, source to image, etc...
type Service map[string]interface{}

// DefaultWorkloadType defines the default service type if no type specified in Appfile
const DefaultWorkloadType = "webservice"

// GetType get type from AppFile
func (s Service) GetType() string {
	t, ok := s["type"]
	if !ok {
		return DefaultWorkloadType
	}
	return t.(string)
}

// GetUserConfigName get user config from AppFile, it will contain config file in it.
func (s Service) GetUserConfigName() string {
	t, ok := s["config"]
	if !ok {
		return ""
	}
	return t.(string)
}

// GetApplicationConfig will get OAM workload and trait information exclude inner section('build','type' and 'config')
func (s Service) GetApplicationConfig() map[string]interface{} {
	config := make(map[string]interface{})
outerLoop:
	for k, v := range s {
		switch k {
		case "build", "type", "config": // skip
			continue outerLoop
		}
		config[k] = v
	}
	return config
}

// RenderServiceToApplicationComponent render all capabilities of a service to CUE values to KubeVela Application.
func (s Service) RenderServiceToApplicationComponent(tm template.Manager, serviceName string) (common.ApplicationComponent, error) {

	// sort out configs by workload/trait
	workloadKeys := map[string]interface{}{}
	var traits []common.ApplicationTrait

	wtype := s.GetType()

	comp := common.ApplicationComponent{
		Name: serviceName,
		Type: wtype,
	}

	for k, v := range s.GetApplicationConfig() {
		if tm.IsTrait(k) {
			trait := common.ApplicationTrait{
				Type: k,
			}
			pts := &runtime.RawExtension{}
			jt, err := json.Marshal(v)
			if err != nil {
				return comp, err
			}
			if err := pts.UnmarshalJSON(jt); err != nil {
				return comp, err
			}
			trait.Properties = pts
			traits = append(traits, trait)
			continue
		}
		workloadKeys[k] = v
	}

	// Handle workloadKeys to settings
	settings := &runtime.RawExtension{}
	pt, err := json.Marshal(workloadKeys)
	if err != nil {
		return comp, err
	}
	if err := settings.UnmarshalJSON(pt); err != nil {
		return comp, err
	}
	comp.Properties = settings

	if len(traits) > 0 {
		comp.Traits = traits
	}

	return comp, nil
}

// GetServices will get all services defined in AppFile
func (af *AppFile) GetServices() map[string]Service {
	return af.Services
}
