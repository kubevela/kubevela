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
