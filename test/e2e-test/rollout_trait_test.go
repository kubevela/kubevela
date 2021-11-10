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

package controllers_test

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	v1 "k8s.io/api/apps/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/intstr"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/yaml"

	common2 "github.com/oam-dev/kubevela/apis/core.oam.dev/common"
	"github.com/oam-dev/kubevela/apis/standard.oam.dev/v1alpha1"
	"github.com/oam-dev/kubevela/pkg/oam"
	"github.com/oam-dev/kubevela/pkg/oam/util"
	"github.com/oam-dev/kubevela/pkg/utils/common"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"

	corev1 "k8s.io/api/core/v1"

	"github.com/oam-dev/kubevela/apis/core.oam.dev/v1beta1"
)

var _ = Describe("rollout related e2e-test,rollout trait test", func() {
	ctx := context.Background()
	var namespaceName, componentName, compRevName string
	var ns corev1.Namespace
	var app v1beta1.Application
	var rollout v1alpha1.Rollout
	var targerDeploy, sourceDeploy v1.Deployment
	var err error

	createAllDef := func() {
		By("install all related definition")
		var cd v1beta1.ComponentDefinition
		Expect(yaml.Unmarshal([]byte(rolloutTestWd), &cd))
		// create the componentDefinition if not exist
		cd.Namespace = namespaceName
		Eventually(
			func() error {
				return k8sClient.Create(ctx, &cd)
			},
			time.Second*3, time.Millisecond*300).Should(SatisfyAny(BeNil(), &util.AlreadyExistMatcher{}))
		var td v1beta1.TraitDefinition
		Expect(yaml.Unmarshal([]byte(rolloutTestTd), &td)).Should(BeNil())
		td.Namespace = namespaceName
		Eventually(func() error {
			return k8sClient.Create(ctx, &td)
		},
			time.Second*3, time.Millisecond*300).Should(SatisfyAny(BeNil(), &util.AlreadyExistMatcher{}))
	}

	createNamespace := func() {
		ns = corev1.Namespace{
			ObjectMeta: metav1.ObjectMeta{
				Name: namespaceName,
			},
		}
		// delete the namespaceName with all its resources
		Eventually(
			func() error {
				return k8sClient.Delete(ctx, &ns, client.PropagationPolicy(metav1.DeletePropagationForeground))
			},
			time.Second*120, time.Millisecond*500).Should(SatisfyAny(BeNil(), &util.NotFoundMatcher{}))
		By("make sure all the resources are removed")
		objectKey := client.ObjectKey{
			Name: namespaceName,
		}
		res := &corev1.Namespace{}
		Eventually(
			func() error {
				return k8sClient.Get(ctx, objectKey, res)
			},
			time.Second*120, time.Millisecond*500).Should(&util.NotFoundMatcher{})
		Eventually(
			func() error {
				return k8sClient.Create(ctx, &ns)
			},
			time.Second*3, time.Millisecond*300).Should(SatisfyAny(BeNil(), &util.AlreadyExistMatcher{}))
	}

	verifySuccess := func(componentRevision string) {
		By("check rollout status have succeed")
		Eventually(func() error {
			rolloutKey := types.NamespacedName{Namespace: namespaceName, Name: componentName}
			rollout = v1alpha1.Rollout{}
			if err := k8sClient.Get(ctx, rolloutKey, &rollout); err != nil {
				return err
			}
			if rollout.Spec.TargetRevisionName != componentRevision {
				return fmt.Errorf("rollout have not point to right targetRevision")
			}
			if rollout.Status.RollingState != v1alpha1.RolloutSucceedState {
				return fmt.Errorf("error rollout status state %s", rollout.Status.RollingState)
			}
			compRevName = rollout.Spec.TargetRevisionName
			if rollout.GetAnnotations() == nil || rollout.GetAnnotations()[oam.AnnotationWorkloadName] != componentRevision {
				return fmt.Errorf("target workload name annotation missmatch  want %s acctually %s",
					rollout.GetAnnotations()[oam.AnnotationWorkloadName], componentRevision)
			}
			deployKey := types.NamespacedName{Namespace: namespaceName, Name: compRevName}
			if err := k8sClient.Get(ctx, deployKey, &targerDeploy); err != nil {
				return err
			}
			gvkStr := rollout.GetAnnotations()[oam.AnnotationWorkloadGVK]
			gvk := map[string]string{}
			if err := json.Unmarshal([]byte(gvkStr), &gvk); err != nil {
				return err
			}
			if gvk["apiVersion"] != "apps/v1" || gvk["kind"] != "Deployment" {
				return fmt.Errorf("error targetWorkload gvk")
			}
			if *targerDeploy.Spec.Replicas != *rollout.Spec.RolloutPlan.TargetSize {
				return fmt.Errorf("targetDeploy replicas missMatch %d, %d", targerDeploy.Spec.Replicas, rollout.Spec.RolloutPlan.TargetSize)
			}
			if targerDeploy.Status.UpdatedReplicas != *targerDeploy.Spec.Replicas {
				return fmt.Errorf("update not finish")
			}
			if len(targerDeploy.OwnerReferences) != 1 {
				return fmt.Errorf("workload ownerReference missMatch")
			}
			// guarantee rollout's owners and  workload's owners are same
			if targerDeploy.OwnerReferences[0].Kind != rollout.OwnerReferences[0].Kind ||
				targerDeploy.OwnerReferences[0].Name != rollout.OwnerReferences[0].Name {
				return fmt.Errorf("workload ownerReference missMatch")
			}
			if rollout.Status.LastSourceRevision == "" {
				return nil
			}
			deployKey = types.NamespacedName{Namespace: namespaceName, Name: rollout.Status.LastSourceRevision}
			if err := k8sClient.Get(ctx, deployKey, &sourceDeploy); err == nil || !apierrors.IsNotFound(err) {
				return fmt.Errorf("source deploy still exist namespace %s deployName %s", namespaceName, rollout.Status.LastSourceRevision)
			}
			return nil
		}, time.Second*60, 300*time.Millisecond).Should(BeNil())
	}

	BeforeEach(func() {
		By("Start to run a test, init whole env")
		namespaceName = randomNamespaceName("rollout-trait-e2e-test")
		app = v1beta1.Application{}
		createNamespace()
		createAllDef()
		componentName = "express-server"
	})

	AfterEach(func() {
		By("Clean up resources after a test")
		Eventually(func() error {
			err := k8sClient.Delete(ctx, &app)
			if err == nil || apierrors.IsNotFound(err) {
				return nil
			}
			return err
		}, 15*time.Second, 300*time.Microsecond).Should(BeNil())
		By(fmt.Sprintf("Delete the entire namespaceName %s", ns.Name))
		// delete the namespaceName with all its resources
		Expect(k8sClient.Delete(ctx, &ns, client.PropagationPolicy(metav1.DeletePropagationBackground))).Should(BeNil())
	})

	It("rollout as a trait whole process e2e-test", func() {
		By("first scale operation")
		Expect(common.ReadYamlToObject("testdata/rollout/deployment/application.yaml", &app)).Should(BeNil())
		app.Namespace = namespaceName
		Expect(k8sClient.Create(ctx, &app)).Should(BeNil())
		verifySuccess("express-server-v1")
		appKey := types.NamespacedName{Namespace: namespaceName, Name: app.Name}
		checkApp := &v1beta1.Application{}
		By("update application upgrade to v2")
		Eventually(func() error {
			if err = k8sClient.Get(ctx, appKey, checkApp); err != nil {
				return err
			}
			checkApp.Spec.Components[0].Properties.Raw = []byte(`{"image":"stefanprodan/podinfo:4.0.3","cpu":"0.1"}`)
			if err = k8sClient.Update(ctx, checkApp); err != nil {
				return err
			}
			return nil
		}, 30*time.Second, 300*time.Millisecond).Should(BeNil())
		verifySuccess("express-server-v2")
		By("update application upgrade to v3")
		Eventually(func() error {
			if err = k8sClient.Get(ctx, appKey, checkApp); err != nil {
				return err
			}
			checkApp.Spec.Components[0].Properties.Raw = []byte(`{"image":"stefanprodan/podinfo:4.0.3","cpu":"0.2"}`)
			if err = k8sClient.Update(ctx, checkApp); err != nil {
				return err
			}
			return nil
		}, 30*time.Second, 300*time.Millisecond).Should(BeNil())
		verifySuccess("express-server-v3")
		By("roll back to v2")
		time.Sleep(30 * time.Second)
		Eventually(func() error {
			if err = k8sClient.Get(ctx, appKey, checkApp); err != nil {
				return err
			}
			checkApp.Spec.Components[0].Traits[0].Properties.Raw = []byte(`{"targetRevision":"express-server-v2","firstBatchReplicas":1,"secondBatchReplicas":1}`)
			if err = k8sClient.Update(ctx, checkApp); err != nil {
				return err
			}
			return nil
		}, 30*time.Second, 300*time.Millisecond).Should(BeNil())
		verifySuccess("express-server-v2")
		By("modify targetSize to scale")
		Eventually(func() error {
			if err = k8sClient.Get(ctx, appKey, checkApp); err != nil {
				return err
			}
			checkApp.Spec.Components[0].Traits[0].Properties.Raw =
				[]byte(`{"targetRevision":"express-server-v2","targetSize":4,"firstBatchReplicas":1,"secondBatchReplicas":1}`)
			if err = k8sClient.Update(ctx, checkApp); err != nil {
				return err
			}
			return nil
		}, 30*time.Second, 300*time.Millisecond).Should(BeNil())
		time.Sleep(12 * time.Second)
		verifySuccess("express-server-v2")
		By("update application upgrade to v4")
		Eventually(func() error {
			if err = k8sClient.Get(ctx, appKey, checkApp); err != nil {
				return err
			}
			checkApp.Spec.Components[0].Properties.Raw = []byte(`{"image":"stefanprodan/podinfo:4.0.3","cpu":"0.3"}`)
			checkApp.Spec.Components[0].Traits[0].Properties.Raw =
				[]byte(`{"firstBatchReplicas":2,"secondBatchReplicas":2,"targetSize":4}`)
			if err = k8sClient.Update(ctx, checkApp); err != nil {
				return err
			}
			return nil
		}, 30*time.Second, 300*time.Millisecond).Should(BeNil())
		verifySuccess("express-server-v4")
		By("update application batch upgrade to v5")
		Eventually(func() error {
			if err = k8sClient.Get(ctx, appKey, checkApp); err != nil {
				return err
			}
			checkApp.Spec.Components[0].Properties.Raw = []byte(`{"image":"stefanprodan/podinfo:4.0.3","cpu":"0.5"}`)
			checkApp.Spec.Components[0].Traits[0].Properties.Raw =
				[]byte(`{"firstBatchReplicas":2,"secondBatchReplicas":2,"targetSize":4,"batchPartition":0}`)
			if err = k8sClient.Update(ctx, checkApp); err != nil {
				return err
			}
			return nil
		}, 30*time.Second, 300*time.Millisecond).Should(BeNil())
		// check rollout paused in partition 0
		time.Sleep(5 * time.Second)
		sourceDeploy := v1.Deployment{}
		Eventually(func() error {
			rolloutKey := types.NamespacedName{Namespace: namespaceName, Name: componentName}
			if err := k8sClient.Get(ctx, rolloutKey, &rollout); err != nil {
				return err
			}
			if rollout.Spec.TargetRevisionName != "express-server-v5" {
				return fmt.Errorf("rollout have not point to right targetRevision")
			}
			if rollout.Status.RollingState != v1alpha1.RollingInBatchesState {
				return fmt.Errorf("error rollout status state %s", rollout.Status.RollingState)
			}
			if rollout.Status.CurrentBatch != 0 {
				return fmt.Errorf("current batchPartition missmatch accutally %d", rollout.Status.CurrentBatch)
			}
			deployKey := types.NamespacedName{Namespace: namespaceName, Name: compRevName}
			if err := k8sClient.Get(ctx, deployKey, &sourceDeploy); err != nil {
				return err
			}
			if *sourceDeploy.Spec.Replicas != 2 {
				return fmt.Errorf("targetDeploy replicas missMatch %d", *sourceDeploy.Spec.Replicas)
			}
			return nil
		}, 300*time.Second, 300*time.Millisecond).Should(BeNil())
		By("continue rollout upgrade legacy batches")
		Eventually(func() error {
			if err = k8sClient.Get(ctx, appKey, checkApp); err != nil {
				return err
			}
			checkApp.Spec.Components[0].Traits[0].Properties.Raw =
				[]byte(`{"firstBatchReplicas":2,"secondBatchReplicas":2,"targetSize":4,"batchPartition":1}`)
			if err = k8sClient.Update(ctx, checkApp); err != nil {
				return err
			}
			return nil
		}, 30*time.Second, 300*time.Millisecond).Should(BeNil())
		verifySuccess("express-server-v5")
		By("delete the application, check workload have been removed")
		Expect(k8sClient.Delete(ctx, checkApp)).Should(BeNil())
		listOptions := []client.ListOption{
			client.InNamespace(namespaceName),
		}
		deployList := &v1.DeploymentList{}
		Eventually(func() error {
			if err := k8sClient.List(ctx, deployList, listOptions...); err != nil {
				return err
			}
			if len(deployList.Items) != 0 {
				return fmt.Errorf("workload have not been removed")
			}
			return nil
		}, 30*time.Second, 300*time.Millisecond).Should(BeNil())
	})

	It("rollout scale up and down without rollout batches", func() {
		By("first scale operation")
		Expect(common.ReadYamlToObject("testdata/rollout/deployment/application.yaml", &app)).Should(BeNil())
		app.Namespace = namespaceName
		Expect(k8sClient.Create(ctx, &app)).Should(BeNil())

		verifySuccess("express-server-v1")
		By("scale again to targetSize 4")
		appKey := types.NamespacedName{Namespace: namespaceName, Name: app.Name}
		checkApp := &v1beta1.Application{}
		Eventually(func() error {
			if err = k8sClient.Get(ctx, appKey, checkApp); err != nil {
				return err
			}
			// scale up without rollout batches, test rollout controller will fill default batches
			checkApp.Spec.Components[0].Traits[0].Properties.Raw =
				[]byte(`{"targetSize":4}`)
			if err = k8sClient.Update(ctx, checkApp); err != nil {
				return err
			}
			return nil
		}, 30*time.Second, 300*time.Millisecond).Should(BeNil())
		Eventually(func() error {
			checkRollout := v1alpha1.Rollout{}
			if err := k8sClient.Get(ctx, types.NamespacedName{Namespace: namespaceName, Name: componentName}, &checkRollout); err != nil {
				return err
			}
			if *checkRollout.Spec.RolloutPlan.TargetSize != 4 {
				return fmt.Errorf("rollout targetSize haven't update")
			}
			if len(checkRollout.Spec.RolloutPlan.RolloutBatches) != 1 {
				return fmt.Errorf("fail to fill rollout batches")
			}
			if checkRollout.Spec.RolloutPlan.RolloutBatches[0].Replicas != intstr.FromInt(2) {
				return fmt.Errorf("fill rollout batches missmatch")
			}
			return nil
		}, 30*time.Second, 300*time.Millisecond).Should(BeNil())
		verifySuccess("express-server-v1")
		checkApp = &v1beta1.Application{}
		By("update application upgrade to v2")
		Eventually(func() error {
			if err = k8sClient.Get(ctx, appKey, checkApp); err != nil {
				return err
			}
			checkApp.Spec.Components[0].Properties.Raw = []byte(`{"image":"stefanprodan/podinfo:4.0.3","cpu":"0.1"}`)
			checkApp.Spec.Components[0].Traits[0].Properties.Raw =
				[]byte(`{"firstBatchReplicas":2,"secondBatchReplicas":2,"targetSize":4}`)
			if err = k8sClient.Update(ctx, checkApp); err != nil {
				return err
			}
			return nil
		}, 30*time.Second, 300*time.Millisecond).Should(BeNil())
		verifySuccess("express-server-v2")

		By("scale down to targetSize 2")
		appKey = types.NamespacedName{Namespace: namespaceName, Name: app.Name}
		checkApp = &v1beta1.Application{}
		Eventually(func() error {
			if err = k8sClient.Get(ctx, appKey, checkApp); err != nil {
				return err
			}
			// scale down without rollout batches, test rollout controller will fill default batches
			checkApp.Spec.Components[0].Traits[0].Properties.Raw =
				[]byte(`{"targetSize":2}`)
			if err = k8sClient.Update(ctx, checkApp); err != nil {
				return err
			}
			return nil
		}, 30*time.Second, 300*time.Millisecond).Should(BeNil())
		Eventually(func() error {
			checkRollout := v1alpha1.Rollout{}
			if err := k8sClient.Get(ctx, types.NamespacedName{Namespace: namespaceName, Name: componentName}, &checkRollout); err != nil {
				return err
			}
			if *checkRollout.Spec.RolloutPlan.TargetSize != 2 {
				return fmt.Errorf("rollout targetSize haven't update")
			}
			if len(checkRollout.Spec.RolloutPlan.RolloutBatches) != 1 {
				return fmt.Errorf("fail to fill rollout batches")
			}
			if checkRollout.Spec.RolloutPlan.RolloutBatches[0].Replicas != intstr.FromInt(2) {
				return fmt.Errorf("fill rollout batches missmatch")
			}
			return nil
		}, 30*time.Second, 300*time.Millisecond).Should(BeNil())
		verifySuccess("express-server-v2")
	})

	It("Delete a component with rollout trait from an application should delete this workload", func() {
		By("first scale operation")
		Expect(common.ReadYamlToObject("testdata/rollout/deployment/multi_comp_app.yaml", &app)).Should(BeNil())
		app.Namespace = namespaceName
		Expect(k8sClient.Create(ctx, &app)).Should(BeNil())
		verifySuccess("express-server-v1")
		componentName = "express-server-another"
		verifySuccess("express-server-another-v1")
		By("delete a component")
		Eventually(func() error {
			checkApp := &v1beta1.Application{}
			if err := k8sClient.Get(ctx, types.NamespacedName{Namespace: namespaceName, Name: app.Name}, checkApp); err != nil {
				return err
			}
			checkApp.Spec.Components = []common2.ApplicationComponent{checkApp.Spec.Components[0]}
			if err := k8sClient.Update(ctx, checkApp); err != nil {
				return err
			}
			return nil
		}, 30*time.Second, 300*time.Millisecond).Should(BeNil())
		By("check deployment have been gc")
		Eventually(func() error {
			checkApp := &v1beta1.Application{}
			if err := k8sClient.Get(ctx, types.NamespacedName{Namespace: namespaceName, Name: app.Name}, checkApp); err != nil {
				return err
			}
			if len(checkApp.Spec.Components) != 1 || checkApp.Spec.Components[0].Name != "express-server" {
				return fmt.Errorf("app hasn't update yet")
			}
			deploy := v1.Deployment{}
			if err := k8sClient.Get(ctx, types.NamespacedName{Namespace: namespaceName, Name: "express-server-another-v1"}, &deploy); err == nil || !apierrors.IsNotFound(err) {
				return fmt.Errorf("another deployment haven't been delete")
			}
			return nil
		}, 30*time.Second, 300*time.Millisecond).Should(BeNil())
	})
})

