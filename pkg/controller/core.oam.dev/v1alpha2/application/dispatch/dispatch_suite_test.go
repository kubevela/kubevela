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

package dispatch

import (
	"context"
	"fmt"
	"math/rand"
	"os"
	"path/filepath"
	"strconv"
	"testing"
	"time"

	"github.com/oam-dev/kubevela/apis/core.oam.dev/common"

	"github.com/crossplane/crossplane-runtime/pkg/test"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes/scheme"
	"k8s.io/utils/pointer"

	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/envtest"

	"github.com/oam-dev/kubevela/apis/core.oam.dev/v1beta1"
	monitorContext "github.com/oam-dev/kubevela/pkg/monitor/context"
	"github.com/oam-dev/kubevela/pkg/oam"
	"github.com/oam-dev/kubevela/pkg/oam/util"
)

var (
	k8sClient client.Client
	testEnv   *envtest.Environment
)

func TestDispatch(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Dispatch Suite")
}

var _ = BeforeSuite(func(done Done) {
	rand.Seed(time.Now().UnixNano())
	By("bootstrapping test environment")
	var yamlPath string
	if _, set := os.LookupEnv("COMPATIBILITY_TEST"); set {
		yamlPath = "../../../../../../test/compatibility-test/testdata"
	} else {
		yamlPath = filepath.Join("../../../../../..", "charts", "vela-core", "crds")
	}
	testEnv = &envtest.Environment{
		ControlPlaneStartTimeout: time.Minute,
		ControlPlaneStopTimeout:  time.Minute,
		UseExistingCluster:       pointer.BoolPtr(false),
		CRDDirectoryPaths:        []string{yamlPath},
	}

	cfg, err := testEnv.Start()
	Expect(err).ToNot(HaveOccurred())
	Expect(cfg).ToNot(BeNil())

	var testScheme = runtime.NewScheme()
	err = v1beta1.SchemeBuilder.AddToScheme(testScheme)
	Expect(err).NotTo(HaveOccurred())
	err = scheme.AddToScheme(testScheme)
	Expect(err).NotTo(HaveOccurred())

	k8sClient, err = client.New(cfg, client.Options{Scheme: testScheme})
	Expect(err).ToNot(HaveOccurred())
	Expect(k8sClient).ToNot(BeNil())
	close(done)
}, 60)

var _ = AfterSuite(func() {
	By("tearing down the test environment")
	err := testEnv.Stop()
	Expect(err).ToNot(HaveOccurred())
})

