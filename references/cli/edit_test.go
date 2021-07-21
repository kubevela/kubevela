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
	"fmt"
	common2 "github.com/oam-dev/kubevela/pkg/utils/common"
	"github.com/oam-dev/kubevela/pkg/utils/util"
	"io/ioutil"
	"os"
	"os/exec"
	"strings"
	"testing"
	"time"
)

func TestNewEditCommand(t *testing.T) {
	// Create test definition
	testDefName := fmt.Sprintf("test-scaler-%d", time.Now().UnixNano())
	testDef := fmt.Sprintf(
		`apiVersion: core.oam.dev/v1beta1
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
        }`, testDefName)
	kubectlCommand := exec.Command("kubectl", "apply", "-f", "-")
	kubectlCommand.Stdin = strings.NewReader(testDef)
	kubectlCommand.Stdout = ioutil.Discard
	kubectlCommand.Stderr = os.Stderr
	defer func() {
		if err := exec.Command("kubectl", "delete", "traitdefinitions.core.oam.dev", testDefName, "--namespace", "vela-system").Run(); err != nil {
			t.Fatalf("Failed to delete test definition: %v\n", err)
		}
	}()
	if err := kubectlCommand.Run(); err != nil {
		t.Fatalf("Failed to create test definition when calling kubectl: %v\n", err)
	}
	cmd := NewEditCommand(common2.Args{}, util.IOStreams{In: os.Stdin, Out: ioutil.Discard, ErrOut: os.Stderr})
	cmd.Flags().StringP("env", "", "", "")

	// validate edit
	cmd.SetArgs([]string{
		"trait", testDefName,
		"--namespace", "vela-system",
		"--editor", "sed -i -e 's/podDisruptive: false/podDisruptive: true/g'",
	})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("Failed to run edit command: %v\n", err)
	}
	out, err := exec.Command("kubectl", "get", "traitdefinitions.core.oam.dev", testDefName, "--namespace", "vela-system", "-o", "jsonpath='{.spec.podDisruptive}'").CombinedOutput()
	if err != nil {
		t.Fatalf("Failed to get updated object: %s\n", out)
	}
	val := strings.Trim(strings.TrimSpace(string(out)), "'")
	if val != "true" {
		t.Fatalf("The object is not updated! Cuurent value: %s, Expected value: true.\n", val)
	}

	// validate --edit-template
	cmd.SetArgs([]string{
		"trait", testDefName,
		"--namespace", "vela-system",
		"--edit-template",
		"--editor", "sed -i -e 's/workload/workloads/g'",
	})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("Failed to run edit command with --edit-template args: %v\n", err)
	}
	out, err = exec.Command("kubectl", "get", "traitdefinitions.core.oam.dev", testDefName, "--namespace", "vela-system", "-o", "jsonpath='{.spec.schematic.cue.template}'").CombinedOutput()
	templateContent := string(out)
	if err != nil {
		t.Fatalf("Failed to get updated object: %s\n", templateContent)
	}
	if strings.Index(templateContent, "workloads") < 0 {
		t.Fatalf("The object template is not updated! Current template: %s", templateContent)
	}

	// test unchanged
	cmd.SetArgs([]string{
		"trait", testDefName,
		"--namespace", "vela-system",
		"--edit-template",
		"--editor", "sed -i -e 's/cpu/cpus/g'",
	})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("Failed to run edit command with --edit-template args during unchange test: %v\n", err)
	}
	out, err = exec.Command("kubectl", "get", "traitdefinitions.core.oam.dev", testDefName, "--namespace", "vela-system", "-o", "jsonpath='{.spec.schematic.cue.template}'").CombinedOutput()
	if err != nil {
		t.Fatalf("Failed to get updated object: %s\n", templateContent)
	}
	if string(out) != templateContent {
		t.Fatalf("Target template should not change.")
	}

	cmd = NewEditCommand(common2.Args{}, util.IOStreams{In: os.Stdin, Out: ioutil.Discard, ErrOut: ioutil.Discard})
	cmd.Flags().StringP("env", "", "", "")

	cmd.SilenceErrors = true
	cmd.SilenceUsage = true
	// test invalid args number
	cmd.SetArgs([]string{"trait"})
	if err := cmd.Execute(); err == nil {
		t.Fatalf("should report invalid parameters error\n")
	}

	// test invalid namespace
	cmd.SetArgs([]string{"trait", testDefName, "--namespace", "default"})
	if err := cmd.Execute(); err == nil {
		t.Fatalf("should report target not found error\n")
	}
}
