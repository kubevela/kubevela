package traitdefinition

import (
	"context"
	"fmt"
	"testing"

	"github.com/crossplane/crossplane-runtime/pkg/test"
	"github.com/google/go-cmp/cmp"
	"github.com/pkg/errors"

	"github.com/oam-dev/kubevela/pkg/oam/util"
)

func TestValidateDefinitionReference(t *testing.T) {
	cases := map[string]struct {
		reason   string
		template string
		want     error
	}{
		"NoExtension": {
			reason:   "An error should be returned if extension is omitted",
			template: "",
			want:     errors.New(failInfoDefRefOmitted),
		},
		"HaveExtentsion_NoTemplate": {
			reason: "An error should be returned if extension template is omitted",
			template: `
  extension:
    notemplate: |-
      fakefield: fakefieldvalue`,
			want: errors.New(failInfoDefRefOmitted),
		},
		"HaveExtension_HaveTemplate": {
			reason: "No error should be returned if have CUE template",
			template: `
  extension:
    template: |-
      patch: {
       spec: replicas: parameter.replicas
      }`,
			want: nil,
		},
	}

	for caseName, tc := range cases {
		t.Run(caseName, func(t *testing.T) {
			tdStr := traitDefStringWithTemplate(tc.template)
			td, err := util.UnMarshalStringToTraitDefinition(tdStr)
			if err != nil {
				t.Fatal("error occurs in generating TraitDefinition string", err.Error())
			}
			err = ValidateDefinitionReference(context.Background(), *td)
			if diff := cmp.Diff(tc.want, err, test.EquateErrors()); diff != "" {
				t.Errorf("\n%s\nValidateDefinitionReference: -want , +got \n%s\n", tc.reason, diff)
			}
		})
	}
}

func traitDefStringWithTemplate(t string) string {
	return fmt.Sprintf(`
apiVersion: core.oam.dev/v1alpha2
kind: TraitDefinition
metadata:
  name: scaler
spec:
%s`, t)
}
