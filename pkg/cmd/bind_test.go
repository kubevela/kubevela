package cmd

import (
	"testing"

	"k8s.io/apimachinery/pkg/runtime"

	"github.com/cloud-native-application/rudrx/pkg/test"
)

func TestNewBindCommand(t *testing.T) {
	TraitsNotApply := traitDefinitionExample.DeepCopy()
	TraitsNotApply.Spec.AppliesToWorkloads = []string{"core.oam.dev/v1alpha2.ContainerizedWorkload"}

	cases := map[string]*test.CliTestCase{
		"WithNoArgs": {
			Resources: test.InitResources{
				Create: []runtime.Object{
					traitDefinitionExample.DeepCopy(),
					//traitTemplateExample.DeepCopy(),
				},
			},
			ExpectedOutput: "Please append the name of an application. Use `rudr bind -h` for more detailed information.",
			Args:           []string{},
			WantException:  true,
		},
		"WithWrongAppconfig": {
			Resources: test.InitResources{
				Create: []runtime.Object{
					traitDefinitionExample.DeepCopy(),
					//traitTemplateExample.DeepCopy(),
				},
			},
			ExpectedOutput: "applicationconfigurations.core.oam.dev \"frontend\" not found",
			Args:           []string{"frontend"},
			WantException:  true,
		},
		"TemplateParametersWork": {
			Resources: test.InitResources{
				Create: []runtime.Object{
					traitDefinitionExample.DeepCopy(),
					//traitTemplateExample.DeepCopy(),
				},
			},
			ExpectedString: "--replicaCount int",
			Args:           []string{"-h"},
		},
		"WorkSuccess": {
			Resources: test.InitResources{
				Create: []runtime.Object{
					appconfigExample.DeepCopy(),
					componentExample.DeepCopy(),
					traitDefinitionExample.DeepCopy(),
					//traitTemplateExample.DeepCopy(),
				},
			},
			ExpectedOutput: "Applying trait for component app2060\nSucceeded!\n",
			Args:           []string{"app2060", "ManualScaler", "--replicaCount", "5"},
		},
	}

	test.NewCliTest(t, scheme, NewBindCommand, cases).Run()
}
