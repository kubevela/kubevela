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

package dispatch

import (
	"fmt"
	"strings"
)

// ConstructResourceTrackerName generates resource tracker name with given app revision name and namespace.
// App revision name and namespace must be non-empty.
// The logic of this package is highly coupled with this naming rule!
func ConstructResourceTrackerName(appRevName, ns string) string {
	return fmt.Sprintf("%s-%s", appRevName, ns)
}

func extractAppNameFromResourceTrackerName(name, ns string) string {
	splits := strings.Split(strings.TrimSuffix(name, "-"+ns), "-")
	return strings.Join(splits[0:len(splits)-1], "-")
}
