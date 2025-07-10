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

package utils

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/oam-dev/kubevela/apis/core.oam.dev/common"
	"github.com/oam-dev/kubevela/apis/core.oam.dev/v1beta1"
	"github.com/oam-dev/kubevela/pkg/oam"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

func TestConstructExtract(t *testing.T) {
	tests := []string{"tam1", "test-comp", "xx", "tt-x-x-c"}
	revisionNum := []int{1, 5, 10, 100000}
	for idx, componentName := range tests {
		t.Run(fmt.Sprintf("tests %d for component[%s]", idx, componentName), func(t *testing.T) {
			revisionName := ConstructRevisionName(componentName, int64(revisionNum[idx]))
			got := ExtractComponentName(revisionName)
			if got != componentName {
				t.Errorf("want to get %s from %s but got %s", componentName, revisionName, got)
			}
			revision, _ := ExtractRevision(revisionName)
			if revision != revisionNum[idx] {
				t.Errorf("want to get %d from %s but got %d", revisionNum[idx], revisionName, revision)
			}
		})
	}
	badRevision := []string{"xx", "yy-", "zz-0.1"}
	t.Run(fmt.Sprintf("tests %s for extractRevision", badRevision), func(t *testing.T) {
		for _, revisionName := range badRevision {
			_, err := ExtractRevision(revisionName)
			if err == nil {
				t.Errorf("want to get err from %s but got nil", revisionName)
			}
		}
	})
}

func TestGetAppRevison(t *testing.T) {
	revisionName, latestRevision := GetAppNextRevision(nil)
	assert.Equal(t, revisionName, "")
	assert.Equal(t, latestRevision, int64(0))
	// the first is always 1
	app := &v1beta1.Application{}
	app.Name = "myapp"
	revisionName, latestRevision = GetAppNextRevision(app)
	assert.Equal(t, revisionName, "myapp-v1")
	assert.Equal(t, latestRevision, int64(1))
	app.Status.LatestRevision = &common.Revision{
		Name:     "myapp-v1",
		Revision: 1,
	}
	// we always automatically advance the revision
	revisionName, latestRevision = GetAppNextRevision(app)
	assert.Equal(t, revisionName, "myapp-v2")
	assert.Equal(t, latestRevision, int64(2))
}

var _ = Describe("removeDefRevLabels", func() {
	Context("when handling empty or nil maps", func() {
		It("should handle empty map", func() {
			labels := map[string]string{}
			removeDefRevLabels(&labels)
			Expect(labels).To(BeEmpty())
		})

		It("should handle map with no definition revision labels", func() {
			labels := map[string]string{
				"app.kubernetes.io/name":    "myapp",
				"app.kubernetes.io/version": "v1.0.0",
				"custom.label":              "value",
			}
			expected := map[string]string{
				"app.kubernetes.io/name":    "myapp",
				"app.kubernetes.io/version": "v1.0.0",
				"custom.label":              "value",
			}
			removeDefRevLabels(&labels)
			Expect(labels).To(Equal(expected))
		})
	})

	Context("when handling mixed labels", func() {
		It("should remove all definition revision labels while preserving others", func() {
			labels := map[string]string{
				// Regular labels that should be preserved
				"app.kubernetes.io/name":    "myapp",
				"app.kubernetes.io/version": "v1.0.0",
				"custom.label":              "value",
				// Definition revision labels that should be removed
				oam.LabelComponentDefinitionRevision + "-webserver": "1",
				oam.LabelTraitDefinitionRevision + "-ingress":       "2",
				oam.LabelWorkflowStepDefinitionRevision + "-deploy": "3",
				oam.LabelPolicyDefinitionRevision + "-security":     "4",
			}

			expected := map[string]string{
				"app.kubernetes.io/name":    "myapp",
				"app.kubernetes.io/version": "v1.0.0",
				"custom.label":              "value",
			}

			removeDefRevLabels(&labels)
			Expect(labels).To(Equal(expected))
		})
	})
})

