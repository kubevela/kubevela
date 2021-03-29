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

package appdeployment

import (
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

type workload struct {
	Object *unstructured.Unstructured

	traits []*workloadTrait
}

type workloadTrait struct {
	Object *unstructured.Unstructured
}

func newWorkload(name, ns string, obj map[string]interface{}, traits []*workloadTrait) *workload {
	u := &unstructured.Unstructured{
		Object: obj,
	}
	u.SetName(name)
	u.SetNamespace(ns)
	return &workload{
		Object: u,
		traits: traits,
	}
}
func newWorkloadTrait(name, ns string, obj map[string]interface{}) *workloadTrait {
	u := &unstructured.Unstructured{
		Object: obj,
	}
	u.SetName(name)
	u.SetNamespace(ns)
	return &workloadTrait{
		Object: u,
	}
}
