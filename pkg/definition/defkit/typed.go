/*
Copyright 2025 The KubeVela Authors.

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

package defkit

import (
	"fmt"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
)

// FromTyped converts a typed Kubernetes object to a Resource.
// This provides compile-time type safety when building resources.
//
// Example:
//
//	import appsv1 "k8s.io/api/apps/v1"
//
//	deployment := &appsv1.Deployment{
//	    ObjectMeta: metav1.ObjectMeta{Name: "my-app"},
//	    Spec: appsv1.DeploymentSpec{
//	        Replicas: ptr.To(int32(3)),
//	        // ... other fields
//	    },
//	}
//	return defkit.FromTyped(deployment)
//
// The returned Resource contains Set operations for all fields in the object.
// You can chain additional Set operations on the returned Resource.
func FromTyped(obj runtime.Object) (*Resource, error) {
	// Convert to unstructured to access fields generically
	unstructuredObj, err := runtime.DefaultUnstructuredConverter.ToUnstructured(obj)
	if err != nil {
		return nil, fmt.Errorf("failed to convert object to unstructured: %w", err)
	}

	// Extract GVK from the unstructured object
	u := &unstructured.Unstructured{Object: unstructuredObj}
	gvk := u.GetObjectKind().GroupVersionKind()

	// If GVK is empty, it means TypeMeta wasn't set
	if gvk.Kind == "" {
		return nil, fmt.Errorf("unable to determine Kind for object; ensure TypeMeta is set on the object")
	}

	// Build apiVersion string
	apiVersion := gvk.Version
	if gvk.Group != "" {
		apiVersion = gvk.Group + "/" + gvk.Version
	}

	// Create resource
	r := NewResource(apiVersion, gvk.Kind)

	// Convert unstructured fields to Set operations
	// Skip apiVersion, kind as they're already set on the Resource
	for key, value := range unstructuredObj {
		if key == "apiVersion" || key == "kind" {
			continue
		}
		setFieldsFromUnstructured(r, key, value)
	}

	return r, nil
}

// MustFromTyped is like FromTyped but panics on error.
// Use this when you're confident the object is valid.
func MustFromTyped(obj runtime.Object) *Resource {
	r, err := FromTyped(obj)
	if err != nil {
		panic(err)
	}
	return r
}

// setFieldsFromUnstructured recursively sets fields on a Resource from unstructured data.
func setFieldsFromUnstructured(r *Resource, path string, value interface{}) {
	switch v := value.(type) {
	case map[string]interface{}:
		// Recursively handle nested objects
		for key, val := range v {
			nestedPath := path + "." + key
			setFieldsFromUnstructured(r, nestedPath, val)
		}
	case []interface{}:
		// For arrays, set the entire array as a literal value
		r.Set(path, Lit(v))
	case string:
		r.Set(path, Lit(v))
	case int64:
		r.Set(path, Lit(int(v)))
	case float64:
		// Check if it's actually an integer
		if v == float64(int64(v)) {
			r.Set(path, Lit(int(v)))
		} else {
			r.Set(path, Lit(v))
		}
	case bool:
		r.Set(path, Lit(v))
	case nil:
		// Skip nil values
	default:
		// For other types, use as literal
		r.Set(path, Lit(v))
	}
}
