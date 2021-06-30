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

package applicationrollout

import (
	"sort"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"

	"github.com/oam-dev/kubevela/apis/types"
	"github.com/oam-dev/kubevela/pkg/controller/utils"
)

var _ = Describe("Application Deployment Common Function Test", func() {
	BeforeEach(func() {

	})

	Context("Test Find Common Component Function", func() {
		var targetApp, sourceApp []*types.ComponentManifest

		It("Test source app is nil", func() {
			targetApp = fillApplication([]string{"a", "b", "c"})
			common := FindCommonComponent(targetApp, nil)
			Expect(common).Should(BeEquivalentTo([]string{"a", "b", "c"}))
		})

		It("Test has one component", func() {
			targetApp = fillApplication([]string{"a", "b", "c"})
			sourceApp = fillApplication([]string{"c"})
			common := FindCommonComponent(targetApp, sourceApp)
			Expect(common).Should(BeEquivalentTo([]string{"c"}))
		})

		It("Test has one common components", func() {
			targetApp = fillApplication([]string{"a", "b", "c"})
			sourceApp = fillApplication([]string{"d", "c"})
			common := FindCommonComponent(targetApp, sourceApp)
			Expect(common).Should(BeEquivalentTo([]string{"c"}))
		})

		It("Test has more than 1 common component", func() {
			targetApp = fillApplication([]string{"b", "a", "c"})
			sourceApp = fillApplication([]string{"c", "b"})
			common := FindCommonComponent(targetApp, sourceApp)
			Expect(common).Should(BeEquivalentTo([]string{"c", "b"}))
		})

		It("Test has more than 1 common component", func() {
			targetApp = fillApplication([]string{"a", "b", "c"})
			sourceApp = fillApplication([]string{"d", "e", "c", "a"})
			common := FindCommonComponent(targetApp, sourceApp)
			Expect(common).Should(BeEquivalentTo([]string{"c", "a"}))
		})
	})
})

func fillApplication(componentNames []string) []*types.ComponentManifest {
	r := make([]*types.ComponentManifest, len(componentNames))
	for i, name := range componentNames {
		r[i] = &types.ComponentManifest{
			RevisionName: utils.ConstructRevisionName(name, 1),
		}
	}
	return r
}

var _ = Describe("Test find common component func", func() {
	It("Test source app is nil", func() {
		target := fillWorkloads([]string{"a", "b", "c"})
		common := FindCommonComponentWithManifest(target, nil)
		sort.Strings(common)
		Expect(common).Should(BeEquivalentTo([]string{"a", "b", "c"}))
	})

	It("Test has one component", func() {
		target := fillWorkloads([]string{"a", "b", "c"})
		source := fillWorkloads([]string{"c"})
		common := FindCommonComponentWithManifest(target, source)
		sort.Strings(common)
		Expect(common).Should(BeEquivalentTo([]string{"c"}))
	})

	It("Test has one common components", func() {
		target := fillWorkloads([]string{"a", "b", "c"})
		source := fillWorkloads([]string{"d", "c"})
		common := FindCommonComponentWithManifest(target, source)
		sort.Strings(common)
		Expect(common).Should(BeEquivalentTo([]string{"c"}))
	})

	It("Test has more than 1 common component", func() {
		target := fillWorkloads([]string{"b", "a", "c"})
		source := fillWorkloads([]string{"c", "b"})
		common := FindCommonComponentWithManifest(target, source)
		sort.Strings(common)
		Expect(common).Should(BeEquivalentTo([]string{"b", "c"}))
	})

	It("Test has more than 1 common component", func() {
		target := fillWorkloads([]string{"a", "b", "c"})
		source := fillWorkloads([]string{"d", "e", "c", "a"})
		common := FindCommonComponentWithManifest(target, source)
		sort.Strings(common)
		Expect(common).Should(BeEquivalentTo([]string{"a", "c"}))
	})
})

func fillWorkloads(componentNames []string) map[string]*unstructured.Unstructured {
	w := make(map[string]*unstructured.Unstructured)
	for _, s := range componentNames {
		// we don't need real workload
		w[s] = nil
	}
	return w
}
