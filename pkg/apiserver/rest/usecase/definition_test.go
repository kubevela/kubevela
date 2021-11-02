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

package usecase

import (
	"context"
	"io/ioutil"

	"github.com/google/go-cmp/cmp"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/yaml"

	"github.com/oam-dev/kubevela/apis/core.oam.dev/v1beta1"
	v1 "github.com/oam-dev/kubevela/pkg/apiserver/rest/apis/v1"
	"github.com/oam-dev/kubevela/pkg/apiserver/rest/utils"
	"github.com/oam-dev/kubevela/pkg/oam/util"
)

var _ = Describe("Test namespace usecase functions", func() {
	var (
		definitionUsecase *definitionUsecaseImpl
	)

	BeforeEach(func() {
		definitionUsecase = &definitionUsecaseImpl{kubeClient: k8sClient, caches: make(map[string]*utils.MemoryCache)}
		err := k8sClient.Create(context.Background(), &corev1.Namespace{
			ObjectMeta: metav1.ObjectMeta{
				Name: "vela-system",
			},
		})
		Expect(err).Should(SatisfyAny(BeNil(), &util.AlreadyExistMatcher{}))
	})
	It("Test ListDefinitions function", func() {
		By("List component definitions")
		webserver, err := ioutil.ReadFile("./testdata/webserver-cd.yaml")
		Expect(err).Should(Succeed())
		var cd v1beta1.ComponentDefinition
		err = yaml.Unmarshal(webserver, &cd)
		Expect(err).Should(Succeed())
		err = k8sClient.Create(context.Background(), &cd)
		Expect(err).Should(Succeed())
		components, err := definitionUsecase.ListDefinitions(context.TODO(), "", "component")
		Expect(err).Should(BeNil())
		Expect(cmp.Diff(len(components), 1)).Should(BeEmpty())
		Expect(cmp.Diff(components[0].Name, "webservice-test")).Should(BeEmpty())
		Expect(components[0].Description).ShouldNot(BeEmpty())

		By("List trait definitions")
		myingress, err := ioutil.ReadFile("./testdata/myingress-td.yaml")
		Expect(err).Should(Succeed())
		var td v1beta1.TraitDefinition
		err = yaml.Unmarshal(myingress, &td)
		Expect(err).Should(Succeed())
		err = k8sClient.Create(context.Background(), &td)
		Expect(err).Should(Succeed())
		traits, err := definitionUsecase.ListDefinitions(context.TODO(), "", "trait")
		Expect(err).Should(BeNil())
		Expect(cmp.Diff(len(traits), 1)).Should(BeEmpty())
		Expect(cmp.Diff(traits[0].Name, "myingress")).Should(BeEmpty())
		Expect(traits[0].Description).ShouldNot(BeEmpty())

		By("List workflow step definitions")
		step, err := ioutil.ReadFile("./testdata/applyapplication-sd.yaml")
		Expect(err).Should(Succeed())
		var sd v1beta1.WorkflowStepDefinition
		err = yaml.Unmarshal(step, &sd)
		Expect(err).Should(Succeed())
		err = k8sClient.Create(context.Background(), &sd)
		Expect(err).Should(Succeed())
		wfstep, err := definitionUsecase.ListDefinitions(context.TODO(), "", "workflowstep")
		Expect(err).Should(BeNil())
		Expect(cmp.Diff(len(wfstep), 1)).Should(BeEmpty())
		Expect(cmp.Diff(wfstep[0].Name, "apply-application")).Should(BeEmpty())
		Expect(wfstep[0].Description).ShouldNot(BeEmpty())
	})

	It("Test DetailDefinition function", func() {
		cm := &corev1.ConfigMap{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "schema-apply-object",
				Namespace: "vela-system",
			},
			Data: map[string]string{
				"openapi-v3-json-schema": `{"properties":{"cluster":{"default":"","description":"Specify the cluster of the object","title":"cluster","type":"string"},"value":{"description":"Specify the value of the object","title":"value","type":"object"}},"required":["value","cluster"],"type":"object"}`,
			},
		}
		err := k8sClient.Create(context.Background(), cm)
		Expect(err).Should(Succeed())
		schema, err := definitionUsecase.DetailDefinition(context.TODO(), "apply-object")
		Expect(err).Should(BeNil())
		Expect(schema.Schema).Should(Equal(&v1.DefinitionSchema{
			Properties: map[string]v1.DefinitionProperties{
				"value": {
					Default:     "",
					Description: "Specify the value of the object",
					Title:       "value",
					Type:        "object",
				},
				"cluster": {
					Default:     "",
					Description: "Specify the cluster of the object",
					Title:       "cluster",
					Type:        "string",
				},
			},
			Required: []string{"value", "cluster"},
			Type:     "object",
		}))
	})
})
