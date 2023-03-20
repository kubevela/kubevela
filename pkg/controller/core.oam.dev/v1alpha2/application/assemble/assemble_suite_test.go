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

package assemble

import (
	"os"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"sigs.k8s.io/yaml"

	"github.com/oam-dev/kubevela/apis/core.oam.dev/v1beta1"
	velatypes "github.com/oam-dev/kubevela/apis/types"
	"github.com/oam-dev/kubevela/pkg/oam"
)

var _ = Describe("Test Assemble Options", func() {
	It("test assemble", func() {
		var (
			compName  = "test-comp"
			namespace = "default"
		)

		appRev := &v1beta1.ApplicationRevision{}
		b, err := os.ReadFile("./testdata/apprevision.yaml")
		/* appRevision test data is generated based on below application
		apiVersion: core.oam.dev/v1beta1
		kind: Application
		metadata:
		  name: test-assemble
		spec:
		  components:
		    - name: test-comp
		      type: webservice
		      properties:
		        image: crccheck/hello-world
		        port: 8000
		      traits:
		        - type: ingress
		          properties:
		            domain: localhost
		            http:
		              "/": 8000
		*/
		Expect(err).Should(BeNil())
		err = yaml.Unmarshal(b, appRev)
		Expect(err).Should(BeNil())

		ao := NewAppManifests(appRev, appParser)
		workloads, traits, _, err := ao.GroupAssembledManifests()
		Expect(err).Should(BeNil())

		By("Verify amount of result resources")
		allResources, err := ao.AssembledManifests()
		Expect(err).Should(BeNil())
		Expect(len(allResources)).Should(Equal(3))

		By("Verify amount of result grouped resources")
		Expect(len(workloads)).Should(Equal(1))
		Expect(len(traits[compName])).Should(Equal(2))

		By("Verify workload metadata (name, namespace, labels, annotations, ownerRef)")
		wl := workloads[compName]
		Expect(wl.GetName()).Should(Equal(compName))
		Expect(wl.GetNamespace()).Should(Equal(namespace))
		labels := wl.GetLabels()
		labelKeys := make([]string, 0, len(labels))
		for k := range labels {
			labelKeys = append(labelKeys, k)
		}
		Expect(labelKeys).Should(ContainElements(
			oam.LabelAppName,
			oam.LabelAppRevision,
			oam.LabelAppRevisionHash,
			oam.LabelAppComponent,
			oam.LabelAppComponentRevision,
			oam.WorkloadTypeLabel,
			oam.LabelOAMResourceType))
		Expect(len(wl.GetAnnotations())).Should(Equal(1))

		By("Verify trait metadata (name, namespace, labels, annotations, ownerRef)")
		trait := traits[compName][0]
		Expect(trait.GetName()).Should(ContainSubstring(compName))
		Expect(trait.GetNamespace()).Should(Equal(namespace))
		labels = trait.GetLabels()
		labelKeys = make([]string, 0, len(labels))
		for k := range labels {
			labelKeys = append(labelKeys, k)
		}
		Expect(labelKeys).Should(ContainElements(
			oam.LabelAppName,
			oam.LabelAppRevision,
			oam.LabelAppRevisionHash,
			oam.LabelAppComponent,
			oam.LabelAppComponentRevision,
			oam.TraitTypeLabel,
			oam.LabelOAMResourceType))
		Expect(len(wl.GetAnnotations())).Should(Equal(1))

		By("Verify referenced scopes")
		scopes, err := ao.ReferencedScopes()
		Expect(err).Should(BeNil())
		wlTypedRef := corev1.ObjectReference{
			APIVersion: "apps/v1",
			Kind:       "Deployment",
			Name:       compName,
		}
		Expect(len(scopes[wlTypedRef]) > 0).Should(BeTrue())
		wlScope := scopes[wlTypedRef][0]
		wantScopeRef := corev1.ObjectReference{
			APIVersion: "core.oam.dev/v1beta1",
			Kind:       "HealthScope",
			Name:       "sample-health-scope",
		}
		Expect(wlScope).Should(Equal(wantScopeRef))
	})

	It("test annotation and label filter", func() {
		var (
			compName     = "frontend"
			workloadName = "test-workload"
		)
		appRev := &v1beta1.ApplicationRevision{}
		b, err := os.ReadFile("./testdata/filter_annotations.yaml")
		Expect(err).Should(BeNil())
		err = yaml.Unmarshal(b, appRev)
		Expect(err).Should(BeNil())
		getKeys := func(m map[string]string) []string {
			var keys []string
			for k := range m {
				keys = append(keys, k)
			}
			return keys
		}
		//this appRev is generated on below this app:
		/*
			metadata:
			  name: website
			  annotations:
			    filter.oam.dev/annotation-keys: "notPassAnno1, notPassAnno2"
			    filter.oam.dev/label-keys: "notPassLabel"
			    notPassAnno1: "Annotation-filtered"
			    notPassAnno2: "Annotation-filtered"
			    canPassAnno: "Annotation-passed"
			  labels:
			    notPassLabel: "Label-filtered"
			    canPassLabel: "Label-passed"
			spec:
			  components:
			    - name: frontend
			      type: webservice
			      properties:
			        image: nginx
		*/

		ao := NewAppManifests(appRev, appParser)
		workloads, _, _, err := ao.GroupAssembledManifests()
		Expect(err).Should(BeNil())

		By("verify labels specified should be filtered")
		wl := workloads[compName]
		labelKeys := getKeys(wl.GetLabels())

		Expect(labelKeys).ShouldNot(ContainElements("notPassLabel"))
		Expect(labelKeys).Should(ContainElements("canPassLabel"))

		By("verify annotations specified should be filtered")
		annotationKeys := getKeys(wl.GetAnnotations())
		Expect(annotationKeys).ShouldNot(ContainElements("notPassAnno1", "notPassAnno2"))
		Expect(annotationKeys).Should(ContainElements("canPassAnno"))

		By("Verify workload metadata (name)")
		Expect(wl.GetName()).Should(Equal(workloadName))
	})
})

