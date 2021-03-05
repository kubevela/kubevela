package controllers_test

import (
	"context"
	"math/rand"
	"strconv"
	"time"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"

	"github.com/crossplane/crossplane-runtime/apis/core/v1alpha1"
	kruise "github.com/openkruise/kruise-api/apps/v1alpha1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	logf "sigs.k8s.io/controller-runtime/pkg/log"

	"github.com/oam-dev/kubevela/apis/core.oam.dev/v1alpha2"
	oamstd "github.com/oam-dev/kubevela/apis/standard.oam.dev/v1alpha1"
	"github.com/oam-dev/kubevela/pkg/controller/utils"
	"github.com/oam-dev/kubevela/pkg/oam/util"
)

var _ = Describe("Test Rolling out Application", func() {
	ctx := context.Background()
	namespace := "rolling"
	var ns corev1.Namespace

	BeforeEach(func() {
		logf.Log.Info("Start to run a test, clean up previous resources")
		namespace = string(strconv.AppendInt([]byte(namespace), rand.Int63(), 16))
		ns = corev1.Namespace{
			ObjectMeta: metav1.ObjectMeta{
				Name: namespace,
			},
		}
		// delete the namespace with all its resources
		Expect(k8sClient.Delete(ctx, &ns, client.PropagationPolicy(metav1.DeletePropagationForeground))).
			Should(SatisfyAny(BeNil(), &util.NotFoundMatcher{}))
		logf.Log.Info("make sure all the resources are removed")
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
		By("Install CloneSet based workloadDefinition")
		var cd v1alpha2.WorkloadDefinition
		Expect(readYaml("testdata/rollout/clonesetDefinition.yaml", &cd)).Should(BeNil())
		// create the workloadDefinition if not exist
		Eventually(
			func() error {
				return k8sClient.Create(ctx, &cd)
			},
			time.Second*3, time.Millisecond*300).Should(SatisfyAny(BeNil(), &util.AlreadyExistMatcher{}))
	})

	AfterEach(func() {
		logf.Log.Info("Clean up resources")
		// delete the namespace with all its resources
		Expect(k8sClient.Delete(ctx, &ns, client.PropagationPolicy(metav1.DeletePropagationForeground))).Should(BeNil())
	})

	It("Basic cloneset rollout", func() {
		By("Apply an application")
		var app v1alpha2.Application
		Expect(readYaml("testdata/rollout/app-source.yaml", &app)).Should(BeNil())
		app.Namespace = namespace
		Expect(k8sClient.Create(ctx, &app)).Should(Succeed())
		By("Get Application latest status after AppConfig created")
		Eventually(
			func() *v1alpha2.Revision {
				k8sClient.Get(ctx, client.ObjectKey{Namespace: namespace, Name: app.Name}, &app)
				return app.Status.LatestRevision
			},
			time.Second*30, time.Millisecond*500).ShouldNot(BeNil())
		By("Wait for AppConfig1 synced")
		var appConfig1 v1alpha2.ApplicationConfiguration
		Eventually(
			func() corev1.ConditionStatus {
				k8sClient.Get(ctx, client.ObjectKey{Namespace: namespace, Name: app.Status.LatestRevision.Name}, &appConfig1)
				return appConfig1.Status.GetCondition(v1alpha1.TypeSynced).Status
			},
			time.Second*30, time.Millisecond*500).Should(BeEquivalentTo(corev1.ConditionTrue))

		By("Mark the application as rolling")
		Expect(readYaml("testdata/rollout/app-source-prep.yaml", &app)).Should(BeNil())
		app.Namespace = namespace
		Expect(k8sClient.Update(ctx, &app)).Should(Succeed())
		By("Wait for AppConfig1 to be templated")
		Eventually(
			func() v1alpha2.RollingStatus {
				k8sClient.Get(ctx, client.ObjectKey{Namespace: namespace, Name: app.Status.LatestRevision.Name}, &appConfig1)
				return appConfig1.Status.RollingStatus
			},
			time.Second*60, time.Millisecond*500).Should(BeEquivalentTo(v1alpha2.RollingTemplated))

		By("Update the application during rolling")
		Expect(readYaml("testdata/rollout/app-target.yaml", &app)).Should(BeNil())
		app.Namespace = namespace
		Expect(k8sClient.Update(ctx, &app)).Should(Succeed())
		By("Get Application latest status after AppConfig created")
		Eventually(
			func() int64 {
				k8sClient.Get(ctx, client.ObjectKey{Namespace: namespace, Name: app.Name}, &app)
				return app.Status.LatestRevision.Revision
			},
			time.Second*10, time.Millisecond*500).ShouldNot(BeEquivalentTo(1))
		By("Wait for AppConfig2 synced")
		var appConfig2 v1alpha2.ApplicationConfiguration
		Eventually(
			func() corev1.ConditionStatus {
				k8sClient.Get(ctx, client.ObjectKey{Namespace: namespace, Name: app.Status.LatestRevision.Name}, &appConfig2)
				return appConfig2.Status.GetCondition(v1alpha1.TypeSynced).Status
			},
			time.Second*60, time.Millisecond*500).Should(BeEquivalentTo(corev1.ConditionTrue))

		By("Wait for AppConfig2 to be templated")
		Eventually(
			func() v1alpha2.RollingStatus {
				k8sClient.Get(ctx, client.ObjectKey{Namespace: namespace, Name: app.Status.LatestRevision.Name}, &appConfig2)
				return appConfig2.Status.RollingStatus
			},
			time.Second*60, time.Millisecond*500).Should(BeEquivalentTo(v1alpha2.RollingTemplated))

		By("Get the cloneset workload")
		var kc kruise.CloneSet
		workloadName := utils.ExtractComponentName(appConfig2.Spec.Components[0].RevisionName)
		Expect(k8sClient.Get(ctx, client.ObjectKey{Namespace: namespace, Name: workloadName},
			&kc)).ShouldNot(HaveOccurred())
		Expect(kc.Spec.UpdateStrategy.Paused).Should(BeTrue())

		By("Apply the application rollout that stops after two batches")
		var appDeploy v1alpha2.ApplicationDeployment
		Expect(readYaml("testdata/rollout/app-deploy-pause.yaml", &appDeploy)).Should(BeNil())
		appDeploy.Namespace = namespace
		Expect(k8sClient.Create(ctx, &appDeploy)).Should(Succeed())

		By("Wait for the rollout phase change to rolling in batches")
		Eventually(
			func() oamstd.RollingState {
				k8sClient.Get(ctx, client.ObjectKey{Namespace: namespace, Name: appDeploy.Name}, &appDeploy)
				return appDeploy.Status.RollingState
			},
			time.Second*60, time.Millisecond*500).Should(BeEquivalentTo(oamstd.RollingInBatchesState))

		By("Wait for rollout to finish two batches")
		Eventually(
			func() int32 {
				k8sClient.Get(ctx, client.ObjectKey{Namespace: namespace, Name: appDeploy.Name}, &appDeploy)
				return appDeploy.Status.CurrentBatch
			},
			time.Second*60, time.Millisecond*500).Should(BeEquivalentTo(1))

		By("Verify that the rollout stops at two batches")
		// wait for the batch to be ready
		Eventually(
			func() oamstd.BatchRollingState {
				k8sClient.Get(ctx, client.ObjectKey{Namespace: namespace, Name: appDeploy.Name}, &appDeploy)
				return appDeploy.Status.BatchRollingState
			},
			time.Second*60, time.Millisecond*500).Should(Equal(oamstd.BatchReadyState))
		// wait for 30 seconds, it should still be at 1
		time.Sleep(30 * time.Second)
		k8sClient.Get(ctx, client.ObjectKey{Namespace: namespace, Name: appDeploy.Name}, &appDeploy)
		Expect(appDeploy.Status.CurrentBatch).Should(BeEquivalentTo(1))
		Expect(appDeploy.Status.BatchRollingState).Should(BeEquivalentTo(oamstd.BatchReadyState))

		Expect(k8sClient.Get(ctx, client.ObjectKey{Namespace: namespace, Name: workloadName},
			&kc)).ShouldNot(HaveOccurred())
		Expect(kc.Status.UpdatedReplicas).Should(BeEquivalentTo(3))
		Expect(kc.Status.UpdatedReadyReplicas).Should(BeEquivalentTo(3))

		By("Finish the application rollout")
		Expect(readYaml("testdata/rollout/app-deploy-finish.yaml", &appDeploy)).Should(BeNil())
		appDeploy.Namespace = namespace
		Expect(k8sClient.Update(ctx, &appDeploy)).Should(Succeed())

		By("Wait for the rollout phase change to succeeded")
		Eventually(
			func() oamstd.RollingState {
				k8sClient.Get(ctx, client.ObjectKey{Namespace: namespace, Name: appDeploy.Name}, &appDeploy)
				return appDeploy.Status.RollingState
			},
			time.Second*60, time.Millisecond*500).Should(Equal(oamstd.RolloutSucceedState))

		By("Wait for rollout to finish two batches")
		Expect(k8sClient.Get(ctx, client.ObjectKey{Namespace: namespace, Name: workloadName},
			&kc)).ShouldNot(HaveOccurred())
		Expect(kc.Status.UpdatedReplicas).Should(BeEquivalentTo(5))
		Expect(kc.Status.UpdatedReadyReplicas).Should(BeEquivalentTo(5))
		// Clean up
		k8sClient.Delete(ctx, &appDeploy)
		k8sClient.Delete(ctx, &appConfig2)
		k8sClient.Delete(ctx, &appConfig1)
		k8sClient.Delete(ctx, &app)
	})
})