// var _ = Describe("getDefRevLabel", func() {
//     Context("when handling valid definition types", func() {
//         It("should generate correct label for ComponentType", func() {
//             label, err := getDefRevLabel("webserver", common.ComponentType)
//             Expect(err).ToNot(HaveOccurred())
//             Expect(label).To(Equal(oam.LabelComponentDefinitionRevision + "-webserver"))
//         })

//         It("should generate correct label for TraitType", func() {
//             label, err := getDefRevLabel("ingress", common.TraitType)
//             Expect(err).ToNot(HaveOccurred())
//             Expect(label).To(Equal(oam.LabelTraitDefinitionRevision + "-ingress"))
//         })

//         It("should generate correct label for WorkflowStepType", func() {
//             label, err := getDefRevLabel("deploy", common.WorkflowStepType)
//             Expect(err).ToNot(HaveOccurred())
//             Expect(label).To(Equal(oam.LabelWorkflowStepDefinitionRevision + "-deploy"))
//         })

//         It("should generate correct label for PolicyType", func() {
//             label, err := getDefRevLabel("security", common.PolicyType)
//             Expect(err).ToNot(HaveOccurred())
//             Expect(label).To(Equal(oam.LabelPolicyDefinitionRevision + "-security"))
//         })
//     })

//     Context("when handling invalid definition types", func() {
//         It("should return error for unknown definition type", func() {
//             label, err := getDefRevLabel("test", "UnknownType")
//             Expect(err).To(HaveOccurred())
//             Expect(err.Error()).To(ContainSubstring("unknown definition type"))
//             Expect(label).To(BeEmpty())
//         })

//         It("should return error for empty definition type", func() {
//             label, err := getDefRevLabel("test", "")
//             Expect(err).To(HaveOccurred())
//             Expect(err.Error()).To(ContainSubstring("unknown definition type"))
//             Expect(label).To(BeEmpty())
//         })
//     })

//     Context("when handling definition names", func() {
//         It("should handle simple definition names", func() {
//             label, err := getDefRevLabel("simple", common.ComponentType)
//             Expect(err).ToNot(HaveOccurred())
//             Expect(label).To(Equal(oam.LabelComponentDefinitionRevision + "-simple"))
//         })

//         It("should handle definition names with hyphens", func() {
//             label, err := getDefRevLabel("web-server", common.ComponentType)
//             Expect(err).ToNot(HaveOccurred())
//             Expect(label).To(Equal(oam.LabelComponentDefinitionRevision + "-web-server"))
//         })

//         It("should handle definition names with underscores", func() {
//             label, err := getDefRevLabel("web_server", common.ComponentType)
//             Expect(err).ToNot(HaveOccurred())
//             Expect(label).To(Equal(oam.LabelComponentDefinitionRevision + "-web_server"))
//         })

//         It("should handle definition names with dots", func() {
//             label, err := getDefRevLabel("my.component", common.ComponentType)
//             Expect(err).ToNot(HaveOccurred())
//             Expect(label).To(Equal(oam.LabelComponentDefinitionRevision + "-my.component"))
//         })

//         It("should handle empty definition names", func() {
//             label, err := getDefRevLabel("", common.ComponentType)
//             Expect(err).ToNot(HaveOccurred())
//             Expect(label).To(Equal(oam.LabelComponentDefinitionRevision + "-"))
//         })
//     })

//     Context("when handling long labels (>63 characters)", func() {
//         It("should truncate long labels and add hash suffix", func() {
//             // Create a long definition name that will result in a label > 63 characters
//             longDefName := "very-long-component-name-that-will-exceed-kubernetes-label-length-limit"
            
//             label, err := getDefRevLabel(longDefName, common.ComponentType)
//             Expect(err).ToNot(HaveOccurred())
//             Expect(len(label)).To(Equal(63))
            
//             // The label should start with the prefix
//             Expect(label).To(HavePrefix(oam.LabelComponentDefinitionRevision))
            
