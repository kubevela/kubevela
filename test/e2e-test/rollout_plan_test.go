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
	"k8s.io/utils/pointer"
	"sigs.k8s.io/controller-runtime/pkg/client"

	oamcomm "github.com/oam-dev/kubevela/apis/core.oam.dev/common"
	"github.com/oam-dev/kubevela/apis/core.oam.dev/v1alpha2"
	"github.com/oam-dev/kubevela/apis/core.oam.dev/v1beta1"
	oamstd "github.com/oam-dev/kubevela/apis/standard.oam.dev/v1alpha1"
	"github.com/oam-dev/kubevela/apis/types"
	"github.com/oam-dev/kubevela/pkg/controller/utils"
	"github.com/oam-dev/kubevela/pkg/oam/util"
	"github.com/oam-dev/kubevela/pkg/utils/common"
)

var _ = Describe("Cloneset based rollout tests", func() {
	ctx := context.Background()
	var namespace string
	var ns corev1.Namespace
	var app v1beta1.Application
	var kc kruise.CloneSet
	var appRollout v1beta1.AppRollout

	createNamespace := func(namespace string) {
		ns = corev1.Namespace{
			ObjectMeta: metav1.ObjectMeta{
				Name: namespace,
			},
		}
		// delete the namespace with all its resources
		Eventually(
			func() error {
				return k8sClient.Delete(ctx, &ns, client.PropagationPolicy(metav1.DeletePropagationForeground))
			},
			time.Second*120, time.Millisecond*500).Should(SatisfyAny(BeNil(), &util.NotFoundMatcher{}))
		By("make sure all the resources are removed")
		objectKey := client.ObjectKey{
			Name: namespace,
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

	applySourceApp := func() {
		By("Apply an application")
		var newApp v1beta1.Application
		Expect(common.ReadYamlToObject("testdata/rollout/cloneset/app-source.yaml", &newApp)).Should(BeNil())
		newApp.Namespace = namespace
		Expect(k8sClient.Create(ctx, &newApp)).Should(Succeed())

		By("Get Application latest status")
		Eventually(
			func() *oamcomm.Revision {
				k8sClient.Get(ctx, client.ObjectKey{Namespace: namespace, Name: newApp.Name}, &app)
				if app.Status.LatestRevision != nil {
					return app.Status.LatestRevision
				}
				return nil
			},
			time.Second*30, time.Millisecond*500).ShouldNot(BeNil())
	}

	applyTargetApp := func() {
		By("Update the application to target spec during rolling")
		var targetApp v1beta1.Application
		Expect(common.ReadYamlToObject("testdata/rollout/cloneset/app-target.yaml", &targetApp)).Should(BeNil())

		Eventually(
			func() error {
				k8sClient.Get(ctx, client.ObjectKey{Namespace: namespace, Name: app.Name}, &app)
				app.Spec = targetApp.Spec
				return k8sClient.Update(ctx, &app)
			}, time.Second*5, time.Millisecond*500).Should(Succeed())
	}

	createAppRolling := func(newAppRollout *v1beta1.AppRollout) {
		By("Mark the application as rolling")
		Eventually(
			func() error {
				return k8sClient.Create(ctx, newAppRollout)
			}, time.Second*5, time.Millisecond).Should(Succeed())
	}

	verifyRolloutOwnsCloneset := func() {
		By("Verify that rollout controller owns the cloneset")
		clonesetName := appRollout.Spec.ComponentList[0]
		Eventually(
			func() string {
				k8sClient.Get(ctx, client.ObjectKey{Namespace: namespace, Name: clonesetName}, &kc)
				clonesetOwner := metav1.GetControllerOf(&kc)
				if clonesetOwner == nil {
					return ""
				}
				return clonesetOwner.Kind
			}, time.Second*10, time.Second).Should(BeEquivalentTo(v1beta1.AppRolloutKind))
		clonesetOwner := metav1.GetControllerOf(&kc)
		Expect(clonesetOwner.APIVersion).Should(BeEquivalentTo(v1beta1.SchemeGroupVersion.String()))
	}

	verifyRolloutSucceeded := func(targetAppName string) {
		By("Wait for the rollout phase change to succeed")
		Eventually(
			func() oamstd.RollingState {
				k8sClient.Get(ctx, client.ObjectKey{Namespace: namespace, Name: appRollout.Name}, &appRollout)
				return appRollout.Status.RollingState
			},
			time.Second*300, time.Second).Should(Equal(oamstd.RolloutSucceedState))
		Expect(appRollout.Status.UpgradedReadyReplicas).Should(BeEquivalentTo(appRollout.Status.RolloutTargetTotalSize))
		Expect(appRollout.Status.UpgradedReplicas).Should(BeEquivalentTo(appRollout.Status.RolloutTargetTotalSize))

		By("Verify AppContext rolling status")
		var appConfig v1alpha2.ApplicationContext

		Eventually(
			func() types.RollingStatus {
				k8sClient.Get(ctx, client.ObjectKey{Namespace: namespace, Name: targetAppName}, &appConfig)
				return appConfig.Status.RollingStatus
			},
			time.Second*60, time.Second).Should(BeEquivalentTo(types.RollingCompleted))

		By("Wait for AppContext to resume the control of cloneset")
		var clonesetOwner *metav1.OwnerReference
		clonesetName := appRollout.Spec.ComponentList[0]
		Eventually(
			func() string {
				err := k8sClient.Get(ctx, client.ObjectKey{Namespace: namespace, Name: clonesetName}, &kc)
				if err != nil {
					return ""
				}
				clonesetOwner = metav1.GetControllerOf(&kc)
				if clonesetOwner != nil {
					return clonesetOwner.Kind
				}
				return ""
			},
			time.Second*30, time.Millisecond*500).Should(BeEquivalentTo(v1alpha2.ApplicationConfigurationKind))
		Expect(clonesetOwner.Name).Should(BeEquivalentTo(targetAppName))
		Expect(kc.Status.UpdatedReplicas).Should(BeEquivalentTo(*kc.Spec.Replicas))
		Expect(kc.Status.UpdatedReadyReplicas).Should(BeEquivalentTo(*kc.Spec.Replicas))
	}

	verifyAppConfigInactive := func(appConfigName string) {
		var appConfig v1alpha2.ApplicationContext
		By("Verify AppConfig is inactive")
		Eventually(
			func() types.RollingStatus {
				k8sClient.Get(ctx, client.ObjectKey{Namespace: namespace, Name: appConfigName}, &appConfig)
				return appConfig.Status.RollingStatus
			},
			time.Second*30, time.Millisecond*500).Should(BeEquivalentTo(types.InactiveAfterRollingCompleted))
	}

	applyTwoAppVersion := func() {
		CreateClonesetDef()
		applySourceApp()
		applyTargetApp()
	}

	revertBackToSource := func() {
		By("Revert the application back to source")
		var sourceApp v1beta1.Application
		Expect(common.ReadYamlToObject("testdata/rollout/cloneset/app-source.yaml", &sourceApp)).Should(BeNil())

		Eventually(
			func() error {
				k8sClient.Get(ctx, client.ObjectKey{Namespace: namespace, Name: app.Name}, &app)
				app.Spec = sourceApp.Spec
				return k8sClient.Update(ctx, &app)
			},
			time.Second*60, time.Millisecond*500).Should(Succeed())

		By("Modify the application rollout with new target and source")
		Eventually(
			func() error {
				k8sClient.Get(ctx, client.ObjectKey{Namespace: namespace, Name: appRollout.Name}, &appRollout)
				appRollout.Spec.SourceAppRevisionName = utils.ConstructRevisionName(app.GetName(), 2)
				appRollout.Spec.TargetAppRevisionName = utils.ConstructRevisionName(app.GetName(), 3)
				appRollout.Spec.RolloutPlan.BatchPartition = nil
				return k8sClient.Update(ctx, &appRollout)
			},
			time.Second*5, time.Millisecond*500).Should(Succeed())

		By("Wait for the rollout phase change to rolling in batches")
		Eventually(
			func() oamstd.RollingState {
				k8sClient.Get(ctx, client.ObjectKey{Namespace: namespace, Name: appRollout.Name}, &appRollout)
				return appRollout.Status.RollingState
			},
			time.Second*10, time.Millisecond*10).Should(BeEquivalentTo(oamstd.RollingInBatchesState))

		verifyRolloutSucceeded(appRollout.Spec.TargetAppRevisionName)

		verifyAppConfigInactive(appRollout.Spec.SourceAppRevisionName)

		// Clean up
		k8sClient.Delete(ctx, &appRollout)
	}

	BeforeEach(func() {
		By("Start to run a test, clean up previous resources")
		namespace = "rolling-e2e-test" // + "-" + strconv.FormatInt(rand.Int63(), 16)
		createNamespace(namespace)
	})

	AfterEach(func() {
		By("Clean up resources after a test")
		k8sClient.Delete(ctx, &app)
		By(fmt.Sprintf("Delete the entire namespace %s", ns.Name))
		// delete the namespace with all its resources
		Expect(k8sClient.Delete(ctx, &ns, client.PropagationPolicy(metav1.DeletePropagationForeground))).Should(BeNil())
		time.Sleep(15 * time.Second)
	})

	It("Test cloneset rollout first time (no source)", func() {
		CreateClonesetDef()
		applySourceApp()
		By("Apply the application rollout go directly to the target")
		var newAppRollout v1beta1.AppRollout
		Expect(common.ReadYamlToObject("testdata/rollout/cloneset/app-rollout.yaml", &newAppRollout)).Should(BeNil())
		newAppRollout.Namespace = namespace
		newAppRollout.Spec.SourceAppRevisionName = ""
		newAppRollout.Spec.TargetAppRevisionName = utils.ConstructRevisionName(app.GetName(), 1)
		createAppRolling(&newAppRollout)
		appRollout.Name = newAppRollout.Name
		verifyRolloutSucceeded(newAppRollout.Spec.TargetAppRevisionName)
		// Clean up
		k8sClient.Delete(ctx, &appRollout)
	})

	It("Test cloneset rollout with a manual check", func() {
		applyTwoAppVersion()

		By("Apply the application rollout to deploy the source")
		var newAppRollout v1beta1.AppRollout
		Expect(common.ReadYamlToObject("testdata/rollout/cloneset/app-rollout.yaml", &newAppRollout)).Should(BeNil())
		newAppRollout.Namespace = namespace
		newAppRollout.Spec.SourceAppRevisionName = ""
		newAppRollout.Spec.TargetAppRevisionName = utils.ConstructRevisionName(app.GetName(), 1)
		createAppRolling(&newAppRollout)
		appRollout.Name = newAppRollout.Name
		verifyRolloutSucceeded(newAppRollout.Spec.TargetAppRevisionName)

		By("Apply the application rollout that stops after the first batch")
		batchPartition := 0
		Eventually(
			func() error {
				k8sClient.Get(ctx, client.ObjectKey{Namespace: namespace, Name: appRollout.Name}, &appRollout)
				appRollout.Spec.SourceAppRevisionName = utils.ConstructRevisionName(app.GetName(), 1)
				appRollout.Spec.TargetAppRevisionName = utils.ConstructRevisionName(app.GetName(), 2)
				appRollout.Spec.RolloutPlan.BatchPartition = pointer.Int32Ptr(int32(batchPartition))
				return k8sClient.Update(ctx, &appRollout)
			}, time.Second*5, time.Millisecond*500).Should(Succeed())

		By("Wait for the rollout phase change to rolling in batches")
		Eventually(
			func() oamstd.RollingState {
				k8sClient.Get(ctx, client.ObjectKey{Namespace: namespace, Name: newAppRollout.Name}, &appRollout)
				return appRollout.Status.RollingState
			},
			time.Second*60, time.Millisecond*500).Should(BeEquivalentTo(oamstd.RollingInBatchesState))

		By("Wait for rollout to finish one batch")
		Eventually(
			func() int32 {
				k8sClient.Get(ctx, client.ObjectKey{Namespace: namespace, Name: appRollout.Name}, &appRollout)
				return appRollout.Status.CurrentBatch
			},
			time.Second*15, time.Millisecond*500).Should(BeEquivalentTo(batchPartition))

		By("Verify that the rollout stops at the first batch")
		// wait for the batch to be ready
		Eventually(
			func() oamstd.BatchRollingState {
				k8sClient.Get(ctx, client.ObjectKey{Namespace: namespace, Name: appRollout.Name}, &appRollout)
				return appRollout.Status.BatchRollingState
			},
			time.Second*30, time.Millisecond*500).Should(Equal(oamstd.BatchReadyState))
		// wait for 15 seconds, it should stop at 1
		time.Sleep(15 * time.Second)
		k8sClient.Get(ctx, client.ObjectKey{Namespace: namespace, Name: appRollout.Name}, &appRollout)
		Expect(appRollout.Status.RollingState).Should(BeEquivalentTo(oamstd.RollingInBatchesState))
		Expect(appRollout.Status.BatchRollingState).Should(BeEquivalentTo(oamstd.BatchReadyState))
		Expect(appRollout.Status.CurrentBatch).Should(BeEquivalentTo(batchPartition))

		verifyRolloutOwnsCloneset()

		By("Finish the application rollout")
		// set the partition as the same size as the array
		appRollout.Spec.RolloutPlan.BatchPartition = pointer.Int32Ptr(int32(len(appRollout.Spec.RolloutPlan.
			RolloutBatches) - 1))
		Expect(k8sClient.Update(ctx, &appRollout)).Should(Succeed())
		verifyRolloutSucceeded(appRollout.Spec.TargetAppRevisionName)

		verifyAppConfigInactive(appRollout.Spec.SourceAppRevisionName)

		// Clean up
		k8sClient.Delete(ctx, &appRollout)
	})

	It("Test pause and modify rollout plan after rolling succeeded", func() {
		CreateClonesetDef()
		applySourceApp()
		By("Apply the application rollout go directly to the target")
		var newAppRollout v1beta1.AppRollout
		Expect(common.ReadYamlToObject("testdata/rollout/cloneset/app-rollout.yaml", &newAppRollout)).Should(BeNil())
		newAppRollout.Namespace = namespace
		newAppRollout.Spec.SourceAppRevisionName = ""
		newAppRollout.Spec.TargetAppRevisionName = utils.ConstructRevisionName(app.GetName(), 1)
		newAppRollout.Spec.RolloutPlan.BatchPartition = nil
		newAppRollout.Spec.RolloutPlan.RolloutBatches = nil
		// webhook would fill the batches
		newAppRollout.Spec.RolloutPlan.TargetSize = pointer.Int32Ptr(5)
		newAppRollout.Spec.RolloutPlan.NumBatches = pointer.Int32Ptr(5)
		createAppRolling(&newAppRollout)

		By("Wait for the rollout phase change to initialize")
		Eventually(
			func() oamstd.RollingState {
				k8sClient.Get(ctx, client.ObjectKey{Namespace: namespace, Name: newAppRollout.Name}, &appRollout)
				return appRollout.Status.RollingState
			},
			time.Second*10, time.Millisecond).Should(BeEquivalentTo(oamstd.RollingInBatchesState))

		By("Pause the rollout")
		Eventually(
			func() error {
				k8sClient.Get(ctx, client.ObjectKey{Namespace: namespace, Name: appRollout.Name}, &appRollout)
				appRollout.Spec.RolloutPlan.Paused = true
				err := k8sClient.Update(ctx, &appRollout)
				return err
			},
			time.Second*5, time.Millisecond*500).Should(Succeed())
		By("Verify that the rollout pauses")
		Eventually(
			func() corev1.ConditionStatus {
				k8sClient.Get(ctx, client.ObjectKey{Namespace: namespace, Name: appRollout.Name}, &appRollout)
				return appRollout.Status.GetCondition(oamstd.BatchPaused).Status
			},
			time.Second*30, time.Millisecond*500).Should(Equal(corev1.ConditionTrue))

		preBatch := appRollout.Status.CurrentBatch
		// wait for 15 seconds, the batch should not move
		time.Sleep(15 * time.Second)
		k8sClient.Get(ctx, client.ObjectKey{Namespace: namespace, Name: appRollout.Name}, &appRollout)
		Expect(appRollout.Status.RollingState).Should(BeEquivalentTo(oamstd.RollingInBatchesState))
		Expect(appRollout.Status.CurrentBatch).Should(BeEquivalentTo(preBatch))
		k8sClient.Get(ctx, client.ObjectKey{Namespace: namespace, Name: appRollout.Name}, &appRollout)
		lt := appRollout.Status.GetCondition(oamstd.BatchPaused).LastTransitionTime
		beforeSleep := metav1.Time{
			Time: time.Now().Add(-15 * time.Second),
		}
		Expect((&lt).Before(&beforeSleep)).Should(BeTrue())

		verifyRolloutOwnsCloneset()

		By("Finish the application rollout")
		// remove the batch restriction
		appRollout.Spec.RolloutPlan.Paused = false
		Expect(k8sClient.Update(ctx, &appRollout)).Should(Succeed())

		verifyRolloutSucceeded(appRollout.Spec.TargetAppRevisionName)
		// record the transition time
		k8sClient.Get(ctx, client.ObjectKey{Namespace: namespace, Name: appRollout.Name}, &appRollout)
		lt = appRollout.Status.GetCondition(oamstd.RolloutSucceed).LastTransitionTime

		// nothing should happen, the transition time should be the same
		verifyRolloutSucceeded(appRollout.Spec.TargetAppRevisionName)
		k8sClient.Get(ctx, client.ObjectKey{Namespace: namespace, Name: appRollout.Name}, &appRollout)
		Expect(appRollout.Status.RollingState).Should(BeEquivalentTo(oamstd.RolloutSucceedState))
		Expect(appRollout.Status.GetCondition(oamstd.RolloutSucceed).LastTransitionTime).Should(BeEquivalentTo(lt))

		// Clean up
		k8sClient.Delete(ctx, &appRollout)
	})

	It("Test rolling back after a successful rollout", func() {
		applyTwoAppVersion()

		By("Apply the application rollout to deploy the source")
		var newAppRollout v1beta1.AppRollout
		Expect(common.ReadYamlToObject("testdata/rollout/cloneset/app-rollout.yaml", &newAppRollout)).Should(BeNil())
		newAppRollout.Namespace = namespace
		newAppRollout.Spec.SourceAppRevisionName = ""
		newAppRollout.Spec.TargetAppRevisionName = utils.ConstructRevisionName(app.GetName(), 1)
		createAppRolling(&newAppRollout)
		appRollout.Name = newAppRollout.Name
		verifyRolloutSucceeded(newAppRollout.Spec.TargetAppRevisionName)

		By("Finish the application rollout")
		Eventually(
			func() error {
				k8sClient.Get(ctx, client.ObjectKey{Namespace: namespace, Name: appRollout.Name}, &appRollout)
				appRollout.Spec.SourceAppRevisionName = utils.ConstructRevisionName(app.GetName(), 1)
				appRollout.Spec.TargetAppRevisionName = utils.ConstructRevisionName(app.GetName(), 2)
				appRollout.Spec.RolloutPlan.BatchPartition = nil
				return k8sClient.Update(ctx, &appRollout)
			}, time.Second*5, time.Millisecond).Should(Succeed())

		By("Wait for the rollout phase change to rolling in batches")
		Eventually(
			func() oamstd.RollingState {
				k8sClient.Get(ctx, client.ObjectKey{Namespace: namespace, Name: appRollout.Name}, &appRollout)
				return appRollout.Status.RollingState
			},
			time.Second*10, time.Millisecond*10).Should(BeEquivalentTo(oamstd.RollingInBatchesState))

		verifyRolloutSucceeded(appRollout.Spec.TargetAppRevisionName)
		verifyAppConfigInactive(appRollout.Spec.SourceAppRevisionName)

		revertBackToSource()
	})

	It("Test rolling back in the middle of rollout", func() {
		applyTwoAppVersion()

		By("Apply the application rollout to deploy the source")
		var newAppRollout v1beta1.AppRollout
		Expect(common.ReadYamlToObject("testdata/rollout/cloneset/app-rollout.yaml", &newAppRollout)).Should(BeNil())
		newAppRollout.Namespace = namespace
		newAppRollout.Spec.SourceAppRevisionName = ""
		newAppRollout.Spec.TargetAppRevisionName = utils.ConstructRevisionName(app.GetName(), 1)
		createAppRolling(&newAppRollout)
		appRollout.Name = newAppRollout.Name
		verifyRolloutSucceeded(newAppRollout.Spec.TargetAppRevisionName)

		By("Finish the application rollout")
		Eventually(
			func() error {
				k8sClient.Get(ctx, client.ObjectKey{Namespace: namespace, Name: appRollout.Name}, &appRollout)
				appRollout.Spec.SourceAppRevisionName = utils.ConstructRevisionName(app.GetName(), 1)
				appRollout.Spec.TargetAppRevisionName = utils.ConstructRevisionName(app.GetName(), 2)
				appRollout.Spec.RolloutPlan.BatchPartition = nil
				return k8sClient.Update(ctx, &appRollout)
			}, time.Second*5, time.Millisecond*500).Should(Succeed())

		By("Wait for the rollout phase change to rolling in batches")
		Eventually(
			func() oamstd.RollingState {
				k8sClient.Get(ctx, client.ObjectKey{Namespace: namespace, Name: appRollout.Name}, &appRollout)
				return appRollout.Status.RollingState
			},
			time.Second*10, time.Millisecond*10).Should(BeEquivalentTo(oamstd.RollingInBatchesState))

		revertBackToSource()
	})

	PIt("Test rolling by changing the definition", func() {
		CreateClonesetDef()
		applySourceApp()
		By("Apply the definition change")
		var cd, newCD v1beta1.ComponentDefinition
		Expect(common.ReadYamlToObject("testdata/rollout/cloneset/clonesetDefinitionModified.yaml.yaml", &newCD)).Should(BeNil())
		Eventually(
			func() error {
				k8sClient.Get(ctx, client.ObjectKey{Namespace: namespace, Name: newCD.Name}, &cd)
				cd.Spec = newCD.Spec
				return k8sClient.Update(ctx, &cd)
			},
			time.Second*3, time.Millisecond*300).Should(Succeed())
		By("Apply the application rollout")
		var newAppRollout v1beta1.AppRollout
		Expect(common.ReadYamlToObject("testdata/rollout/cloneset/app-rollout.yaml", &newAppRollout)).Should(BeNil())
		newAppRollout.Namespace = namespace
		newAppRollout.Spec.RolloutPlan.BatchPartition = pointer.Int32Ptr(int32(len(newAppRollout.Spec.RolloutPlan.
			RolloutBatches) - 1))
		createAppRolling(&newAppRollout)

		verifyRolloutSucceeded(appRollout.Spec.TargetAppRevisionName)
		verifyAppConfigInactive(appRollout.Spec.SourceAppRevisionName)
		// Clean up
		k8sClient.Delete(ctx, &appRollout)
	})
})
