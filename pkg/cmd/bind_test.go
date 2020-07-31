package cmd

/*
func TestNewBindCommand(t *testing.T) {
	TraitsNotApply := traitDefinitionExample.DeepCopy()
	TraitsNotApply.Spec.AppliesToWorkloads = []string{"core.oam.dev/v1alpha2.ContainerizedWorkload"}

	cases := map[string]*test.CliTestCase{
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
*/