//             // The label should be different from the original (non-truncated) version
//             originalLabel := oam.LabelComponentDefinitionRevision + "-" + longDefName
//             Expect(label).ToNot(Equal(originalLabel))
//         })

//         It("should generate consistent hash for the same long input", func() {
//             longDefName := "very-long-component-name-that-will-exceed-kubernetes-label-length-limit"
            
//             label1, err1 := getDefRevLabel(longDefName, common.ComponentType)
//             label2, err2 := getDefRevLabel(longDefName, common.ComponentType)
            
//             Expect(err1).ToNot(HaveOccurred())
//             Expect(err2).ToNot(HaveOccurred())
//             Expect(label1).To(Equal(label2))
//         })

//         It("should generate different hashes for different long inputs", func() {
//             longDefName1 := "very-long-component-name-that-will-exceed-kubernetes-label-length-limit-1"
//             longDefName2 := "very-long-component-name-that-will-exceed-kubernetes-label-length-limit-2"
            
//             label1, err1 := getDefRevLabel(longDefName1, common.ComponentType)
//             label2, err2 := getDefRevLabel(longDefName2, common.ComponentType)
            
//             Expect(err1).ToNot(HaveOccurred())
//             Expect(err2).ToNot(HaveOccurred())
//             Expect(label1).ToNot(Equal(label2))
//         })

//         It("should handle edge case where label is exactly 63 characters", func() {
//             // Calculate the exact length needed to make the label exactly 63 characters
//             prefix := oam.LabelComponentDefinitionRevision + "-"
//             remainingLength := 63 - len(prefix)
//             exactDefName := string(make([]byte, remainingLength))
//             for i := range exactDefName {
//                 exactDefName = exactDefName[:i] + "a" + exactDefName[i+1:]
//             }
            
//             label, err := getDefRevLabel(exactDefName, common.ComponentType)
//             Expect(err).ToNot(HaveOccurred())
//             Expect(len(label)).To(Equal(63))
//             Expect(label).To(Equal(prefix + exactDefName))
//         })
//     })

//     Context("when handling different definition types with same name", func() {
//         It("should generate different labels for same name but different types", func() {
//             defName := "common-name"
            
//             componentLabel, err1 := getDefRevLabel(defName, common.ComponentType)
//             traitLabel, err2 := getDefRevLabel(defName, common.TraitType)
//             workflowLabel, err3 := getDefRevLabel(defName, common.WorkflowStepType)
//             policyLabel, err4 := getDefRevLabel(defName, common.PolicyType)
            
//             Expect(err1).ToNot(HaveOccurred())
//             Expect(err2).ToNot(HaveOccurred())
//             Expect(err3).ToNot(HaveOccurred())
//             Expect(err4).ToNot(HaveOccurred())
            
//             // All labels should be different
//             labels := []string{componentLabel, traitLabel, workflowLabel, policyLabel}
//             for i := 0; i < len(labels); i++ {
//                 for j := i + 1; j < len(labels); j++ {
//                     Expect(labels[i]).ToNot(Equal(labels[j]))
//                 }
//             }
//         })
//     })

//     Context("when handling special characters in definition names", func() {
//         It("should handle definition names with numbers", func() {
//             label, err := getDefRevLabel("component-v2", common.ComponentType)
//             Expect(err).ToNot(HaveOccurred())
//             Expect(label).To(Equal(oam.LabelComponentDefinitionRevision + "-component-v2"))
//         })

//         It("should handle definition names with mixed case", func() {
//             label, err := getDefRevLabel("MyComponent", common.ComponentType)
//             Expect(err).ToNot(HaveOccurred())
//             Expect(label).To(Equal(oam.LabelComponentDefinitionRevision + "-MyComponent"))
//         })

//         It("should handle definition names with special characters", func() {
//             label, err := getDefRevLabel("my-component_v1.0", common.ComponentType)
//             Expect(err).ToNot(HaveOccurred())
//             Expect(label).To(Equal(oam.LabelComponentDefinitionRevision + "-my-component_v1.0"))
//         })
//     })
// })
