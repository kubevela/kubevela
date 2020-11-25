package routes

import (
	"context"
	"errors"
	"time"

	"github.com/crossplane/oam-kubernetes-runtime/pkg/oam"

	"github.com/oam-dev/kubevela/apis/v1alpha1"

	"github.com/crossplane/oam-kubernetes-runtime/apis/core/v1alpha2"
	"github.com/crossplane/oam-kubernetes-runtime/pkg/oam/util"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	certmanager "github.com/wonderflow/cert-manager-api/pkg/apis/certmanager/v1"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/api/networking/v1beta1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
)

var _ = Describe("Route Trait Integration Test", func() {
	// common var init
	ctx := context.Background()
	namespaceName := "routetrait-integration-test"

	podPort := 8000
	issuerName := "my-issuer"

	var ns corev1.Namespace
	getComponent := func(workloadType, compName string) (v1alpha2.Component, map[string]string, map[string]string) {
		podTemplateLabel := map[string]string{"standard.oam.dev": "oam-test-deployment", "workload.oam.dev/type": workloadType}
		workloadLabel := map[string]string{"standard.oam.dev": "oam-test-deployment", "app.oam.dev/component": compName, "app.oam.dev/name": "test-app-" + compName}
		basedeploy := &appsv1.Deployment{
			TypeMeta: metav1.TypeMeta{
				Kind:       "Deployment",
				APIVersion: "apps/v1",
			},
			ObjectMeta: metav1.ObjectMeta{
				Labels: podTemplateLabel,
			},
			Spec: appsv1.DeploymentSpec{
				Selector: &metav1.LabelSelector{
					MatchLabels: podTemplateLabel,
				},
				Template: corev1.PodTemplateSpec{
					ObjectMeta: metav1.ObjectMeta{
						Labels: podTemplateLabel,
					},
					Spec: corev1.PodSpec{
						Containers: []corev1.Container{
							{
								Name:            "container-name",
								Image:           "crccheck/hello-world",
								ImagePullPolicy: corev1.PullNever,
								Ports: []corev1.ContainerPort{
									{
										ContainerPort: int32(podPort),
									}}}}}}},
		}
		var rp = int32(1)
		if workloadType == "webservice" {
			basePodSpecc := &v1alpha1.PodSpecWorkload{
				TypeMeta: metav1.TypeMeta{
					Kind:       "PodSpecWorkload",
					APIVersion: "standard.oam.dev/v1alpha1",
				},
				ObjectMeta: metav1.ObjectMeta{
					Labels: podTemplateLabel,
				},
				Spec: v1alpha1.PodSpecWorkloadSpec{
					Replicas: &rp,
					PodSpec: corev1.PodSpec{
						Containers: []corev1.Container{
							{
								Name:            "container-name",
								Image:           "crccheck/hello-world",
								ImagePullPolicy: corev1.PullNever,
								Ports: []corev1.ContainerPort{
									{
										ContainerPort: int32(podPort),
									}}}}}},
			}
			return v1alpha2.Component{
				TypeMeta: metav1.TypeMeta{
					Kind:       "Component",
					APIVersion: "core.oam.dev/v1alpha2",
				},
				ObjectMeta: metav1.ObjectMeta{
					Name:      compName,
					Namespace: ns.Name,
				},
				Spec: v1alpha2.ComponentSpec{
					Workload: runtime.RawExtension{Object: basePodSpecc},
				},
			}, workloadLabel, podTemplateLabel
		}
		return v1alpha2.Component{
			TypeMeta: metav1.TypeMeta{
				Kind:       "Component",
				APIVersion: "core.oam.dev/v1alpha2",
			},
			ObjectMeta: metav1.ObjectMeta{
				Name:      compName,
				Namespace: ns.Name,
			},
			Spec: v1alpha2.ComponentSpec{
				Workload: runtime.RawExtension{Object: basedeploy},
			},
		}, workloadLabel, podTemplateLabel
	}

	getAC := func(compName string) v1alpha2.ApplicationConfiguration {
		return v1alpha2.ApplicationConfiguration{
			TypeMeta: metav1.TypeMeta{
				Kind:       "ApplicationConfiguration",
				APIVersion: "core.oam.dev/v1alpha2",
			},
			ObjectMeta: metav1.ObjectMeta{
				Namespace: ns.Name,
				Name:      "test-app-" + compName,
			},
			Spec: v1alpha2.ApplicationConfigurationSpec{
				Components: []v1alpha2.ApplicationConfigurationComponent{{
					ComponentName: compName,
					Traits: []v1alpha2.ComponentTrait{
						{
							Trait: runtime.RawExtension{Object: &unstructured.Unstructured{
								Object: map[string]interface{}{
									"apiVersion": "standard.oam.dev/v1alpha1",
									"kind":       "Route",
									"metadata": map[string]interface{}{
										"labels": map[string]interface{}{
											oam.TraitTypeLabel: "route",
										},
									},
									"spec": map[string]interface{}{
										"host": "mycomp.mytest.com",
										"tls": map[string]interface{}{
											"issuerName": issuerName,
										}}}}}}}}}}}
	}
	BeforeEach(func() {
		logf.Log.Info("[TEST] Set up resources before an integration test")
		ns = corev1.Namespace{
			ObjectMeta: metav1.ObjectMeta{
				Name: namespaceName,
			},
		}

		By("Create the Namespace for test")
		Expect(k8sClient.Create(ctx, &ns)).Should(SatisfyAny(Succeed(), &util.AlreadyExistMatcher{}))

		By("Create the Issuer for test")
		Expect(k8sClient.Create(context.Background(), &certmanager.Issuer{
			ObjectMeta: metav1.ObjectMeta{Name: issuerName, Namespace: namespaceName},
			Spec:       certmanager.IssuerSpec{IssuerConfig: certmanager.IssuerConfig{SelfSigned: &certmanager.SelfSignedIssuer{}}},
		})).Should(SatisfyAny(Succeed(), &util.AlreadyExistMatcher{}))
	})

	AfterEach(func() {
		// Control-runtime test environment has a bug that can't delete resources like deployment/namespaces
		// We have to use different names to segregate between tests
		logf.Log.Info("[TEST] Clean up resources after an integration test")
	})

	It("Test with child resource no podSpecable but has service child using webservice workload", func() {
		compName := "test-webservice"
		comp, _, _ := getComponent("webservice", compName)
		ac := getAC(compName)
		Expect(k8sClient.Create(ctx, &comp)).ToNot(HaveOccurred())
		Expect(k8sClient.Create(ctx, &ac)).ToNot(HaveOccurred())

		By("Check that we have created the route")
		createdRoute := v1alpha1.Route{}
		var traitName string
		Eventually(
			func() error {
				err := k8sClient.Get(ctx,
					types.NamespacedName{Namespace: ns.Name, Name: ac.Name},
					&ac)
				if err != nil {
					return err
				}
				if len(ac.Status.Workloads) < 1 || len(ac.Status.Workloads[0].Traits) < 1 {
					return errors.New("workload or trait not ready")
				}
				traitName = ac.Status.Workloads[0].Traits[0].Reference.Name
				err = k8sClient.Get(ctx,
					types.NamespacedName{Namespace: ns.Name, Name: traitName},
					&createdRoute)
				if err != nil {
					return err
				}
				if len(createdRoute.Status.Ingresses) == 0 {
					return errors.New("no ingress created")
				}
				return nil
			},
			time.Second*30, time.Millisecond*500).Should(BeNil())

		By("Check that we have created the ingress")
		createdIngress := v1beta1.Ingress{}
		Eventually(
			func() error {
				return k8sClient.Get(ctx,
					types.NamespacedName{Namespace: ns.Name, Name: createdRoute.Status.Ingresses[0].Name},
					&createdIngress)
			},
			time.Second*30, time.Millisecond*500).Should(BeNil())
		logf.Log.Info("[TEST] Get the created ingress", "ingress rules", createdIngress.Spec.Rules)
		Expect(createdIngress.GetNamespace()).Should(Equal(namespaceName))
		Expect(len(createdIngress.Spec.Rules)).Should(Equal(1))
		Expect(createdIngress.Spec.Rules[0].Host).Should(Equal("mycomp.mytest.com"))
		Expect(createdIngress.Spec.Rules[0].HTTP.Paths[0].Backend.ServiceName).Should(Equal(compName))
		Expect(createdIngress.Spec.Rules[0].HTTP.Paths[0].Backend.ServicePort.IntVal).Should(Equal(int32(8080)))
	})
	It("Test with podSpec label with no podSpecPath using deployment workload", func() {
		compName := "test-deployment"
		comp, _, deploylabel := getComponent("deployment", compName)
		ac := getAC(compName)
		Expect(k8sClient.Create(ctx, &comp)).ToNot(HaveOccurred())
		Expect(k8sClient.Create(ctx, &ac)).ToNot(HaveOccurred())

		By("Check that we have created the route")
		createdRoute := v1alpha1.Route{}
		var traitName string
		Eventually(
			func() error {
				err := k8sClient.Get(ctx,
					types.NamespacedName{Namespace: ns.Name, Name: ac.Name},
					&ac)
				if err != nil {
					return err
				}
				if len(ac.Status.Workloads) < 1 || len(ac.Status.Workloads[0].Traits) < 1 {
					return errors.New("workload or trait not ready")
				}
				traitName = ac.Status.Workloads[0].Traits[0].Reference.Name
				err = k8sClient.Get(ctx,
					types.NamespacedName{Namespace: ns.Name, Name: traitName},
					&createdRoute)
				if err != nil {
					return err
				}
				if len(createdRoute.Status.Ingresses) == 0 {
					return errors.New("no ingress created")
				}
				return nil
			},
			time.Second*30, time.Millisecond*500).Should(BeNil())

		By("Check that we have created the ingress")
		createdIngress := v1beta1.Ingress{}
		Eventually(
			func() error {
				return k8sClient.Get(ctx,
					types.NamespacedName{Namespace: ns.Name, Name: createdRoute.Status.Ingresses[0].Name},
					&createdIngress)
			},
			time.Second*30, time.Millisecond*500).Should(BeNil())
		logf.Log.Info("[TEST] Get the created ingress", "ingress rules", createdIngress.Spec.Rules)
		Expect(createdIngress.GetNamespace()).Should(Equal(namespaceName))
		Expect(len(createdIngress.Spec.Rules)).Should(Equal(1))
		Expect(createdIngress.Spec.Rules[0].Host).Should(Equal("mycomp.mytest.com"))
		Expect(createdIngress.Spec.Rules[0].HTTP.Paths[0].Backend.ServiceName).Should(Equal(traitName))
		Expect(createdIngress.Spec.Rules[0].HTTP.Paths[0].Backend.ServicePort.IntVal).Should(Equal(int32(8000)))

		By("Check that we have created the service")
		createdSvc := corev1.Service{}
		Eventually(
			func() error {
				return k8sClient.Get(ctx,
					types.NamespacedName{Namespace: ns.Name, Name: traitName},
					&createdSvc)
			},
			time.Second*30, time.Millisecond*500).Should(BeNil())
		logf.Log.Info("[TEST] Get the created service", "service ports", createdSvc.Spec.Ports)
		Expect(createdSvc.Spec.Selector).Should(Equal(deploylabel))
		Expect(createdSvc.Spec.Ports[0].TargetPort.IntVal).Should(Equal(int32(podPort)))
	})
	It("Test with podSpecPath specified using deploy workload", func() {
		compName := "test-deploy"
		comp, _, _ := getComponent("deploy", compName)
		ac := getAC(compName)
		Expect(k8sClient.Create(ctx, &comp)).ToNot(HaveOccurred())
		Expect(k8sClient.Create(ctx, &ac)).ToNot(HaveOccurred())

		By("Check that we have created the route")
		createdRoute := v1alpha1.Route{}
		var traitName string
		Eventually(
			func() error {
				err := k8sClient.Get(ctx,
					types.NamespacedName{Namespace: ns.Name, Name: ac.Name},
					&ac)
				if err != nil {
					return err
				}
				if len(ac.Status.Workloads) < 1 || len(ac.Status.Workloads[0].Traits) < 1 {
					return errors.New("workload or trait not ready")
				}
				traitName = ac.Status.Workloads[0].Traits[0].Reference.Name
				err = k8sClient.Get(ctx,
					types.NamespacedName{Namespace: ns.Name, Name: traitName},
					&createdRoute)
				if err != nil {
					return err
				}
				if len(createdRoute.Status.Ingresses) == 0 {
					return errors.New("no ingress created")
				}
				return nil
			},
			time.Second*30, time.Millisecond*500).Should(BeNil())

		By("Check that we have created the ingress")
		createdIngress := v1beta1.Ingress{}
		Eventually(
			func() error {
				return k8sClient.Get(ctx,
					types.NamespacedName{Namespace: ns.Name, Name: createdRoute.Status.Ingresses[0].Name},
					&createdIngress)
			},
			time.Second*30, time.Millisecond*500).Should(BeNil())
		logf.Log.Info("[TEST] Get the created ingress", "ingress rules", createdIngress.Spec.Rules)
		Expect(createdIngress.GetNamespace()).Should(Equal(namespaceName))
		Expect(len(createdIngress.Spec.Rules)).Should(Equal(1))
		Expect(createdIngress.Spec.Rules[0].Host).Should(Equal("mycomp.mytest.com"))
		Expect(createdIngress.Spec.Rules[0].HTTP.Paths[0].Backend.ServiceName).Should(Equal(traitName))
		Expect(createdIngress.Spec.Rules[0].HTTP.Paths[0].Backend.ServicePort.IntVal).Should(Equal(int32(8000)))

		By("Check that we have created the service")
		createdSvc := corev1.Service{}
		Eventually(
			func() error {
				return k8sClient.Get(ctx,
					types.NamespacedName{Namespace: ns.Name, Name: traitName},
					&createdSvc)
			},
			time.Second*30, time.Millisecond*500).Should(BeNil())
		logf.Log.Info("[TEST] Get the created service", "service ports", createdSvc.Spec.Ports)
		for k, v := range map[string]string{"app.oam.dev/component": compName, "app.oam.dev/name": "test-app-" + compName} {
			Expect(createdSvc.Spec.Selector).Should(HaveKeyWithValue(k, v))
		}
		Expect(createdSvc.Spec.Ports[0].TargetPort.IntVal).Should(Equal(int32(podPort)))
	})
})
