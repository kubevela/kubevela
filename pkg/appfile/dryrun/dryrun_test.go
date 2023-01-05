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
	"bytes"
	"context"
	"encoding/json"
	"os"
	"strings"

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

var _ = Describe("Test dry run with policies", func() {
	It("Test dry run with override policy", func() {

		webservice, err := os.ReadFile("../../../charts/vela-core/templates/defwithtemplate/webservice.yaml")
		Expect(err).Should(BeNil())
		webserviceYAML := strings.Replace(string(webservice), "{{ include \"systemDefinitionNamespace\" . }}", types.DefaultKubeVelaNS, 1)
		wwd := v1beta1.ComponentDefinition{}
		Expect(yaml.Unmarshal([]byte(webserviceYAML), &wwd)).Should(BeNil())
		Expect(k8sClient.Create(context.TODO(), &wwd)).Should(BeNil())

		scaler, err := os.ReadFile("../../../charts/vela-core/templates/defwithtemplate/scaler.yaml")
		Expect(err).Should(BeNil())
		scalerYAML := strings.Replace(string(scaler), "{{ include \"systemDefinitionNamespace\" . }}", types.DefaultKubeVelaNS, 1)
		var td v1beta1.TraitDefinition
		Expect(yaml.Unmarshal([]byte(scalerYAML), &td)).Should(BeNil())
		Expect(k8sClient.Create(context.TODO(), &td)).Should(BeNil())

		//deploy, err := os.ReadFile("../../../charts/vela-core/templates/defwithtemplate/deploy.yaml")
		//Expect(err).Should(BeNil())
		//deployYAML := strings.Replace(string(deploy), "{{ include \"systemDefinitionNamespace\" . }}", types.DefaultKubeVelaNS, 1)
		//var wfsd v1beta1.WorkflowStepDefinition
		//Expect(yaml.Unmarshal([]byte(deployYAML), &wfsd)).Should(BeNil())
		//Expect(k8sClient.Create(context.TODO(), &wfsd)).Should(BeNil())

		appYAML := readDataFromFile("./testdata/testing-dry-run-override-policy.yaml")
		app := &v1beta1.Application{}
		Expect(yaml.Unmarshal([]byte(appYAML), &app)).Should(BeNil())

		var buff = bytes.Buffer{}
		err = dryrunOpt.ExecuteDryRunWithPolicies(context.TODO(), app, &buff)
		Expect(err).Should(BeNil())
		Expect(buff.String()).Should(ContainSubstring("# Application(testing-app with topology target-default)"))
		Expect(buff.String()).Should(ContainSubstring("# Application(testing-app with topology target-prod)"))
		Expect(buff.String()).Should(ContainSubstring("name: testing-dryrun"))
		Expect(buff.String()).Should(ContainSubstring("kind: Deployment"))
		Expect(buff.String()).Should(ContainSubstring("replicas: 1"))
		Expect(buff.String()).Should(ContainSubstring("replicas: 3"))
	})
})
