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

package application

import (
	"os"
	"testing"

	utilfeature "k8s.io/apiserver/pkg/util/feature"

	"github.com/oam-dev/kubevela/pkg/features"
)

// $( ) expressions are off by default, so these tests would exercise the
// pass-through path rather than the thing they are about. Turning both gates on
// for the package puts them back where they were before the gate existed.
//
// Tests that are about the gate itself set it explicitly and do not rely on this.
func TestMain(m *testing.M) {
	if err := utilfeature.DefaultMutableFeatureGate.SetFromMap(map[string]bool{
		string(features.EnableCelExpressions):      true,
		string(features.RequireCelExpressionOptIn): false,
	}); err != nil {
		panic(err)
	}
	os.Exit(m.Run())
}
