/*
Copyright 2021 The Crossplane Authors.

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

package applicationconfiguration

import (
	"reflect"

	"github.com/crossplane/crossplane-runtime/pkg/fieldpath"
	"github.com/pkg/errors"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"

	"github.com/oam-dev/kubevela/apis/core.oam.dev/v1alpha2"
)

// dag is the dependency graph for an AppConfig.
type dag struct {
	Sources map[string]*dagSource
}

// dagSource represents the object information with DataOutput
type dagSource struct {
	// ObjectRef refers to the object this source come from.
	ObjectRef *corev1.ObjectReference

	Conditions []v1alpha2.ConditionRequirement
}

// newDAG creates a fresh new DAG.
func newDAG() *dag {
	return &dag{
		Sources: make(map[string]*dagSource),
	}
}

// AddSource adds a data output source into the DAG.
func (d *dag) AddSource(sourceName string, ref *corev1.ObjectReference, m []v1alpha2.ConditionRequirement) {
	d.Sources[sourceName] = &dagSource{
		ObjectRef:  ref,
		Conditions: m,
	}
}

func fillDataInputValue(obj *unstructured.Unstructured, fs []string, val interface{}, strategyMergeKeys []string) error {
	paved := fieldpath.Pave(obj.Object)
	for _, fp := range fs {
		toSet := val

		// Special case for slice because we will append or strategyMerge instead of rewriting.
		if reflect.TypeOf(val).Kind() == reflect.Slice {
			raw, err := paved.GetValue(fp)
			if err != nil {
				if fieldpath.IsNotFound(err) {
					raw = make([]interface{}, 0)
				} else {
					return err
				}
			}
			l := raw.([]interface{})
			toSet = strategyMergeSlice(l, val.([]interface{}), strategyMergeKeys)
		}

		err := paved.SetValue(fp, toSet)
		if err != nil {
			return errors.Wrap(err, "paved.SetValue() failed")
		}
	}
	return nil
}

func getElementValueByKeys(ele interface{}, keys []string) map[string]string {
	mappedEle, ok := ele.(map[string]interface{})
	if !ok {
		return nil
	}
	pavedEle := fieldpath.Pave(mappedEle)
	var result = make(map[string]string)
	for _, key := range keys {
		keyValuePatch, err := pavedEle.GetString(key)
		if err != nil {
			continue
		}
		result[key] = keyValuePatch
	}
	return result
}

func compareResults(base, patch map[string]string) bool {
	for k, v := range patch {
		vv, ok := base[k]
		if ok && vv == v {
			return true
		}
	}
	return false
}

func strategyMergeSlice(base, patch []interface{}, keys []string) []interface{} {
	if len(keys) == 0 {
		// By default, no merge keys, append only.
		base = append(base, patch...)
		return base
	}
	for _, patchElement := range patch {
		// get all values by mergeKeys from patch element
		patchKeyResults := getElementValueByKeys(patchElement, keys)
		if len(patchKeyResults) == 0 {
			base = append(base, patchElement)
			continue
		}
		var match = false
		for idx, v := range base {
			// get all values by mergeKeys from base element
			baseKeyResults := getElementValueByKeys(v, keys)
			// compare the key value pairs
			match = compareResults(baseKeyResults, patchKeyResults)
			if !match {
				continue
			}
			base[idx] = patchElement
			break
		}
		if match {
			continue
		}
		// if no matches, append at last
		base = append(base, patchElement)
	}
	return base
}
