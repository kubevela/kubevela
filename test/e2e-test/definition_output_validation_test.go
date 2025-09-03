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
		k8sClient.DeleteAllOf(ctx, &v1beta1.WorkflowStepDefinition{}, client.InNamespace(namespace))

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
			Expect(err.Error()).To(ContainSubstring("resource type not found on cluster"))
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
			Expect(err.Error()).To(ContainSubstring("resource type not found on cluster"))
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
			Expect(err.Error()).To(ContainSubstring("resource type not found on cluster"))
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

		It("Should reject ComponentDefinition with multiple invalid CRDs", func() {
			componentDef := &v1beta1.ComponentDefinition{
				TypeMeta: metav1.TypeMeta{
					Kind:       "ComponentDefinition",
					APIVersion: "core.oam.dev/v1beta1",
				},
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-comp-multiple-invalid",
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
	validService: {
		apiVersion: "v1"
		kind: "Service"
		metadata: name: parameter.name + "-svc"
		spec: {
			selector: app: parameter.name
			ports: [{port: 80}]
		}
	}
	invalidCRD1: {
		apiVersion: "custom.io/v1alpha1"
		kind: "CustomResource"
		metadata: name: parameter.name + "-custom1"
		spec: {foo: "bar"}
	}
	invalidCRD2: {
		apiVersion: "another.io/v1beta1"
		kind: "AnotherResource"
		metadata: name: parameter.name + "-custom2"
		spec: {enabled: true}
	}
}`,
						},
					},
				},
			}

			err := k8sClient.Create(ctx, componentDef)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("resource type not found on cluster"))
		})

		It("Should handle definitions with complex CUE expressions", func() {
			componentDef := &v1beta1.ComponentDefinition{
				TypeMeta: metav1.TypeMeta{
					Kind:       "ComponentDefinition",
					APIVersion: "core.oam.dev/v1beta1",
				},
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-comp-cue-expressions",
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
import "strings"

parameter: {
	name: string
	image: string
	enableService: bool | *false
}

output: {
	apiVersion: "apps/v1"
	kind: "Deployment"
	metadata: {
		name: strings.ToLower(parameter.name)
		labels: {
			app: parameter.name
			version: "v1"
		}
	}
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

if parameter.enableService {
	outputs: service: {
		apiVersion: "v1"
		kind: "Service"
		metadata: name: parameter.name + "-svc"
		spec: {
			selector: app: parameter.name
			ports: [{port: 80, targetPort: 8080}]
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

		It("Should reject TraitDefinition with mixed valid and invalid outputs", func() {
			traitDef := &v1beta1.TraitDefinition{
				TypeMeta: metav1.TypeMeta{
					Kind:       "TraitDefinition",
					APIVersion: "core.oam.dev/v1beta1",
				},
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-trait-mixed-validity",
					Namespace: namespace,
				},
				Spec: v1beta1.TraitDefinitionSpec{
					Schematic: &common.Schematic{
						CUE: &common.CUE{
							Template: `
parameter: {
	replicas: int | *2
	cpu: string | *"100m"
	memory: string | *"128Mi"
}

outputs: {
	// Valid resources
	service: {
		apiVersion: "v1"
		kind: "Service"
		metadata: name: context.name + "-svc"
		spec: {
			selector: app: context.name
			ports: [{
				port: 80
				targetPort: 8080
			}]
		}
	}
	configmap: {
		apiVersion: "v1"
		kind: "ConfigMap"
		metadata: name: context.name + "-config"
		data: {
			cpu: parameter.cpu
			memory: parameter.memory
		}
	}
	// Invalid CRD - should cause failure
	customMonitor: {
		apiVersion: "monitoring.custom.io/v1alpha1"
		kind: "ServiceMonitor"
		metadata: name: context.name + "-monitor"
		spec: {
			selector: {
				matchLabels: {
					app: context.name
				}
			}
			endpoints: [{
				port: "metrics"
				interval: "30s"
			}]
		}
	}
	// Non-K8s object (should be ignored)
	metadata: {
		version: "1.0.0"
		features: ["monitoring", "scaling"]
	}
}`,
						},
					},
				},
			}

			err := k8sClient.Create(ctx, traitDef)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("resource type not found on cluster"))
		})

		It("Should accept PolicyDefinition with only standard Kubernetes resources", func() {
			policyDef := &v1beta1.PolicyDefinition{
				TypeMeta: metav1.TypeMeta{
					Kind:       "PolicyDefinition",
					APIVersion: "core.oam.dev/v1beta1",
				},
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-policy-standard-resources",
					Namespace: namespace,
				},
				Spec: v1beta1.PolicyDefinitionSpec{
					Schematic: &common.Schematic{
						CUE: &common.CUE{
							Template: `
parameter: {
	namespace: string
	enabled: bool | *true
	labels: {...} | *{}
}

output: {
	apiVersion: "v1"
	kind: "Namespace"
	metadata: {
		name: parameter.namespace
		labels: parameter.labels
	}
}

outputs: {
	networkPolicy: {
		apiVersion: "networking.k8s.io/v1"
		kind: "NetworkPolicy"
		metadata: {
			name: parameter.namespace + "-default-deny"
			namespace: parameter.namespace
		}
		spec: {
			podSelector: {}
			policyTypes: ["Ingress", "Egress"]
		}
	}
	resourceQuota: {
		apiVersion: "v1"
		kind: "ResourceQuota"
		metadata: {
			name: parameter.namespace + "-quota"
			namespace: parameter.namespace
		}
		spec: {
			hard: {
				"requests.cpu": "4"
				"requests.memory": "8Gi"
				"limits.cpu": "8"
				"limits.memory": "16Gi"
				"pods": "10"
			}
		}
	}
	limitRange: {
		apiVersion: "v1"
		kind: "LimitRange"
		metadata: {
			name: parameter.namespace + "-limits"
			namespace: parameter.namespace
		}
		spec: {
			limits: [{
				type: "Container"
				default: {
					cpu: "100m"
					memory: "128Mi"
				}
				defaultRequest: {
					cpu: "50m"
					memory: "64Mi"
				}
			}]
		}
	}
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

		It("Should validate definitions with empty outputs", func() {
			componentDef := &v1beta1.ComponentDefinition{
				TypeMeta: metav1.TypeMeta{
					Kind:       "ComponentDefinition",
					APIVersion: "core.oam.dev/v1beta1",
				},
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-comp-empty-outputs",
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

// Empty outputs should be valid
outputs: {}`,
						},
					},
				},
			}

			err := k8sClient.Create(ctx, componentDef)
			Expect(err).NotTo(HaveOccurred())

			// Clean up
			Expect(k8sClient.Delete(ctx, componentDef)).Should(Succeed())
		})

		It("Should reject WorkflowStepDefinition with non-existent CRD in outputs", func() {
			workflowStepDef := &v1beta1.WorkflowStepDefinition{
				TypeMeta: metav1.TypeMeta{
					Kind:       "WorkflowStepDefinition",
					APIVersion: "core.oam.dev/v1beta1",
				},
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-workflowstep-with-invalid-output",
					Namespace: namespace,
				},
				Spec: v1beta1.WorkflowStepDefinitionSpec{
					Schematic: &common.Schematic{
						CUE: &common.CUE{
							Template: `
parameter: {
	namespace: string
	name: string
}

output: {
	apiVersion: "v1"
	kind: "ConfigMap"
	metadata: {
		name: parameter.name
		namespace: parameter.namespace
	}
	data: {
		status: "created"
	}
}

outputs: {
	invalidStep: {
		apiVersion: "workflow.custom.io/v1alpha1"
		kind: "WorkflowStep"
		metadata: name: parameter.name + "-step"
		spec: {
			type: "custom"
			enabled: true
		}
	}
}`,
						},
					},
				},
			}

			err := k8sClient.Create(ctx, workflowStepDef)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("resource type not found on cluster"))
		})

		It("Should accept WorkflowStepDefinition with valid resources", func() {
			workflowStepDef := &v1beta1.WorkflowStepDefinition{
				TypeMeta: metav1.TypeMeta{
					Kind:       "WorkflowStepDefinition",
					APIVersion: "core.oam.dev/v1beta1",
				},
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-workflowstep-with-valid-outputs",
					Namespace: namespace,
				},
				Spec: v1beta1.WorkflowStepDefinitionSpec{
					Schematic: &common.Schematic{
						CUE: &common.CUE{
							Template: `
parameter: {
	namespace: string
	name: string
	data: {...}
}

output: {
	apiVersion: "v1"
	kind: "ConfigMap"
	metadata: {
		name: parameter.name
		namespace: parameter.namespace
	}
	data: parameter.data
}

outputs: {
	secret: {
		apiVersion: "v1"
		kind: "Secret"
		metadata: {
			name: parameter.name + "-secret"
			namespace: parameter.namespace
		}
		type: "Opaque"
		data: {
			key: "dmFsdWU="  // base64 encoded "value"
		}
	}
}`,
						},
					},
				},
			}

			err := k8sClient.Create(ctx, workflowStepDef)
			Expect(err).NotTo(HaveOccurred())

			// Clean up
			Expect(k8sClient.Delete(ctx, workflowStepDef)).Should(Succeed())
		})

		It("Should reject WorkflowStepDefinition with mixed valid and invalid resources", func() {
			workflowStepDef := &v1beta1.WorkflowStepDefinition{
				TypeMeta: metav1.TypeMeta{
					Kind:       "WorkflowStepDefinition",
					APIVersion: "core.oam.dev/v1beta1",
				},
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-workflowstep-mixed-outputs",
					Namespace: namespace,
				},
				Spec: v1beta1.WorkflowStepDefinitionSpec{
					Schematic: &common.Schematic{
						CUE: &common.CUE{
							Template: `
parameter: {
	name: string
	namespace: string
}

output: {
	apiVersion: "batch/v1"
	kind: "Job"
	metadata: {
		name: parameter.name
		namespace: parameter.namespace
	}
	spec: {
		template: {
			spec: {
				containers: [{
					name: "job"
					image: "busybox"
					command: ["echo", "hello"]
				}]
				restartPolicy: "Never"
			}
		}
	}
}

outputs: {
	// Valid resource
	cronJob: {
		apiVersion: "batch/v1"
		kind: "CronJob"
		metadata: {
			name: parameter.name + "-cron"
			namespace: parameter.namespace
		}
		spec: {
			schedule: "*/5 * * * *"
			jobTemplate: {
				spec: {
					template: {
						spec: {
							containers: [{
								name: "cron"
								image: "busybox"
								command: ["echo", "cron"]
							}]
							restartPolicy: "Never"
						}
					}
				}
			}
		}
	}
	// Invalid custom resource
	customScheduler: {
		apiVersion: "scheduler.custom.io/v1beta1"
		kind: "CustomScheduler"
		metadata: {
			name: parameter.name + "-scheduler"
		}
		spec: {
			interval: "5m"
			action: "notify"
		}
	}
}`,
						},
					},
				},
			}

			err := k8sClient.Create(ctx, workflowStepDef)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("resource type not found on cluster"))
		})

		It("Should accept WorkflowStepDefinition without outputs", func() {
			workflowStepDef := &v1beta1.WorkflowStepDefinition{
				TypeMeta: metav1.TypeMeta{
					Kind:       "WorkflowStepDefinition",
					APIVersion: "core.oam.dev/v1beta1",
				},
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-workflowstep-no-outputs",
					Namespace: namespace,
				},
				Spec: v1beta1.WorkflowStepDefinitionSpec{
					Schematic: &common.Schematic{
						CUE: &common.CUE{
							Template: `
parameter: {
	message: string
	delay: int | *1
}

// This workflow step just processes data without creating resources
processedData: {
	message: parameter.message
	timestamp: "2024-01-01T00:00:00Z"
	processed: true
}`,
						},
					},
				},
			}

			err := k8sClient.Create(ctx, workflowStepDef)
			Expect(err).NotTo(HaveOccurred())

			// Clean up
			Expect(k8sClient.Delete(ctx, workflowStepDef)).Should(Succeed())
		})

		It("Should reject ComponentDefinition with invalid apiVersion format", func() {
			componentDef := &v1beta1.ComponentDefinition{
				TypeMeta: metav1.TypeMeta{
					Kind:       "ComponentDefinition",
					APIVersion: "core.oam.dev/v1beta1",
				},
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-comp-invalid-apiversion",
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
				name: "nginx"
				image: "nginx:latest"
			}]
		}
	}
}

outputs: {
	invalidResource: {
		apiVersion: "invalid-format-no-slash"
		kind: "SomeResource"
		metadata: name: parameter.name
		spec: {}
	}
}`,
						},
					},
				},
			}

			err := k8sClient.Create(ctx, componentDef)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("resource type not found on cluster"))
		})
	})
})
