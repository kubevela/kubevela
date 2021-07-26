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

package cli

/*
func delDir(dir string, t *testing.T) {
	if err := os.RemoveAll(dir); err != nil {
		t.Fatalf("failed to remove dir %s: %v", dir, err)
	}
}

func testGetVelaDefinitionLocalDir(t *testing.T) string {
	if velaDefDir, err := GetVelaDefinitionLocalDir(); err != nil {
		t.Fatalf("failed to get vela definition local dir: %v", err)
		return ""
	} else {
		return velaDefDir
	}
}

func testDirExist(dir string, t *testing.T) {
	if _, err := os.Stat(dir); err != nil {
		t.Fatalf("failed to find dir %s: %v", dir, err)
	}
}

func getArgs(t *testing.T) common2.Args {
	c := common2.Args{}
	if err := c.SetConfig(); err != nil {
		t.Fatalf("failed to set kube config: %v", err)
	}
	c.Schema = k8sruntime.NewScheme()
	if err := clientgoscheme.AddToScheme(c.Schema); err != nil {
		t.Fatalf("failed to add client-go scheme")
	}
	if err := oamcore.AddToScheme(c.Schema); err != nil {
		t.Fatalf("failed to set oam core scheme")
	}
	if _, err := c.GetClient(); err != nil {
		t.Fatalf("failed to get kube client: %v", err)
	}
	return c
}

func getTestTrait(t *testing.T) (string, *unstructured.Unstructured) {
	traitName := fmt.Sprintf("scaler-test-%d", time.Now().UnixNano())
	testDef := fmt.Sprintf(`apiVersion: core.oam.dev/v1beta1
kind: TraitDefinition
metadata:
  annotations:
    definition.oam.dev/description: "Manually scale the component."
  name: %s
  namespace: vela-system
spec:
  appliesToWorkloads:
    - deployments.apps
  podDisruptive: false
  schematic:
    cue:
      template: |
        patch: {
          spec: replicas: parameter.replicas
        }
        parameter: {
          // +usage=Specify the number of workload
          replicas: *1 | int
        }`, traitName)
	obj := &unstructured.Unstructured{}
	if err := yaml.Unmarshal([]byte(testDef), obj); err != nil {
		t.Fatalf("failed to unmarshal test trait: %v", err)
	}
	obj.SetNamespace("vela-system")
	return traitName, obj
}

func testGetCRD(c common2.Args, key client.ObjectKey, obj *unstructured.Unstructured, t *testing.T) {
	if err := c.Client.Get(context.Background(), key, obj); err != nil {
		t.Fatalf("failed to get crd %s: %v", key, err)
	}
}

func testCreateCRD(c common2.Args, obj *unstructured.Unstructured, t *testing.T) {
	if err := c.Client.Create(context.Background(), obj); err != nil {
		t.Fatalf("failed to create crd %s: %v", obj.GetName(), err)
	}
}

func testDeleteCRD(c common2.Args, obj *unstructured.Unstructured, t *testing.T) {
	if err := c.Client.Delete(context.Background(), obj); err != nil {
		t.Fatalf("failed to delete crd %s: %v", obj.GetName(), err)
	}
}

func testWriteDefDir(dir string, create_dir bool, t *testing.T) (string, string) {
	baseYaml := `description: Manually scale the component.
spec:
  appliesToWorkloads:
    - webservice
    - worker
  podDisruptive: true`
	templateCue := fmt.Sprintf(`patch: {
  spec: replicas: parameter.replicas
}
parameter: {
  // +usage=Specify the number of workload (ts=%d)
  replicas: *1 | int
}`, time.Now().UnixNano())
	if create_dir {
		if err := os.MkdirAll(dir, 0700); err != nil {
			t.Fatalf("failed to create dir %s: %v", dir, err)
		}
	}
	baseYamlFilename := filepath.Join(dir, DefinitionBaseFilename)
	if err := ioutil.WriteFile(baseYamlFilename, []byte(baseYaml), 0600); err != nil {
		t.Fatalf("failed to create base YAML file %s: %v", baseYamlFilename, err)
	}
	templateCueFilename := filepath.Join(dir, DefinitionTemplateFilename)
	if err := ioutil.WriteFile(templateCueFilename, []byte(templateCue), 0600); err != nil {
		t.Fatalf("failed to create template CUE file %s: %v", templateCueFilename, err)
	}
	return baseYaml, templateCue
}

func initCommand(cmd *cobra.Command) {
	cmd.SilenceErrors = true
	cmd.SilenceUsage = true
	cmd.Flags().StringP("env", "", "", "")
	cmd.Flags().StringP(Namespace, "n", "", "")
	cmd.SetOut(ioutil.Discard)
}

func TestNewDefinitionCommandGroup(t *testing.T) {
	cmd := DefinitionCommandGroup(common2.Args{})
	initCommand(cmd)
	cmd.SetArgs([]string{"-h"})
	if err := cmd.PersistentPreRunE(cmd, []string{}); err != nil {
		t.Fatalf("failed to execute PersistentPreRunE: %v", err)
	}
	if err := cmd.Execute(); err != nil {
		t.Fatalf("failed to execute definition command: %v", err)
	}
}

func TestNewDefinitionCreateCommand(t *testing.T) {
	cmd := NewDefinitionInitCommand(common2.Args{})
	initCommand(cmd)
	ts := time.Now().UnixNano()
	traitName := fmt.Sprintf("trait.test-%d", ts)
	velaDefDir := testGetVelaDefinitionLocalDir(t)
	var err error
	// create normally
	cmd.SetArgs([]string{traitName})
	traitDir := filepath.Join(velaDefDir, traitName)
	defer delDir(traitDir, t)
	if err = cmd.Execute(); err != nil {
		t.Fatalf("failed to create new definition %s: %v", traitName, err)
	}
	if _, err = os.Stat(traitDir); err != nil {
		t.Fatalf("failed to find def directory: %v", err)
	}
	testDirExist(traitDir, t)
	// create again, should fail
	cmd.SetArgs([]string{traitName})
	if err = cmd.Execute(); err == nil {
		t.Fatalf("should fail due to existing dir %s", traitDir)
	}
	// test invalid args
	cmd.SetArgs([]string{"trait.test.def"})
	if err = cmd.Execute(); err == nil {
		t.Fatalf("should fail due to invalid typed name %s", traitDir)
	}
	// create again with overwrite flag, should succeed
	cmd.SetArgs([]string{traitName, "--overwrite"})
	if err = cmd.Execute(); err != nil {
		t.Fatalf("failed to overwrite definition %s", traitDir)
	}
	// test customized directory
	customizedDir := "./.test/definitions/"
	cmd.SetArgs([]string{traitName, customizedDir})
	traitDir = filepath.Join(customizedDir, traitName)
	defer delDir(traitDir, t)
	if err = cmd.Execute(); err != nil {
		t.Fatalf("failed to create new definition %s: %v", traitName, err)
	}
	testDirExist(traitDir, t)
}

func TestNewDefinitionEditCommand(t *testing.T) {
	c := getArgs(t)
	traitName, traitObj := getTestTrait(t)
	testCreateCRD(c, traitObj, t)
	defer testDeleteCRD(c, traitObj, t)
	cmd := NewDefinitionEditCommand(c)
	initCommand(cmd)
	// should fail due to invalid typed name
	cmd.SetArgs([]string{traitName})
	if err := cmd.Execute(); err == nil {
		t.Fatalf("should fail due to invalid typed name")
	}
	// normal exec
	cmd.SetArgs([]string{"trait." + traitName, "-n", "vela-system", "--editor", "sed -i -e 's/Manually/manually/g'"})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("failed to edit definition: %v", err)
	}
	testGetCRD(c, types.NamespacedName{Namespace: "vela-system", Name: traitName}, traitObj, t)
	desc, ok, err := unstructured.NestedString(traitObj.Object, DefinitionDescriptionKeys...)
	if err != nil || !ok {
		t.Fatalf("failed to get description from definition: %v|%v", ok, err)
	}
	if strings.Contains(desc, "Manually") || !strings.Contains(desc, "manually") {
		t.Fatalf("failed to edit base YAML, expected description with 'manully', actual: %s", desc)
	}
	// normal exec with no change
	cmd.SetArgs([]string{"trait." + traitName, "-n", "vela-system", "--editor", "sed -i -e 's/Manually/manually/g'"})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("failed to edit definition: %v", err)
	}
	testGetCRD(c, types.NamespacedName{Namespace: "vela-system", Name: traitName}, traitObj, t)
	_desc, ok, err := unstructured.NestedString(traitObj.Object, DefinitionDescriptionKeys...)
	if err != nil || !ok {
		t.Fatalf("failed to get description from definition: %v|%v", ok, err)
	}
	if desc != _desc {
		t.Fatalf("failed to do unchanged edit\n=== Expected ===\n%s\n=== Actual ===\n%s\n", desc, _desc)
	}
	// edit template
	cmd.SetArgs([]string{"trait." + traitName, "-n", "vela-system", "--editor", "sed -i -e 's/workload/workloads/g'", "--edit-template"})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("failed to edit definition: %v", err)
	}
	testGetCRD(c, types.NamespacedName{Namespace: "vela-system", Name: traitName}, traitObj, t)
	template, ok, err := unstructured.NestedString(traitObj.Object, DefinitionTemplateKeys...)
	if err != nil || !ok {
		t.Fatalf("failed to get template from definition: %v|%v", ok, err)
	}
	if !strings.Contains(template, "workloads") {
		t.Fatalf("failed to edit template CUE, expected description with 'workloads', actual: %s", template)
	}
}

func TestNewDefinitionDownloadCommand(t *testing.T) {
	velaDefDir := testGetVelaDefinitionLocalDir(t)
	c := getArgs(t)
	traitName, traitObj := getTestTrait(t)
	traitTypedName := "trait." + traitName
	testCreateCRD(c, traitObj, t)
	defer testDeleteCRD(c, traitObj, t)
	cmd := NewDefinitionGetCommand(c)
	initCommand(cmd)
	// no namespace specified, should fail
	cmd.SetArgs([]string{traitTypedName})
	if err := cmd.Execute(); err == nil {
		t.Fatalf("should fail due to namespace unspecified")
	}
	// normal exec
	cmd.SetArgs([]string{"trait." + traitName, "-n", "vela-system"})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("failed to run def download: %v", err)
	}
	traitPath := filepath.Join(velaDefDir, traitTypedName)
	testDirExist(traitPath, t)
	defer delDir(traitPath, t)
	// should fail due to duplicated dir
	cmd.SetArgs([]string{"trait." + traitName, "-n", "vela-system"})
	if err := cmd.Execute(); err == nil {
		t.Fatalf("should fail due to duplicated dir")
	}
	// test specified directory
	cmd.SetArgs([]string{"trait." + traitName, "./.test/", "-n", "vela-system"})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("failed to run def download with specified path: %v", err)
	}
	traitPath = filepath.Join("./.test/", traitTypedName)
	testDirExist(traitPath, t)
	defer delDir(traitPath, t)
}

func TestNewDefinitionApplyCommand(t *testing.T) {
	velaDefDir := testGetVelaDefinitionLocalDir(t)
	c := getArgs(t)
	traitName := fmt.Sprintf("scaler-test-%d", time.Now().UnixNano())
	testDirname := fmt.Sprintf("./.test/trait.%s", traitName)
	_, templateCue := testWriteDefDir(testDirname, true, t)
	defer delDir(testDirname, t)
	cmd := NewDefinitionApplyCommand(c)
	initCommand(cmd)
	// no invalid dirname, should fail
	cmd.SetArgs([]string{"./.test/trait-test"})
	if err := cmd.Execute(); err == nil {
		t.Fatalf("should fail due to invalid definition typed name")
	}
	// normal exec
	cmd.SetArgs([]string{testDirname, "-n", "vela-system"})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("failed to run def apply: %v", err)
	}
	_o := &unstructured.Unstructured{}
	_o.SetGroupVersionKind(v1beta1.TraitDefinitionGroupVersionKind)
	_o.SetNamespace("vela-system")
	_o.SetName(traitName)
	defer testDeleteCRD(c, _o, t)
	testGetCRD(c, types.NamespacedName{Namespace: "vela-system", Name: traitName}, _o, t)
	_templateCue, ok, err := unstructured.NestedString(_o.Object, DefinitionTemplateKeys...)
	if err != nil || !ok {
		t.Fatalf("failed to set correct template, err: %s", err)
	}
	if templateCue != _templateCue {
		t.Fatalf("failed to set correct template\n=== Expected ===\n%s\n=== Found ===\n%s\n", templateCue, _templateCue)
	}
	// test default directory with dry-run
	oldTraitName := traitName
	oldTestDirname := testDirname
	traitName = fmt.Sprintf("scaler-test-%d", time.Now().UnixNano())
	testDirname = filepath.Join(velaDefDir, fmt.Sprintf("trait.%s", traitName))
	testWriteDefDir(testDirname, true, t)
	defer delDir(testDirname, t)
	cmd.SetArgs([]string{"trait." + traitName, "-n", "vela-system", "--dry-run"})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("failed to run def apply: %v", err)
	}
	_o = &unstructured.Unstructured{}
	_o.SetGroupVersionKind(v1beta1.TraitDefinitionGroupVersionKind)
	_o.SetNamespace("vela-system")
	_o.SetName(traitName)
	err = c.Client.Get(context.Background(), types.NamespacedName{Namespace: "vela-system", Name: traitName}, _o)
	if err == nil || !errors.IsNotFound(err) {
		t.Fatalf("dry-run apply should not apply to k8s")
	}
	// test default directory updating previously created one
	traitName = oldTraitName
	testDirname = oldTestDirname
	_, templateCue = testWriteDefDir(testDirname, false, t)
	cmd = NewDefinitionApplyCommand(c)
	initCommand(cmd)
	cmd.SetArgs([]string{testDirname, "-n", "vela-system"})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("failed to run def apply: %v", err)
	}
	_o = &unstructured.Unstructured{}
	_o.SetGroupVersionKind(v1beta1.TraitDefinitionGroupVersionKind)
	_o.SetNamespace("vela-system")
	_o.SetName(traitName)
	testGetCRD(c, types.NamespacedName{Namespace: "vela-system", Name: traitName}, _o, t)
	_templateCue, ok, err = unstructured.NestedString(_o.Object, DefinitionTemplateKeys...)
	if err != nil || !ok {
		t.Fatalf("failed to set correct template, err: %s", err)
	}
	if templateCue != _templateCue {
		t.Fatalf("failed to set correct template\n=== Expected ===\n%s\n=== Found ===\n%s\n", templateCue, _templateCue)
	}
}

func TestLoadCUE(t *testing.T) {
	r := &cue.Runtime{}
	bits := load.Instances([]string{"/Users/yinda/Codes/OAM/temp/scaler.cue", "/Users/yinda/Codes/OAM/temp/def.cue"}, nil)
	for _, bi := range bits {
		if bi.Err != nil {
			fmt.Println("Error during load:", bi.Err)
			continue
		}
		I, err := r.Build(bi)
		if err != nil {
			fmt.Println("Error during build:", bi.Err)
			continue
		}
		// get the root value and print it
		value := I.Value()
		fmt.Println("root value:", value)
		// Validate the value
		err = value.Validate()
		if err != nil {
			fmt.Println("Error during validate:", err)
			continue
		}
		//valstr, err := sets.ToString(val)
		//if err != nil {
		//	fmt.Println("Error for tostring", err)
		//}
		//fmt.Println("Eval Value: ", valstr)
	}
}

func TestGenerateCUE(t *testing.T) {
	//ast.NewIdent()
	//ast.NewSel()

	//expr := ast.NewBinExpr(token.COLON, ast.NewString("name"), ast.NewString("val"))
	//r := cue.Runtime{}
	//inst, err := r.CompileExpr(expr)
	//if err != nil {
	//	fmt.Println("error", err)
	//}
	//inst.
	//fmt.Print("value", inst.)
	r := gocodec.New(&cue.Runtime{}, &gocodec.Config{})
	m := map[string]interface{}{
		"a": "b",
		"b": map[string]interface{}{
			"c": 5,
		},
	}
	v, err := r.Decode(m)
	if err != nil {
		fmt.Println("err", err)
	} else {
		fmt.Printf("value: %v\n", v)
	}
}

func TestGetDefCUE(t *testing.T) {
	c := dynamic.NewForConfigOrDie(config.GetConfigOrDie())
	r := c.Resource(schema.GroupVersionResource{Group: v1beta1.Group, Version: v1beta1.Version, Resource: "traitdefinitions"})
	obj, err := r.Namespace("vela-system").Get(context.Background(), "scaler", metav1.GetOptions{})
	if err != nil {
		t.Errorf("k8s %v", err)
	}
	defx := &common.DefinitionX{Unstructured: *obj}
	val, err := defx.ToCUE()
	if err != nil {
		t.Errorf("defx %v", err)
	}
	fmt.Printf("%v\n", val)

	newDef := &common.DefinitionX{}
	if err := newDef.FromCUE(val); err != nil {
		t.Errorf("fromcue %v", err)
	}
	s, _ := yaml.Marshal(newDef.Object)
	fmt.Printf("from val: %v", string(s))
}
*/