var _ = Describe("Test AppManifestsDispatcher", func() {
	var (
		ctx                                             = context.Background()
		logCtx                                          = monitorContext.NewTraceContext(ctx, "")
		namespace                                       string
		deploy1, deploy2, deploy3, svc1, svc2, pv1, pv2 *unstructured.Unstructured
		appRev1, appRev2, appRev3                       *v1beta1.ApplicationRevision
	)

	var (
		deployName1 = "deploy-1"
		deployName2 = "deploy-2"
		deployName3 = "deploy-3"
		svcName1    = "svc-1"
		svcName2    = "svc-2"
		// persistent volume is cluster-scoped, generate random name for each case to avoid interference
		pvName1, pvName2 string
		appRevName1      = "app-v1"
		appRevName2      = "app-v2"
		appRevName3      = "app-v3"
	)

	BeforeEach(func() {
		By("Create test namespace")
		namespace = fmt.Sprintf("%s-%s", "dispatch-test", strconv.FormatInt(rand.Int63(), 16))
		Expect(k8sClient.Create(ctx, &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: namespace}})).Should(Succeed())

		By("Init test data")
		var (
			d1, d2, d3 *appsv1.Deployment       // represent common workload
			s1, s2     *corev1.Service          // represent common trait
			p1, p2     *corev1.PersistentVolume // represent cluster-scoped resource
		)
		d := &appsv1.Deployment{TypeMeta: metav1.TypeMeta{APIVersion: "apps/v1", Kind: "Deployment"}}
		d.SetNamespace(namespace)
		d.Spec.Selector = &metav1.LabelSelector{MatchLabels: map[string]string{"app": "nginx"}}
		d.Spec.Template = corev1.PodTemplateSpec{
			ObjectMeta: metav1.ObjectMeta{Labels: map[string]string{"app": "nginx"}},
			Spec: corev1.PodSpec{Containers: []corev1.Container{{
				Name:  "nginx",
				Image: "nginx: 1.14.2",
				Ports: []corev1.ContainerPort{{Name: "nginx", ContainerPort: int32(8080)}}}}},
		}
		d1 = d.DeepCopy()
		d1.SetName(deployName1)
		deploy1, _ = util.Object2Unstructured(d1)
		d2 = d.DeepCopy()
		d2.SetName(deployName2)
		deploy2, _ = util.Object2Unstructured(d2)
		d3 = d.DeepCopy()
		d3.SetName(deployName3)
		deploy3, _ = util.Object2Unstructured(d3)

		svc := &corev1.Service{
			TypeMeta: metav1.TypeMeta{APIVersion: "v1", Kind: "Service"},
			Spec:     corev1.ServiceSpec{Ports: []corev1.ServicePort{{Port: int32(8080)}}},
		}
		svc.SetNamespace(namespace)
		s1 = svc.DeepCopy()
		s1.SetName(svcName1)
		svc1, _ = util.Object2Unstructured(s1)
		s2 = svc.DeepCopy()
		s2.SetName(svcName2)
		svc2, _ = util.Object2Unstructured(s2)

		pv := &corev1.PersistentVolume{
			TypeMeta: metav1.TypeMeta{APIVersion: "v1", Kind: "PersistentVolume"},
			Spec: corev1.PersistentVolumeSpec{
				Capacity: corev1.ResourceList{"storage": resource.MustParse("10Gi")},
				PersistentVolumeSource: corev1.PersistentVolumeSource{
					HostPath: &corev1.HostPathVolumeSource{Path: "/mnt/data"},
				},
				AccessModes:      []corev1.PersistentVolumeAccessMode{"ReadWriteOnce"},
				StorageClassName: "manual",
			},
		}
		pvName1 = "pv-1-" + namespace
		pvName2 = "pv-2-" + namespace
		p1 = pv.DeepCopy()
		p1.SetName(pvName1)
		pv1, _ = util.Object2Unstructured(p1)
		p2 = pv.DeepCopy()
		p2.SetName(pvName2)
		pv2, _ = util.Object2Unstructured(p2)

		appRev := &v1beta1.ApplicationRevision{
			TypeMeta: metav1.TypeMeta{
				APIVersion: "core.oam.dev",
				Kind:       "ApplicationRevision",
			},
		}
		appRev.SetNamespace(namespace)
		appRev1 = appRev.DeepCopy()
		appRev1.SetName(appRevName1)
		appRev1.SetUID("fake-uid-app-revision-1")
		appRev2 = appRev.DeepCopy()
		appRev2.SetName(appRevName2)
		appRev2.SetUID("fake-uid-app-revision-2")
		appRev3 = appRev.DeepCopy()
		appRev3.SetName(appRevName3)
		appRev3.SetUID("fake-uid-app-revision-2")
	})

	AfterEach(func() {
		Expect(k8sClient.DeleteAllOf(ctx, &corev1.PersistentVolume{})).Should(Succeed())
		Expect(k8sClient.Delete(ctx, &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: namespace}})).Should(Succeed())
	})

	When("Dispatch without upgrading", func() {
		// real scenario case: release v1
		It("Test dispatch manifests", func() {
			By("Dispatch application revision 1")
			dp := NewAppManifestsDispatcher(k8sClient, appRev1)
			rt, err := dp.Dispatch(logCtx, []*unstructured.Unstructured{deploy1, svc1, pv1})
			Expect(err).Should(BeNil())
			By("Verify resources are applied successfully")
			Expect(k8sClient.Get(ctx, client.ObjectKey{Name: deployName1, Namespace: namespace}, &appsv1.Deployment{})).Should(Succeed())
			Expect(k8sClient.Get(ctx, client.ObjectKey{Name: svcName1, Namespace: namespace}, &corev1.Service{})).Should(Succeed())
			Expect(k8sClient.Get(ctx, client.ObjectKey{Name: pvName1}, &corev1.PersistentVolume{})).Should(Succeed())
			getPV1 := &corev1.PersistentVolume{}
			Expect(k8sClient.Get(ctx, client.ObjectKey{Name: pvName1}, getPV1)).Should(Succeed())

			By("Verify resource tracker's name")
			wantName := ConstructResourceTrackerName(appRevName1, namespace)
			Expect(rt.Name).Should(Equal(wantName))

			By("Verify resource tracker records all applied resources")
			recordedNames := []string{}
			for _, r := range rt.Status.TrackedResources {
				recordedNames = append(recordedNames, r.Name)
			}
			Expect(recordedNames).Should(ContainElements(deployName1, svcName1, pvName1))
		})
	})

	When("Dispatch for upgrading", func() {
		// real scenario case: reconcile v1 => v1
		It("Test use resource tracker from the same revision as the one being dispatched", func() {
			By("Dispatch application revision 1")
			dp := NewAppManifestsDispatcher(k8sClient, appRev1)
			rtForAppV1, err := dp.Dispatch(logCtx, []*unstructured.Unstructured{deploy1, svc1, pv1})
			Expect(err).Should(BeNil())
			By("Verify resources are applied successfully")
			Expect(k8sClient.Get(ctx, client.ObjectKey{Name: deployName1, Namespace: namespace}, &appsv1.Deployment{})).Should(Succeed())
			Expect(k8sClient.Get(ctx, client.ObjectKey{Name: svcName1, Namespace: namespace}, &corev1.Service{})).Should(Succeed())
			Expect(k8sClient.Get(ctx, client.ObjectKey{Name: pvName1}, &corev1.PersistentVolume{})).Should(Succeed())

			By("Dispatch application revision 1 again with v1 as latest RT")
			dp = NewAppManifestsDispatcher(k8sClient, appRev1).EndAndGC(rtForAppV1)
			_, err = dp.Dispatch(logCtx, []*unstructured.Unstructured{deploy1, svc1, pv1})
			Expect(err).Should(BeNil())
			By("Verify resources still exist")
			Expect(k8sClient.Get(ctx, client.ObjectKey{Name: deployName1, Namespace: namespace}, &appsv1.Deployment{})).Should(Succeed())
			Expect(k8sClient.Get(ctx, client.ObjectKey{Name: svcName1, Namespace: namespace}, &corev1.Service{})).Should(Succeed())
			Expect(k8sClient.Get(ctx, client.ObjectKey{Name: pvName1}, &corev1.PersistentVolume{})).Should(Succeed())
		})

		It("Test upgrade and garbage collect", func() {
			// real scenario case: upgrade v1 => v2 and GC
			By("Dispatch application revision 1")
			dp := NewAppManifestsDispatcher(k8sClient, appRev1)
			rtForAppV1, err := dp.Dispatch(logCtx, []*unstructured.Unstructured{deploy1, svc1, pv1})
			Expect(err).Should(BeNil())
			By("Verify resources are applied successfully")
			Expect(k8sClient.Get(ctx, client.ObjectKey{Name: deployName1, Namespace: namespace}, &appsv1.Deployment{})).Should(Succeed())
			Expect(k8sClient.Get(ctx, client.ObjectKey{Name: svcName1, Namespace: namespace}, &corev1.Service{})).Should(Succeed())
			Expect(k8sClient.Get(ctx, client.ObjectKey{Name: pvName1}, &corev1.PersistentVolume{})).Should(Succeed())

			By("Dispatch application revision 2 with v1 as latest RT")
			dp2 := NewAppManifestsDispatcher(k8sClient, appRev2).EndAndGC(rtForAppV1)
			_, err = dp2.Dispatch(logCtx, []*unstructured.Unstructured{deploy2, svc2, pv2})
			Expect(err).Should(BeNil())
			By("Verify v2 resources are applied successfully")
			Expect(k8sClient.Get(ctx, client.ObjectKey{Name: deployName2, Namespace: namespace}, &appsv1.Deployment{})).Should(Succeed())
			Expect(k8sClient.Get(ctx, client.ObjectKey{Name: svcName2, Namespace: namespace}, &corev1.Service{})).Should(Succeed())
			Expect(k8sClient.Get(ctx, client.ObjectKey{Name: pvName2}, &corev1.PersistentVolume{})).Should(Succeed())

			By("Verify v1 resources are deleted (GC works)")
			Expect(k8sClient.Get(ctx, client.ObjectKey{Name: deployName1, Namespace: namespace}, &appsv1.Deployment{})).Should(util.NotFoundMatcher{})
			Expect(k8sClient.Get(ctx, client.ObjectKey{Name: svcName1, Namespace: namespace}, &corev1.Service{})).Should(util.NotFoundMatcher{})
			getPV1 := &corev1.PersistentVolume{}
			Expect(k8sClient.Get(ctx, client.ObjectKey{Name: pvName1}, getPV1)).Should(Succeed())
			Expect(persistentVolumeIsDeleted(getPV1)).Should(BeTrue())

			By("Verify v1 resource tracker is deleted (GC works)")
			Expect(k8sClient.Get(ctx, client.ObjectKey{Name: ConstructResourceTrackerName(appRevName1, namespace)},
				&v1beta1.ResourceTracker{})).Should(util.NotFoundMatcher{})
		})

		It("Test upgrade and garbage collect legacy resource tracker", func() {
			// real scenario case: upgrade v1 => v2 (error occurs) => v3
			By("Dispatch application revision 1")
			dp := NewAppManifestsDispatcher(k8sClient, appRev1)
			rtForAppV1, err := dp.Dispatch(logCtx, []*unstructured.Unstructured{deploy1, svc1, pv1})
			Expect(err).Should(BeNil())
			By("Verify resources are applied successfully")
			Expect(k8sClient.Get(ctx, client.ObjectKey{Name: deployName1, Namespace: namespace}, &appsv1.Deployment{})).Should(Succeed())
			Expect(k8sClient.Get(ctx, client.ObjectKey{Name: svcName1, Namespace: namespace}, &corev1.Service{})).Should(Succeed())
			Expect(k8sClient.Get(ctx, client.ObjectKey{Name: pvName1}, &corev1.PersistentVolume{})).Should(Succeed())

			By("Dispatch application revision 2 with v1 as latest RT")
			dp2 := NewAppManifestsDispatcher(k8sClient, appRev2).EndAndGC(rtForAppV1)
			By("Prepare a bad resource in order to fail the applying")
			badRsrc := &unstructured.Unstructured{}
			badRsrc.SetName("bad")
			_, err = dp2.Dispatch(logCtx, []*unstructured.Unstructured{deploy2, svc2, pv2, badRsrc})
			By("Verify dispatch failed")
			Expect(err).ShouldNot(BeNil())

			By("Verify part of v2 resources are applied successfully")
			Expect(k8sClient.Get(ctx, client.ObjectKey{Name: deployName2, Namespace: namespace}, &appsv1.Deployment{})).Should(Succeed())
			Expect(k8sClient.Get(ctx, client.ObjectKey{Name: svcName2, Namespace: namespace}, &corev1.Service{})).Should(Succeed())
			Expect(k8sClient.Get(ctx, client.ObjectKey{Name: pvName2}, &corev1.PersistentVolume{})).Should(Succeed())

			By("Verify v1 resources are not deleted (GC not work)")
			Expect(k8sClient.Get(ctx, client.ObjectKey{Name: deployName1, Namespace: namespace}, &appsv1.Deployment{})).Should(Succeed())
			Expect(k8sClient.Get(ctx, client.ObjectKey{Name: svcName1, Namespace: namespace}, &corev1.Service{})).Should(Succeed())
			getPV1 := &corev1.PersistentVolume{}
			Expect(k8sClient.Get(ctx, client.ObjectKey{Name: pvName1}, getPV1)).Should(Succeed())
			Expect(persistentVolumeIsDeleted(getPV1)).Should(BeFalse())

			By("Verify v1 resource tracker is not deleted (GC not work)")
			Expect(k8sClient.Get(ctx, client.ObjectKey{Name: ConstructResourceTrackerName(appRevName1, namespace)},
				&v1beta1.ResourceTracker{})).Should(Succeed())

			// because dispatching v2 failed, app controller still uses v1 as latest revision
			By("Dispatch application revision 3 with v1 as latest RT")
			dp3 := NewAppManifestsDispatcher(k8sClient, appRev3).EndAndGC(rtForAppV1)
			_, err = dp3.Dispatch(logCtx, []*unstructured.Unstructured{deploy3, deploy2})
			Expect(err).Should(BeNil())

			By("Verify v1 resources are deleted (GC works)")
			Expect(k8sClient.Get(ctx, client.ObjectKey{Name: deployName1, Namespace: namespace}, &appsv1.Deployment{})).Should(util.NotFoundMatcher{})
			Expect(k8sClient.Get(ctx, client.ObjectKey{Name: svcName1, Namespace: namespace}, &corev1.Service{})).Should(util.NotFoundMatcher{})
			getPV1 = &corev1.PersistentVolume{}
			Expect(k8sClient.Get(ctx, client.ObjectKey{Name: pvName1}, getPV1)).Should(Succeed())
			Expect(persistentVolumeIsDeleted(getPV1)).Should(BeTrue())

			By("Verify v1 resource tracker is deleted (GC works)")
			Expect(k8sClient.Get(ctx, client.ObjectKey{Name: ConstructResourceTrackerName(appRevName1, namespace)},
				&v1beta1.ResourceTracker{})).Should(util.NotFoundMatcher{})

			By("Verify v3 can re-use v2's resource")
			Expect(k8sClient.Get(ctx, client.ObjectKey{Name: deployName2, Namespace: namespace}, &appsv1.Deployment{})).Should(Succeed())

			By("Verify v2 resource tracker is deleted (GC works)")
			Expect(k8sClient.Get(ctx, client.ObjectKey{Name: ConstructResourceTrackerName(appRevName2, namespace)},
				&v1beta1.ResourceTracker{})).Should(util.NotFoundMatcher{})
		})

		It("Test upgrade: v2 has the same resources as v1", func() {
			// real scenario case: upgrade v1 => v2 and GC
			// and v2 has same resouces as v1, e.g., a trait
			By("Dispatch application revision 1")
			dp := NewAppManifestsDispatcher(k8sClient, appRev1)
			rtForAppV1, err := dp.Dispatch(logCtx, []*unstructured.Unstructured{deploy1, svc1, pv1})
			Expect(err).Should(BeNil())
			getSvc1 := &corev1.Service{}
			Expect(k8sClient.Get(ctx, client.ObjectKey{Name: svcName1, Namespace: namespace}, getSvc1)).Should(Succeed())
			By("Verify serivce1's owner is resource tracker 1")
			owner := metav1.GetControllerOf(getSvc1)
			Expect(owner.Name).Should(Equal(rtForAppV1.Name))

			By("Dispatch application revision 2 with v1 as latest RT")
			dp2 := NewAppManifestsDispatcher(k8sClient, appRev2).EndAndGC(rtForAppV1)
			rtForAppV2, err := dp2.Dispatch(logCtx, []*unstructured.Unstructured{deploy2, svc1, pv2}) // manifests have 'svc1'
			Expect(err).Should(BeNil())

			By("Verify v1 resources, expect svc1, are deleted (GC works)")
			Expect(k8sClient.Get(ctx, client.ObjectKey{Name: deployName1, Namespace: namespace}, &appsv1.Deployment{})).Should(util.NotFoundMatcher{})
			getPV1 := &corev1.PersistentVolume{}
			Expect(k8sClient.Get(ctx, client.ObjectKey{Name: pvName1}, getPV1)).Should(Succeed())
			Expect(persistentVolumeIsDeleted(getPV1)).Should(BeTrue())

			By("Verify svc1's owner has beend updated to resource tracker 2")
			getSvc1 = &corev1.Service{}
			Expect(k8sClient.Get(ctx, client.ObjectKey{Name: svcName1, Namespace: namespace}, getSvc1)).Should(Succeed())
			owner = metav1.GetControllerOf(getSvc1)
			Expect(owner.Name).Should(Equal(rtForAppV2.Name))
		})

		It("Test upgrade but skip garbage collect", func() {
			// real scenario case: template v2 while rollout v1 => v2
			By("Dispatch application revision 1")
			dp := NewAppManifestsDispatcher(k8sClient, appRev1)
			rtForAppV1, err := dp.Dispatch(logCtx, []*unstructured.Unstructured{deploy1, svc1, pv1})
			Expect(err).Should(BeNil())
			By("Verify resources are applied successfully")
			Expect(k8sClient.Get(ctx, client.ObjectKey{Name: deployName1, Namespace: namespace}, &appsv1.Deployment{})).Should(Succeed())
			Expect(k8sClient.Get(ctx, client.ObjectKey{Name: svcName1, Namespace: namespace}, &corev1.Service{})).Should(Succeed())
			Expect(k8sClient.Get(ctx, client.ObjectKey{Name: pvName1}, &corev1.PersistentVolume{})).Should(Succeed())

			By("Dispatch application revision 2 with v1 as latest RT")
			dp2 := NewAppManifestsDispatcher(k8sClient, appRev2)
			By("Ask dispatcher to skip GC")
			dp2 = dp2.StartAndSkipGC(rtForAppV1)
			_, err = dp2.Dispatch(logCtx, []*unstructured.Unstructured{deploy2, svc2, pv2})
			Expect(err).Should(BeNil())
			By("Verify v2 resources are applied successfully")
			Expect(k8sClient.Get(ctx, client.ObjectKey{Name: deployName2, Namespace: namespace}, &appsv1.Deployment{})).Should(Succeed())
			Expect(k8sClient.Get(ctx, client.ObjectKey{Name: svcName2, Namespace: namespace}, &corev1.Service{})).Should(Succeed())
			Expect(k8sClient.Get(ctx, client.ObjectKey{Name: pvName2}, &corev1.PersistentVolume{})).Should(Succeed())

			By("Verify v1 resources still exist (skip GC)")
			Expect(k8sClient.Get(ctx, client.ObjectKey{Name: deployName1, Namespace: namespace}, &appsv1.Deployment{})).Should(Succeed())
			Expect(k8sClient.Get(ctx, client.ObjectKey{Name: svcName1, Namespace: namespace}, &corev1.Service{})).Should(Succeed())
			Expect(k8sClient.Get(ctx, client.ObjectKey{Name: pvName1}, &corev1.PersistentVolume{})).Should(Succeed())

			By("Verify v1 resource tracker still exist (skip GC)")
			Expect(k8sClient.Get(ctx, client.ObjectKey{Name: ConstructResourceTrackerName(appRevName1, namespace)},
				&v1beta1.ResourceTracker{})).Should(Succeed())
		})
	})
})

