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

package applicationrollout

import (
	"context"
	encoding "encoding/json"
	"testing"

	"github.com/ghodss/yaml"
	. "github.com/onsi/ginkgo"
	"k8s.io/utils/pointer"

	. "github.com/onsi/gomega"

	"github.com/oam-dev/kubevela/apis/core.oam.dev/common"
	"github.com/oam-dev/kubevela/apis/core.oam.dev/v1beta1"
	"github.com/oam-dev/kubevela/apis/standard.oam.dev/v1alpha1"

	//"github.com/oam-dev/kubevela/pkg/builtin/kind"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	//"k8s.io/apimachinery/pkg/util/intstr"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	//velatypes "github.com/oam-dev/kubevela/apis/types"
)

var _ = Describe("Test Approllout Controller", func() {
	ctx := context.TODO()

	functioningAppRollout := &v1beta1.AppRollout{
		TypeMeta: metav1.TypeMeta{
			Kind:       "AppRollout",
			APIVersion: "core.oam.dev/v1beta1",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      "functioning-AppRollout",
			Namespace: "functioning-AppRollout",
		},
		Spec: v1beta1.AppRolloutSpec{
			TargetAppRevisionName: "target1",
			SourceAppRevisionName: "source1",
		},
		Status: common.AppRolloutStatus{
			RolloutStatus: v1alpha1.RolloutStatus{
				RollingState: v1alpha1.RolloutSucceedState,
			},
		},
	}
	//var num int32 = 5
	// scaleRollout := &v1beta1.AppRollout{
	// 	TypeMeta: metav1.TypeMeta{
	// 		Kind:       "AppRollout",
	// 		APIVersion: "core.oam.dev/v1beta1",
	// 	},
	// 	ObjectMeta: metav1.ObjectMeta{
	// 		Name:      "scaleRollout",
	// 		Namespace: "scaleRollout",
	// 	},
	// 	Spec: v1beta1.AppRolloutSpec{
	// 		TargetAppRevisionName: "test-rolling-v1",
	// 		ComponentList: []string{
	// 			"metrics-provider",
	// 		},
	// 		RolloutPlan: v1alpha1.RolloutPlan{
	// 			RolloutStrategy: "IncreaseFirst",
	// 			RolloutBatches: []v1alpha1.RolloutBatch{
	// 				{
	// 					Replicas: intstr.FromInt(5),
	// 				},
	// 			},
	// 			TargetSize: &num,
	// 		},
	// 	},
	// }

	cd := &v1beta1.ComponentDefinition{}
	cdDefJson, _ := yaml.YAMLToJSON([]byte(webServiceYaml))

	BeforeEach(func() {
		Expect(encoding.Unmarshal(cdDefJson, cd)).Should(BeNil())
		Expect(k8sClient.Create(ctx, cd.DeepCopy())).Should(BeNil())
	})

	AfterEach(func() {
		var tobeDeletedDeployDef v1beta1.WorkloadDefinition
		Expect(k8sClient.Get(ctx, client.ObjectKey{Name: "deployment", Namespace: "default"}, &tobeDeletedDeployDef)).Should(SatisfyAny(BeNil()))
		Expect(k8sClient.Delete(ctx, &tobeDeletedDeployDef)).Should(SatisfyAny(BeNil()))
		By("[TEST] Clean up resources after an integration test")
	})

	It("appRollout will set event", func() {
		ns := &corev1.Namespace{
			ObjectMeta: metav1.ObjectMeta{
				Name: "vela-test-AppRollout-event",
			},
		}
		functioningAppRollout.SetNamespace(ns.Name)
		Expect(k8sClient.Create(ctx, ns)).Should(BeNil())
		Expect(k8sClient.Create(ctx, functioningAppRollout.DeepCopyObject())).Should(BeNil())

		appKey := client.ObjectKey{
			Name:      functioningAppRollout.Name,
			Namespace: functioningAppRollout.Namespace,
		}

		reconcileOnceAfterFinalizer(reconciler, reconcile.Request{NamespacedName: appKey})

		events, err := recorder.GetEventsWithName(functioningAppRollout.Name)
		Expect(err).Should(BeNil())
		Expect(len(events)).ShouldNot(Equal(0))
		for _, event := range events {
			Expect(event.EventType).ShouldNot(Equal(corev1.EventTypeWarning))
			Expect(event.EventType).Should(Equal(corev1.EventTypeNormal))
		}

	})

})

