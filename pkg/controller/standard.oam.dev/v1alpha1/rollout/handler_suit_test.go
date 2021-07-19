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

package rollout

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/crossplane/crossplane-runtime/pkg/event"

	"github.com/oam-dev/kubevela/apis/standard.oam.dev/v1alpha1"

	"github.com/ghodss/yaml"
	appsv1 "k8s.io/api/apps/v1"

	"github.com/crossplane/crossplane-runtime/pkg/fieldpath"

	"github.com/oam-dev/kubevela/pkg/oam"

	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"

	"github.com/oam-dev/kubevela/pkg/oam/util"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

var _ = Describe("Test rollout related handler func", func() {
	namespace := "rollout-test-namespace"
	ctx := context.Background()

	BeforeEach(func() {
		Expect(k8sClient.Create(ctx, &v1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: namespace}})).Should(SatisfyAny(BeNil(), util.AlreadyExistMatcher{}))
	})

	It("Test assemble workload info", func() {
		tarWorkload := &unstructured.Unstructured{}
		tarWorkload.SetAPIVersion("apps/v1")
		tarWorkload.SetKind("Deployment")

		srcWorkload := &unstructured.Unstructured{}
		srcWorkload.SetAPIVersion("apps/v1")
		srcWorkload.SetKind("Deployment")
		compName := "comp-test"
		h := handler{
			reconciler: &reconciler{
				Client: k8sClient,
			},
			rollout: &v1alpha1.Rollout{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: namespace,
				}},
			targetWorkload: tarWorkload,
			sourceWorkload: srcWorkload,
			targetRevName:  "comp-test-v2",
			sourceRevName:  "comp-test-v1",
			compName:       compName,
		}
		util.AddLabels(h.targetWorkload, map[string]string{oam.LabelAppComponent: compName})
		util.AddLabels(h.sourceWorkload, map[string]string{oam.LabelAppComponent: compName})
		h.setWorkloadBaseInfo()
		Expect(h.targetWorkload.GetName()).Should(BeEquivalentTo(compName))
		Expect(h.sourceWorkload.GetName()).Should(BeEquivalentTo(compName))
		Expect(h.targetWorkload.GetNamespace()).Should(BeEquivalentTo(namespace))
		Expect(h.sourceWorkload.GetNamespace()).Should(BeEquivalentTo(namespace))
		tarLabel := h.targetWorkload.GetLabels()
		Expect(len(tarLabel)).Should(BeEquivalentTo(2))
		Expect(tarLabel[oam.LabelAppComponentRevision]).Should(BeEquivalentTo("comp-test-v2"))
		srcLabel := h.sourceWorkload.GetLabels()
		Expect(len(srcLabel)).Should(BeEquivalentTo(2))
		Expect(srcLabel[oam.LabelAppComponentRevision]).Should(BeEquivalentTo("comp-test-v1"))

		Expect(h.assembleWorkload(ctx)).Should(BeNil())
		Expect(h.targetWorkload.GetName()).Should(BeEquivalentTo("comp-test-v2"))
		Expect(h.sourceWorkload.GetName()).Should(BeEquivalentTo("comp-test-v1"))
		pv := fieldpath.Pave(h.targetWorkload.UnstructuredContent())
		Expect(pv.GetBool("spec.paused")).Should(BeEquivalentTo(true))
		replicas, err := pv.GetInteger("spec.replicas")
		Expect(err).Should(BeNil())
		Expect(replicas).Should(BeEquivalentTo(0))
	})

	It("Test prepare workload from revision", func() {
		compName := "metrics-provider"
		ctlV1 := new(appsv1.ControllerRevision)
		ctlV1Json, err := yaml.YAMLToJSON([]byte(compRevisionV1))
		Expect(err).Should(BeNil())
		Expect(json.Unmarshal(ctlV1Json, ctlV1)).Should(BeNil())
		Expect(k8sClient.Create(ctx, ctlV1)).Should(BeNil())

		ctlV2 := new(appsv1.ControllerRevision)
		ctlV2Json, err := yaml.YAMLToJSON([]byte(compRevisionV2))
		Expect(err).Should(BeNil())
		Expect(json.Unmarshal(ctlV2Json, ctlV2)).Should(BeNil())
		Expect(k8sClient.Create(ctx, ctlV2)).Should(BeNil())

		h := handler{
			reconciler: &reconciler{
				Client: k8sClient,
			},
			rollout: &v1alpha1.Rollout{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: namespace,
				}},
			targetRevName: "metrics-provider-v2",
			sourceRevName: "metrics-provider-v1",
			compName:      compName,
		}

		Eventually(func() error {
			wd, err := h.extractWorkload(ctx, namespace, h.targetRevName)
			if err != nil {
				return err
			}
			if wd == nil || wd.GetKind() != "Deployment" {
				return fmt.Errorf("extract error")
			}
			return nil
		}, 15*time.Second, 300*time.Millisecond).Should(BeNil())
		Eventually(func() error {
			wd, err := h.extractWorkload(ctx, namespace, h.sourceRevName)
			if err != nil {
				return err
			}
			if wd == nil || wd.GetKind() != "Deployment" {
				return fmt.Errorf("extract error")
			}
			return nil
		}, 15*time.Second, 300*time.Millisecond).Should(BeNil())
	})

	Describe("Test Handle rollout modified", func() {
		It("succeed rollout", func() {
			h := handler{
				reconciler: &reconciler{
					Client: k8sClient,
					record: event.NewNopRecorder(),
				},
				rollout: &v1alpha1.Rollout{
					ObjectMeta: metav1.ObjectMeta{
						Namespace: namespace,
					},
					Spec: v1alpha1.RolloutSpec{
						TargetRevisionName: "metrics-provider-v2",
						SourceRevisionName: "metrics-provider-v1",
					},
					Status: v1alpha1.CompRolloutStatus{
						RolloutStatus: v1alpha1.RolloutStatus{
							RollingState: v1alpha1.RolloutSucceedState,
						},
					},
				},
			}
			h.handleRolloutModified()
			Expect(h.targetRevName).Should(BeEquivalentTo("metrics-provider-v2"))
			Expect(h.sourceRevName).Should(BeEquivalentTo("metrics-provider-v1"))
		})

		It("middle state rollout", func() {
			h := handler{
				reconciler: &reconciler{
					Client: k8sClient,
					record: event.NewNopRecorder(),
				},
				rollout: &v1alpha1.Rollout{
					ObjectMeta: metav1.ObjectMeta{
						Namespace: namespace,
					},
					Spec: v1alpha1.RolloutSpec{
						TargetRevisionName: "metrics-provider-v3",
						SourceRevisionName: "metrics-provider-v2",
					},
					Status: v1alpha1.CompRolloutStatus{
						RolloutStatus: v1alpha1.RolloutStatus{
							RollingState: v1alpha1.RollingInBatchesState,
						},
						LastUpgradedTargetRevision: "metrics-provider-v2",
						LastSourceRevision:         "metrics-provider-v1",
					},
				},
			}
			h.handleRolloutModified()
			Expect(h.targetRevName).Should(BeEquivalentTo("metrics-provider-v2"))
			Expect(h.sourceRevName).Should(BeEquivalentTo("metrics-provider-v1"))
		})
	})
})

