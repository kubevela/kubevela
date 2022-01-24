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
	"fmt"
	"io/ioutil"

	"github.com/oam-dev/kubevela/pkg/oam/util"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/yaml"

	"github.com/oam-dev/kubevela/apis/types"
	"github.com/oam-dev/kubevela/pkg/apiserver/rest/utils"
)

var _ = Describe("addon usecase test", func() {
	var ctx context.Context

	BeforeEach(func() {
		ctx = context.Background()
		Expect(k8sClient.Create(ctx, &v1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: types.DefaultKubeVelaNS}})).Should(SatisfyAny(BeNil(), util.AlreadyExistMatcher{}))
	})

	It("Test render customize ui-schema", func() {
		schemaData, err := ioutil.ReadFile("testdata/addon-uischema-test.yaml")

		addonName := "test"
		Expect(err).Should(BeNil())
		jsonData, err := yaml.YAMLToJSON(schemaData)
		Expect(err).Should(BeNil())
		cm := v1.ConfigMap{
			TypeMeta:   metav1.TypeMeta{APIVersion: "v1", Kind: "ConfigMap"},
			ObjectMeta: metav1.ObjectMeta{Namespace: types.DefaultKubeVelaNS, Name: fmt.Sprintf("addon-uischema-%s", addonName)},
			Data: map[string]string{
				types.UISchema: string(jsonData),
			}}

		Expect(k8sClient.Create(ctx, &cm)).Should(BeNil())
		defaultSchema := []*utils.UIParameter{
			{
				JSONKey: "version",
				Sort:    3,
			},
			{
				JSONKey: "domain",
				Sort:    8,
			},
		}
		res := renderAddonCustomUISchema(ctx, k8sClient, addonName, defaultSchema)
		Expect(len(res)).Should(BeEquivalentTo(2))
		for _, re := range res {
			if re.JSONKey == "version" {
				Expect(re.Validate.DefaultValue.(string)).Should(BeEquivalentTo("1.2.0-rc1"))
				Expect(re.Sort).Should(BeEquivalentTo(1))
			}
			if re.JSONKey == "domain" {
				Expect(re.Sort).Should(BeEquivalentTo(9))
			}
		}
	})
})
