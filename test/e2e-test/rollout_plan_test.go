package controllers_test

import (
	"context"
	"fmt"
	"strconv"
	"time"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"

	cpv1 "github.com/crossplane/crossplane-runtime/apis/core/v1alpha1"
	kruise "github.com/openkruise/kruise-api/apps/v1alpha1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/utils/pointer"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/oam-dev/kubevela/apis/core.oam.dev/v1alpha2"
	oamstd "github.com/oam-dev/kubevela/apis/standard.oam.dev/v1alpha1"
	"github.com/oam-dev/kubevela/pkg/controller/utils"
	"github.com/oam-dev/kubevela/pkg/oam"
	"github.com/oam-dev/kubevela/pkg/oam/util"
)

var _ = Describe("Cloneset based rollout tests", func() {
	ctx := context.Background()
	var namespace, clonesetName string
	var ns corev1.Namespace
	var app v1alpha2.Application
	var appConfig1, appConfig2 v1alpha2.ApplicationConfiguration
	var kc kruise.CloneSet
	var appRollout v1alpha2.AppRollout

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
		By("Install CloneSet based workloadDefinition")
		var cd v1alpha2.WorkloadDefinition
		Expect(readYaml("testdata/rollout/cloneset/clonesetDefinition.yaml", &cd)).Should(BeNil())
		// create the workloadDefinition if not exist
		Eventually(
			func() error {
				return k8sClient.Create(ctx, &cd)
			},
			time.Second*3, time.Millisecond*300).Should(SatisfyAny(BeNil(), &util.AlreadyExistMatcher{}))
	}

	VerifyAppConfigTemplated := func(revision int64) {
		var appConfigName string
		By("Get Application latest status after AppConfig created")
		Eventually(
			func() int64 {
				k8sClient.Get(ctx, client.ObjectKey{Namespace: namespace, Name: app.Name}, &app)
				return app.Status.LatestRevision.Revision
			},
			time.Second*30, time.Millisecond*500).Should(BeEquivalentTo(revision))
		appConfigName = app.Status.LatestRevision.Name
		By(fmt.Sprintf("Wait for AppConfig %s synced", appConfigName))
		var appConfig v1alpha2.ApplicationConfiguration
		Eventually(
			func() corev1.ConditionStatus {
				k8sClient.Get(ctx, client.ObjectKey{Namespace: namespace, Name: appConfigName}, &appConfig)
				return appConfig.Status.GetCondition(cpv1.TypeSynced).Status
			},
			time.Second*30, time.Millisecond*500).Should(BeEquivalentTo(corev1.ConditionTrue))

		By(fmt.Sprintf("Wait for AppConfig %s to be templated", appConfigName))
		Eventually(
			func() v1alpha2.RollingStatus {
				k8sClient.Get(ctx, client.ObjectKey{Namespace: namespace, Name: appConfigName}, &appConfig)
				return appConfig.Status.RollingStatus
			},
			time.Second*60, time.Millisecond*500).Should(BeEquivalentTo(v1alpha2.RollingTemplated))
	}

	ApplySourceApp := func() {
		By("Apply an application")
		var newApp v1alpha2.Application
		Expect(readYaml("testdata/rollout/cloneset/app-source.yaml", &newApp)).Should(BeNil())
		newApp.Namespace = namespace
		Expect(k8sClient.Create(ctx, &newApp)).Should(Succeed())

		By("Get Application latest status after AppConfig created")
		Eventually(
			func() *v1alpha2.Revision {
				k8sClient.Get(ctx, client.ObjectKey{Namespace: namespace, Name: newApp.Name}, &app)
				return app.Status.LatestRevision
			},
			time.Second*30, time.Millisecond*500).ShouldNot(BeNil())
		By("Wait for AppConfig1 synced")
		Eventually(
			func() corev1.ConditionStatus {
				k8sClient.Get(ctx, client.ObjectKey{Namespace: namespace, Name: app.Status.LatestRevision.Name}, &appConfig1)
				return appConfig1.Status.GetCondition(cpv1.TypeSynced).Status
			},
			time.Second*30, time.Millisecond*500).Should(BeEquivalentTo(corev1.ConditionTrue))
	}

	MarkAppRolling := func(revision int64) {
		By("Mark the application as rolling")
		Eventually(
			func() error {
				k8sClient.Get(ctx, client.ObjectKey{Namespace: namespace, Name: app.Name}, &app)
				app.SetAnnotations(util.MergeMapOverrideWithDst(app.GetAnnotations(),
					map[string]string{oam.AnnotationRollingComponent: app.Spec.Components[0].Name,
						oam.AnnotationAppRollout: strconv.FormatBool(true)}))
				return k8sClient.Update(ctx, &app)
			}, time.Second*5, time.Millisecond*500).Should(Succeed())

		VerifyAppConfigTemplated(revision)
	}

	ApplyTargetApp := func() {
		By("Update the application to target spec during rolling")
		var targetApp v1alpha2.Application
		Expect(readYaml("testdata/rollout/cloneset/app-target.yaml", &targetApp)).Should(BeNil())

		Eventually(
			func() error {
				k8sClient.Get(ctx, client.ObjectKey{Namespace: namespace, Name: app.Name}, &app)
				app.Spec = targetApp.Spec
				return k8sClient.Update(ctx, &app)
			}, time.Second*5, time.Millisecond*500).Should(Succeed())

		VerifyAppConfigTemplated(2)

		By("Remove the application rolling annotation")
		Eventually(
			func() error {
				k8sClient.Get(ctx, client.ObjectKey{Namespace: namespace, Name: app.Name}, &app)
				util.RemoveAnnotations(&app, []string{oam.AnnotationAppRollout})
				return k8sClient.Update(ctx, &app)
			}, time.Second*5, time.Millisecond*500).Should(Succeed())

		Eventually(
			func() corev1.ConditionStatus {
				k8sClient.Get(ctx, client.ObjectKey{Namespace: namespace, Name: app.Status.LatestRevision.Name}, &appConfig2)
				return appConfig2.Status.GetCondition(cpv1.TypeSynced).Status
			},
			time.Second*30, time.Millisecond*500).Should(BeEquivalentTo(corev1.ConditionTrue))
	}

	VerifyCloneSetPaused := func() {
		By("Get the cloneset workload and make sure it's paused")
		clonesetName = utils.ExtractComponentName(appConfig2.Spec.Components[0].RevisionName)
		Expect(k8sClient.Get(ctx, client.ObjectKey{Namespace: namespace, Name: clonesetName},
			&kc)).ShouldNot(HaveOccurred())
		Expect(kc.Spec.UpdateStrategy.Paused).Should(BeTrue())
	}

	VerifyRolloutOwnsCloneset := func() {
		By("Verify that rollout controller owns the cloneset")
		Eventually(
			func() string {
				k8sClient.Get(ctx, client.ObjectKey{Namespace: namespace, Name: clonesetName}, &kc)
				clonesetOwner := metav1.GetControllerOf(&kc)
				if clonesetOwner == nil {
					return ""
				}
				return clonesetOwner.Kind
			}, time.Second*10, time.Second).Should(BeEquivalentTo(v1alpha2.AppRolloutKind))
		clonesetOwner := metav1.GetControllerOf(&kc)
		Expect(clonesetOwner.APIVersion).Should(BeEquivalentTo(v1alpha2.SchemeGroupVersion.String()))
	}

	VerifyRolloutSucceeded := func() {
		By("Wait for the rollout phase change to succeed")
		Eventually(
			func() oamstd.RollingState {
				k8sClient.Get(ctx, client.ObjectKey{Namespace: namespace, Name: appRollout.Name}, &appRollout)
				return appRollout.Status.RollingState
			},
			time.Second*240, time.Second).Should(Equal(oamstd.RolloutSucceedState))
		Expect(appRollout.Status.UpgradedReadyReplicas).Should(BeEquivalentTo(appRollout.Status.RolloutTargetSize))
		Expect(appRollout.Status.UpgradedReplicas).Should(BeEquivalentTo(appRollout.Status.RolloutTargetSize))
	}

	VerifyAppConfigRollingCompleted := func(appConfigName string) {
		By("Wait for AppConfig2 to resume the control of cloneset")
		var clonesetOwner *metav1.OwnerReference
		Eventually(
			func() string {
				k8sClient.Get(ctx, client.ObjectKey{Namespace: namespace, Name: clonesetName}, &kc)
				clonesetOwner = metav1.GetControllerOf(&kc)
				if clonesetOwner != nil {
					return clonesetOwner.Kind
				}
				return ""
			},
			time.Second*5, time.Millisecond*500).Should(BeEquivalentTo(v1alpha2.ApplicationConfigurationKind))
		Expect(clonesetOwner.Name).Should(BeEquivalentTo(appConfigName))
		Expect(kc.Status.UpdatedReplicas).Should(BeEquivalentTo(*kc.Spec.Replicas))
		Expect(kc.Status.UpdatedReadyReplicas).Should(BeEquivalentTo(*kc.Spec.Replicas))

		By("Verify AppConfig rolling status")
		var appConfig v1alpha2.ApplicationConfiguration

		Eventually(
			func() v1alpha2.RollingStatus {
				k8sClient.Get(ctx, client.ObjectKey{Namespace: namespace, Name: appConfigName}, &appConfig)
				return appConfig.Status.RollingStatus
			},
			time.Second*30, time.Millisecond*500).Should(BeEquivalentTo(v1alpha2.RollingCompleted))
	}

	VerifyAppConfigInactive := func(appConfigName string) {
		By("Verify AppConfig is inactive")
		var appConfig v1alpha2.ApplicationConfiguration

		Eventually(
			func() v1alpha2.RollingStatus {
				k8sClient.Get(ctx, client.ObjectKey{Namespace: namespace, Name: appConfigName}, &appConfig)
				return appConfig.Status.RollingStatus
			},
			time.Second*30, time.Millisecond*500).Should(BeEquivalentTo(v1alpha2.InactiveAfterRollingCompleted))
	}

	ApplyTwoAppVersion := func() {
		CreateClonesetDef()
		ApplySourceApp()
		MarkAppRolling(1)
		ApplyTargetApp()
		VerifyCloneSetPaused()
	}

	RevertBackToSource := func() {
		By("Revert the change by first marking the application as rolling")
		MarkAppRolling(2)

		By("Revert the application back to source")
		var sourceApp v1alpha2.Application
		Expect(readYaml("testdata/rollout/cloneset/app-source.yaml", &sourceApp)).Should(BeNil())
		sourceApp.SetAnnotations(util.MergeMapOverrideWithDst(app.GetAnnotations(),
			map[string]string{oam.AnnotationRollingComponent: app.Spec.Components[0].Name,
				oam.AnnotationAppRollout: strconv.FormatBool(true)}))
		Eventually(
			func() error {
				k8sClient.Get(ctx, client.ObjectKey{Namespace: namespace, Name: app.Name}, &app)
				app.Spec = sourceApp.Spec
				return k8sClient.Update(ctx, &app)
			},
			time.Second*60, time.Millisecond*500).Should(Succeed())

		By("Wait for AppConfig3 to be templated")
		VerifyAppConfigTemplated(3)
		var appConfig3 v1alpha2.ApplicationConfiguration
		k8sClient.Get(ctx, client.ObjectKey{Namespace: namespace, Name: app.Status.LatestRevision.Name}, &appConfig3)

		By("Modify the application rollout with new target and source")
		Eventually(
			func() error {
				k8sClient.Get(ctx, client.ObjectKey{Namespace: namespace, Name: appRollout.Name}, &appRollout)
				appRollout.Spec.SourceAppRevisionName = appConfig2.Name
				appRollout.Spec.TargetAppRevisionName = appConfig3.Name
				appRollout.Spec.RolloutPlan.BatchPartition = nil
				return k8sClient.Update(ctx, &appRollout)
			},
			time.Second*5, time.Millisecond*500).Should(Succeed())

		VerifyRolloutOwnsCloneset()

		By("Verify AppConfig rolling status")

		VerifyRolloutSucceeded()

		VerifyAppConfigRollingCompleted(appConfig3.Name)

		VerifyAppConfigInactive(appConfig2.Name)

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
		k8sClient.Delete(ctx, &appConfig2)
		k8sClient.Delete(ctx, &appConfig1)
		k8sClient.Delete(ctx, &app)
		By(fmt.Sprintf("Delete the entire namespace %s", ns.Name))
		// delete the namespace with all its resources
		Expect(k8sClient.Delete(ctx, &ns, client.PropagationPolicy(metav1.DeletePropagationForeground))).Should(BeNil())
		time.Sleep(15 * time.Second)
	})

	It("Test cloneset rollout first time (no source)", func() {
		ApplyTwoAppVersion()
		By("Apply the application rollout go directly to the target")
		var newAppRollout v1alpha2.AppRollout
		Expect(readYaml("testdata/rollout/cloneset/app-rollout.yaml", &newAppRollout)).Should(BeNil())
		newAppRollout.Namespace = namespace
		newAppRollout.Spec.SourceAppRevisionName = ""
		Expect(k8sClient.Create(ctx, &newAppRollout)).Should(Succeed())

		By("Wait for the rollout phase change to rolling in batches")
		Eventually(
			func() oamstd.RollingState {
				k8sClient.Get(ctx, client.ObjectKey{Namespace: namespace, Name: newAppRollout.Name}, &appRollout)
				return appRollout.Status.RollingState
			},
			time.Second*60, time.Millisecond*500).Should(BeEquivalentTo(oamstd.RollingInBatchesState))

		VerifyRolloutOwnsCloneset()

		VerifyRolloutSucceeded()

		VerifyAppConfigRollingCompleted(appConfig2.Name)
		// Clean up
		k8sClient.Delete(ctx, &appRollout)
	})

	It("Test cloneset rollout with a manual check", func() {
		ApplyTwoAppVersion()

		By("Apply the application rollout that stops after the first batch")
		var newAppRollout v1alpha2.AppRollout
		Expect(readYaml("testdata/rollout/cloneset/app-rollout.yaml", &newAppRollout)).Should(BeNil())
		newAppRollout.Namespace = namespace
		batchPartition := 0
		newAppRollout.Spec.RolloutPlan.BatchPartition = pointer.Int32Ptr(int32(batchPartition))
		Expect(k8sClient.Create(ctx, &newAppRollout)).Should(Succeed())

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

		VerifyRolloutOwnsCloneset()

		By("Finish the application rollout")
		// set the partition as the same size as the array
		appRollout.Spec.RolloutPlan.BatchPartition = pointer.Int32Ptr(int32(len(appRollout.Spec.RolloutPlan.
			RolloutBatches) - 1))
		Expect(k8sClient.Update(ctx, &appRollout)).Should(Succeed())

		VerifyRolloutSucceeded()

		VerifyAppConfigRollingCompleted(appConfig2.Name)

		VerifyAppConfigInactive(appConfig1.Name)

		// Clean up
		k8sClient.Delete(ctx, &appRollout)
	})

	It("Test pause and modify rollout plan after rolling succeeded", func() {
		ApplyTwoAppVersion()

		By("Apply the application rollout")
		var newAppRollout v1alpha2.AppRollout
		Expect(readYaml("testdata/rollout/cloneset/app-rollout.yaml", &newAppRollout)).Should(BeNil())
		newAppRollout.Namespace = namespace
		Expect(k8sClient.Create(ctx, &newAppRollout)).Should(Succeed())

		By("Wait for the rollout phase change to rolling in batches")
		Eventually(
			func() oamstd.RollingState {
				k8sClient.Get(ctx, client.ObjectKey{Namespace: namespace, Name: newAppRollout.Name}, &appRollout)
				return appRollout.Status.RollingState
			},
			time.Second*60, time.Millisecond*500).Should(BeEquivalentTo(oamstd.RollingInBatchesState))

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
		// wait for the batch to be ready
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

		VerifyRolloutOwnsCloneset()

		By("Finish the application rollout")
		// remove the batch restriction
		appRollout.Spec.RolloutPlan.Paused = false
		appRollout.Spec.RolloutPlan.BatchPartition = nil
		Expect(k8sClient.Update(ctx, &appRollout)).Should(Succeed())

		VerifyRolloutSucceeded()
		// record the transition time
		k8sClient.Get(ctx, client.ObjectKey{Namespace: namespace, Name: appRollout.Name}, &appRollout)
		lt = appRollout.Status.GetCondition(oamstd.RolloutSucceed).LastTransitionTime

		// move the batch partition back but should be rejected
		appRollout.Spec.RolloutPlan.BatchPartition = pointer.Int32Ptr(0)
		Expect(k8sClient.Update(ctx, &appRollout)).ShouldNot(Succeed())

		// nothing should happen, the transition time should be the same
		VerifyRolloutSucceeded()
		k8sClient.Get(ctx, client.ObjectKey{Namespace: namespace, Name: appRollout.Name}, &appRollout)
		Expect(appRollout.Status.RollingState).Should(BeEquivalentTo(oamstd.RolloutSucceedState))
		Expect(appRollout.Status.GetCondition(oamstd.RolloutSucceed).LastTransitionTime).Should(BeEquivalentTo(lt))

		// Clean up
		k8sClient.Delete(ctx, &appRollout)
	})

	It("Test rolling back after a successful rollout", func() {
		ApplyTwoAppVersion()

		By("Apply the application rollout")
		var newAppRollout v1alpha2.AppRollout
		Expect(readYaml("testdata/rollout/cloneset/app-rollout.yaml", &newAppRollout)).Should(BeNil())
		newAppRollout.Namespace = namespace
		Expect(k8sClient.Create(ctx, &newAppRollout)).Should(Succeed())
		By("Wait for the rollout phase change to rolling in batches")
		Eventually(
			func() oamstd.RollingState {
				k8sClient.Get(ctx, client.ObjectKey{Namespace: namespace, Name: newAppRollout.Name}, &appRollout)
				return appRollout.Status.RollingState
			},
			time.Second*60, time.Millisecond*500).Should(BeEquivalentTo(oamstd.RollingInBatchesState))
		VerifyRolloutOwnsCloneset()

		VerifyRolloutSucceeded()

		VerifyAppConfigRollingCompleted(appConfig2.Name)

		VerifyAppConfigInactive(appConfig1.Name)

		RevertBackToSource()
	})

	It("Test rolling back after a failed rollout", func() {
		ApplyTwoAppVersion()

		By("Apply the application rollout that stops after the first batche")
		var newAppRollout v1alpha2.AppRollout
		Expect(readYaml("testdata/rollout/cloneset/app-rollout.yaml", &newAppRollout)).Should(BeNil())
		newAppRollout.Namespace = namespace
		batchPartition := 1
		newAppRollout.Spec.RolloutPlan.BatchPartition = pointer.Int32Ptr(int32(batchPartition))
		Expect(k8sClient.Create(ctx, &newAppRollout)).Should(Succeed())

		By("Wait for the rollout phase change to rolling in batches")
		Eventually(
			func() oamstd.RollingState {
				k8sClient.Get(ctx, client.ObjectKey{Namespace: namespace, Name: newAppRollout.Name}, &appRollout)
				return appRollout.Status.RollingState
			},
			time.Second*10, time.Millisecond*500).Should(BeEquivalentTo(oamstd.RollingInBatchesState))

		By("Wait for rollout to finish the batches")
		Eventually(
			func() int32 {
				k8sClient.Get(ctx, client.ObjectKey{Namespace: namespace, Name: appRollout.Name}, &appRollout)
				return appRollout.Status.CurrentBatch
			},
			time.Second*60, time.Millisecond*500).Should(BeEquivalentTo(batchPartition))

		By("Verify that the rollout stops")
		// wait for the batch to be ready
		Eventually(
			func() oamstd.BatchRollingState {
				k8sClient.Get(ctx, client.ObjectKey{Namespace: namespace, Name: appRollout.Name}, &appRollout)
				return appRollout.Status.BatchRollingState
			},
			time.Second*60, time.Millisecond*500).Should(Equal(oamstd.BatchReadyState))

		By("Move back the partition to cause the rollout to fail")
		Eventually(
			func() error {
				k8sClient.Get(ctx, client.ObjectKey{Namespace: namespace, Name: appRollout.Name}, &appRollout)
				appRollout.Spec.RolloutPlan.BatchPartition = pointer.Int32Ptr(0)
				return k8sClient.Update(ctx, &appRollout)
			},
			time.Second*15, time.Millisecond*500).Should(Succeed())

		By("Wait for the rollout phase change to fail")
		Eventually(
			func() oamstd.RollingState {
				k8sClient.Get(ctx, client.ObjectKey{Namespace: namespace, Name: newAppRollout.Name}, &appRollout)
				return appRollout.Status.RollingState
			},
			time.Second*10, time.Millisecond*500).Should(BeEquivalentTo(oamstd.RolloutFailedState))

		RevertBackToSource()
	})

	PIt("Test rolling by changing the definition", func() {
		CreateClonesetDef()
		ApplySourceApp()
		MarkAppRolling(1)
		By("Apply the definition change")
		var cd, newCD v1alpha2.WorkloadDefinition
		Expect(readYaml("testdata/rollout/cloneset/clonesetDefinitionModified.yaml.yaml", &newCD)).Should(BeNil())
		Eventually(
			func() error {
				k8sClient.Get(ctx, client.ObjectKey{Namespace: namespace, Name: newCD.Name}, &cd)
				cd.Spec = newCD.Spec
				return k8sClient.Update(ctx, &cd)
			},
			time.Second*3, time.Millisecond*300).Should(Succeed())
		VerifyAppConfigTemplated(2)
		By("Apply the application rollout")
		var newAppRollout v1alpha2.AppRollout
		Expect(readYaml("testdata/rollout/cloneset/app-rollout.yaml", &newAppRollout)).Should(BeNil())
		newAppRollout.Namespace = namespace
		newAppRollout.Spec.RolloutPlan.BatchPartition = pointer.Int32Ptr(int32(len(newAppRollout.Spec.RolloutPlan.
			RolloutBatches) - 1))
		Expect(k8sClient.Create(ctx, &newAppRollout)).Should(Succeed())

		VerifyRolloutOwnsCloneset()

		VerifyRolloutSucceeded()

		VerifyAppConfigRollingCompleted(appConfig2.Name)

		VerifyAppConfigInactive(appConfig1.Name)

		// Clean up
		k8sClient.Delete(ctx, &appRollout)
	})
})
