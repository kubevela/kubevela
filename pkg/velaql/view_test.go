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
	"os"
	"strconv"
	"testing"
	"time"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"github.com/stretchr/testify/assert"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	pkgtypes "k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/oam-dev/kubevela/apis/types"
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

func TestParseViewIntoConfigMap(t *testing.T) {
	cases := []struct {
		cueStr  string
		succeed bool
	}{
		{
			cueStr:  `{`,
			succeed: false,
		},
		{
			cueStr:  `{}`,
			succeed: false,
		},
		{
			cueStr: `{}
status: something`,
			succeed: false,
		},
		{
			cueStr: `something: {}
status: something`,
			succeed: true,
		},
		{
			cueStr: `something: {}
export: something`,
			succeed: true,
		},
	}
	for _, c := range cases {
		cm, err := ParseViewIntoConfigMap(c.cueStr, "name")
		assert.Equal(t, c.succeed, err == nil, err)
		if err == nil {
			assert.Equal(t, c.cueStr, cm.Data["template"])
		}
	}
}

func getViewConfigMap(name string) (*corev1.ConfigMap, error) {
	cm := &corev1.ConfigMap{
		TypeMeta: metav1.TypeMeta{
			APIVersion: "v1",
			Kind:       "ConfigMap",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: types.DefaultKubeVelaNS,
		},
	}

	err := k8sClient.Get(context.TODO(), pkgtypes.NamespacedName{
		Namespace: cm.GetNamespace(),
		Name:      cm.GetName(),
	}, cm)

	if err != nil {
		return nil, err
	}

	return cm, nil
}

var _ = Describe("test StoreViewFromFile", func() {
	Describe("normal creation", func() {
		It("from local file", func() {
			cueStr := "something: {}\nstatus: something"
			filename := "norm-create.cue"
			err := os.WriteFile(filename, []byte(cueStr), 0600)
			Expect(err).Should(Succeed())
			defer os.RemoveAll(filename)
			viewName := "test-view-" + strconv.FormatInt(time.Now().UnixNano(), 10)
			err = StoreViewFromFile(context.TODO(), k8sClient, filename, viewName)
			Expect(err).Should(Succeed())
			// We should be able to get it.
			_, err = getViewConfigMap(viewName)
			Expect(err).Should(Succeed())
		})

		It("update a previously updated view", func() {
			cueStr := "something: {}\nstatus: something"
			filename := "norm-create.cue"
			err := os.WriteFile(filename, []byte(cueStr), 0600)
			Expect(err).Should(Succeed())
			defer os.RemoveAll(filename)
			viewName := "test-view-" + strconv.FormatInt(time.Now().UnixNano(), 10)
			err = StoreViewFromFile(context.TODO(), k8sClient, filename, viewName)
			Expect(err).Should(Succeed())
			// Update it
			newCueStr := "something: {a: \"123\"}\nstatus: something"
			err = os.WriteFile(filename, []byte(newCueStr), 0600)
			Expect(err).Should(Succeed())
			err = StoreViewFromFile(context.TODO(), k8sClient, filename, viewName)
			Expect(err).Should(Succeed())
			// It should be updates
			cm, err := getViewConfigMap(viewName)
			Expect(err).Should(Succeed())
			Expect(cm.Data["template"]).Should(Equal(newCueStr))
		})
	})

	Describe("failed creation", func() {
		It("from local file", func() {
			filename := "failed-create-non-existent.cue"
			err := StoreViewFromFile(context.TODO(), k8sClient, filename, "1234")
			Expect(err).ShouldNot(Succeed())
		})

		It("invalid cue", func() {
			cueStr := "status: what-is-this"
			filename := "failed-create-invalid.cue"
			err := os.WriteFile(filename, []byte(cueStr), 0600)
			Expect(err).Should(Succeed())
			defer os.RemoveAll(filename)
			err = StoreViewFromFile(context.TODO(), k8sClient, filename, "5678")
			Expect(err).ShouldNot(Succeed())
		})
	})
})
