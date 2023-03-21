/*
Copyright 2023 The KubeVela Authors.

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

package cuegen

// RegisterAny registers go types' package+name as any type({...} in CUE)
//
// Example:RegisterAny("*k8s.io/apimachinery/pkg/apis/meta/v1/unstructured.Unstructured")
//
// Default any types are: map[string]interface{}, map[string]any, interface{}, any
func (g *Generator) RegisterAny(types ...string) {
	for _, t := range types {
		g.anyTypes[t] = struct{}{}
	}
}
