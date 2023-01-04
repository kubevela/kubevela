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

package features

import (
	"k8s.io/apimachinery/pkg/util/runtime"
	"k8s.io/component-base/featuregate"
)

var (
	// APIServerMutableFeatureGate is a mutable version of APIServerFeatureGate
	APIServerMutableFeatureGate featuregate.MutableFeatureGate = featuregate.NewFeatureGate()

	// APIServerFeatureGate is a shared global FeatureGate for apiserver.
	APIServerFeatureGate featuregate.FeatureGate = APIServerMutableFeatureGate
)

const (
	// APIServerEnableImpersonation whether to enable impersonation for APIServer
	APIServerEnableImpersonation featuregate.Feature = "EnableImpersonation"
	// APIServerEnableAdminImpersonation whether to disable User admin impersonation for APIServer
	APIServerEnableAdminImpersonation featuregate.Feature = "EnableAdminImpersonation"
)

func init() {
	runtime.Must(APIServerMutableFeatureGate.Add(map[featuregate.Feature]featuregate.FeatureSpec{
		APIServerEnableImpersonation:      {Default: false, PreRelease: featuregate.Alpha},
		APIServerEnableAdminImpersonation: {Default: true, PreRelease: featuregate.Alpha},
	}))
}