var _ = Describe("Test handleSkipGC func", func() {
	var namespaceName string
	ctx := context.Background()
	logCtx := monitorContext.NewTraceContext(ctx, "")
	BeforeEach(func() {
		namespaceName = fmt.Sprintf("%s-%s", "dispatch-gc-skip-test", strconv.FormatInt(rand.Int63(), 16))
		Expect(k8sClient.Create(ctx, &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: namespaceName}}))
	})

	It("Test GC skip func ", func() {
		handler := GCHandler{c: k8sClient, appRev: v1beta1.ApplicationRevision{Spec: v1beta1.ApplicationRevisionSpec{
			Application: v1beta1.Application{Spec: v1beta1.ApplicationSpec{Components: []common.ApplicationComponent{{Name: "mywebservice"}}}},
		}}}
		wlName := "test-workload"
		resourceTracker := v1beta1.ResourceTracker{
			ObjectMeta: metav1.ObjectMeta{
				Name: wlName,
				UID:  "test-uid",
			},
		}
		skipWorkload := &appsv1.Deployment{TypeMeta: metav1.TypeMeta{APIVersion: "apps/v1", Kind: "Deployment"}}
		skipWorkload.SetNamespace(namespaceName)
		skipWorkload.SetName(wlName)
		skipWorkload.SetLabels(map[string]string{oam.LabelAppComponent: "mywebservice"})
		skipWorkload.SetOwnerReferences([]metav1.OwnerReference{*metav1.NewControllerRef(
			&resourceTracker, v1beta1.ResourceTrackerKindVersionKind),
			metav1.OwnerReference{UID: "app-uid", Name: "test-app", APIVersion: v1beta1.SchemeGroupVersion.String(), Kind: v1beta1.ApplicationKind}})
		skipWorkload.SetAnnotations(map[string]string{
			oam.AnnotationSkipGC: "true",
		})
		skipWorkload.Spec.Selector = &metav1.LabelSelector{MatchLabels: map[string]string{"component": "mywebservice"}}
		skipWorkload.Spec.Template = corev1.PodTemplateSpec{ObjectMeta: metav1.ObjectMeta{Labels: map[string]string{"component": "mywebservice"}},
			Spec: corev1.PodSpec{Containers: []corev1.Container{{
				Name:  "nginx",
				Image: "nginx: 1.14.2",
				Ports: []corev1.ContainerPort{{Name: "nginx", ContainerPort: int32(8080)}}}}}}
		u, err := util.Object2Unstructured(skipWorkload)
		Expect(err).Should(BeNil())
		Expect(k8sClient.Create(ctx, skipWorkload)).Should(BeNil())
		skipGC, err := handler.handleResourceSkipGC(logCtx, u, &resourceTracker)
		Expect(err).Should(BeNil())
		Expect(skipGC).Should(BeTrue())

		checkWl := skipWorkload.DeepCopy()
		Expect(k8sClient.Get(ctx, types.NamespacedName{Namespace: checkWl.GetNamespace(), Name: checkWl.GetName()}, checkWl)).Should(BeNil())
		Expect(len(checkWl.GetOwnerReferences())).Should(BeEquivalentTo(1))
		Expect(checkWl.GetOwnerReferences()[0].UID).Should(BeEquivalentTo("app-uid"))
	})

	It("Test GC skip func, mock client return error", func() {
		handler := GCHandler{c: &test.MockClient{
			MockGet: test.NewMockGetFn(fmt.Errorf("this isn't a not found error")),
		}}
		isSkip, err := handler.handleResourceSkipGC(logCtx, &unstructured.Unstructured{}, &v1beta1.ResourceTracker{})
		Expect(err).ShouldNot(BeNil())
		Expect(isSkip).Should(BeEquivalentTo(false))
	})
})

