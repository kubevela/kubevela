/*
Copyright 2026 The KubeVela Authors.

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

package application

import (
	"os"
	"os/exec"
	"regexp"
	"strings"
	"testing"
)

// chartCUEExperiment is the CUE_EXPERIMENT the chart sets on the controller by
// default (featureGates.enableCueExpVariable). It selects the older evaluator,
// which reports some reads differently, and CUE reads it once per process, so
// the policy context tests are run again in a process of their own with it set.
const chartCUEExperiment = "evalv3=0,keepvalidators=0"

// policyContextTests run again under chartCUEExperiment, and each must report a pass there, so
// a name that matches no test fails the re-run.
var policyContextTests = []string{
	"TestPolicyContextAtAdmission",
	"TestPolicyContextLookups",
	"TestPolicyContextViewUnknownExplicitPolicy",
}

func TestPolicyContextUnderChartCUEExperiment(t *testing.T) {
	if os.Getenv("CUE_EXPERIMENT") != "" {
		t.Skip("already running under CUE_EXPERIMENT=" + os.Getenv("CUE_EXPERIMENT"))
	}
	cmd := exec.Command(os.Args[0], "-test.run", "^("+strings.Join(policyContextTests, "|")+")$", "-test.count=1", "-test.v") //nolint:gosec // re-runs this test binary
	cmd.Env = append(os.Environ(), "CUE_EXPERIMENT="+chartCUEExperiment)
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("policy context tests fail under CUE_EXPERIMENT=%s: %v\n%s", chartCUEExperiment, err, out)
	}
	for _, name := range policyContextTests {
		if !regexp.MustCompile(`(?m)^--- PASS: ` + regexp.QuoteMeta(name) + ` `).Match(out) {
			t.Fatalf("%s did not run under CUE_EXPERIMENT=%s:\n%s", name, chartCUEExperiment, out)
		}
	}
}

// policyContextSpecs are the Ginkgo containers run again under chartCUEExperiment.
var policyContextSpecs = []string{
	"Applications are judged by the context their policies declare",
}

func TestPolicyContextSpecsUnderChartCUEExperiment(t *testing.T) {
	runSpecsUnderChartCUEExperiment(t, policyContextSpecs)
}

// runSpecsUnderChartCUEExperiment runs this package's Ginkgo suite again, in a process of its own
// with chartCUEExperiment set, focused on containers. Each container must report specs there.
func runSpecsUnderChartCUEExperiment(t *testing.T, containers []string) {
	t.Helper()
	if os.Getenv("CUE_EXPERIMENT") != "" {
		t.Skip("already running under CUE_EXPERIMENT=" + os.Getenv("CUE_EXPERIMENT"))
	}
	focus := make([]string, len(containers))
	for i, c := range containers {
		focus[i] = regexp.QuoteMeta(c)
	}
	cmd := exec.Command(os.Args[0], "-test.run", "^TestAPIs$", "-test.count=1", "-ginkgo.v", "-ginkgo.focus", strings.Join(focus, "|")) //nolint:gosec // re-runs this test binary
	cmd.Env = append(os.Environ(), "CUE_EXPERIMENT="+chartCUEExperiment)
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("policy specs fail under CUE_EXPERIMENT=%s: %v\n%s", chartCUEExperiment, err, out)
	}
	for _, c := range containers {
		if !strings.Contains(string(out), c) {
			t.Fatalf("%q ran no specs under CUE_EXPERIMENT=%s:\n%s", c, chartCUEExperiment, out)
		}
	}
}
