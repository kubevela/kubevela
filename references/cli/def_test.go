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

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"k8s.io/apimachinery/pkg/api/errors"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	common3 "github.com/oam-dev/kubevela/apis/core.oam.dev/common"
	"github.com/oam-dev/kubevela/apis/core.oam.dev/v1beta1"
	pkgdef "github.com/oam-dev/kubevela/pkg/definition"
	common2 "github.com/oam-dev/kubevela/pkg/utils/common"
)

const (
	// VelaTestNamespace namespace for hosting objects used during test
	VelaTestNamespace = "vela-test-system"
)

func initArgs() common2.Args {
	arg := common2.Args{}
	arg.SetClient(fake.NewClientBuilder().WithScheme(common2.Scheme).Build())
	return arg
}

func initCommand(cmd *cobra.Command) {
	cmd.SilenceErrors = true
	cmd.SilenceUsage = true
	cmd.Flags().StringP("env", "", "", "")
	cmd.SetOut(io.Discard)
	cmd.SetErr(io.Discard)
}

func createTrait(c common2.Args, t *testing.T) string {
	traitName := fmt.Sprintf("my-trait-%d", time.Now().UnixNano())
	createNamespacedTrait(c, traitName, VelaTestNamespace, t)
	return traitName
}

func createNamespacedTrait(c common2.Args, name string, ns string, t *testing.T) {
	traitName := fmt.Sprintf("my-trait-%d", time.Now().UnixNano())
	client, err := c.GetClient()
	if err != nil {
		t.Fatalf("failed to get client: %v", err)
	}
	if err := client.Create(context.Background(), &v1beta1.TraitDefinition{
		ObjectMeta: v1.ObjectMeta{
			Name:      name,
			Namespace: ns,
			Annotations: map[string]string{
				pkgdef.DescriptionKey: "My test-trait " + traitName,
			},
		},
		Spec: v1beta1.TraitDefinitionSpec{
			Schematic: &common3.Schematic{CUE: &common3.CUE{Template: "parameter: {}"}},
		},
	}); err != nil {
		t.Fatalf("failed to create trait: %v", err)
	}
}

func createLocalTraitAt(traitName string, localPath string, t *testing.T) string {
	s := fmt.Sprintf(`// k8s metadata
"%s": {
        type:   "trait"
        description: "My test-trait %s"
        attributes: {
                appliesToWorkloads: ["webservice", "worker"]
                podDisruptive: true
        }
}

// template
template: {
        patch: {
                spec: {
                        replicas: *1 | int
                }
        }
        parameter: {
                // +usage=Specify the number of workloads
                replicas: *1 | int
        }
}
`, traitName, traitName)
	filename := filepath.Join(localPath, traitName+".cue")
	if err := os.WriteFile(filename, []byte(s), 0600); err != nil {
		t.Fatalf("failed to write temp trait file %s: %v", filename, err)
	}
	return filename
}

func createLocalTrait(t *testing.T) (string, string) {
	traitName := fmt.Sprintf("my-trait-%d", time.Now().UnixNano())
	filename := createLocalTraitAt(traitName, os.TempDir(), t)
	return traitName, filename
}

func createLocalTraits(t *testing.T) string {
	dirname, err := os.MkdirTemp(os.TempDir(), "vela-def-test-*")
	if err != nil {
		t.Fatalf("failed to create temporary directory: %v", err)
	}
	for i := 0; i < 3; i++ {
		createLocalTraitAt(fmt.Sprintf("trait-%d", i), dirname, t)
	}
	return dirname
}

func createLocalDeploymentYAML(t *testing.T) string {
	s := `apiVersion: apps/v1
kind: Deployment
metadata:
  name: "main"
Spec:
  image: "busybox"
---
apiVersion: apps/v1
kind: Deployment
metadata:
  name: "secondary"
Spec:
  image: "busybox"
`
	filename := filepath.Join(os.TempDir(), fmt.Sprintf("%d-deployments.yaml", time.Now().UnixNano()))
	if err := os.WriteFile(filename, []byte(s), 0600); err != nil {
		t.Fatalf("failed to create temp deployments file %s: %v", filename, err)
	}
	return filename
}

func removeFile(filename string, t *testing.T) {
	if err := os.Remove(filename); err != nil {
		t.Fatalf("failed to remove file %s: %v", filename, err)
	}
}

func removeDir(dirname string, t *testing.T) {
	if err := os.RemoveAll(dirname); err != nil {
		t.Fatalf("failed to remove dir %s: %v", dirname, err)
	}
}

func TestNewDefinitionCommandGroup(t *testing.T) {
	cmd := DefinitionCommandGroup(common2.Args{}, "")
	initCommand(cmd)
	cmd.SetArgs([]string{"-h"})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("failed to execute definition command: %v", err)
	}
}

