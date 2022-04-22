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

package config

import (
	v1 "k8s.io/api/core/v1"

	"github.com/oam-dev/kubevela/apis/types"
)

// ProjectMatched will check whether a config secret can be used in a given project
func ProjectMatched(s *v1.Secret, project string) bool {
	if s.Labels[types.LabelConfigProject] == "" || s.Labels[types.LabelConfigProject] == project {
		return true
	}
	return false
}
