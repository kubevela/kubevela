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

package e2e_multicluster_test

import (
	"context"
	"slices"
	"strings"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	kerrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/yaml"

	"github.com/oam-dev/kubevela/apis/core.oam.dev/common"
	"github.com/oam-dev/kubevela/apis/core.oam.dev/v1beta1"
	"github.com/oam-dev/kubevela/pkg/oam"
)

// Component reads across clusters: a named cluster, the producer beside the
// reader, and every placement of a topology policy, read through the cluster
// gateway rather than within one API server.
var _ = Describe("Component reads across clusters", func() {
	var namespace string
	var hubCtx context.Context
	var workerCtx context.Context

	BeforeEach(func() {
		hubCtx, workerCtx, namespace = initializeContextAndNamespace()
	})

	AfterEach(func() {
		cleanUpNamespace(hubCtx, workerCtx, namespace)
	})

	create := func(doc string) {
		GinkgoHelper()
		app := &v1beta1.Application{}
		Expect(yaml.Unmarshal([]byte(strings.NewReplacer("NAMESPACE", namespace, "WORKER", WorkerClusterName).Replace(doc)), app)).To(Succeed())
		app.SetNamespace(namespace)
		annotations := app.GetAnnotations()
		if annotations == nil {
			annotations = map[string]string{}
		}
		annotations[oam.AnnotationCelExpressions] = "true"
		app.SetAnnotations(annotations)
		Eventually(func(g Gomega) {
			g.Expect(k8sClient.Create(hubCtx, app)).Should(Succeed())
		}).WithTimeout(10 * time.Second).WithPolling(2 * time.Second).Should(Succeed())
	}

	data := func(ctx context.Context, name string) func() (map[string]string, error) {
		return func() (map[string]string, error) {
			cm := &corev1.ConfigMap{}
			if err := k8sClient.Get(ctx, types.NamespacedName{Namespace: namespace, Name: name}, cm); err != nil {
				return nil, err
			}
			return cm.Data, nil
		}
	}

	It("reads a named cluster, and in each cluster the producer beside the reader", func() {
		create(`
metadata: {name: reads-clusters}
spec:
  components:
    - name: db
      type: k8s-objects
      properties:
        objects:
          - {apiVersion: v1, kind: ConfigMap, metadata: {name: mc-db}, data: {host: 'db.$(context.cluster)'}}
    - name: reader
      type: k8s-objects
      properties:
        objects:
          - apiVersion: v1
            kind: ConfigMap
            metadata: {name: mc-reader}
            data:
              remote: '$(component.db.cluster("WORKER").output.data.host)'
              beside: '$(component.db.output.data.host)'
  policies:
    - {name: both, type: topology, properties: {clusters: ["local", "WORKER"], namespace: NAMESPACE}}
  workflow:
    steps:
      - {name: deploy-both, type: deploy, properties: {policies: ["both"]}}
`)
		Eventually(data(hubCtx, "mc-reader")).WithTimeout(3 * time.Minute).WithPolling(3 * time.Second).Should(Equal(map[string]string{
			"remote": "db." + WorkerClusterName,
			"beside": "db.local",
		}))
		Eventually(data(workerCtx, "mc-reader")).WithTimeout(3*time.Minute).WithPolling(3*time.Second).Should(Equal(map[string]string{
			"remote": "db." + WorkerClusterName,
			"beside": "db." + WorkerClusterName,
		}), "the worker's reader reads the worker's db beside it")
		app := &v1beta1.Application{}
		Expect(k8sClient.Get(hubCtx, types.NamespacedName{Namespace: namespace, Name: "reads-clusters"}, app)).Should(Succeed())
		Expect(app.Status.Dependencies).To(Equal([]common.ComponentDependency{
			{Component: "reader", DependsOn: "db", Source: common.DependencySourceExpression},
			{Component: "reader", DependsOn: "db", Source: common.DependencySourceExpression, Cluster: WorkerClusterName},
		}))
	})

	// A DAG runs both steps at once; the edge step still waits for the hub step,
	// since its reader reads a component only the hub step deploys.
	gatedConfig := func() {
		GinkgoHelper()
		def := &v1beta1.ComponentDefinition{
			ObjectMeta: metav1.ObjectMeta{Name: "gated-config", Namespace: namespace},
			Spec: v1beta1.ComponentDefinitionSpec{
				Workload: common.WorkloadTypeDescriptor{Definition: common.WorkloadGVK{APIVersion: "v1", Kind: "ConfigMap"}},
				Schematic: &common.Schematic{CUE: &common.CUE{Template: `
parameter: {name: string, ready: *"false" | string}
output: {
	apiVersion: "v1"
	kind:       "ConfigMap"
	metadata: name: parameter.name
	data: {host: "db.hub", ready: parameter.ready}
}
`}},
				Status: &common.Status{HealthPolicy: `isHealth: context.output.data.ready == "true"`},
			},
		}
		Expect(k8sClient.Create(hubCtx, def)).Should(Succeed())
		Eventually(func(g Gomega) {
			latest := &v1beta1.ComponentDefinition{}
			g.Expect(k8sClient.Get(hubCtx, types.NamespacedName{Namespace: namespace, Name: "gated-config"}, latest)).Should(Succeed())
			g.Expect(latest.Status.LatestRevision).ShouldNot(BeNil())
		}).WithTimeout(time.Minute).WithPolling(time.Second).Should(Succeed())
	}

	application := func(name string) func(g Gomega) *v1beta1.Application {
		return func(g Gomega) *v1beta1.Application {
			app := &v1beta1.Application{}
			g.Expect(k8sClient.Get(hubCtx, types.NamespacedName{Namespace: namespace, Name: name}, app)).Should(Succeed())
			return app
		}
	}

	setReady := func(name, producer string) {
		GinkgoHelper()
		Eventually(func(g Gomega) {
			app := application(name)(g)
			i := slices.IndexFunc(app.Spec.Components, func(c common.ApplicationComponent) bool { return c.Name == "db" })
			g.Expect(i).ShouldNot(Equal(-1), "the application has a component db")
			app.Spec.Components[i].Properties.Raw = []byte(`{"name":"` + producer + `","ready":"true"}`)
			g.Expect(k8sClient.Update(hubCtx, app)).Should(Succeed())
		}).WithTimeout(30 * time.Second).WithPolling(time.Second).Should(Succeed())
	}

	// A read that names a cluster implies no dependsOn: in a DAG both steps run at
	// once, and the edge step retries its reader until the hub's producer is ready.
	It("retries a read of a named cluster until it can be answered, when nothing orders it", func() {
		gatedConfig()
		create(`
metadata: {name: reads-dag}
spec:
  components:
    - {name: db, type: gated-config, properties: {name: dag-db}}
    - name: reader
      type: k8s-objects
      properties:
        objects:
          - {apiVersion: v1, kind: ConfigMap, metadata: {name: dag-reader}, data: {host: '$(component.db.cluster("local").output.data.host)'}}
  policies:
    - {name: hub, type: topology, properties: {clusters: ["local"], namespace: NAMESPACE}}
    - {name: edge, type: topology, properties: {clusters: ["WORKER"], namespace: NAMESPACE}}
    - {name: only-db, type: override, properties: {selector: ["db"]}}
    - {name: only-reader, type: override, properties: {selector: ["reader"]}}
  workflow:
    mode: {steps: DAG}
    steps:
      - {name: deploy-edge, type: deploy, properties: {policies: ["edge", "only-reader"]}}
      - {name: deploy-hub, type: deploy, properties: {policies: ["hub", "only-db"]}}
`)
		stepMessage := func(g Gomega) string {
			app := &v1beta1.Application{}
			g.Expect(k8sClient.Get(hubCtx, types.NamespacedName{Namespace: namespace, Name: "reads-dag"}, app)).Should(Succeed())
			g.Expect(app.Status.Workflow).ShouldNot(BeNil())
			for _, s := range app.Status.Workflow.Steps {
				if s.Name == "deploy-edge" {
					return s.Message
				}
			}
			return ""
		}
		Eventually(stepMessage).WithTimeout(time.Minute).WithPolling(2*time.Second).Should(ContainSubstring(`is waiting for component "db"`),
			"the step says what its reader waits on")
		Expect(application("reads-dag")(Default).Status.Dependencies).To(Equal([]common.ComponentDependency{
			{Component: "reader", DependsOn: "db", Source: common.DependencySourceExpression, Cluster: "local"}}))
		Consistently(func() bool {
			_, err := data(workerCtx, "dag-reader")()
			return kerrors.IsNotFound(err)
		}).WithTimeout(15*time.Second).WithPolling(3*time.Second).Should(BeTrue(), "the reader is not applied until the hub's db is ready")

		By("the hub's producer becomes healthy")
		setReady("reads-dag", "dag-db")
		Eventually(data(workerCtx, "dag-reader")).WithTimeout(3 * time.Minute).WithPolling(3 * time.Second).Should(Equal(map[string]string{
			"host": "db.hub",
		}))
		Eventually(func(g Gomega) common.ApplicationPhase {
			return application("reads-dag")(g).Status.Phase
		}).WithTimeout(3*time.Minute).WithPolling(3*time.Second).Should(Equal(common.ApplicationRunning), "and the workflow finishes")
	})

	// One deploy step places both components in both clusters; the worker's
	// reader reads the hub's producer, which no dependsOn orders, so the step
	// retries it until the hub's producer is ready.
	It("retries a reader inside one deploy step until the producer at the cluster it names is ready", func() {
		gatedConfig()
		create(`
metadata: {name: reads-placed}
spec:
  components:
    - {name: db, type: gated-config, properties: {name: placed-db}}
    - name: reader
      type: k8s-objects
      properties:
        objects:
          - {apiVersion: v1, kind: ConfigMap, metadata: {name: placed-reader}, data: {host: '$(component.db.cluster("local").output.data.host)'}}
  policies:
    - {name: both, type: topology, properties: {clusters: ["local", "WORKER"], namespace: NAMESPACE}}
  workflow:
    steps:
      - {name: deploy-both, type: deploy, properties: {policies: ["both"]}}
`)
		Eventually(func(g Gomega) []common.ComponentDependency {
			return application("reads-placed")(g).Status.Dependencies
		}).WithTimeout(time.Minute).WithPolling(2 * time.Second).Should(Equal([]common.ComponentDependency{
			{Component: "reader", DependsOn: "db", Source: common.DependencySourceExpression, Cluster: "local"}}))
		Consistently(func() bool {
			_, err := data(workerCtx, "placed-reader")()
			return kerrors.IsNotFound(err)
		}).WithTimeout(15*time.Second).WithPolling(3*time.Second).Should(BeTrue(), "the worker's reader waits on the hub's db")

		By("the producer becomes healthy")
		setReady("reads-placed", "placed-db")
		Eventually(data(workerCtx, "placed-reader")).WithTimeout(3 * time.Minute).WithPolling(3 * time.Second).Should(Equal(map[string]string{
			"host": "db.hub",
		}))
	})

	// A step that names no cluster records the local cluster blank, while a read
	// names it "local"; the producer must still be found.
	It("reads from the worker a producer applied by a step that names no cluster", func() {
		create(`
metadata: {name: reads-local-spelling}
spec:
  components:
    - name: db
      type: k8s-objects
      properties:
        objects:
          - {apiVersion: v1, kind: ConfigMap, metadata: {name: spelling-db}, data: {host: db.hub}}
    - name: reader
      type: k8s-objects
      properties:
        objects:
          - {apiVersion: v1, kind: ConfigMap, metadata: {name: spelling-reader}, data: {host: '$(component.db.cluster("local").output.data.host)'}}
  workflow:
    steps:
      - {name: db, type: apply-component, properties: {component: db, namespace: NAMESPACE}}
      - {name: reader, type: apply-component, properties: {component: reader, cluster: WORKER, namespace: NAMESPACE}}
`)
		Eventually(data(workerCtx, "spelling-reader")).WithTimeout(3 * time.Minute).WithPolling(3 * time.Second).Should(Equal(map[string]string{
			"host": "db.hub",
		}))
	})
})
