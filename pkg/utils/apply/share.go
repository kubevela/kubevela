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

	"k8s.io/utils/strings/slices"

	"github.com/oam-dev/kubevela/apis/core.oam.dev/v1beta1"
)

const (
	sharedBySep = ","
)

// AddSharer add sharer
func AddSharer(sharedBy string, app *v1beta1.Application) string {
	key := GetAppKey(app)
	sharers := strings.Split(sharedBy, sharedBySep)
	existing := slices.Contains(sharers, key)
	if !existing {
		sharers = append(slices.Filter(nil, sharers, func(s string) bool {
			return s != ""
		}), key)
	}
	return strings.Join(sharers, sharedBySep)
}

// ContainsSharer check if the shared-by annotation contains the target application
func ContainsSharer(sharedBy string, app *v1beta1.Application) bool {
	key := GetAppKey(app)
	sharers := strings.Split(sharedBy, sharedBySep)
	return slices.Contains(sharers, key)
}

// FirstSharer get the first sharer of the application
func FirstSharer(sharedBy string) string {
	sharers := strings.Split(sharedBy, sharedBySep)
	return sharers[0]
}

// RemoveSharer remove sharer
func RemoveSharer(sharedBy string, app *v1beta1.Application) string {
	key := GetAppKey(app)
	sharers := strings.Split(sharedBy, sharedBySep)
	sharers = slices.Filter(nil, sharers, func(s string) bool {
		return s != key
	})
	return strings.Join(sharers, sharedBySep)
}
