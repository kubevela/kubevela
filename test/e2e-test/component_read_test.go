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

package controllers_test

import (
	"context"
	"fmt"
	"strings"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	kerrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/yaml"

	oamcomm "github.com/oam-dev/kubevela/apis/core.oam.dev/common"
	"github.com/oam-dev/kubevela/apis/core.oam.dev/v1beta1"
)

// These specs cover reading one component's output from another: the value
// arriving in text and typed positions, a trait reading it, waiting for a
// producer, chains, placement by namespace, and what admission refuses.
//
// Every producer and reader is a ConfigMap, so a spec asserts on rendered data
// without waiting for a workload to become ready. A gated-config producer is
// healthy only once its data says ready, so a spec can hold one unhealthy.
var _ = Describe("Component reads", func() {
	ctx := context.Background()

	var namespaceName string
	var ns corev1.Namespace
	var extraNamespaces []corev1.Namespace

	// A plain patch trait: its parameters land in labels, so a trait read is
	// observable on the ConfigMap it patches.
	const labelTrait = `
parameter: {db: string}
patch: metadata: labels: {"read-db": parameter.db}
`

	// A source for the spec that mixes all three roots.
	const regionSource = `
schema: {region: string}
$internal: {key: "component-read-region", keyInputs: []}
output: {region: "eu-west"}
parameter: {}
`

	// A ConfigMap healthy only once its ready parameter is "true".
	const gatedConfig = `
parameter: {name: string, ready: *"false" | string}
output: {
	apiVersion: "v1"
	kind:       "ConfigMap"
	metadata: name: parameter.name
	data: {host: "db.internal", ready: parameter.ready}
}
`

	appFromYAML := func(doc string) *v1beta1.Application {
		GinkgoHelper()
		app := &v1beta1.Application{}
		Expect(yaml.Unmarshal([]byte(strings.ReplaceAll(doc, "NAMESPACE", namespaceName)), app)).To(Succeed())
		app.Namespace = namespaceName
		return optIn(app)
	}

	createApp := func(app *v1beta1.Application) {
		GinkgoHelper()
		Eventually(func() error {
			err := k8sClient.Create(ctx, app)
			if err != nil && !strings.Contains(err.Error(), "not found") {
				StopTrying("Application refused").Wrap(err).Now()
			}
			return err
		}, 30*time.Second, time.Second).Should(Succeed())
	}

	configMapData := func(namespace, name string) (map[string]string, error) {
		cm := &corev1.ConfigMap{}
		if err := k8sClient.Get(ctx, client.ObjectKey{Namespace: namespace, Name: name}, cm); err != nil {
			return nil, err
		}
		return cm.Data, nil
	}

	configMapLabels := func(namespace, name string) (map[string]string, error) {
		cm := &corev1.ConfigMap{}
		if err := k8sClient.Get(ctx, client.ObjectKey{Namespace: namespace, Name: name}, cm); err != nil {
			return nil, err
		}
		return cm.Labels, nil
	}

	serviceMessage := func(app, component string) string {
		a := &v1beta1.Application{}
		if err := k8sClient.Get(ctx, client.ObjectKey{Namespace: namespaceName, Name: app}, a); err != nil {
			return err.Error()
		}
		for _, svc := range a.Status.Services {
			if svc.Name == component {
				return svc.Message
			}
		}
		return ""
	}

	stepMessage := func(app, step string) string {
		a := &v1beta1.Application{}
		if err := k8sClient.Get(ctx, client.ObjectKey{Namespace: namespaceName, Name: app}, a); err != nil {
			return err.Error()
		}
		if a.Status.Workflow == nil {
			return ""
		}
		for _, s := range a.Status.Workflow.Steps {
			if s.Name == step {
				return s.Message
			}
		}
		return ""
	}

	dependencies := func(app string) []oamcomm.ComponentDependency {
		a := &v1beta1.Application{}
		if err := k8sClient.Get(ctx, client.ObjectKey{Namespace: namespaceName, Name: app}, a); err != nil {
			return nil
		}
		return a.Status.Dependencies
	}

	setReady := func(app string) {
		GinkgoHelper()
		Eventually(func() error {
			current := &v1beta1.Application{}
			if err := k8sClient.Get(ctx, client.ObjectKey{Namespace: namespaceName, Name: app}, current); err != nil {
				return err
			}
			for i := range current.Spec.Components {
				if current.Spec.Components[i].Name == "db" {
					current.Spec.Components[i].Properties.Raw = []byte(`{"name":"gated-db","ready":"true"}`)
					return k8sClient.Update(ctx, current)
				}
			}
			return fmt.Errorf("application %s has no component db", app)
		}, 30*time.Second, time.Second).Should(Succeed())
	}

	extraNamespace := func(prefix string) string {
		name := randomNamespaceName(prefix)
		extraNamespaces = append(extraNamespaces, createNamespace(ctx, name))
		return name
	}

	BeforeEach(func() {
		namespaceName = randomNamespaceName("component-read-e2e")
		ns = createNamespace(ctx, namespaceName)
		extraNamespaces = nil
		applyDefinition(ctx, exprTraitDefinition(namespaceName, "read-label", labelTrait))
		applyDefinition(ctx, &v1beta1.ComponentDefinition{
			ObjectMeta: metav1.ObjectMeta{Name: "gated-config", Namespace: namespaceName},
			Spec: v1beta1.ComponentDefinitionSpec{
				Workload:  oamcomm.WorkloadTypeDescriptor{Definition: oamcomm.WorkloadGVK{APIVersion: "v1", Kind: "ConfigMap"}},
				Schematic: &oamcomm.Schematic{CUE: &oamcomm.CUE{Template: gatedConfig}},
				Status:    &oamcomm.Status{HealthPolicy: `isHealth: context.output.data.ready == "true"`},
			},
		})
		applyDefinition(ctx, &v1beta1.SourceDefinition{
			ObjectMeta: metav1.ObjectMeta{Name: "component-read-region", Namespace: namespaceName},
			Spec: v1beta1.SourceDefinitionSpec{
				Schematic: &oamcomm.Schematic{CUE: &oamcomm.CUE{Template: regionSource}},
			},
		})
	})

	AfterEach(func() {
		By("Clean up the component-read namespaces")
		for _, n := range append([]corev1.Namespace{ns}, extraNamespaces...) {
			n := n
			Expect(k8sClient.Delete(ctx, &n, client.PropagationPolicy(metav1.DeletePropagationBackground))).Should(BeNil())
		}
		nsLabel := client.MatchingLabels{"sourcedefinition.oam.dev/namespace": namespaceName}
		Eventually(func() error {
			return k8sClient.DeleteAllOf(ctx, &corev1.ConfigMap{}, client.InNamespace("vela-system"), nsLabel)
		}, 30*time.Second, time.Second).Should(Succeed())
	})

	It("delivers a producer's output into text, typed values and a trait", func() {
		createApp(appFromYAML(`
metadata: {name: reads-basic}
spec:
  components:
    - name: db
      type: k8s-objects
      properties:
        objects:
          - {apiVersion: v1, kind: ConfigMap, metadata: {name: db-info}, data: {host: db.internal, port: "5432"}}
    - name: api
      type: k8s-objects
      properties:
        objects:
          - apiVersion: v1
            kind: ConfigMap
            metadata: {name: api-config}
            data:
              url: 'postgres://$(component.db.output.data.host):$(component.db.output.data.port)/shop'
              next: '$(string(int(component.db.output.data.port) + 1))'
              labelKey: '$(component.db.output.metadata.name)'
      traits:
        - type: read-label
          properties: {db: '$(component.db.output.metadata.name)'}
`))
		verifyApplicationPhase(ctx, namespaceName, "reads-basic", oamcomm.ApplicationRunning)
		Eventually(func() (map[string]string, error) {
			return configMapData(namespaceName, "api-config")
		}, 60*time.Second, 2*time.Second).Should(Equal(map[string]string{
			"url":      "postgres://db.internal:5432/shop",
			"next":     "5433",
			"labelKey": "db-info",
		}))
		Eventually(func() (map[string]string, error) {
			return configMapLabels(namespaceName, "api-config")
		}, 60*time.Second, 2*time.Second).Should(HaveKeyWithValue("read-db", "db-info"))
	})

	It("falls back under has(), and waits on an unguarded field until the producer has it", func() {
		app := appFromYAML(`
metadata: {name: reads-wait}
spec:
  components:
    - name: db
      type: k8s-objects
      properties:
        objects:
          - {apiVersion: v1, kind: ConfigMap, metadata: {name: wait-db}, data: {host: db.internal}}
    - name: guarded
      type: k8s-objects
      properties:
        objects:
          - apiVersion: v1
            kind: ConfigMap
            metadata: {name: wait-guarded}
            data: {tls: '$(has(component.db.output.data.tls) ? component.db.output.data.tls : "off")'}
    - name: waiting
      type: k8s-objects
      properties:
        objects:
          - {apiVersion: v1, kind: ConfigMap, metadata: {name: wait-unguarded}, data: {tls: '$(component.db.output.data.tls)'}}
`)
		createApp(app)
		Eventually(func() (map[string]string, error) {
			return configMapData(namespaceName, "wait-guarded")
		}, 60*time.Second, 2*time.Second).Should(Equal(map[string]string{"tls": "off"}))

		By("the unguarded reader is not applied, and says what it waits on")
		Consistently(func() bool {
			_, err := configMapData(namespaceName, "wait-unguarded")
			return kerrors.IsNotFound(err)
		}, 20*time.Second, 2*time.Second).Should(BeTrue(), "the reader must be absent, not merely unreadable")
		Eventually(func() string { return serviceMessage("reads-wait", "waiting") },
			60*time.Second, 2*time.Second).Should(ContainSubstring("component.db.output.data.tls"))
		Eventually(func() string { return stepMessage("reads-wait", "waiting") },
			60*time.Second, 2*time.Second).Should(And(ContainSubstring("waiting for"), ContainSubstring("component.db.output.data.tls")),
			"a wait no dependsOn can express is named in the step too")

		By("the producer gains the field")
		Eventually(func() error {
			current := &v1beta1.Application{}
			if err := k8sClient.Get(ctx, client.ObjectKey{Namespace: namespaceName, Name: "reads-wait"}, current); err != nil {
				return err
			}
			current.Spec.Components[0].Properties.Raw = []byte(
				`{"objects":[{"apiVersion":"v1","kind":"ConfigMap","metadata":{"name":"wait-db"},"data":{"host":"db.internal","tls":"on"}}]}`)
			return k8sClient.Update(ctx, current)
		}, 30*time.Second, time.Second).Should(Succeed())
		Eventually(func() (map[string]string, error) {
			return configMapData(namespaceName, "wait-unguarded")
		}, 120*time.Second, 2*time.Second).Should(Equal(map[string]string{"tls": "on"}))
		verifyApplicationPhase(ctx, namespaceName, "reads-wait", oamcomm.ApplicationRunning)
	})

	It("holds a reader's step pending on its producer's until the producer is healthy", func() {
		createApp(appFromYAML(`
metadata: {name: reads-order}
spec:
  components:
    - name: db
      type: gated-config
      properties: {name: gated-db}
    - name: api
      type: k8s-objects
      properties:
        objects:
          - {apiVersion: v1, kind: ConfigMap, metadata: {name: order-api}, data: {host: '$(component.db.output.data.host)'}}
`))
		Eventually(func() string { return stepMessage("reads-order", "api") },
			60*time.Second, 2*time.Second).Should(Equal("Pending on DependsOn: db"))
		Expect(dependencies("reads-order")).To(Equal([]oamcomm.ComponentDependency{
			{Component: "api", DependsOn: "db", Source: oamcomm.DependencySourceExpression}}),
			"the status records the dependency and where it is declared")
		Consistently(func() bool {
			_, err := configMapData(namespaceName, "order-api")
			return kerrors.IsNotFound(err)
		}, 15*time.Second, 3*time.Second).Should(BeTrue(), "the reader is not applied while its producer is unhealthy")

		By("the producer becomes healthy")
		setReady("reads-order")
		Eventually(func() (map[string]string, error) {
			return configMapData(namespaceName, "order-api")
		}, 120*time.Second, 2*time.Second).Should(Equal(map[string]string{"host": "db.internal"}))
		verifyApplicationPhase(ctx, namespaceName, "reads-order", oamcomm.ApplicationRunning)
	})

	It("holds a reader inside a deploy step until its producer is healthy in that placement", func() {
		createApp(appFromYAML(`
metadata: {name: reads-deploy-order}
spec:
  components:
    - name: db
      type: gated-config
      properties: {name: gated-db}
    - name: api
      type: k8s-objects
      properties:
        objects:
          - {apiVersion: v1, kind: ConfigMap, metadata: {name: deploy-order-api}, data: {host: '$(component.db.output.data.host)'}}
  policies:
    - {name: here, type: topology, properties: {clusters: [local], namespace: NAMESPACE}}
  workflow:
    steps:
      - {name: deploy-here, type: deploy, properties: {policies: [here]}}
`))
		Eventually(func() string { return stepMessage("reads-deploy-order", "deploy-here") },
			60*time.Second, 2*time.Second).Should(ContainSubstring("api is waiting dependents"))
		Expect(dependencies("reads-deploy-order")).To(Equal([]oamcomm.ComponentDependency{
			{Component: "api", DependsOn: "db", Source: oamcomm.DependencySourceExpression}}))
		Consistently(func() bool {
			_, err := configMapData(namespaceName, "deploy-order-api")
			return kerrors.IsNotFound(err)
		}, 15*time.Second, 3*time.Second).Should(BeTrue(), "the reader is not applied while its producer is unhealthy")

		By("the producer becomes healthy")
		setReady("reads-deploy-order")
		Eventually(func() (map[string]string, error) {
			return configMapData(namespaceName, "deploy-order-api")
		}, 120*time.Second, 2*time.Second).Should(Equal(map[string]string{"host": "db.internal"}))
		verifyApplicationPhase(ctx, namespaceName, "reads-deploy-order", oamcomm.ApplicationRunning)
	})

	It("deletes a producer only once its reader is gone, as dependsOn orders garbage collection", func() {
		createApp(appFromYAML(`
metadata: {name: reads-gc}
spec:
  components:
    - name: db
      type: k8s-objects
      properties:
        objects:
          - {apiVersion: v1, kind: ConfigMap, metadata: {name: gc-db}, data: {host: db.internal}}
    - name: api
      type: k8s-objects
      properties:
        objects:
          - apiVersion: v1
            kind: ConfigMap
            metadata: {name: gc-api, finalizers: [e2e.oam.dev/hold]}
            data: {host: '$(component.db.output.data.host)'}
  policies:
    - {name: gc, type: garbage-collect, properties: {order: dependency}}
`))
		Eventually(func() (map[string]string, error) {
			return configMapData(namespaceName, "gc-api")
		}, 60*time.Second, 2*time.Second).Should(Equal(map[string]string{"host": "db.internal"}))

		By("deleting the application while the reader is held by a finalizer")
		Expect(k8sClient.Delete(ctx, &v1beta1.Application{ObjectMeta: metav1.ObjectMeta{Name: "reads-gc", Namespace: namespaceName}})).To(Succeed())
		Eventually(func() bool {
			cm := &corev1.ConfigMap{}
			return k8sClient.Get(ctx, client.ObjectKey{Namespace: namespaceName, Name: "gc-api"}, cm) == nil && cm.DeletionTimestamp != nil
		}, 60*time.Second, 2*time.Second).Should(BeTrue(), "the reader is being deleted")
		Consistently(func() error {
			_, err := configMapData(namespaceName, "gc-db")
			return err
		}, 15*time.Second, 3*time.Second).Should(Succeed(), "the producer stays while its reader exists")

		By("releasing the reader")
		Eventually(func() error {
			cm := &corev1.ConfigMap{}
			if err := k8sClient.Get(ctx, client.ObjectKey{Namespace: namespaceName, Name: "gc-api"}, cm); err != nil {
				return err
			}
			cm.Finalizers = nil
			return k8sClient.Update(ctx, cm)
		}, 30*time.Second, time.Second).Should(Succeed())
		Eventually(func() bool {
			_, err := configMapData(namespaceName, "gc-db")
			return kerrors.IsNotFound(err)
		}, 120*time.Second, 2*time.Second).Should(BeTrue(), "then the producer goes")
	})

	It("chains reads, and carries a producer's change down the chain", func() {
		chain := func(value string) *v1beta1.Application {
			return appFromYAML(fmt.Sprintf(`
metadata: {name: reads-chain}
spec:
  components:
    - name: a
      type: k8s-objects
      properties:
        objects:
          - {apiVersion: v1, kind: ConfigMap, metadata: {name: chain-a}, data: {v: %s}}
    - name: b
      type: k8s-objects
      properties:
        objects:
          - {apiVersion: v1, kind: ConfigMap, metadata: {name: chain-b}, data: {v: '$(component.a.output.data.v)-b'}}
    - name: c
      type: k8s-objects
      properties:
        objects:
          - {apiVersion: v1, kind: ConfigMap, metadata: {name: chain-c}, data: {v: '$(component.b.output.data.v)-c'}}
`, value))
		}
		createApp(chain("a1"))
		Eventually(func() (map[string]string, error) {
			return configMapData(namespaceName, "chain-c")
		}, 90*time.Second, 2*time.Second).Should(Equal(map[string]string{"v": "a1-b-c"}))

		Eventually(func() error {
			current := &v1beta1.Application{}
			if err := k8sClient.Get(ctx, client.ObjectKey{Namespace: namespaceName, Name: "reads-chain"}, current); err != nil {
				return err
			}
			current.Spec = chain("a2").Spec
			return k8sClient.Update(ctx, current)
		}, 30*time.Second, time.Second).Should(Succeed())
		Eventually(func() (map[string]string, error) {
			return configMapData(namespaceName, "chain-c")
		}, 120*time.Second, 2*time.Second).Should(Equal(map[string]string{"v": "a2-b-c"}))
	})

	It("reads a producer in another namespace with namespace(), and hints when a plain read cannot pair", func() {
		dataNS := extraNamespace("component-read-data")
		appNS := extraNamespace("component-read-app")
		createApp(appFromYAML(strings.NewReplacer("DATA_NS", dataNS, "APP_NS", appNS).Replace(`
metadata: {name: reads-namespaces}
spec:
  components:
    - name: db
      type: k8s-objects
      properties:
        objects:
          - {apiVersion: v1, kind: ConfigMap, metadata: {name: ns-db}, data: {host: 'db.$(context.namespace)'}}
    - name: named
      type: k8s-objects
      properties:
        objects:
          - {apiVersion: v1, kind: ConfigMap, metadata: {name: ns-named}, data: {host: '$(component.db.namespace("DATA_NS").output.data.host)'}}
    - name: plain
      type: k8s-objects
      properties:
        objects:
          - {apiVersion: v1, kind: ConfigMap, metadata: {name: ns-plain}, data: {host: '$(component.db.output.data.host)'}}
  policies:
    - {name: data, type: topology, properties: {clusters: ["local"], namespace: DATA_NS}}
    - {name: app, type: topology, properties: {clusters: ["local"], namespace: APP_NS}}
    - {name: only-db, type: override, properties: {selector: ["db"]}}
    - {name: only-readers, type: override, properties: {selector: ["named", "plain"]}}
  workflow:
    steps:
      - {name: data, type: deploy, properties: {policies: ["data", "only-db"]}}
      - {name: app, type: deploy, properties: {policies: ["app", "only-readers"]}}
`)))
		Eventually(func() (map[string]string, error) {
			return configMapData(appNS, "ns-named")
		}, 90*time.Second, 2*time.Second).Should(Equal(map[string]string{"host": "db." + dataNS}))

		Eventually(func() string { return serviceMessage("reads-namespaces", "plain") },
			90*time.Second, 2*time.Second).Should(ContainSubstring(fmt.Sprintf(`component.db.namespace(%q)`, dataNS)))
		_, err := configMapData(appNS, "ns-plain")
		Expect(kerrors.IsNotFound(err)).To(BeTrue(), "a plain read with no producer beside it must not be applied: %v", err)
	})

	It("reads a component alongside a source and context, and still reports the source", func() {
		createApp(appFromYAML(`
metadata: {name: reads-mixed}
spec:
  sources:
    - {name: where, type: component-read-region}
  components:
    - name: store
      type: k8s-objects
      properties:
        objects:
          - {apiVersion: v1, kind: ConfigMap, metadata: {name: mixed-store}, data: {host: store.internal}}
    - name: web
      type: k8s-objects
      properties:
        objects:
          - apiVersion: v1
            kind: ConfigMap
            metadata: {name: mixed-web}
            data: {where: '$(component.store.output.data.host + "." + source.where.region + "." + context.appName)'}
`))
		Eventually(func() (map[string]string, error) {
			return configMapData(namespaceName, "mixed-web")
		}, 90*time.Second, 2*time.Second).Should(Equal(map[string]string{"where": "store.internal.eu-west.reads-mixed"}))

		// Status is rebuilt on every reconcile from a render outside the
		// workflow, which is where a reader's source reads were once lost.
		Consistently(func() string {
			a := &v1beta1.Application{}
			if err := k8sClient.Get(ctx, client.ObjectKey{Namespace: namespaceName, Name: "reads-mixed"}, a); err != nil {
				return err.Error()
			}
			for _, s := range a.Status.Sources {
				if s.Name == "where" {
					return s.Phase
				}
			}
			return "missing"
		}, 45*time.Second, 5*time.Second).Should(Equal("Resolved"))
	})

	DescribeTable("refuses at admission a read that cannot be answered",
		func(doc, want string) {
			err := k8sClient.Create(ctx, appFromYAML(doc))
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring(want))
		},
		Entry("an unknown component", `
metadata: {name: refuse-unknown}
spec:
  components:
    - {name: api, type: k8s-objects, properties: {objects: [{apiVersion: v1, kind: ConfigMap, metadata: {name: x}, data: {v: '$(component.nope.output.data.v)'}}]}}
`, `the application has no component "nope"`),
		Entry("a component reading itself", `
metadata: {name: refuse-self}
spec:
  components:
    - {name: api, type: k8s-objects, properties: {objects: [{apiVersion: v1, kind: ConfigMap, metadata: {name: x}, data: {v: '$(component.api.output.data.v)'}}]}}
`, `reads its own output`),
		Entry("a cycle", `
metadata: {name: refuse-cycle}
spec:
  components:
    - {name: a, type: k8s-objects, properties: {objects: [{apiVersion: v1, kind: ConfigMap, metadata: {name: a}, data: {v: '$(component.b.output.data.v)'}}]}}
    - {name: b, type: k8s-objects, properties: {objects: [{apiVersion: v1, kind: ConfigMap, metadata: {name: b}, data: {v: '$(component.a.output.data.v)'}}]}}
`, `wait on each other in a cycle`),
		Entry("a placement call after the output", `
metadata: {name: refuse-misplaced}
spec:
  components:
    - {name: db, type: k8s-objects, properties: {objects: [{apiVersion: v1, kind: ConfigMap, metadata: {name: db}, data: {v: x}}]}}
    - {name: api, type: k8s-objects, properties: {objects: [{apiVersion: v1, kind: ConfigMap, metadata: {name: x}, data: {v: '$(component.db.output.cluster("local").data.v)'}}]}}
`, `go straight after component.<name>`),
		Entry("every placement of a component", `
metadata: {name: refuse-placements}
spec:
  components:
    - {name: db, type: k8s-objects, properties: {objects: [{apiVersion: v1, kind: ConfigMap, metadata: {name: db}, data: {v: x}}]}}
    - {name: api, type: k8s-objects, properties: {objects: [{apiVersion: v1, kind: ConfigMap, metadata: {name: x}, data: {v: '$(component.db.placements("nope")[0].output.data.v)'}}]}}
`, `undeclared reference to 'placements'`),
		Entry("a cluster that is not registered", `
metadata: {name: refuse-cluster}
spec:
  components:
    - {name: db, type: k8s-objects, properties: {objects: [{apiVersion: v1, kind: ConfigMap, metadata: {name: db}, data: {v: x}}]}}
    - {name: api, type: k8s-objects, properties: {objects: [{apiVersion: v1, kind: ConfigMap, metadata: {name: x}, data: {v: '$(component.db.cluster("no-such-cluster").output.data.v)'}}]}}
`, `no cluster "no-such-cluster" is registered`),
		Entry("a workflow step", `
metadata: {name: refuse-step}
spec:
  components:
    - {name: db, type: k8s-objects, properties: {objects: [{apiVersion: v1, kind: ConfigMap, metadata: {name: db}, data: {v: x}}]}}
  workflow:
    steps:
      - {name: s, type: notification, properties: {slack: {url: {value: '$(component.db.output.data.v)'}, message: {text: x}}}}
`, `"component" cannot be read here`),
	)
})