func Test_isRolloutModified(t *testing.T) {
	tests := map[string]struct {
		appRollout v1beta1.AppRollout
		want       bool
	}{
		"initial case when no source or target set": {
			appRollout: v1beta1.AppRollout{
				Spec: v1beta1.AppRolloutSpec{
					TargetAppRevisionName: "target1",
					SourceAppRevisionName: "source1",
				},
				Status: common.AppRolloutStatus{
					RolloutStatus: v1alpha1.RolloutStatus{
						RollingState: v1alpha1.RollingInBatchesState,
					},
				},
			},
			want: false,
		},
		"scale no change case": {
			appRollout: v1beta1.AppRollout{
				Spec: v1beta1.AppRolloutSpec{
					TargetAppRevisionName: "target1",
				},
				Status: common.AppRolloutStatus{
					RolloutStatus: v1alpha1.RolloutStatus{
						RollingState: v1alpha1.RollingInBatchesState,
					},
					LastUpgradedTargetAppRevision: "target1",
				},
			},
			want: false,
		},
		"rollout no change case": {
			appRollout: v1beta1.AppRollout{
				Spec: v1beta1.AppRolloutSpec{
					TargetAppRevisionName: "target1",
					SourceAppRevisionName: "source1",
				},
				Status: common.AppRolloutStatus{
					RolloutStatus: v1alpha1.RolloutStatus{
						RollingState: v1alpha1.RollingInBatchesState,
					},
					LastUpgradedTargetAppRevision: "target1",
					LastSourceAppRevision:         "source1",
				},
			},
			want: false,
		},
		"scale change case": {
			appRollout: v1beta1.AppRollout{
				Spec: v1beta1.AppRolloutSpec{
					TargetAppRevisionName: "target2",
				},
				Status: common.AppRolloutStatus{
					RolloutStatus: v1alpha1.RolloutStatus{
						RollingState: v1alpha1.RollingInBatchesState,
					},
					LastUpgradedTargetAppRevision: "target1",
				},
			},
			want: true,
		},
		"rollout one change case": {
			appRollout: v1beta1.AppRollout{
				Spec: v1beta1.AppRolloutSpec{
					TargetAppRevisionName: "target2",
					SourceAppRevisionName: "source1",
				},
				Status: common.AppRolloutStatus{
					RolloutStatus: v1alpha1.RolloutStatus{
						RollingState: v1alpha1.RollingInBatchesState,
					},
					LastUpgradedTargetAppRevision: "target1",
					LastSourceAppRevision:         "source1",
				},
			},
			want: true,
		},
		"rollout both change case": {
			appRollout: v1beta1.AppRollout{
				Spec: v1beta1.AppRolloutSpec{
					TargetAppRevisionName: "target2",
					SourceAppRevisionName: "source2",
				},
				Status: common.AppRolloutStatus{
					RolloutStatus: v1alpha1.RolloutStatus{
						RollingState: v1alpha1.RollingInBatchesState,
					},
					LastUpgradedTargetAppRevision: "target1",
					LastSourceAppRevision:         "source1",
				},
			},
			want: true,
		},
		"deleting both change case": {
			appRollout: v1beta1.AppRollout{
				Spec: v1beta1.AppRolloutSpec{
					TargetAppRevisionName: "target2",
					SourceAppRevisionName: "source2",
				},
				Status: common.AppRolloutStatus{
					RolloutStatus: v1alpha1.RolloutStatus{
						RollingState: v1alpha1.RolloutDeletingState,
					},
					LastUpgradedTargetAppRevision: "target1",
					LastSourceAppRevision:         "source1",
				},
			},
			want: false,
		},
		"restart a scale operation": {
			appRollout: v1beta1.AppRollout{
				Spec: v1beta1.AppRolloutSpec{
					TargetAppRevisionName: "target1",
					RolloutPlan: v1alpha1.RolloutPlan{
						TargetSize: pointer.Int32Ptr(1),
					},
				},
				Status: common.AppRolloutStatus{
					RolloutStatus: v1alpha1.RolloutStatus{
						RollingState:      v1alpha1.RolloutSucceedState,
						RolloutTargetSize: 2,
					},
					LastUpgradedTargetAppRevision: "target1",
				},
			},
			want: true,
		},
		"scale have finished": {
			appRollout: v1beta1.AppRollout{
				Spec: v1beta1.AppRolloutSpec{
					TargetAppRevisionName: "target1",
					RolloutPlan: v1alpha1.RolloutPlan{
						TargetSize: pointer.Int32Ptr(2),
					},
				},
				Status: common.AppRolloutStatus{
					RolloutStatus: v1alpha1.RolloutStatus{
						RollingState:      v1alpha1.RolloutSucceedState,
						RolloutTargetSize: 2,
					},
					LastUpgradedTargetAppRevision: "target1",
				},
			},
			want: false,
		},
	}
	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			if got := isRolloutModified(tt.appRollout); got != tt.want {
				t.Errorf("isRolloutModified() = %v, want %v", got, tt.want)
			}
		})
	}
}

