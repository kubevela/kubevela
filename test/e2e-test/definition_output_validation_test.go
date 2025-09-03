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

package controllers_test

import (
	"context"
	"fmt"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/oam-dev/kubevela/apis/core.oam.dev/common"
	"github.com/oam-dev/kubevela/apis/core.oam.dev/v1beta1"
	"github.com/oam-dev/kubevela/pkg/oam/util"
)

var _ = Describe("Definition Output Validation E2E tests", func() {
	ctx := context.Background()

	var namespace string
	var ns corev1.Namespace

	BeforeEach(func() {
		namespace = randomNamespaceName("def-output-validation")
		ns = corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: namespace}}

		Eventually(func() error {
			return k8sClient.Create(ctx, &ns)
		}, time.Second*3, time.Microsecond*300).Should(SatisfyAny(BeNil(), &util.AlreadyExistMatcher{}))
	})

	AfterEach(func() {
		By("Clean up resources after a test")
		k8sClient.DeleteAllOf(ctx, &v1beta1.ComponentDefinition{}, client.InNamespace(namespace))
		k8sClient.DeleteAllOf(ctx, &v1beta1.TraitDefinition{}, client.InNamespace(namespace))
		k8sClient.DeleteAllOf(ctx, &v1beta1.PolicyDefinition{}, client.InNamespace(namespace))

		By(fmt.Sprintf("Delete the entire namespace %s", ns.Name))
		Expect(k8sClient.Delete(ctx, &ns, client.PropagationPolicy(metav1.DeletePropagationForeground))).Should(Succeed())
	})

	Context("Test validation for outputs with non-existent CRDs", func() {

		It("Should reject ComponentDefinition with non-existent CRD in outputs", func() {
			componentDef := &v1beta1.ComponentDefinition{
				TypeMeta: metav1.TypeMeta{
					Kind:       "ComponentDefinition",
					APIVersion: "core.oam.dev/v1beta1",
				},
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-comp-with-invalid-output",
					Namespace: namespace,
				},
				Spec: v1beta1.ComponentDefinitionSpec{
					Workload: common.WorkloadTypeDescriptor{
						Definition: common.WorkloadGVK{
							APIVersion: "apps/v1",
							Kind:       "Deployment",
						},
					},
					Schematic: &common.Schematic{
						CUE: &common.CUE{
							Template: `
parameter: {
	name: string
	image: string
}

output: {
	apiVersion: "apps/v1"
	kind: "Deployment"
	metadata: name: parameter.name
	spec: {
		selector: matchLabels: app: parameter.name
		template: {
			metadata: labels: app: parameter.name
			spec: containers: [{
				name: parameter.name
				image: parameter.image
			}]
		}
	}
}

outputs: {
	nonExistentResource: {
		apiVersion: "custom.io/v1alpha1"
		kind: "NonExistentResource"
		metadata: name: parameter.name + "-custom"
		spec: {
			foo: "bar"
		}
	}
}`,
						},
					},
				},
			}

			err := k8sClient.Create(ctx, componentDef)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("does not exist on the cluster"))
		})

		It("Should accept ComponentDefinition with only valid resources in outputs", func() {
			componentDef := &v1beta1.ComponentDefinition{
				TypeMeta: metav1.TypeMeta{
					Kind:       "ComponentDefinition",
					APIVersion: "core.oam.dev/v1beta1",
				},
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-comp-with-valid-outputs",
					Namespace: namespace,
				},
				Spec: v1beta1.ComponentDefinitionSpec{
					Workload: common.WorkloadTypeDescriptor{
						Definition: common.WorkloadGVK{
							APIVersion: "apps/v1",
							Kind:       "Deployment",
						},
					},
					Schematic: &common.Schematic{
						CUE: &common.CUE{
							Template: `
parameter: {
	name: string
	image: string
}

output: {
	apiVersion: "apps/v1"
	kind: "Deployment"
	metadata: name: parameter.name
	spec: {
		selector: matchLabels: app: parameter.name
		template: {
			metadata: labels: app: parameter.name
			spec: containers: [{
				name: parameter.name
				image: parameter.image
			}]
		}
	}
}

outputs: {
	service: {
		apiVersion: "v1"
		kind: "Service"
		metadata: name: parameter.name + "-svc"
		spec: {
			selector: app: parameter.name
			ports: [{
				port: 80
				targetPort: 8080
			}]
		}
	}
	configmap: {
		apiVersion: "v1"
		kind: "ConfigMap"
		metadata: name: parameter.name + "-config"
		data: {
			config: "some-config"
		}
	}
}`,
						},
					},
				},
			}

			err := k8sClient.Create(ctx, componentDef)
			Expect(err).NotTo(HaveOccurred())

			// Clean up
			Expect(k8sClient.Delete(ctx, componentDef)).Should(Succeed())
		})

		It("Should reject TraitDefinition with non-existent CRD in outputs", func() {
			traitDef := &v1beta1.TraitDefinition{
				TypeMeta: metav1.TypeMeta{
					Kind:       "TraitDefinition",
					APIVersion: "core.oam.dev/v1beta1",
				},
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-trait-with-invalid-output",
					Namespace: namespace,
				},
				Spec: v1beta1.TraitDefinitionSpec{
					Schematic: &common.Schematic{
						CUE: &common.CUE{
							Template: `
parameter: {
	replicas: int
}

outputs: {
	hpa: {
		apiVersion: "autoscaling/v2"
		kind: "HorizontalPodAutoscaler"
		metadata: name: context.name + "-hpa"
		spec: {
			scaleTargetRef: {
				apiVersion: "apps/v1"
				kind: "Deployment"
				name: context.name
			}
			minReplicas: parameter.replicas
			maxReplicas: parameter.replicas * 2
		}
	}
	customResource: {
		apiVersion: "nonexistent.io/v1"
		kind: "NonExistentCRD"
		metadata: name: context.name + "-custom"
		spec: {
			data: "test"
		}
	}
}`,
						},
					},
				},
			}

			err := k8sClient.Create(ctx, traitDef)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("does not exist on the cluster"))
		})

		It("Should accept TraitDefinition with valid resources", func() {
			traitDef := &v1beta1.TraitDefinition{
				TypeMeta: metav1.TypeMeta{
					Kind:       "TraitDefinition",
					APIVersion: "core.oam.dev/v1beta1",
				},
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-trait-with-valid-outputs",
					Namespace: namespace,
				},
				Spec: v1beta1.TraitDefinitionSpec{
					Schematic: &common.Schematic{
						CUE: &common.CUE{
							Template: `
parameter: {
	replicas: int
}

patch: {
	spec: replicas: parameter.replicas
}`,
						},
					},
				},
			}

			err := k8sClient.Create(ctx, traitDef)
			Expect(err).NotTo(HaveOccurred())

			// Clean up
			Expect(k8sClient.Delete(ctx, traitDef)).Should(Succeed())
		})

		It("Should reject PolicyDefinition with non-existent CRD in outputs", func() {
			policyDef := &v1beta1.PolicyDefinition{
				TypeMeta: metav1.TypeMeta{
					Kind:       "PolicyDefinition",
					APIVersion: "core.oam.dev/v1beta1",
				},
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-policy-with-invalid-output",
					Namespace: namespace,
				},
				Spec: v1beta1.PolicyDefinitionSpec{
					Schematic: &common.Schematic{
						CUE: &common.CUE{
							Template: `
parameter: {
	namespace: string
}

output: {
	apiVersion: "v1"
	kind: "Namespace"
	metadata: name: parameter.namespace
}

outputs: {
	invalidResource: {
		apiVersion: "custom.io/v1beta1"
		kind: "CustomPolicy"
		metadata: name: parameter.namespace + "-policy"
		spec: {
			enabled: true
		}
	}
}`,
						},
					},
				},
			}

			err := k8sClient.Create(ctx, policyDef)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("does not exist on the cluster"))
		})

		It("Should accept PolicyDefinition with valid resources", func() {
			policyDef := &v1beta1.PolicyDefinition{
				TypeMeta: metav1.TypeMeta{
					Kind:       "PolicyDefinition",
					APIVersion: "core.oam.dev/v1beta1",
				},
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-policy-with-valid-output",
					Namespace: namespace,
				},
				Spec: v1beta1.PolicyDefinitionSpec{
					Schematic: &common.Schematic{
						CUE: &common.CUE{
							Template: `
parameter: {
	namespace: string
}

output: {
	apiVersion: "v1"
	kind: "Namespace"
	metadata: name: parameter.namespace
}`,
						},
					},
				},
			}

			err := k8sClient.Create(ctx, policyDef)
			Expect(err).NotTo(HaveOccurred())

			// Clean up
			Expect(k8sClient.Delete(ctx, policyDef)).Should(Succeed())
		})

		It("Should handle definitions with mixed valid and non-K8s resources", func() {
			componentDef := &v1beta1.ComponentDefinition{
				TypeMeta: metav1.TypeMeta{
					Kind:       "ComponentDefinition",
					APIVersion: "core.oam.dev/v1beta1",
				},
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-comp-with-mixed-outputs",
					Namespace: namespace,
				},
				Spec: v1beta1.ComponentDefinitionSpec{
					Workload: common.WorkloadTypeDescriptor{
						Definition: common.WorkloadGVK{
							APIVersion: "apps/v1",
							Kind:       "Deployment",
						},
					},
					Schematic: &common.Schematic{
						CUE: &common.CUE{
							Template: `
parameter: {
	name: string
	image: string
}

output: {
	apiVersion: "apps/v1"
	kind: "Deployment"
	metadata: name: parameter.name
	spec: {
		selector: matchLabels: app: parameter.name
		template: {
			metadata: labels: app: parameter.name
			spec: containers: [{
				name: parameter.name
				image: parameter.image
			}]
		}
	}
}

outputs: {
	service: {
		apiVersion: "v1"
		kind: "Service"
		metadata: name: parameter.name + "-svc"
		spec: {
			selector: app: parameter.name
			ports: [{port: 80}]
		}
	}
	customData: {
		field1: "value1"
		field2: "value2"
		nested: {
			data: "some-data"
		}
	}
}`,
						},
					},
				},
			}

			err := k8sClient.Create(ctx, componentDef)
			Expect(err).NotTo(HaveOccurred())

			// Clean up
			Expect(k8sClient.Delete(ctx, componentDef)).Should(Succeed())
		})

		It("Should handle update operations with validation", func() {
			// First create a valid definition
			componentDef := &v1beta1.ComponentDefinition{
				TypeMeta: metav1.TypeMeta{
					Kind:       "ComponentDefinition",
					APIVersion: "core.oam.dev/v1beta1",
				},
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-comp-update-validation",
					Namespace: namespace,
				},
				Spec: v1beta1.ComponentDefinitionSpec{
					Workload: common.WorkloadTypeDescriptor{
						Definition: common.WorkloadGVK{
							APIVersion: "apps/v1",
							Kind:       "Deployment",
						},
					},
					Schematic: &common.Schematic{
						CUE: &common.CUE{
							Template: `
parameter: {
	name: string
}

output: {
	apiVersion: "apps/v1"
	kind: "Deployment"
	metadata: name: parameter.name
	spec: {
		selector: matchLabels: app: parameter.name
		template: {
			metadata: labels: app: parameter.name
			spec: containers: [{
				name: "app"
				image: "nginx"
			}]
		}
	}
}`,
						},
					},
				},
			}

			err := k8sClient.Create(ctx, componentDef)
			Expect(err).NotTo(HaveOccurred())

			// Try to update with invalid output
			Eventually(func() error {
				var existing v1beta1.ComponentDefinition
				if err := k8sClient.Get(ctx, client.ObjectKey{Name: componentDef.Name, Namespace: namespace}, &existing); err != nil {
					return err
				}
				existing.Spec.Schematic.CUE.Template = `
parameter: {
	name: string
}

output: {
	apiVersion: "apps/v1"
	kind: "Deployment"
	metadata: name: parameter.name
	spec: {
		selector: matchLabels: app: parameter.name
		template: {
			metadata: labels: app: parameter.name
			spec: containers: [{
				name: "app"
				image: "nginx"
			}]
		}
	}
}

outputs: {
	invalidCRD: {
		apiVersion: "nonexistent.io/v1"
		kind: "NonExistentResource"
		metadata: name: parameter.name
		spec: {}
	}
}`
				return k8sClient.Update(ctx, &existing)
			}, time.Second*5, time.Millisecond*500).Should(HaveOccurred())

			// Clean up
			Expect(k8sClient.Delete(ctx, componentDef)).Should(Succeed())
		})
	})
})