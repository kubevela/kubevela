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

package apply

import (
	"strings"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"k8s.io/utils/strings/slices"
)

const (
	sharedBySep = ","
)

// CanonicalOwnerKey returns an owner key as <kind>/<namespace>/<name>. A key without a kind
// (<namespace>/<name>, or <name> in the default namespace) was written before owner kinds
// existed and belongs to Config.DefaultOwnerKind; with no default kind it is returned as is.
func CanonicalOwnerKey(key string) string {
	kind := current().DefaultOwnerKind
	if kind == "" || key == "" {
		return key
	}
	switch parts := strings.Split(key, "/"); len(parts) {
	case 1:
		return kind + "/" + metav1.NamespaceDefault + "/" + key
	case 2:
		return kind + "/" + key
	default:
		return key
	}
}

func sameOwner(a, b string) bool {
	return a == b || CanonicalOwnerKey(a) == CanonicalOwnerKey(b) // usually written the same way
}

// AddSharer adds the owner key to a shared-by list, unless the owner is already there in
// either form. The key is written as given.
func AddSharer(sharedBy string, key string) string {
	sharers := slices.Filter(nil, strings.Split(sharedBy, sharedBySep), func(s string) bool {
		return s != ""
	})
	for _, s := range sharers {
		if sameOwner(s, key) {
			return strings.Join(sharers, sharedBySep)
		}
	}
	return strings.Join(append(sharers, key), sharedBySep)
}

// ContainsSharer reports whether a shared-by list contains the owner, in either key form
func ContainsSharer(sharedBy string, key string) bool {
	for _, s := range strings.Split(sharedBy, sharedBySep) {
		if s != "" && sameOwner(s, key) {
			return true
		}
	}
	return false
}

// FirstSharer is the owner key that controls a shared resource: the first in the list, skipping
// empty entries, which a hand-written or legacy annotation can hold. "" if there is no sharer.
func FirstSharer(sharedBy string) string {
	for _, s := range strings.Split(sharedBy, sharedBySep) {
		if s != "" {
			return s
		}
	}
	return ""
}

// RemoveSharer removes the owner, in every key form, from a shared-by list
func RemoveSharer(sharedBy string, key string) string {
	sharers := strings.Split(sharedBy, sharedBySep)
	sharers = slices.Filter(nil, sharers, func(s string) bool {
		return s != "" && !sameOwner(s, key)
	})
	return strings.Join(sharers, sharedBySep)
}
