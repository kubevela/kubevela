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

package appfile

import (
	"os"
	"testing"

	"github.com/oam-dev/kubevela/pkg/utils/system"
	"github.com/oam-dev/kubevela/references/appfile/driver"
)

func TestGetStorage(t *testing.T) {
	_ = os.Setenv(system.StorageDriverEnv, driver.ConfigMapDriverName)

	store := &Storage{driver.NewConfigMapStorage()}
	tests := []struct {
		name string
		want *Storage
	}{
		{name: "TestGetStorage_ConfigMap", want: store},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := GetStorage(); got.Name() != tt.want.Name() {
				t.Errorf("GetStorage() = %v, want %v", got, tt.want)
			}
		})
	}
}
