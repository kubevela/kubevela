package application

import (
	"context"
	"fmt"
	"strings"
	"testing"

	"github.com/oam-dev/kubevela/apis/core.oam.dev/v1alpha2"
)

func TestValidateTraitNameFn(t *testing.T) {
	type args struct {
		in0 context.Context
		v   ValidatingApp
	}

	comp1 := v1alpha2.ApplicationComponent{
		Name: "myservice1",
		Traits: append(make([]v1alpha2.ApplicationTrait, 0),
			v1alpha2.ApplicationTrait{Name: "webservice"}),
	}
	comp2 := v1alpha2.ApplicationComponent{
		Name: "myservice2",
		Traits: append(make([]v1alpha2.ApplicationTrait, 0),
			v1alpha2.ApplicationTrait{Name: "webservice"},
			v1alpha2.ApplicationTrait{Name: "webservice"}),
	}

	components := make([]v1alpha2.ApplicationComponent, 0)
	validatedApp := &v1alpha2.Application{
		Spec: v1alpha2.ApplicationSpec{
			Components: append(components, comp1),
		},
	}
	validateFailedApp := &v1alpha2.Application{
		Spec: v1alpha2.ApplicationSpec{
			Components: append(components, comp2),
		},
	}
	tests := []struct {
		name string
		args args
		want string
	}{
		{
			name: "Validate_Success_Trait_Name",
			args: args{
				in0: context.Background(),
				v: ValidatingApp{
					app: validatedApp,
				},
			},
			want: "",
		},
		{
			name: "Validate_Failed_Trait_Name_Because_of_Component",
			args: args{
				in0: context.Background(),
				v: ValidatingApp{
					app: validateFailedApp,
				},
			},
			want: fmt.Sprintf(errFmtTraitNameConflict, "webservice", comp2.Name),
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := ValidateTraitNameFn(tt.args.in0, tt.args.v); len(got) != 0 && !strings.Contains(got[0].Error(), tt.want) {
				t.Errorf("ValidateTraitNameFn() = %v, want %v", got, tt.want)
			}
		})
	}
}
