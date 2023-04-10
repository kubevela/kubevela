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
	"bytes"
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
	"sigs.k8s.io/yaml"

	common3 "github.com/oam-dev/kubevela/apis/core.oam.dev/common"
	"github.com/oam-dev/kubevela/apis/core.oam.dev/v1beta1"
	pkgdef "github.com/oam-dev/kubevela/pkg/definition"
	addonutil "github.com/oam-dev/kubevela/pkg/utils/addon"
	common2 "github.com/oam-dev/kubevela/pkg/utils/common"
	"github.com/oam-dev/kubevela/pkg/utils/util"
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
	return createTraitWithOwnerAddon(c, "", t)
}

func createTraitWithOwnerAddon(c common2.Args, addonName string, t *testing.T) string {
	traitName := fmt.Sprintf("my-trait-%d", time.Now().UnixNano())
	createNamespacedTrait(c, traitName, VelaTestNamespace, addonName, t)
	return traitName
}

func createNamespacedTrait(c common2.Args, name string, ns string, ownerAddon string, t *testing.T) {
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
			OwnerReferences: []v1.OwnerReference{{
				Name: addonutil.Addon2AppName(ownerAddon),
			}},
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
	cmd := DefinitionCommandGroup(common2.Args{}, "", util.IOStreams{In: os.Stdin, Out: os.Stdout, ErrOut: os.Stderr})
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
	const (
		defVswitchFileName = "alibaba-vswitch.yaml"
		defRedisFileName   = "tencent-redis.yaml"
	)
	testcases := []struct {
		name   string
		args   []string
		output string
		errMsg string
		want   string
	}{
		{
			name: "normal",
			args: []string{"vswitch", "-t", "component", "--provider", "alibaba", "--desc", "xxx", "--git", "https://github.com/kubevela-contrib/terraform-modules.git", "--path", "alibaba/vswitch"},
		},
		{
			name:   "normal from local",
			args:   []string{"vswitch", "-t", "component", "--provider", "tencent", "--desc", "xxx", "--local", "test-data/redis.tf", "--output", defRedisFileName},
			output: defRedisFileName,
			want: `apiVersion: core.oam.dev/v1beta1
kind: ComponentDefinition
metadata:
  annotations:
    definition.oam.dev/description: xxx
  creationTimestamp: null
  labels:
    type: terraform
  name: tencent-vswitch
  namespace: vela-system
spec:
  schematic:
    terraform:
      configuration: |
        terraform {
          required_providers {
            tencentcloud = {
              source = "tencentcloudstack/tencentcloud"
            }
          }
        }

        resource "tencentcloud_redis_instance" "main" {
          type_id           = 8
          availability_zone = var.availability_zone
          name              = var.instance_name
          password          = var.user_password
          mem_size          = var.mem_size
          port              = var.port
        }

        output "DB_IP" {
          value = tencentcloud_redis_instance.main.ip
        }

        output "DB_PASSWORD" {
          value = var.user_password
        }

        output "DB_PORT" {
          value = var.port
        }

        variable "availability_zone" {
          description = "The available zone ID of an instance to be created."
          type        = string
          default = "ap-chengdu-1"
        }

        variable "instance_name" {
          description = "redis instance name"
          type        = string
          default     = "sample"
        }

        variable "user_password" {
          description = "redis instance password"
          type        = string
          default     = "IEfewjf2342rfwfwYYfaked"
        }

        variable "mem_size" {
          description = "redis instance memory size"
          type        = number
          default     = 1024
        }

        variable "port" {
          description = "The port used to access a redis instance."
          type        = number
          default     = 6379
        }
      providerRef:
        name: tencent
        namespace: default
  workload:
    definition:
      apiVersion: terraform.core.oam.dev/v1beta2
      kind: Configuration
status: {}
`,
		},
		{
			name:   "print in a file",
			args:   []string{"vswitch", "-t", "component", "--provider", "alibaba", "--desc", "xxx", "--git", "https://github.com/kubevela-contrib/terraform-modules.git", "--path", "alibaba/vswitch", "--output", defVswitchFileName},
			output: defVswitchFileName,
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
      apiVersion: terraform.core.oam.dev/v1beta2
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
		{
			name:   "git and local could be set at the same time",
			args:   []string{"vswitch", "-t", "component", "--provider", "alibaba", "--desc", "test", "--git", "xxx", "--local", "yyy"},
			errMsg: "only one of --git and --local can be set",
		},
		{
			name:   "local file doesn't exist",
			args:   []string{"vswitch", "-t", "component", "--provider", "tencent", "--desc", "xxx", "--local", "test-data/redis2.tf"},
			errMsg: "failed to read Terraform configuration from file",
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
			} else if tc.want != "" {
				data, err := os.ReadFile(tc.output)
				defer os.Remove(tc.output)
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
	cmd.SetArgs([]string{traitName, "-n" + VelaTestNamespace})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("unexpeced error when executing get command: %v", err)
	}
	// test multi trait
	cmd = NewDefinitionGetCommand(c)
	initCommand(cmd)
	createNamespacedTrait(c, traitName, "default", "", t)
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

	// Load test DefinitionRevisions files into client
	dir := filepath.Join("..", "..", "pkg", "definition", "testdata")
	testFiles, err := os.ReadDir(dir)
	assert.NoError(t, err, "read testdata failed")
	for _, file := range testFiles {
		if !strings.HasSuffix(file.Name(), ".yaml") {
			continue
		}
		content, err := os.ReadFile(filepath.Join(dir, file.Name()))
		assert.NoError(t, err)
		def := &v1beta1.DefinitionRevision{}
		err = yaml.Unmarshal(content, def)
		assert.NoError(t, err)
		client, err := c.GetClient()
		assert.NoError(t, err)
		err = client.Create(context.TODO(), def)
		assert.NoError(t, err, "cannot create "+file.Name())
	}

	// test get revision list
	cmd = NewDefinitionGetCommand(c)
	initCommand(cmd)
	cmd.SetArgs([]string{"webservice", "--revisions", "--namespace=rev-test-ns"})
	err = cmd.Execute()
	assert.NoError(t, err)

	// test get a non-existent revision
	cmd = NewDefinitionGetCommand(c)
	initCommand(cmd)
	cmd.SetArgs([]string{"webservice", "--revision=3"})
	err = cmd.Execute()
	assert.NotNil(t, err, "should have not found error")

	// test get a revision
	cmd = NewDefinitionGetCommand(c)
	initCommand(cmd)
	cmd.SetArgs([]string{"webservice", "--revision=1", "--namespace=rev-test-ns"})
	err = cmd.Execute()
	assert.NoError(t, err)
}

func TestNewDefinitionGenDocCommand(t *testing.T) {
	c := initArgs()
	cmd := NewDefinitionGenDocCommand(c, util.IOStreams{In: os.Stdin, Out: os.Stdout, ErrOut: os.Stderr})
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
	// with addon filter
	cmd = NewDefinitionListCommand(c)
	initCommand(cmd)
	cmd.SetArgs([]string{"--from", "non-existent-addon"})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("applying addon filter should not return error, err: %v", err)
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
	cmd.SetArgs([]string{traitName, "-n", VelaTestNamespace})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("unexpeced error when executing edit command: %v", err)
	}
	// test no change
	cmd = NewDefinitionEditCommand(c)
	initCommand(cmd)
	createNamespacedTrait(c, traitName, "default", "", t)
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
	ioStreams := util.IOStreams{In: os.Stdin, Out: bytes.NewBuffer(nil), ErrOut: bytes.NewBuffer(nil)}
	// dry-run test
	cmd := NewDefinitionApplyCommand(c, ioStreams)
	initCommand(cmd)
	_, traitFilename := createLocalTrait(t)
	defer removeFile(traitFilename, t)
	cmd.SetArgs([]string{traitFilename, "--dry-run"})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("unexpeced error when executing apply command: %v", err)
	}
	// normal test and reapply
	cmd = NewDefinitionApplyCommand(c, ioStreams)
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
	cmd.SetArgs([]string{traitName, "-n", VelaTestNamespace})
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
	_, traitFilename2 := createLocalTrait(t)
	_, traitFilename3 := createLocalTrait(t)
	defer removeFile(traitFilename, t)
	cmd.SetArgs([]string{traitFilename})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("unexpeced error when executing vet command: %v", err)
	}
	cmd.SetArgs([]string{traitFilename, traitFilename2, traitFilename3})
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
	cmd.SetArgs([]string{traitFilename, traitFilename2, traitFilename3})
	if err = cmd.Execute(); err == nil {
		t.Fatalf("expect validation failed but error not found")
	}
	cmd.SetArgs([]string{"./test-data/defvet"})
	if err = cmd.Execute(); err != nil {
		t.Fatalf("unexpeced error when executing vet command: %v", err)
	}
}

func TestNewDefinitionGenAPICommand(t *testing.T) {
	c := initArgs()
	cmd := NewDefinitionGenAPICommand(c)
	initCommand(cmd)
	internalDefPath := "../../vela-templates/definitions/internal/"

	cmd.SetArgs([]string{"-f", internalDefPath, "-o", "../vela-sdk-gen", "--init", "--verbose"})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("unexpeced error when executing genapi command: %v", err)
	}
}
