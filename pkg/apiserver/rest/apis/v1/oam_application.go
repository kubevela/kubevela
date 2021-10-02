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

package v1

import (
	"github.com/oam-dev/kubevela/apis/core.oam.dev/common"
	"github.com/oam-dev/kubevela/apis/core.oam.dev/v1beta1"
)

// ApplicationRequest represents application request for APIServer
type ApplicationRequest struct {
	Components []common.ApplicationComponent `json:"components"`
	Policies   []v1beta1.AppPolicy           `json:"policies,omitempty"`
	Workflow   *v1beta1.Workflow             `json:"workflow,omitempty"`
}

// ApplicationResponse represents application response for APIServer
type ApplicationResponse struct {
	APIVersion string                  `json:"apiVersion"`
	Kind       string                  `json:"kind"`
	Spec       v1beta1.ApplicationSpec `json:"spec"`
	Status     common.AppStatus        `json:"status"`
}
