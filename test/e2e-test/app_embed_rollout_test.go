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
	"fmt"
	"time"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	kruise "github.com/openkruise/kruise-api/apps/v1alpha1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	ctypes "k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/utils/pointer"
	"sigs.k8s.io/controller-runtime/pkg/client"

	apicommon "github.com/oam-dev/kubevela/apis/core.oam.dev/common"
	"github.com/oam-dev/kubevela/apis/core.oam.dev/v1beta1"
	"github.com/oam-dev/kubevela/apis/standard.oam.dev/v1alpha1"
	"github.com/oam-dev/kubevela/pkg/controller/utils"
	"github.com/oam-dev/kubevela/pkg/oam/util"
	"github.com/oam-dev/kubevela/pkg/utils/common"
)

var _ = Describe("rollout related e2e-test,Cloneset based app embed rollout tests", func() {
	ctx := context.Background()
	var namespaceName string
	var ns corev1.Namespace
	var kc kruise.CloneSet
	var app v1beta1.Application
	var appName string
	initialProperty := `{"cmd":["./podinfo","stress-cpu=1"],"image":"stefanprodan/podinfo:4.0.3","port":8080,"replicas":6}`

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

	CreateClonesetDef := func() {
		By("Install CloneSet based componentDefinition")
		var cd v1beta1.ComponentDefinition
		Expect(common.ReadYamlToObject("testdata/rollout/cloneset/clonesetDefinition.yaml", &cd)).Should(BeNil())
		// create the componentDefinition if not exist
		Eventually(
			func() error {
				return k8sClient.Create(ctx, &cd)
			},
			time.Second*3, time.Millisecond*300).Should(SatisfyAny(BeNil(), &util.AlreadyExistMatcher{}))
	}

	CreateIngressDef := func() {
		By("Install Ingress trait definition")
		var td v1beta1.TraitDefinition
		Expect(common.ReadYamlToObject("testdata/rollout/cloneset/ingressDefinition.yaml", &td)).Should(BeNil())
		// create the traitDefinition if not exist
		Eventually(
			func() error {
				return k8sClient.Create(ctx, &td)
			},
			time.Second*3, time.Millisecond*300).Should(SatisfyAny(BeNil(), &util.AlreadyExistMatcher{}))
	}

	generateNewApp := func(appName, namespace, compType string, plan *v1alpha1.RolloutPlan) *v1beta1.Application {
		return &v1beta1.Application{
			TypeMeta: metav1.TypeMeta{
				Kind:       "Application",
				APIVersion: "core.oam.dev/v1beta1",
			},
			ObjectMeta: metav1.ObjectMeta{
				Name:      appName,
				Namespace: namespace,
			},
			Spec: v1beta1.ApplicationSpec{
				Components: []apicommon.ApplicationComponent{
					{
						Name: appName,
						Type: compType,
						Properties: &runtime.RawExtension{
							Raw: []byte(initialProperty),
						},
					},
				},
				RolloutPlan: plan,
			},
		}
	}

	BeforeEach(func() {
		By("Start to run a test, clean up previous resources")
		namespaceName = randomNamespaceName("app-rollout-e2e-test")
		createNamespace()
		CreateClonesetDef()
		CreateIngressDef()
	})

	AfterEach(func() {
		By("Clean up resources after a test")
		k8sClient.DeleteAllOf(ctx, &v1beta1.Application{}, client.InNamespace(namespaceName))
		k8sClient.DeleteAllOf(ctx, &v1beta1.ComponentDefinition{}, client.InNamespace(namespaceName))
		k8sClient.DeleteAllOf(ctx, &v1beta1.WorkloadDefinition{}, client.InNamespace(namespaceName))
		k8sClient.DeleteAllOf(ctx, &v1beta1.TraitDefinition{}, client.InNamespace(namespaceName))

		By(fmt.Sprintf("Delete the entire namespaceName %s", ns.Name))
		// delete the namespaceName with all its resources
		Expect(k8sClient.Delete(ctx, &ns, client.PropagationPolicy(metav1.DeletePropagationBackground))).Should(BeNil())
	})

	verifyRolloutSucceeded := func(targetAppRevisionName string, cpu string) {
		By("verify application status")
		Eventually(
			func() error {
				app = v1beta1.Application{}
				if err := k8sClient.Get(ctx, client.ObjectKey{Namespace: namespaceName, Name: appName}, &app); err != nil {
					return err
				}
				if app.Status.Rollout == nil {
					return fmt.Errorf("application is under creating, app status rollout is nil, %v", app.Status)
				}
				if app.Status.Rollout.LastUpgradedTargetAppRevision != app.Status.LatestRevision.Name {
					return fmt.Errorf("rollout controller haven't handle this change, targetRevision isn't right")
				}
				if app.Status.Rollout.RollingState != v1alpha1.RolloutSucceedState {
					return fmt.Errorf("app status rollingStatus not succeed acctually %s", app.Status.Rollout.RollingState)
				}
				if app.Status.Phase != apicommon.ApplicationRunning {
					return fmt.Errorf("app status not running acctually %s", app.Status.Phase)
				}
				return nil
			},
			time.Second*120, time.Second).Should(BeNil())
		Expect(app.Status.Rollout.UpgradedReadyReplicas).Should(BeEquivalentTo(app.Status.Rollout.RolloutTargetSize))
		Expect(app.Status.Rollout.UpgradedReplicas).Should(BeEquivalentTo(app.Status.Rollout.RolloutTargetSize))
		clonesetName := app.Spec.Components[0].Name

		By("Verify cloneset  status")
		var clonesetOwner *metav1.OwnerReference
		Eventually(
			func() error {
				if err := k8sClient.Get(ctx, client.ObjectKey{Namespace: namespaceName, Name: clonesetName}, &kc); err != nil {
					return err
				}
				clonesetOwner = metav1.GetControllerOf(&kc)
				if clonesetOwner == nil {
					return fmt.Errorf("cloneset don't have any controller owner")
				}
				if clonesetOwner.Kind != v1beta1.ResourceTrackerKind {
					return fmt.Errorf("cloneset owner mismatch wants %s actually  %s", v1beta1.ResourceTrackerKind, clonesetOwner.Kind)
				}
				if kc.Status.UpdatedReplicas != *kc.Spec.Replicas {
					return fmt.Errorf("upgraded pod number error")
				}
				resourceTrackerName := fmt.Sprintf("%s-%s", targetAppRevisionName, app.Namespace)
				if clonesetOwner.Name != resourceTrackerName {
					return fmt.Errorf("resourceTracker haven't take back controller owner")
				}
				return nil
			},
			time.Second*30, time.Millisecond*500).Should(BeNil())
		By("Verify  pod status")
		Eventually(func() error {
			podList := corev1.PodList{}
			if err := k8sClient.List(ctx, &podList, client.MatchingLabels(kc.Spec.Template.Labels), client.InNamespace(namespaceName)); err != nil {
				return err
			}
			if len(podList.Items) != int(*kc.Spec.Replicas) {
				return fmt.Errorf("pod number error")
			}
			for _, pod := range podList.Items {
				if pod.Status.Phase != corev1.PodRunning {
					return fmt.Errorf("pod status error %s", pod.Status.Phase)
				}
				if pod.Spec.Containers[0].Command[1] != fmt.Sprintf("stress-cpu=%s", cpu) {
					return fmt.Errorf("pod cmmond haven't updated")
				}
			}
			return nil
		}, time.Second*120, time.Microsecond).Should(BeNil())
	}

	updateAppWithCpuAndPlan := func(app *v1beta1.Application, cpu string, plan *v1alpha1.RolloutPlan) {
		Eventually(func() error {
			checkApp := new(v1beta1.Application)
			if err := k8sClient.Get(ctx, ctypes.NamespacedName{Namespace: namespaceName, Name: app.Name}, checkApp); err != nil {
				return err
			}
			updateProperty := fmt.Sprintf(`{"cmd":["./podinfo","stress-cpu=%s"],"image":"stefanprodan/podinfo:4.0.3","port":8080,"replicas":6}`, cpu)
			checkApp.Spec.Components[0].Properties.Raw = []byte(updateProperty)
			checkApp.Spec.RolloutPlan = plan
			if err := k8sClient.Update(ctx, checkApp); err != nil {
				return err
			}
			return nil
		}, time.Second*30, time.Microsecond*300).Should(BeNil())
	}

	It("Test upgrade application", func() {
		plan := &v1alpha1.RolloutPlan{
			RolloutStrategy: v1alpha1.IncreaseFirstRolloutStrategyType,
			RolloutBatches: []v1alpha1.RolloutBatch{
				{
					Replicas: intstr.FromString("50%"),
				},
				{
					Replicas: intstr.FromString("50%"),
				},
			},
			TargetSize: pointer.Int32Ptr(6),
		}
		appName = "app-rollout-1"
		app := generateNewApp(appName, namespaceName, "clonesetservice", plan)
		Expect(k8sClient.Create(ctx, app)).Should(BeNil())
		verifyRolloutSucceeded(utils.ConstructRevisionName(appName, 1), "1")
		updateAppWithCpuAndPlan(app, "2", plan)
		verifyRolloutSucceeded(utils.ConstructRevisionName(appName, 2), "2")
		updateAppWithCpuAndPlan(app, "3", plan)
		verifyRolloutSucceeded(utils.ConstructRevisionName(appName, 3), "3")
	})

	It("Test application only upgrade batchPartition", func() {
		plan := &v1alpha1.RolloutPlan{
			RolloutStrategy: v1alpha1.IncreaseFirstRolloutStrategyType,
			RolloutBatches: []v1alpha1.RolloutBatch{
				{
					Replicas: intstr.FromString("50%"),
				},
				{
					Replicas: intstr.FromString("50%"),
				},
			},
			TargetSize: pointer.Int32Ptr(6),
		}
		appName = "app-roll-out-2"
		app := generateNewApp(appName, namespaceName, "clonesetservice", plan)
		Expect(k8sClient.Create(ctx, app)).Should(BeNil())
		verifyRolloutSucceeded(utils.ConstructRevisionName(appName, 1), "1")
		app.Spec.RolloutPlan.BatchPartition = pointer.Int32Ptr(0)
		plan = &v1alpha1.RolloutPlan{
			RolloutStrategy: v1alpha1.IncreaseFirstRolloutStrategyType,
			RolloutBatches: []v1alpha1.RolloutBatch{
				{
					Replicas: intstr.FromString("50%"),
				},
				{
					Replicas: intstr.FromString("50%"),
				},
			},
			TargetSize:     pointer.Int32Ptr(6),
			BatchPartition: pointer.Int32Ptr(0),
		}
		updateAppWithCpuAndPlan(app, "2", plan)

		By("upgrade first batch partition, verify the middle state")
		// give controller some time to upgrade one batch
		time.Sleep(15 * time.Second)
		Eventually(func() error {
			checkApp := new(v1beta1.Application)
			if err := k8sClient.Get(ctx, ctypes.NamespacedName{Name: appName, Namespace: namespaceName}, checkApp); err != nil {
				return err
			}
			if checkApp.Status.Rollout.LastUpgradedTargetAppRevision != utils.ConstructRevisionName(appName, 2) {
				return fmt.Errorf("app status lastTargetRevision mismatch")
			}
			if checkApp.Status.Rollout.LastSourceAppRevision != utils.ConstructRevisionName(appName, 1) {
				return fmt.Errorf("app status lastSourceRevision mismatch")
			}
			if checkApp.Status.Rollout.RollingState != v1alpha1.RollingInBatchesState {
				return fmt.Errorf("app status rolling state mismatch")
			}
			if checkApp.Status.Rollout.UpgradedReplicas != 3 || checkApp.Status.Rollout.UpgradedReadyReplicas != 3 {
				return fmt.Errorf("app status upgraded status error")
			}
			if checkApp.Status.Phase != apicommon.ApplicationRollingOut {
				return fmt.Errorf("app status phase error")
			}
			return nil
		}, time.Second*120, time.Microsecond*300).Should(BeNil())
		clonesetName := app.Spec.Components[0].Name
		Eventually(
			func() error {
				if err := k8sClient.Get(ctx, client.ObjectKey{Namespace: namespaceName, Name: clonesetName}, &kc); err != nil {
					return err
				}
				if kc.Status.UpdatedReplicas != 3 {
					return fmt.Errorf("upgraded pod number error")
				}
				return nil
			},
			time.Second*120, time.Millisecond*500).Should(BeNil())
		By("Verify rollout first batch  pod status")
		Eventually(func() error {
			podList := corev1.PodList{}
			if err := k8sClient.List(ctx, &podList, client.MatchingLabels(kc.Spec.Template.Labels), client.InNamespace(namespaceName)); err != nil {
				return err
			}
			if len(podList.Items) != int(*kc.Spec.Replicas) {
				return fmt.Errorf("pod number error %d", len(podList.Items))
			}
			middlePodRes := map[string]int{}
			for _, pod := range podList.Items {
				if pod.Spec.Containers[0].Command[1] == fmt.Sprintf("stress-cpu=%d", 1) {
					middlePodRes[utils.ConstructRevisionName(appName, 1)]++
				}
				if pod.Spec.Containers[0].Command[1] == fmt.Sprintf("stress-cpu=%d", 1) {
					middlePodRes[utils.ConstructRevisionName(appName, 2)]++
				}
				Expect(pod.Status.Phase).Should(Equal(corev1.PodRunning))
			}
			if middlePodRes[utils.ConstructRevisionName(appName, 1)] != 3 {
				return fmt.Errorf("revison-1 pod number error ")
			}
			if middlePodRes[utils.ConstructRevisionName(appName, 2)] != 3 {
				return fmt.Errorf("revison-2 pod number error")
			}
			return nil
		}, time.Second*30, time.Microsecond*300).Should(BeNil())

		By("continue rollout next partition and verify status")
		checkApp := new(v1beta1.Application)
		Expect(k8sClient.Get(ctx, ctypes.NamespacedName{Namespace: namespaceName, Name: appName}, checkApp)).Should(BeNil())
		plan = checkApp.Spec.RolloutPlan
		plan.BatchPartition = pointer.Int32Ptr(1)
		updateAppWithCpuAndPlan(app, "2", plan)
		verifyRolloutSucceeded(utils.ConstructRevisionName(appName, 2), "2")
		By("update again continue rollout to revision-3")
		updateAppWithCpuAndPlan(app, "3", plan)
		verifyRolloutSucceeded(utils.ConstructRevisionName(appName, 3), "3")
	})

	It("Test upgrade application in middle of  rolling out", func() {
		plan := &v1alpha1.RolloutPlan{
			RolloutStrategy: v1alpha1.IncreaseFirstRolloutStrategyType,
			RolloutBatches: []v1alpha1.RolloutBatch{
				{
					Replicas: intstr.FromString("50%"),
				},
				{
					Replicas: intstr.FromString("50%"),
				},
			},
			TargetSize: pointer.Int32Ptr(6),
		}
		appName = "app-rollout-3"
		app := generateNewApp(appName, namespaceName, "clonesetservice", plan)
		Expect(k8sClient.Create(ctx, app)).Should(BeNil())
		verifyRolloutSucceeded(utils.ConstructRevisionName(appName, 1), "1")
		updateAppWithCpuAndPlan(app, "2", plan)

		By("Wait for the rollout phase change to rolling in batches")
		Eventually(func() error {
			checkApp := new(v1beta1.Application)
			if err := k8sClient.Get(ctx, ctypes.NamespacedName{Name: appName, Namespace: namespaceName}, checkApp); err != nil {
				return err
			}
			if checkApp.Status.Rollout.LastUpgradedTargetAppRevision != utils.ConstructRevisionName(appName, 2) {
				return fmt.Errorf("app status lastTargetRevision mismatch actually %s ", checkApp.Status.Rollout.LastUpgradedTargetAppRevision)
			}
			if checkApp.Status.Rollout.LastSourceAppRevision != utils.ConstructRevisionName(appName, 1) {
				return fmt.Errorf("app status lastSourceRevision mismatch actually %s ", checkApp.Status.Rollout.LastSourceAppRevision)
			}
			if checkApp.Status.Rollout.RollingState != v1alpha1.RollingInBatchesState {
				return fmt.Errorf("app status rolling state mismatch")
			}
			return nil
		}, time.Second*60, time.Microsecond*300).Should(BeNil())

		By("update app in middle of rollout and verify status")
		updateAppWithCpuAndPlan(app, "3", plan)
		verifyRolloutSucceeded(utils.ConstructRevisionName(appName, 3), "3")
	})

	It("Test pause  in middle of embed app rolling out", func() {
		plan := &v1alpha1.RolloutPlan{
			RolloutStrategy: v1alpha1.IncreaseFirstRolloutStrategyType,
			RolloutBatches: []v1alpha1.RolloutBatch{
				{
					Replicas: intstr.FromString("50%"),
				},
				{
					Replicas: intstr.FromString("50%"),
				},
			},
			TargetSize: pointer.Int32Ptr(6),
		}
		appName = "app-rollout-4"
		app := generateNewApp(appName, namespaceName, "clonesetservice", plan)
		Expect(k8sClient.Create(ctx, app)).Should(BeNil())
		verifyRolloutSucceeded(utils.ConstructRevisionName(appName, 1), "1")
		updateAppWithCpuAndPlan(app, "2", plan)

		By("Wait for rollout phase change to rolling in batches")
		checkApp := new(v1beta1.Application)
		Eventually(func() error {
			if err := k8sClient.Get(ctx, ctypes.NamespacedName{Name: appName, Namespace: namespaceName}, checkApp); err != nil {
				return err
			}
			if checkApp.Status.Rollout.LastUpgradedTargetAppRevision != utils.ConstructRevisionName(appName, 2) {
				return fmt.Errorf("app status lastTargetRevision mismatch actually %s ", checkApp.Status.Rollout.LastUpgradedTargetAppRevision)
			}
			if checkApp.Status.Rollout.LastSourceAppRevision != utils.ConstructRevisionName(appName, 1) {
				return fmt.Errorf("app status lastSourceRevision mismatch actually %s ", checkApp.Status.Rollout.LastSourceAppRevision)
			}
			if checkApp.Status.Rollout.RollingState != v1alpha1.RollingInBatchesState {
				return fmt.Errorf("app status rolling state mismatch")
			}
			return nil
		}, time.Second*60, time.Microsecond*300).Should(BeNil())

		By("pause app in middle of rollout and verify status")
		plan.Paused = true
		updateAppWithCpuAndPlan(app, "2", plan)
		By("verify update rolloutPlan shouldn't create new revision")
		Expect(k8sClient.Get(ctx, ctypes.NamespacedName{Name: appName, Namespace: namespaceName}, checkApp)).Should(BeNil())
		Expect(checkApp.Status.LatestRevision.Name).Should(BeEquivalentTo(utils.ConstructRevisionName(appName, 2)))
		By("Verify that the app rollout pauses")
		Eventually(func() error {
			if err := k8sClient.Get(ctx, ctypes.NamespacedName{Name: appName, Namespace: namespaceName}, checkApp); err != nil {
				return err
			}
			if checkApp.Status.Rollout.GetCondition(v1alpha1.BatchPaused).Status != corev1.ConditionTrue {
				return fmt.Errorf("rollout status not paused")
			}
			return nil
		}, time.Second*30, time.Microsecond*300).Should(BeNil())
		preBatch := checkApp.Status.Rollout.CurrentBatch
		sleepTime := 10 * time.Second
		time.Sleep(sleepTime)
		Expect(k8sClient.Get(ctx, ctypes.NamespacedName{Name: appName, Namespace: namespaceName}, checkApp)).Should(BeNil())
		Expect(checkApp.Status.Rollout.RollingState).Should(BeEquivalentTo(v1alpha1.RollingInBatchesState))
		Expect(checkApp.Status.Rollout.CurrentBatch).Should(BeEquivalentTo(preBatch))
		transitTime := checkApp.Status.Rollout.GetCondition(v1alpha1.BatchPaused).LastTransitionTime
		beforeSleep := metav1.Time{
			Time: time.Now().Add(sleepTime),
		}
		Expect(transitTime.Before(&beforeSleep)).Should(BeTrue())
		By("continue rollout and verify status ")
		plan.Paused = false
		updateAppWithCpuAndPlan(app, "2", plan)
		By("verify update rolloutPlan shouldn't create new revision")
		Expect(k8sClient.Get(ctx, ctypes.NamespacedName{Name: appName, Namespace: namespaceName}, checkApp)).Should(BeNil())
		Expect(checkApp.Status.LatestRevision.Name).Should(BeEquivalentTo(utils.ConstructRevisionName(appName, 2)))
		verifyRolloutSucceeded(utils.ConstructRevisionName(appName, 2), "2")
	})

	It("Test rollout with trait", func() {
		plan := &v1alpha1.RolloutPlan{
			RolloutStrategy: v1alpha1.IncreaseFirstRolloutStrategyType,
			RolloutBatches: []v1alpha1.RolloutBatch{
				{
					Replicas: intstr.FromString("50%"),
				},
				{
					Replicas: intstr.FromString("50%"),
				},
			},
			TargetSize: pointer.Int32Ptr(6),
		}
		appName = "app-rollout-5"
		app := generateNewApp(appName, namespaceName, "clonesetservice", plan)
		ingressProperties := `{"domain":"test-1.example.com","http":{"/":8080}}`
		app.Spec.Components[0].Traits = []apicommon.ApplicationTrait{{Type: "ingress", Properties: &runtime.RawExtension{Raw: []byte(ingressProperties)}}}
		Expect(k8sClient.Create(ctx, app)).Should(BeNil())
		verifyRolloutSucceeded(utils.ConstructRevisionName(appName, 1), "1")
		updateAppWithCpuAndPlan(app, "2", plan)
		By("rollout to v2")
		checkApp := new(v1beta1.Application)
		verifyRolloutSucceeded(utils.ConstructRevisionName(appName, 2), "2")
		Expect(k8sClient.Get(ctx, ctypes.NamespacedName{Name: appName, Namespace: namespaceName}, checkApp)).Should(BeNil())
		Expect(checkApp.Status.LatestRevision.Name).Should(BeEquivalentTo(utils.ConstructRevisionName(appName, 2)))
		updateAppWithCpuAndPlan(app, "3", plan)
		By("rollout to v3")
		verifyRolloutSucceeded(utils.ConstructRevisionName(appName, 3), "3")
		Expect(k8sClient.Get(ctx, ctypes.NamespacedName{Name: appName, Namespace: namespaceName}, checkApp)).Should(BeNil())
		Expect(checkApp.Status.LatestRevision.Name).Should(BeEquivalentTo(utils.ConstructRevisionName(appName, 3)))
	})

	It("Test rollout with another component only rollout first component", func() {
		plan := &v1alpha1.RolloutPlan{
			RolloutStrategy: v1alpha1.IncreaseFirstRolloutStrategyType,
			RolloutBatches: []v1alpha1.RolloutBatch{
				{
					Replicas: intstr.FromString("50%"),
				},
				{
					Replicas: intstr.FromString("50%"),
				},
			},
			TargetSize: pointer.Int32Ptr(6),
		}
		appName = "app-rollout-6"
		app := generateNewApp(appName, namespaceName, "clonesetservice", plan)
		annotherComp := apicommon.ApplicationComponent{
			Name: "another-comp",
			Type: "clonesetservice",
			Properties: &runtime.RawExtension{
				Raw: []byte(initialProperty),
			},
		}
		app.Spec.Components = append(app.Spec.Components, annotherComp)
		Expect(k8sClient.Create(ctx, app)).Should(BeNil())
		verifyRolloutSucceeded(utils.ConstructRevisionName(appName, 1), "1")
		updateAppWithCpuAndPlan(app, "2", plan)

		checkApp := new(v1beta1.Application)
		By("verify update rolloutPlan shouldn't create new revision")
		verifyRolloutSucceeded(utils.ConstructRevisionName(appName, 2), "2")
		Expect(k8sClient.Get(ctx, ctypes.NamespacedName{Name: appName, Namespace: namespaceName}, checkApp)).Should(BeNil())
		Expect(checkApp.Status.LatestRevision.Name).Should(BeEquivalentTo(utils.ConstructRevisionName(appName, 2)))
		updateAppWithCpuAndPlan(app, "3", plan)
		verifyRolloutSucceeded(utils.ConstructRevisionName(appName, 3), "3")
		Expect(k8sClient.Get(ctx, ctypes.NamespacedName{Name: appName, Namespace: namespaceName}, checkApp)).Should(BeNil())
		Expect(checkApp.Status.LatestRevision.Name).Should(BeEquivalentTo(utils.ConstructRevisionName(appName, 3)))
	})

	//  TODO add more corner case tests
	//  update application by clean rolloutPlan strategy in the middle of rollout process
})
