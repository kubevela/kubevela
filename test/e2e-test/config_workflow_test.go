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

package controllers_test

import (
	"context"
	"time"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/klog/v2"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/yaml"

	"github.com/oam-dev/kubevela/apis/core.oam.dev/common"
	"github.com/oam-dev/kubevela/apis/core.oam.dev/v1beta1"
	"github.com/oam-dev/kubevela/pkg/oam/util"
)

var _ = Describe("Test application of the specified definition version", func() {
	ctx := context.Background()

	var namespace string
	var ns corev1.Namespace

	BeforeEach(func() {
		namespace = randomNamespaceName("config-e2e-test")
		ns = corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: namespace}}

		Eventually(func() error {
			return k8sClient.Create(ctx, &ns)
		}, time.Second*3, time.Microsecond*300).Should(SatisfyAny(BeNil(), &util.AlreadyExistMatcher{}))
	})

	It("Test the workflow", func() {
		var app v1beta1.Application
		Expect(yaml.Unmarshal([]byte(manageConfigApplication), &app)).Should(BeNil())
		app.Namespace = namespace
		Expect(k8sClient.Create(context.TODO(), &app)).Should(BeNil())
		Eventually(
			func() common.ApplicationPhase {
				var getAPP v1beta1.Application
				if err := k8sClient.Get(ctx, client.ObjectKey{Namespace: namespace, Name: app.Name}, &getAPP); err != nil {
					klog.Errorf("fail to query the app %s", err.Error())
				}
				klog.Infof("the application status is %s (%+v)", getAPP.Status.Phase, getAPP.Status.Workflow)
				return getAPP.Status.Phase
			},
			time.Second*30, time.Second*2).Should(Equal(common.ApplicationRunning))
	})

	AfterEach(func() {
		By("Clean up resources after a test")
		k8sClient.DeleteAllOf(ctx, &v1beta1.Application{}, client.InNamespace(namespace))
	})
})

var manageConfigApplication = `
kind: Application
apiVersion: core.oam.dev/v1beta1
metadata:
  name: test-config
  namespace: "config-e2e-test"
spec:
  components: []
  workflow:
    steps:
    - name: write-config
      type: create-config
      properties:
        name: test
        config: 
          key1: value1
          key2: 2
          key3: true
          key4: 
            key5: value5
    - name: read-config
      type: read-config
      properties:
        name: test
      outputs:
      - fromKey: config
        name: read-config
    - name: delete-config
      type: delete-config
      properties:
        name: test
`