const (
	rolloutTestWd = `# Code generated by KubeVela templates. DO NOT EDIT.
apiVersion: core.oam.dev/v1beta1
kind: ComponentDefinition
metadata:
  name: webservice
spec:
  workload:
    definition:
      apiVersion: apps/v1
      kind: Deployment
  schematic:
    cue:
      template: |
        output: {
                        	apiVersion: "apps/v1"
                        	kind:       "Deployment"
                        	spec: {
                        		selector: matchLabels: {
                        			"app.oam.dev/component": context.name
                        		}

                        		template: {
                        			metadata: labels: {
                        				"app.oam.dev/component": context.name
                        				}
                        				spec: {
                        					containers: [{
                        						name:  context.name
                                    image: parameter.image
                                    if parameter["cpu"] != _|_ {
                                           resources: {
                                           limits:
                                              cpu: parameter.cpu
                                           requests: cpu: "0"
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

                        	cpu?: 0.5|string
                        }

`
	rolloutTestTd = `apiVersion: core.oam.dev/v1beta1
kind: TraitDefinition
metadata:
  name: rollout
spec:
  manageWorkload: true
  schematic:
    cue:
      template: |
        outputs: rollout: {
                	apiVersion: "standard.oam.dev/v1alpha1"
                	kind:       "Rollout"
                	metadata: {
                		 name:  context.name
                         namespace: context.namespace
                	}
                	spec: {
                           targetRevisionName: parameter.targetRevision
                           componentName: context.name
                           rolloutPlan: {
                           	rolloutStrategy: "DecreaseFirst"
                            if parameter.firstBatchReplicas != _|_ && parameter.secondBatchReplicas != _|_ {
                                rolloutBatches:[
                            	{ replicas: parameter.firstBatchReplicas},
                            	{ replicas: parameter.secondBatchReplicas}]
                            }
                            targetSize: parameter.targetSize
                             if parameter["batchPartition"] != _|_ {
                                 batchPartition:  parameter.batchPartition
                              }
                            }
                		 }
                	}

                 parameter: {
                     targetRevision: *context.revision|string
                     targetSize: *2|int
                     firstBatchReplicas?: int
                     secondBatchReplicas?: int
                     batchPartition?: int
                 }`
)