const (
	compRevisionV1 = `
apiVersion: apps/v1
kind: ControllerRevision
metadata:
  labels:
    app.oam.dev/component-revision-hash: ec7fede55af903d5
    controller.oam.dev/component: metrics-provider
  name: metrics-provider-v1
  namespace: rollout-test-namespace
data:
  apiVersion: core.oam.dev/v1alpha2
  kind: Component
  metadata: 
    name: metrics-provider
  spec:
    workload:
      apiVersion: apps/v1
      kind: Deployment
      metadata:
        labels:
          app.oam.dev/component: metrics-provider
          app.oam.dev/name: test-rolling
          workload.oam.dev/type: webservice
      spec:
        selector:
          matchLabels:
            app.oam.dev/component: metrics-provider
        template:
          metadata:
            labels:
              app.oam.dev/component: metrics-provider
          spec:
            containers:
            - command:
              - ./podinfo
              - stress-cpu=1
              image: stefanprodan/podinfo:4.0.6
              name: metrics-provider
              ports:
              - containerPort: 8080
`
	compRevisionV2 = `
apiVersion: apps/v1
kind: ControllerRevision
metadata:
  labels:
    app.oam.dev/component-revision-hash: acdd0c76bd3c8f07
    controller.oam.dev/component: metrics-provider
  name: metrics-provider-v2
  namespace: rollout-test-namespace
data:
  apiVersion: core.oam.dev/v1alpha2
  kind: Component
  metadata:
    name: metrics-provider
  spec:
    workload:
      apiVersion: apps/v1
      kind: Deployment
      metadata:
        labels:
          app.oam.dev/component: metrics-provider
          app.oam.dev/name: test-rolling
          workload.oam.dev/type: webservice
      spec:
        selector:
          matchLabels:
            app.oam.dev/component: metrics-provider
        template:
          metadata:
            labels:
              app.oam.dev/component: metrics-provider
          spec:
            containers:
            - command:
              - ./podinfo
              - stress-cpu=1
              image: stefanprodan/podinfo:5.0.2
              name: metrics-provider
              ports:
              - containerPort: 8080
`
)
