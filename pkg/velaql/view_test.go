/*
 Copyright 2021. The KubeVela Authors.

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

package velaql

import (
	"context"
	"fmt"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

var _ = Describe("Test VelaQL View", func() {
	var ctx = context.Background()

	It("Test query a sample view", func() {
		parameter := map[string]string{
			"apiVersion": "v1",
			"kind":       "Pod",
			"name":       pod.Name,
		}

		velaQL := fmt.Sprintf("%s{%s}.%s", readView.Name, Map2URLParameter(parameter), "objStatus")
		query, err := ParseVelaQL(velaQL)
		Expect(err).ShouldNot(HaveOccurred())

		queryValue, err := viewHandler.QueryView(context.Background(), query)
		Expect(err).Should(BeNil())

		podStatus := corev1.PodStatus{}
		Expect(queryValue.UnmarshalTo(&podStatus)).Should(BeNil())
	})

	It("Test query view with wrong request", func() {
		parameter := map[string]string{
			"apiVersion": "v1",
			"kind":       "Pod",
			"name":       pod.Name,
		}

		By("query view with an non-existent result")
		velaQL := fmt.Sprintf("%s{%s}.%s", readView.Name, Map2URLParameter(parameter), "appStatus")
		query, err := ParseVelaQL(velaQL)
		Expect(err).ShouldNot(HaveOccurred())
		v, err := viewHandler.QueryView(context.Background(), query)
		Expect(err).ShouldNot(HaveOccurred())
		s, err := v.String()
		Expect(err).ShouldNot(HaveOccurred())
		Expect(s).Should(Equal("null\n"))

		By("query an non-existent view")
		velaQL = fmt.Sprintf("%s{%s}.%s", "view-resource", Map2URLParameter(parameter), "objStatus")
		query, err = ParseVelaQL(velaQL)
		Expect(err).ShouldNot(HaveOccurred())
		_, err = viewHandler.QueryView(context.Background(), query)
		Expect(err).Should(HaveOccurred())
	})

	It("Test apply resource in view", func() {
		parameter := map[string]string{
			"apiVersion": "v1",
			"kind":       "Namespace",
			"name":       "test-namespace",
		}
		velaQL := fmt.Sprintf("%s{%s}.%s", applyView.Name, Map2URLParameter(parameter), "objStatus")
		query, err := ParseVelaQL(velaQL)
		Expect(err).ShouldNot(HaveOccurred())
		_, err = viewHandler.QueryView(context.Background(), query)
		Expect(err).ShouldNot(HaveOccurred())

		ns := corev1.Namespace{}
		Expect(k8sClient.Get(ctx, client.ObjectKey{Name: "test-namespace"}, &ns)).Should(BeNil())
	})
})

func Map2URLParameter(parameter map[string]string) string {
	var res string
	for k, v := range parameter {
		res += fmt.Sprintf("%s=\"%s\",", k, v)
	}
	return res
}
