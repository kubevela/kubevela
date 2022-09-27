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

// IntegrationRenderContext the default context values for render the integration
type IntegrationRenderContext struct {
	Name      string `json:"name"`
	Namespace string `json:"namespace"`
}

// ReadIntegrationProvider the provide function for reading the integration properties
type ReadIntegrationProvider func(ctx context.Context, namespace string, name string) (map[string]interface{}, error)
