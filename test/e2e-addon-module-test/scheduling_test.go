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

package addonmoduletest

import (
	"strings"
	"testing"

	"github.com/onsi/ginkgo/v2"
	"github.com/onsi/ginkgo/v2/types"
)

func TestAddonModuleScheduling(t *testing.T) {
	report := ginkgo.PreviewSpecs("Addon/module scheduling")
	serialScenarios := map[string]bool{"05": true, "09": true, "15": true, "18": true}
	var versionLocations []types.CodeLocation
	serial, parallel := 0, 0
	for _, spec := range report.SpecReports {
		if spec.LeafNodeType != types.NodeTypeIt {
			continue
		}
		id := ""
		for _, labels := range spec.ContainerHierarchyLabels {
			for _, label := range labels {
				if strings.HasPrefix(label, "addon-module-scenario-") {
					id = strings.TrimPrefix(label, "addon-module-scenario-")
				}
			}
		}
		if spec.IsSerial != serialScenarios[id] {
			t.Errorf("%s: Serial=%v, want %v", spec.FullText(), spec.IsSerial, serialScenarios[id])
		}
		if spec.IsSerial {
			serial++
		} else {
			parallel++
		}
		if id != "" && id != "01" && !spec.IsInOrderedContainer {
			t.Errorf("scenario %s lost its dependent-step ordering", id)
		}
		if id == "08" || id == "07" {
			versionLocations = append(versionLocations, spec.ContainerHierarchyLocations[0])
		} else if id != "" && spec.IsInOrderedContainer {
			// Other scenarios must have their own outermost Ordered Context,
			// not inherit a suite-wide Ordered Describe that pins one worker.
			if len(spec.ContainerHierarchyTexts) != 2 {
				t.Errorf("unexpected scenario hierarchy: %v", spec.ContainerHierarchyTexts)
			}
		}
	}
	if serial == 0 || parallel == 0 || len(versionLocations) == 0 {
		t.Fatalf("empty scheduling phase: serial=%d parallel=%d versions=%d", serial, parallel, len(versionLocations))
	}
	for _, location := range versionLocations {
		if location != versionLocations[0] {
			t.Fatal("scenarios 08 and 07 are not in the same ordered chain")
		}
	}
	t.Logf("discovered %d parallel-capable and %d serial specs", parallel, serial)
}
