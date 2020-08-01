package workload

/*
func TestNewRunCommand(t *testing.T) {
	// workloadTemplateExample2 := workloadTemplateExample.DeepCopy()
	workloaddefExample2 := workloaddefExample.DeepCopy()
	workloaddefExample2.Annotations["short"] = "containerized"

	cases := map[string]*test.CliTestCase{
		"WorkloadNotDefinited": {
			Resources: test.InitResources{
				Create: []runtime.Object{
					workloaddefExample.DeepCopy(),
					//workloadTemplateExample.DeepCopy(),
				},
			},
			WantException:  true,
			ExpectedString: "You must specify a workload, like containerizedworkloads.core.oam.dev",
			Args:           []string{},
		},
		"WorkloadShortWork": {
			Resources: test.InitResources{
				Create: []runtime.Object{
					workloaddefExample2.DeepCopy(),
					//workloadTemplateExample2.DeepCopy(),
				},
			},
			WantException:  true,
			ExpectedString: "You must specify a workload, like containerized",
			Args:           []string{},
		},
		"PortFlagNotSet": {
			Resources: test.InitResources{
				Create: []runtime.Object{
					workloaddefExample2.DeepCopy(),
					//workloadTemplateExample2.DeepCopy(),
				},
			},
			ExpectedResources: []runtime.Object{
				appconfigExample,
				componentExample,
			},
			WantException:  true,
			ExpectedString: "Flag `port` is NOT set, please check and try again.",
			Args:           []string{"containerized", "app2060", "nginx:1.9.4"},
		},
		"TemplateParametersWork": {
			Resources: test.InitResources{
				Create: []runtime.Object{
					workloaddefExample2.DeepCopy(),
					//workloadTemplateExample2.DeepCopy(),
				},
			},
			ExpectedString: "-p, --port",
			Args:           []string{"containerized", "-h"},
		},
		"AppConfigCreated": {
			Resources: test.InitResources{
				Create: []runtime.Object{
					workloaddefExample2.DeepCopy(),
					//workloadTemplateExample2.DeepCopy(),
				},
			},
			ExpectedExistResources: []runtime.Object{
				appconfigExample,
				componentExample,
			},
			ExpectedOutput: "Creating AppConfig app2060\nSUCCEED\n",
			Args:           []string{"containerized", "app2060", "nginx:1.9.4", "-p", "80"},
		},
	}

	test.NewCliTest(t, scheme, NewRunCommand, cases).Run()
}
*/
