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

package util

import (
	"fmt"
	"time"

	"cuelang.org/go/pkg/strings"
)

// GenerateVersion Generate version numbers by time
func GenerateVersion(pre string) string {
	timeStr := time.Now().Format("20060102150405.000")
	timeStr = strings.Replace(timeStr, ".", "", 1)
	if pre != "" {
		return fmt.Sprintf("%s-%s", pre, timeStr)
	}
	return timeStr
}
