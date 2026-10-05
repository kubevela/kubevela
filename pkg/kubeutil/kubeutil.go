/*
Copyright 2026 The KubeVela Authors.

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

// Package kubeutil holds the small object helpers the resourcekeeper library needs, copied
// from pkg/oam/util and pkg/utils so the library depends on neither. Behaviour matches the
// originals, which kubeutil_test.go checks.
package kubeutil

import (
	"errors"
	"strconv"
	"strings"

	kerrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

// ErrBadRevision is returned by ExtractRevisionNum for a name without a -v<N> suffix.
const ErrBadRevision = "bad revision name"

type labelAnnotationObject interface {
	GetLabels() map[string]string
	SetLabels(labels map[string]string)
	GetAnnotations() map[string]string
	SetAnnotations(annotations map[string]string)
}

// AddLabels merges labels into the object's labels, overriding existing keys.
func AddLabels(o labelAnnotationObject, labels map[string]string) {
	o.SetLabels(mergeOverride(o.GetLabels(), labels))
}

// AddAnnotations merges annotations into the object's annotations, overriding existing keys.
func AddAnnotations(o labelAnnotationObject, annos map[string]string) {
	o.SetAnnotations(mergeOverride(o.GetAnnotations(), annos))
}

// RemoveLabels removes the given label keys.
func RemoveLabels(o labelAnnotationObject, removeKeys []string) {
	exist := o.GetLabels()
	for _, key := range removeKeys {
		delete(exist, key)
	}
	o.SetLabels(exist)
}

// RemoveAnnotations removes the given annotation keys.
func RemoveAnnotations(o labelAnnotationObject, removeKeys []string) {
	exist := o.GetAnnotations()
	for _, key := range removeKeys {
		delete(exist, key)
	}
	o.SetAnnotations(exist)
}

// ExtractRevisionNum returns N from a revision name ending <delimiter>vN.
func ExtractRevisionNum(appRevision string, delimiter string) (int, error) {
	splits := strings.Split(appRevision, delimiter)
	// check some bad appRevision name, eg:v1, appv2
	if len(splits) == 1 {
		return 0, errors.New(ErrBadRevision)
	}
	// check some bad appRevision name, eg:myapp-a1
	if !strings.HasPrefix(splits[len(splits)-1], "v") {
		return 0, errors.New(ErrBadRevision)
	}
	return strconv.Atoi(strings.TrimPrefix(splits[len(splits)-1], "v"))
}

// IsClusterScope reports whether gvk is a cluster-scoped kind.
func IsClusterScope(gvk schema.GroupVersionKind, mapper meta.RESTMapper) (bool, error) {
	mappings, err := mapper.RESTMappings(gvk.GroupKind(), gvk.Version)
	isClusterScope := len(mappings) > 0 && mappings[0].Scope.Name() == meta.RESTScopeNameRoot
	return isClusterScope, err
}

// EscapeResourceNameToLabelValue makes a resource name usable as a label value.
func EscapeResourceNameToLabelValue(resourceName string) string {
	return strings.ReplaceAll(resourceName, ":", "_")
}

// IsNotFoundOrClusterNotExists reports a NotFound error, or cluster-gateway's error for a
// cluster that does not exist (matched on its message, as pkg/multicluster does).
func IsNotFoundOrClusterNotExists(err error) bool {
	return kerrors.IsNotFound(err) || strings.Contains(err.Error(), "no such cluster")
}

// mergeOverride returns src with dst's entries written over it; nil if both are nil.
func mergeOverride(src, dst map[string]string) map[string]string {
	if src == nil && dst == nil {
		return nil
	}
	r := make(map[string]string)
	for k, v := range src {
		r[k] = v
	}
	for k, v := range dst {
		r[k] = v
	}
	return r
}
