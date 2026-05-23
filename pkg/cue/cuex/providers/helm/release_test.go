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

package helm

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"helm.sh/helm/v3/pkg/chart"
)

var _ = Describe("release", func() {

	Describe("computeReleaseFingerprint", func() {
		It("should be deterministic for same inputs", func() {
			ch := &chart.Chart{Metadata: &chart.Metadata{Version: "1.2.3"}}
			values := map[string]interface{}{"replicas": 2}

			fp1 := computeReleaseFingerprint(ch, values)
			fp2 := computeReleaseFingerprint(ch, values)
			Expect(fp1).To(Equal(fp2))
		})

		It("should differ for different values", func() {
			ch := &chart.Chart{Metadata: &chart.Metadata{Version: "1.2.3"}}
			fp1 := computeReleaseFingerprint(ch, map[string]interface{}{"replicas": 2})
			fp2 := computeReleaseFingerprint(ch, map[string]interface{}{"replicas": 3})
			Expect(fp1).ToNot(Equal(fp2))
		})

		It("should encode the chart version", func() {
			ch := &chart.Chart{Metadata: &chart.Metadata{Version: "1.2.3"}}
			fp := computeReleaseFingerprint(ch, map[string]interface{}{"replicas": 2})
			Expect(fp).To(ContainSubstring("1.2.3"))
		})

		It("should differ for different chart versions", func() {
			ch1 := &chart.Chart{Metadata: &chart.Metadata{Version: "1.0.0"}}
			ch2 := &chart.Chart{Metadata: &chart.Metadata{Version: "2.0.0"}}
			values := map[string]interface{}{"key": "val"}

			fp1 := computeReleaseFingerprint(ch1, values)
			fp2 := computeReleaseFingerprint(ch2, values)
			Expect(fp1).ToNot(Equal(fp2))
		})

		It("should handle nil chart metadata", func() {
			fp := computeReleaseFingerprint(nil, map[string]interface{}{"replicas": 2})
			Expect(fp).ToNot(BeEmpty())
		})

		It("should treat nil values and empty map as equivalent", func() {
			// Helm stores release.Config as nil when no values were supplied
			// at install time, but mergeValues returns an empty map for the
			// same logical input. Without normalising the two, the dedup
			// check at the call site would mis-fire on every reconcile and
			// trigger spurious helm upgrades for releases installed with
			// empty/optional valuesFrom sources.
			ch := &chart.Chart{Metadata: &chart.Metadata{Version: "1.2.3"}}
			fpNil := computeReleaseFingerprint(ch, nil)
			fpEmpty := computeReleaseFingerprint(ch, map[string]interface{}{})
			Expect(fpNil).To(Equal(fpEmpty))
		})
	})

})
