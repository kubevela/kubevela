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

package context

import "context"

// DefaultContext the default context template
var DefaultContext = []byte(`
	context: {
		name: string
		namespace: string
	}
`)

// ConfigRenderContext the default context values for render the config
type ConfigRenderContext struct {
	Name      string `json:"name"`
	Namespace string `json:"namespace"`
}

// ReadConfigProvider the provide function for reading the config properties
type ReadConfigProvider func(ctx context.Context, namespace string, name string) (map[string]interface{}, error)
