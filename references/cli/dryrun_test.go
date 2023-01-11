/*
 Copyright 2022 The KubeVela Authors.

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
	"os"
	"strings"

	wfv1alpha1 "github.com/kubevela/workflow/api/v1alpha1"

	"github.com/oam-dev/kubevela/apis/core.oam.dev/v1alpha1"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"sigs.k8s.io/yaml"

	"github.com/oam-dev/kubevela/apis/core.oam.dev/v1beta1"
	"github.com/oam-dev/kubevela/apis/types"
	common2 "github.com/oam-dev/kubevela/pkg/utils/common"
)

var _ = Describe("Testing dry-run", func() {

	It("Testing dry-run", func() {

		webservice, err := os.ReadFile("../../charts/vela-core/templates/defwithtemplate/webservice.yaml")
		Expect(err).Should(BeNil())
		webserviceYAML := strings.Replace(string(webservice), "{{ include \"systemDefinitionNamespace\" . }}", types.DefaultKubeVelaNS, 1)
		wwd := v1beta1.ComponentDefinition{}
		Expect(yaml.Unmarshal([]byte(webserviceYAML), &wwd)).Should(BeNil())
		Expect(k8sClient.Create(context.TODO(), &wwd)).Should(BeNil())

		scaler, err := os.ReadFile("../../charts/vela-core/templates/defwithtemplate/scaler.yaml")
		Expect(err).Should(BeNil())
		scalerYAML := strings.Replace(string(scaler), "{{ include \"systemDefinitionNamespace\" . }}", types.DefaultKubeVelaNS, 1)
		var td v1beta1.TraitDefinition
		Expect(yaml.Unmarshal([]byte(scalerYAML), &td)).Should(BeNil())
		Expect(k8sClient.Create(context.TODO(), &td)).Should(BeNil())

		c := common2.Args{}
		c.SetConfig(cfg)
		c.SetClient(k8sClient)
		opt := DryRunCmdOptions{ApplicationFiles: []string{"test-data/dry-run/testing-dry-run-1.yaml"}, OfflineMode: false}
		buff, err := DryRunApplication(&opt, c, "")
		Expect(err).Should(BeNil())
		Expect(buff.String()).Should(ContainSubstring("name: testing-dryrun"))
		Expect(buff.String()).Should(ContainSubstring("kind: Deployment"))
		Expect(buff.String()).Should(ContainSubstring("replicas: 1"))
	})

	It("Testing dry-run with policy", func() {
		deploy, err := os.ReadFile("../../charts/vela-core/templates/defwithtemplate/deploy.yaml")
		Expect(err).Should(BeNil())
		deployYAML := strings.Replace(string(deploy), "{{ include \"systemDefinitionNamespace\" . }}", types.DefaultKubeVelaNS, 1)
		var wfsd v1beta1.WorkflowStepDefinition
		Expect(yaml.Unmarshal([]byte(deployYAML), &wfsd)).Should(BeNil())
		Expect(k8sClient.Create(context.TODO(), &wfsd)).Should(BeNil())

		c := common2.Args{}
		c.SetConfig(cfg)
		c.SetClient(k8sClient)
		opt := DryRunCmdOptions{ApplicationFiles: []string{"test-data/dry-run/testing-dry-run-2.yaml"}, OfflineMode: false}
		buff, err := DryRunApplication(&opt, c, "")
		Expect(err).Should(BeNil())
		Expect(buff.String()).Should(ContainSubstring("# Application(testing-app with topology target-default)"))
		Expect(buff.String()).Should(ContainSubstring("name: testing-dryrun"))
		Expect(buff.String()).Should(ContainSubstring("kind: Deployment"))
		Expect(buff.String()).Should(ContainSubstring("replicas: 1"))
	})

	It("Testing dry-run with workflow", func() {

		c := common2.Args{}
		c.SetConfig(cfg)
		c.SetClient(k8sClient)
		opt := DryRunCmdOptions{ApplicationFiles: []string{"test-data/dry-run/testing-dry-run-3.yaml"}, OfflineMode: false}
		buff, err := DryRunApplication(&opt, c, "")
		Expect(err).Should(BeNil())
		Expect(buff.String()).Should(ContainSubstring("# Application(testing-app with topology target-default)"))
		Expect(buff.String()).Should(ContainSubstring("# Application(testing-app with topology target-prod)"))
		Expect(buff.String()).Should(ContainSubstring("name: testing-dryrun"))
		Expect(buff.String()).Should(ContainSubstring("kind: Deployment"))
		Expect(buff.String()).Should(ContainSubstring("replicas: 1"))
		Expect(buff.String()).Should(ContainSubstring("replicas: 3"))
	})

	It("Testing dry-run with ref workflow", func() {

		policy, err := os.ReadFile("test-data/dry-run/testing-policy.yaml")
		Expect(err).Should(BeNil())
		var p v1alpha1.Policy
		Expect(yaml.Unmarshal([]byte(policy), &p)).Should(BeNil())
		Expect(k8sClient.Create(context.TODO(), &p)).Should(BeNil())

		workflow, err := os.ReadFile("test-data/dry-run/testing-wf.yaml")
		Expect(err).Should(BeNil())
		var wf wfv1alpha1.Workflow
		Expect(yaml.Unmarshal([]byte(workflow), &wf)).Should(BeNil())
		Expect(k8sClient.Create(context.TODO(), &wf)).Should(BeNil())

		c := common2.Args{}
		c.SetConfig(cfg)
		c.SetClient(k8sClient)
		opt := DryRunCmdOptions{ApplicationFiles: []string{"test-data/dry-run/testing-dry-run-4.yaml"}, OfflineMode: false}
		buff, err := DryRunApplication(&opt, c, "")
		Expect(err).Should(BeNil())
		Expect(buff.String()).Should(ContainSubstring("# Application(testing-app with topology deploy-somewhere)"))
		Expect(buff.String()).Should(ContainSubstring("name: testing-dryrun"))
		Expect(buff.String()).Should(ContainSubstring("kind: Deployment"))
	})

	It("Testing dry-run without application provided", func() {

		c := common2.Args{}
		c.SetConfig(cfg)
		c.SetClient(k8sClient)
		opt := DryRunCmdOptions{ApplicationFiles: []string{"test-data/dry-run/testing-policy.yaml"}, OfflineMode: false}
		_, err := DryRunApplication(&opt, c, "")
		Expect(err).ShouldNot(BeNil())
		Expect(err.Error()).Should(ContainSubstring("no application provided"))

	})

	It("Testing dry-run with more than one applications provided", func() {

		c := common2.Args{}
		c.SetConfig(cfg)
		c.SetClient(k8sClient)
		opt := DryRunCmdOptions{ApplicationFiles: []string{"test-data/dry-run/testing-dry-run-1.yaml", "test-data/dry-run/testing-dry-run-2.yaml"}, OfflineMode: false}
		_, err := DryRunApplication(&opt, c, "")
		Expect(err).ShouldNot(BeNil())
		Expect(err.Error()).Should(ContainSubstring("more than one applications provided"))

	})

	It("Testing dry-run with more than one workflow provided", func() {

		c := common2.Args{}
		c.SetConfig(cfg)
		c.SetClient(k8sClient)
		opt := DryRunCmdOptions{ApplicationFiles: []string{"test-data/dry-run/testing-dry-run-1.yaml", "test-data/dry-run/testing-wf.yaml", "test-data/dry-run/testing-wf.yaml"}, OfflineMode: false}
		_, err := DryRunApplication(&opt, c, "")
		Expect(err).ShouldNot(BeNil())
		Expect(err.Error()).Should(ContainSubstring("more than one external workflow provided"))

	})

	It("Testing dry-run with unexpected file", func() {

		c := common2.Args{}
		c.SetConfig(cfg)
		c.SetClient(k8sClient)
		opt := DryRunCmdOptions{ApplicationFiles: []string{"test-data/dry-run/testing-trait.yaml"}, OfflineMode: false}
		_, err := DryRunApplication(&opt, c, "")
		Expect(err).ShouldNot(BeNil())
		Expect(err.Error()).Should(ContainSubstring("is not application, policy or workflow"))

	})

	It("Testing dry-run with unexpected file", func() {

		c := common2.Args{}
		c.SetConfig(cfg)
		c.SetClient(k8sClient)
		opt := DryRunCmdOptions{ApplicationFiles: []string{"test-data/dry-run/testing-trait.yaml"}, OfflineMode: false}
		_, err := DryRunApplication(&opt, c, "")
		Expect(err).ShouldNot(BeNil())
		Expect(err.Error()).Should(ContainSubstring("is not application, policy or workflow"))

	})

	It("Testing dry-run merging with external workflow and policy", func() {

		c := common2.Args{}
		c.SetConfig(cfg)
		c.SetClient(k8sClient)
		opt := DryRunCmdOptions{ApplicationFiles: []string{"test-data/dry-run/testing-dry-run-5.yaml", "test-data/dry-run/testing-wf.yaml", "test-data/dry-run/testing-policy.yaml"}, OfflineMode: false, MergeStandaloneFiles: true}
		buff, err := DryRunApplication(&opt, c, "")
		Expect(err).Should(BeNil())
		Expect(buff.String()).Should(ContainSubstring("# Application(testing-app with topology deploy-somewhere)"))
		Expect(buff.String()).Should(ContainSubstring("name: testing-dryrun"))
		Expect(buff.String()).Should(ContainSubstring("kind: Deployment"))
	})

	It("Testing dry-run with standalone policy", func() {

		c := common2.Args{}
		c.SetConfig(cfg)
		c.SetClient(k8sClient)
		opt := DryRunCmdOptions{ApplicationFiles: []string{"test-data/dry-run/testing-dry-run-5.yaml", "test-data/dry-run/testing-policy.yaml"}, OfflineMode: false, MergeStandaloneFiles: false}
		buff, err := DryRunApplication(&opt, c, "")
		Expect(err).Should(BeNil())
		Expect(buff.String()).Should(ContainSubstring("WARNING: policy deploy-somewhere not referenced by application"))
		Expect(buff.String()).Should(ContainSubstring("name: testing-dryrun"))
		Expect(buff.String()).Should(ContainSubstring("kind: Deployment"))
	})

	It("Testing dry-run with standalone workflow", func() {

		c := common2.Args{}
		c.SetConfig(cfg)
		c.SetClient(k8sClient)
		opt := DryRunCmdOptions{ApplicationFiles: []string{"test-data/dry-run/testing-dry-run-5.yaml", "test-data/dry-run/testing-wf.yaml"}, OfflineMode: false, MergeStandaloneFiles: false}
		buff, err := DryRunApplication(&opt, c, "")
		Expect(err).Should(BeNil())
		Expect(buff.String()).Should(ContainSubstring("WARNING: workflow testing-wf not referenced by application"))
		Expect(buff.String()).Should(ContainSubstring("name: testing-dryrun"))
		Expect(buff.String()).Should(ContainSubstring("kind: Deployment"))
	})

	It("Testing dry-run offline", func() {

		c := common2.Args{}
		c.SetConfig(cfg)
		c.SetClient(k8sClient)
		opt := DryRunCmdOptions{ApplicationFiles: []string{"test-data/dry-run/testing-dry-run-6.yaml"}, DefinitionFile: "test-data/dry-run/testing-worker-def.yaml", OfflineMode: true}
		buff, err := DryRunApplication(&opt, c, "")
		Expect(err).Should(BeNil())
		Expect(buff.String()).Should(ContainSubstring("# Application(testing-app)"))
		Expect(buff.String()).Should(ContainSubstring("name: testing-dryrun"))
		Expect(buff.String()).Should(ContainSubstring("kind: Deployment"))
		Expect(buff.String()).Should(ContainSubstring("workload.oam.dev/type: myworker"))
	})
})
