package cmd

import (
	"testing"

	"k8s.io/apimachinery/pkg/runtime"

	"github.com/cloud-native-application/rudrx/pkg/test"
)

func TestNewTraitCommand(t *testing.T) {
	TraitsNotApply := traitDefinitionExample.DeepCopy()
	TraitsNotApply.Spec.AppliesToWorkloads = []string{}

	cases := map[string]*test.CliTestCase{
		"PrintTraits": {
			Resources: test.InitResources{
				Create: []runtime.Object{
					traitDefinitionExample.DeepCopy(),
				},
			},
			ExpectedString: "manualscalertrait.core.oam.dev",
			Args:           []string{},
		},
		"TraitsNotApply": {
			Resources: test.InitResources{
				Create: []runtime.Object{
					TraitsNotApply,
				},
			},
			ExpectedOutput: "NAME	SHORT	DEFINITION	APPLIES TO	STATUS\n",
			Args: []string{},
		},
	}

	test.NewCliTest(t, scheme, NewTraitsCommand, cases).Run()
}