func TestNewDefinitionInitCommand(t *testing.T) {
	c := initArgs()
	// test normal
	cmd := NewDefinitionInitCommand(c)
	initCommand(cmd)
	cmd.SetArgs([]string{"my-ingress", "-t", "trait", "--desc", "test ingress"})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("unexpected error when executing init command: %v", err)
	}

	// test interactive
	cmd = NewDefinitionInitCommand(initArgs())
	initCommand(cmd)
	componentName := "my-webservice"
	cmd.SetArgs([]string{componentName, "--interactive"})
	templateFilename := createLocalDeploymentYAML(t)
	filename := strings.Replace(templateFilename, ".yaml", ".cue", 1)
	defer removeFile(templateFilename, t)
	defer removeFile(filename, t)
	inputs := fmt.Sprintf("comp\ncomponent\nMy webservice component.\n%s\n%s\n", templateFilename, filename)
	reader := strings.NewReader(inputs)
	cmd.SetIn(reader)
	if err := cmd.Execute(); err != nil {
		t.Fatalf("unexpeced error when executing init command interactively: %v", err)
	}
}

func TestNewDefinitionInitCommand4Terraform(t *testing.T) {
	const defFileName = "alibaba-vswitch.yaml"
	testcases := []struct {
		name   string
		args   []string
		errMsg string
		want   string
	}{
		{
			name: "normal",
			args: []string{"vswitch", "-t", "component", "--provider", "alibaba", "--desc", "xxx", "--git", "https://github.com/kubevela-contrib/terraform-modules.git", "--path", "alibaba/vswitch"},
		},
		{
			name: "print in a file",
			args: []string{"vswitch", "-t", "component", "--provider", "alibaba", "--desc", "xxx", "--git", "https://github.com/kubevela-contrib/terraform-modules.git", "--path", "alibaba/vswitch", "--output", defFileName},
			want: `apiVersion: core.oam.dev/v1beta1
kind: ComponentDefinition
metadata:
  annotations:
    definition.oam.dev/description: xxx
  creationTimestamp: null
  labels:
    type: terraform
  name: alibaba-vswitch
  namespace: vela-system
spec:
  schematic:
    terraform:
      configuration: https://github.com/kubevela-contrib/terraform-modules.git
      path: alibaba/vswitch
      type: remote
  workload:
    definition:
      apiVersion: terraform.core.oam.dev/v1beta1
      kind: Configuration
status: {}`,
		},
		{
			name:   "not supported component",
			args:   []string{"vswitch", "-t", "trait", "--provider", "alibaba"},
			errMsg: "provider is only valid when the type of the definition is component",
		},
		{
			name:   "not supported cloud provider",
			args:   []string{"vswitch", "-t", "component", "--provider", "xxx"},
			errMsg: "Provider `xxx` is not supported.",
		},
		{
			name:   "git is not right",
			args:   []string{"vswitch", "-t", "component", "--provider", "alibaba", "--desc", "test", "--git", "xxx"},
			errMsg: "invalid git url",
		},
	}

	for _, tc := range testcases {
		t.Run(tc.name, func(t *testing.T) {
			c := initArgs()
			cmd := NewDefinitionInitCommand(c)
			initCommand(cmd)
			cmd.SetArgs(tc.args)
			err := cmd.Execute()
			if err != nil && !strings.Contains(err.Error(), tc.errMsg) {
				t.Fatalf("unexpected error when executing init command: %v", err)
			} else {
				data, err := os.ReadFile(defFileName)
				defer os.Remove(defFileName)
				assert.Nil(t, err)
				if !strings.Contains(string(data), tc.want) {
					t.Fatalf("unexpected output: %s", string(data))
				}
			}
		})
	}
}

func TestNewDefinitionGetCommand(t *testing.T) {
	c := initArgs()

	// normal test
	cmd := NewDefinitionGetCommand(c)
	initCommand(cmd)
	traitName := createTrait(c, t)
	cmd.SetArgs([]string{traitName})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("unexpeced error when executing get command: %v", err)
	}
	// test multi trait
	cmd = NewDefinitionGetCommand(c)
	initCommand(cmd)
	createNamespacedTrait(c, traitName, "default", t)
	cmd.SetArgs([]string{traitName})
	if err := cmd.Execute(); err == nil {
		t.Fatalf("expect found multiple traits error, but not found")
	}
	// test no trait
	cmd = NewDefinitionGetCommand(c)
	initCommand(cmd)
	cmd.SetArgs([]string{traitName + "s"})
	if err := cmd.Execute(); err == nil {
		t.Fatalf("expect found no trait error, but not found")
	}
}

func TestNewDefinitionGenDocCommand(t *testing.T) {
	c := initArgs()
	cmd := NewDefinitionGenDocCommand(c)
	assert.NotNil(t, cmd.Execute())

	cmd.SetArgs([]string{"alibaba-xxxxxxx"})
	assert.NotNil(t, cmd.Execute())
}

func TestNewDefinitionListCommand(t *testing.T) {
	c := initArgs()
	// normal test
	cmd := NewDefinitionListCommand(c)
	initCommand(cmd)
	_ = createTrait(c, t)
	cmd.SetArgs([]string{})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("unexpeced error when executing list command: %v", err)
	}
	// test no trait
	cmd = NewDefinitionListCommand(c)
	initCommand(cmd)
	cmd.SetArgs([]string{"--namespace", "default"})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("no trait found should not return error, err: %v", err)
	}
}

