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
