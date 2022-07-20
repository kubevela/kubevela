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

package dryrun

import (
	"context"
	"encoding/json"

	"github.com/oam-dev/kubevela/apis/types"

	"github.com/google/go-cmp/cmp"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"sigs.k8s.io/yaml"

	"github.com/oam-dev/kubevela/apis/core.oam.dev/v1beta1"
)

var _ = Describe("Test DryRun", func() {
	It("Test DryRun", func() {
		appYAML := readDataFromFile("./testdata/dryrun-app.yaml")
		By("Prepare test data")

		app := &v1beta1.Application{}
		b, err := yaml.YAMLToJSON([]byte(appYAML))
		Expect(err).Should(BeNil())
		err = json.Unmarshal(b, app)
		Expect(err).Should(BeNil())

		By("Validate App With Empty Namespace")
		err = dryrunOpt.ValidateApp(context.Background(), "./testdata/dryrun-app.yaml")
		Expect(err).Should(BeNil())

		By("Execute DryRun")
		comps, _, err := dryrunOpt.ExecuteDryRun(context.Background(), app)
		Expect(err).Should(BeNil())

		expectCompYAML := readDataFromFile("./testdata/dryrun-exp-comp.yaml")
		By("Verify generated Comp")
		Expect(comps).ShouldNot(BeEmpty())
		var expC = types.ComponentManifest{}
		err = yaml.Unmarshal([]byte(expectCompYAML), &expC)
		Expect(err).Should(BeNil())
		diff := cmp.Diff(&expC, comps[0])
		Expect(diff).Should(BeEmpty())
	})
})
