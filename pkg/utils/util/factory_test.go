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
	"testing"

	"github.com/oam-dev/kubevela/version"
)

func TestGenerateLeaderElectionID(t *testing.T) {
	version.VelaVersion = "v10.13.0"
	if id := GenerateLeaderElectionID("kubevela", true); id != "kubevela-v10-13-0" {
		t.Errorf("id is not as expected(%s != kubevela-v10-13-0)", id)
		return
	}
	if id := GenerateLeaderElectionID("kubevela", false); id != "kubevela" {
		t.Errorf("id is not as expected(%s != kubevela)", id)
		return
	}
}
