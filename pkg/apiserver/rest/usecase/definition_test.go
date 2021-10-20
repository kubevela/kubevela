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
	"sigs.k8s.io/yaml"

	"github.com/oam-dev/kubevela/apis/core.oam.dev/v1beta1"
	"github.com/oam-dev/kubevela/pkg/apiserver/rest/utils"
)

var _ = Describe("Test namespace usecase functions", func() {
	var (
		definitionUsecase *definitionUsecaseImpl
	)
	BeforeEach(func() {
		definitionUsecase = &definitionUsecaseImpl{kubeClient: k8sClient, caches: make(map[string]*utils.MemoryCache)}
	})
	It("Test ListComponentDefinitions function", func() {
		bs, err := ioutil.ReadFile("./testdata/webserver-cd.yaml")
		Expect(err).Should(Succeed())
		var test v1beta1.ComponentDefinition
		err = yaml.Unmarshal(bs, &test)
		Expect(err).Should(Succeed())
		err = k8sClient.Create(context.Background(), &test)
		Expect(err).Should(Succeed())
		components, err := definitionUsecase.ListComponentDefinitions(context.TODO(), "")
		Expect(err).Should(BeNil())
		Expect(cmp.Diff(len(components), 1)).Should(BeEmpty())
		Expect(cmp.Diff(components[0].Name, "webservice-test")).Should(BeEmpty())
		Expect(components[0].Description).ShouldNot(BeEmpty())
	})
})