var _ = Describe("Test compatibility code", func() {
	var namespaceName string
	var appName string
	ctx := context.Background()
	BeforeEach(func() {
		namespaceName = fmt.Sprintf("%s-%s", "compatibility-code-test", strconv.FormatInt(rand.Int63(), 16))
		appName = "test-app"
		Expect(k8sClient.Create(ctx, &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: namespaceName}}))
	})

	It("Test GC skip func ", func() {
		a := AppManifestsDispatcher{c: k8sClient, currentRTName: appName + "-v4-" + namespaceName, namespace: namespaceName}
		resourceTracker_old := v1beta1.ResourceTracker{
			ObjectMeta: metav1.ObjectMeta{
				Name: appName + "-v1-" + namespaceName,
				Labels: map[string]string{
					oam.LabelAppNamespace: namespaceName,
					oam.LabelAppName:      appName,
				},
			},
		}
		resourceTracker_new := v1beta1.ResourceTracker{
			ObjectMeta: metav1.ObjectMeta{
				Name: appName + "-v2-" + namespaceName,
				Labels: map[string]string{
					"app.oam.dev/namesapce": namespaceName,
					oam.LabelAppName:        appName,
				},
			},
		}
		resourceTracker_previous := v1beta1.ResourceTracker{
			ObjectMeta: metav1.ObjectMeta{
				Name: appName + "-v3-" + namespaceName,
				Labels: map[string]string{
					oam.LabelAppNamespace: namespaceName,
					oam.LabelAppName:      appName,
				},
			},
		}
		a.previousRT = &resourceTracker_previous
		Expect(a.c.Create(ctx, &resourceTracker_old)).Should(BeNil())
		Expect(a.c.Create(ctx, &resourceTracker_new)).Should(BeNil())
		Expect(a.retrieveLegacyResourceTrackers(ctx)).Should(BeNil())
		Expect(len(a.legacyRTs)).Should(BeEquivalentTo(2))
		res := map[types.UID]bool{}
		for _, rt := range a.legacyRTs {
			res[rt.UID] = true
		}
		Expect(res[resourceTracker_old.UID]).Should(BeTrue())
		Expect(res[resourceTracker_new.UID]).Should(BeTrue())
	})
})

// in envtest, no gc controller can delete PersistentVolume because of its finalizer
// so we just use deletion timestamp to verify its deletion
func persistentVolumeIsDeleted(pv *corev1.PersistentVolume) bool {
	return !pv.ObjectMeta.DeletionTimestamp.IsZero()
}