var _ = Describe("Test handleCheckManageWorkloadTrait func", func() {
	It("Test every situation", func() {
		traitDefs := map[string]*v1beta1.TraitDefinition{
			"rollout": {
				Spec: v1beta1.TraitDefinitionSpec{
					ManageWorkload: true,
				},
			},
			"normal": {
				Spec: v1beta1.TraitDefinitionSpec{},
			},
		}
		appRev := v1beta1.ApplicationRevision{
			Spec: v1beta1.ApplicationRevisionSpec{
				ApplicationRevisionCompressibleFields: v1beta1.ApplicationRevisionCompressibleFields{
					TraitDefinitions: traitDefs,
				},
			},
		}
		rolloutTrait := &unstructured.Unstructured{}
		rolloutTrait.SetLabels(map[string]string{oam.TraitTypeLabel: "rollout"})

		normalTrait := &unstructured.Unstructured{}
		normalTrait.SetLabels(map[string]string{oam.TraitTypeLabel: "normal"})

		workload := unstructured.Unstructured{}
		workload.SetLabels(map[string]string{
			oam.WorkloadTypeLabel: "webservice",
		})

		comps := []*velatypes.ComponentManifest{
			{
				Traits: []*unstructured.Unstructured{
					rolloutTrait,
					normalTrait,
				},
				StandardWorkload: &workload,
			},
		}

		HandleCheckManageWorkloadTrait(appRev, comps)
		Expect(len(rolloutTrait.GetLabels())).Should(BeEquivalentTo(2))
		Expect(rolloutTrait.GetLabels()[oam.LabelManageWorkloadTrait]).Should(BeEquivalentTo("true"))
		Expect(len(normalTrait.GetLabels())).Should(BeEquivalentTo(1))
		Expect(normalTrait.GetLabels()[oam.LabelManageWorkloadTrait]).Should(BeEquivalentTo(""))
	})
})