func TestNewDefinitionEditCommand(t *testing.T) {
	c := initArgs()
	// normal test
	cmd := NewDefinitionEditCommand(c)
	initCommand(cmd)
	traitName := createTrait(c, t)
	if err := os.Setenv("EDITOR", "sed -i -e 's/test-trait/TestTrait/g'"); err != nil {
		t.Fatalf("failed to set editor env: %v", err)
	}
	cmd.SetArgs([]string{traitName})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("unexpeced error when executing edit command: %v", err)
	}
	// test no change
	cmd = NewDefinitionEditCommand(c)
	initCommand(cmd)
	createNamespacedTrait(c, traitName, "default", t)
	if err := os.Setenv("EDITOR", "sed -i -e 's/test-trait-test/TestTrait/g'"); err != nil {
		t.Fatalf("failed to set editor env: %v", err)
	}
	cmd.SetArgs([]string{traitName, "-n", "default"})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("unexpeced error when executing edit command: %v", err)
	}
}

func TestNewDefinitionRenderCommand(t *testing.T) {
	c := initArgs()
	// normal test
	cmd := NewDefinitionRenderCommand(c)
	initCommand(cmd)
	_ = os.Setenv(HelmChartFormatEnvName, "true")
	_, traitFilename := createLocalTrait(t)
	defer removeFile(traitFilename, t)
	cmd.SetArgs([]string{traitFilename})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("unexpeced error when executing redner command: %v", err)
	}
	// directory read/write test
	_ = os.Setenv(HelmChartFormatEnvName, "system")
	dirname := createLocalTraits(t)
	defer removeDir(dirname, t)
	outputDir, err := os.MkdirTemp(os.TempDir(), "vela-def-tests-output-*")
	if err != nil {
		t.Fatalf("failed to create temporary output dir: %v", err)
	}
	defer removeDir(outputDir, t)
	cmd = NewDefinitionRenderCommand(c)
	initCommand(cmd)
	cmd.SetArgs([]string{dirname, "-o", outputDir, "--message", "Author: KubeVela"})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("unexpeced error when executing render command: %v", err)
	}
	// directory read/print test
	_ = os.WriteFile(filepath.Join(dirname, "temp.json"), []byte("hello"), 0600)
	_ = os.WriteFile(filepath.Join(dirname, "temp.cue"), []byte("hello"), 0600)
	cmd = NewDefinitionRenderCommand(c)
	initCommand(cmd)
	cmd.SetArgs([]string{dirname})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("unexpeced error when executing render command: %v", err)
	}
}

func TestNewDefinitionApplyCommand(t *testing.T) {
	c := initArgs()
	// dry-run test
	cmd := NewDefinitionApplyCommand(c)
	initCommand(cmd)
	_, traitFilename := createLocalTrait(t)
	defer removeFile(traitFilename, t)
	cmd.SetArgs([]string{traitFilename, "--dry-run"})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("unexpeced error when executing apply command: %v", err)
	}
	// normal test and reapply
	cmd = NewDefinitionApplyCommand(c)
	initCommand(cmd)
	cmd.SetArgs([]string{traitFilename})
	for i := 0; i < 2; i++ {
		if err := cmd.Execute(); err != nil {
			t.Fatalf("unexpeced error when executing apply command: %v", err)
		}
	}
}

func TestNewDefinitionDelCommand(t *testing.T) {
	c := initArgs()
	cmd := NewDefinitionDelCommand(c)
	initCommand(cmd)
	traitName := createTrait(c, t)
	reader := strings.NewReader("yes\n")
	cmd.SetIn(reader)
	cmd.SetArgs([]string{traitName})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("unexpeced error when executing del command: %v", err)
	}
	obj := &v1beta1.TraitDefinition{}
	client, err := c.GetClient()
	if err != nil {
		t.Fatalf("failed to get client: %v", err)
	}
	if err := client.Get(context.Background(), types.NamespacedName{
		Namespace: VelaTestNamespace,
		Name:      traitName,
	}, obj); !errors.IsNotFound(err) {
		t.Fatalf("should not found target definition %s, err: %v", traitName, err)
	}
	if err := cmd.Execute(); err == nil {
		t.Fatalf("should encounter not found error, but no error found")
	}
}

func TestNewDefinitionVetCommand(t *testing.T) {
	c := initArgs()
	cmd := NewDefinitionValidateCommand(c)
	initCommand(cmd)
	_, traitFilename := createLocalTrait(t)
	defer removeFile(traitFilename, t)
	cmd.SetArgs([]string{traitFilename})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("unexpeced error when executing vet command: %v", err)
	}
	bs, err := os.ReadFile(traitFilename)
	if err != nil {
		t.Fatalf("failed to read trait file %s: %v", traitFilename, err)
	}
	bs = []byte(string(bs) + "abc:{xa}")
	if err = os.WriteFile(traitFilename, bs, 0600); err != nil {
		t.Fatalf("failed to modify trait file %s: %v", traitFilename, err)
	}
	if err = cmd.Execute(); err == nil {
		t.Fatalf("expect validation failed but error not found")
	}
}
