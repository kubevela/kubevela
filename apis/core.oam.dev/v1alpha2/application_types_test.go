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

package v1alpha2

import (
	"reflect"
	"testing"
)

func TestApplicationGetComponent(t *testing.T) {
	ac1 := ApplicationComponent{
		Name:         "ac1",
		WorkloadType: "type1",
	}
	ac2 := ApplicationComponent{
		Name:         "ac2",
		WorkloadType: "type2",
	}
	tests := map[string]struct {
		app           *Application
		componentName string
		want          *ApplicationComponent
	}{
		"test get one": {
			app: &Application{
				Spec: ApplicationSpec{
					Components: []ApplicationComponent{
						ac1, ac2,
					},
				},
			},
			componentName: ac1.WorkloadType,
			want:          &ac1,
		},
		"test get none": {
			app: &Application{
				Spec: ApplicationSpec{
					Components: []ApplicationComponent{
						ac2,
					},
				},
			},
			componentName: ac1.WorkloadType,
			want:          nil,
		},
	}
	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			if got := tt.app.GetComponent(tt.componentName); !reflect.DeepEqual(got, tt.want) {
				t.Errorf("GetComponent() = %v, want %v", got, tt.want)
			}
		})
	}
}
