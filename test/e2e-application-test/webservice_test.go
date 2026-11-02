/*
Copyright 2026 The KubeVela Authors.

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

package application_test

import (
	"context"
	"time"

	"github.com/guidewire-oss/sawchain"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

const (
	sawchainTimeout  = 2 * time.Minute
	sawchainInterval = 2 * time.Second
)

var _ = Describe("Webservice component with scaler trait (Sawchain)", Ordered, Label("core-application"), func() {
	var (
		ctx       context.Context
		sc        *sawchain.Sawchain
		namespace string
	)

	BeforeAll(func() {
		ctx = context.Background()
		namespace = randomNamespaceName("webservice-sawchain")
		Expect(k8sClient.Create(ctx, &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: namespace}})).To(Succeed())
		// Sawchain injects no built-in bindings, so the namespace must be supplied explicitly
		sc = sawchain.New(GinkgoTB(), k8sClient, sawchainTimeout, sawchainInterval, map[string]any{"namespace": namespace})
	})

	AfterAll(func() {
		Expect(k8sClient.Delete(ctx, &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: namespace}})).To(Succeed())
	})

	When("the Application has a webservice component and no traits", func() {
		BeforeAll(func() {
			sc.CreateAndWait(ctx, "testdata/webservice/application.tpl.yaml")
		})

		It("renders the Deployment", func() {
			deployment := &appsv1.Deployment{}
			Eventually(sc.CheckFunc(ctx, deployment, `
				apiVersion: apps/v1
				kind: Deployment
				metadata:
				  name: frontend
				  namespace: ($namespace)
				spec:
				  template:
				    spec:
				      containers:
				      - name: frontend
				        image: oamdev/testapp:v1
				        ports:
				        - containerPort: 8080
				status:
				  readyReplicas: 1
			`), sawchainTimeout, sawchainInterval).Should(Succeed())
			Expect(deployment).To(sc.HaveStatusCondition("Available", "True"))
		})

		It("renders the exposed Service", func() {
			Eventually(sc.CheckFunc(ctx, `
				apiVersion: v1
				kind: Service
				metadata:
				  name: frontend
				  namespace: ($namespace)
				spec:
				  ports:
				  - port: 8080
				    targetPort: 8080
			`), sawchainTimeout, sawchainInterval).Should(Succeed())
		})

		It("reports the Application as running and healthy", func() {
			Eventually(sc.CheckFunc(ctx, `
				apiVersion: core.oam.dev/v1beta1
				kind: Application
				metadata:
				  name: webservice-sawchain
				  namespace: ($namespace)
				status:
				  status: running
				  services:
				  - name: frontend
				    healthy: true
			`), sawchainTimeout, sawchainInterval).Should(Succeed())
		})
	})

	When("the scaler trait is added", func() {
		BeforeAll(func() {
			sc.UpdateAndWait(ctx, `
				apiVersion: core.oam.dev/v1beta1
				kind: Application
				metadata:
				  name: webservice-sawchain
				  namespace: ($namespace)
				spec:
				  components:
				  - name: frontend
				    type: webservice
				    properties:
				      image: oamdev/testapp:v1
				      ports:
				      - port: 8080
				        expose: true
				    traits:
				    - type: scaler
				      properties:
				        replicas: 2
			`)
		})

		It("scales the Deployment to 2 replicas", func() {
			Eventually(sc.CheckFunc(ctx, `
				apiVersion: apps/v1
				kind: Deployment
				metadata:
				  name: frontend
				  namespace: ($namespace)
				spec:
				  replicas: 2
				status:
				  readyReplicas: 2
			`), sawchainTimeout, sawchainInterval).Should(Succeed())
		})

		It("keeps the Application running and healthy", func() {
			Eventually(sc.CheckFunc(ctx, `
				apiVersion: core.oam.dev/v1beta1
				kind: Application
				metadata:
				  name: webservice-sawchain
				  namespace: ($namespace)
				status:
				  status: running
				  services:
				  - name: frontend
				    healthy: true
			`), sawchainTimeout, sawchainInterval).Should(Succeed())
		})
	})

	When("the scaler trait is removed", func() {
		BeforeAll(func() {
			// Merge patch: null deletes the traits field
			sc.UpdateAndWait(ctx, `
				apiVersion: core.oam.dev/v1beta1
				kind: Application
				metadata:
				  name: webservice-sawchain
				  namespace: ($namespace)
				spec:
				  components:
				  - name: frontend
				    type: webservice
				    properties:
				      image: oamdev/testapp:v1
				      ports:
				      - port: 8080
				        expose: true
				    traits: null
			`)
		})

		It("returns the Deployment to 1 replica and keeps the Application running", func() {
			Eventually(sc.CheckFunc(ctx, `
				apiVersion: apps/v1
				kind: Deployment
				metadata:
				  name: frontend
				  namespace: ($namespace)
				spec:
				  replicas: 1
				status:
				  readyReplicas: 1
			`), sawchainTimeout, sawchainInterval).Should(Succeed())
			Eventually(sc.CheckFunc(ctx, `
				apiVersion: core.oam.dev/v1beta1
				kind: Application
				metadata:
				  name: webservice-sawchain
				  namespace: ($namespace)
				status:
				  status: running
				  services:
				  - name: frontend
				    healthy: true
			`), sawchainTimeout, sawchainInterval).Should(Succeed())
		})
	})
})