func reconcileOnceAfterFinalizer(r reconcile.Reconciler, req reconcile.Request) (reconcile.Result, error) {
	r.Reconcile(req)
	r.Reconcile(req)

	return r.Reconcile(req)
}

const (
	webServiceYaml = `
		apiVersion: core.oam.dev/v1beta1
		kind: ComponentDefinition
		metadata:
		name: webservice
		annotations:
			definition.oam.dev/description: "Describes long-running, scalable, containerized services that have a stable network endpoint to receive external network traffic from customers."
		spec:
		workload:
			definition:
			apiVersion: apps/v1
			kind: Deployment
		schematic:
			cue:
			template: |
				import (
					apps "kube/apps/v1"
				)
				output: apps.#Deployment
				output: {
					spec: {
						selector: matchLabels: {
							"app.oam.dev/component": context.name
						}
						if parameter["replicas"] != _|_ {
							replicas: parameter.replicas
						}
						template: {
							metadata: labels: {
								"app.oam.dev/component": context.name
							}
				
							spec: {
								containers: [{
									name:  context.name
									image: parameter.image
				
									if parameter["cmd"] != _|_ {
										command: parameter.cmd
									}
				
									if parameter["env"] != _|_ {
										env: parameter.env
									}
				
									if context["config"] != _|_ {
										env: context.config
									}
				
									ports: [{
										containerPort: parameter.port
									}]
				
									if parameter["cpu"] != _|_ {
										resources: {
											limits:
												cpu: parameter.cpu
											requests:
												cpu: parameter.cpu
										}
									}
								}]
							}
						}
					}
				}
				parameter: {
					// +usage=Which image would you like to use for your service
					// +short=i
					image: string
				
					// +usage=Commands to run in the container
					cmd?: [...string]
				
					// +usage=Which port do you want customer traffic sent to
					// +short=p
					port: *80 | int
					// +usage=Define arguments by using environment variables
					env?: [...{
						// +usage=Environment variable name
						name: string
						// +usage=The value of the environment variable
						value?: string
						// +usage=Specifies a source the value of this var should come from
						valueFrom?: {
							// +usage=Selects a key of a secret in the pod's namespace
							secretKeyRef: {
								// +usage=The name of the secret in the pod's namespace to select from
								name: string
								// +usage=The key of the secret to select from. Must be a valid secret key
								key: string
							}
						}
					}]
					// +usage=Number of CPU units for the service
					cpu?: string
					// +usage=Number of pods in the deployment
					replicas?: int
				}
	`

	ApplicationSourceYaml = `
		apiVersion: core.oam.dev/v1beta1
		kind: Application
		metadata:
		name: test-rolling
		annotations:
			"app.oam.dev/rollout-template": "true"
		spec:
		components:
			- name: metrics-provider
			type: webservice
			properties:
				cmd:
				- ./podinfo
				- stress-cpu=1
				image: stefanprodan/podinfo:4.0.6
				port: 8080
	`
	ApplicationTargetYaml = `
		apiVersion: core.oam.dev/v1beta1
		kind: Application
		metadata:
		name: test-rolling
		annotations:
			"app.oam.dev/rollout-template": "true"
		spec:
		components:
			- name: metrics-provider
			type: webservice
			properties:
				cmd:
				- ./podinfo
				- stress-cpu=1
				image: stefanprodan/podinfo:5.0.2
				port: 8080
	`
)
