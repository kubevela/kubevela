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
	"errors"

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
