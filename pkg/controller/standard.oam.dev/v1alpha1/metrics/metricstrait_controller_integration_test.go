package metrics

import (
	"context"
	"reflect"
	"time"

	monitoringv1 "github.com/coreos/prometheus-operator/pkg/apis/monitoring/v1"
	runtimev1alpha1 "github.com/crossplane/crossplane-runtime/apis/core/v1alpha1"
	"github.com/crossplane/oam-kubernetes-runtime/pkg/oam/util"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/utils/pointer"
	logf "sigs.k8s.io/controller-runtime/pkg/log"

	"github.com/oam-dev/kubevela/apis/standard.oam.dev/v1alpha1"
)

var (
	metricsTraitKind       = reflect.TypeOf(v1alpha1.MetricsTrait{}).Name()
	metricsTraitAPIVersion = v1alpha1.SchemeGroupVersion.String()
	deploymentKind         = reflect.TypeOf(appsv1.Deployment{}).Name()
	deploymentAPIVersion   = appsv1.SchemeGroupVersion.String()
)

var _ = Describe("Metrics Trait Integration Test", func() {
	// common var init
	ctx := context.Background()
	namespaceName := "metricstrait-integration-test"
	traitLabel := map[string]string{"trait": "metricsTraitBase"}
	deployLabel := map[string]string{"standard.oam.dev": "oam-test-deployment"}
	podPort := 8080
	targetPort := intstr.FromInt(podPort)
	metricsPath := "/notMetrics"
	scheme := "http"
	var ns corev1.Namespace
	var metricsTraitBase v1alpha1.MetricsTrait
	var workloadBase appsv1.Deployment

	BeforeEach(func() {
		logf.Log.Info("[TEST] Set up resources before an integration test")
		ns = corev1.Namespace{
			ObjectMeta: metav1.ObjectMeta{
				Name: namespaceName,
			},
		}
		By("Create the Namespace for test")
		Expect(k8sClient.Create(ctx, &ns)).Should(SatisfyAny(Succeed(), &util.AlreadyExistMatcher{}))
		metricsTraitBase = v1alpha1.MetricsTrait{
			TypeMeta: metav1.TypeMeta{
				Kind:       metricsTraitKind,
				APIVersion: metricsTraitAPIVersion,
			},
			ObjectMeta: metav1.ObjectMeta{
				Namespace: ns.Name,
				Labels:    traitLabel,
			},
			Spec: v1alpha1.MetricsTraitSpec{
				ScrapeService: v1alpha1.ScapeServiceEndPoint{
					TargetPort: targetPort,
					Path:       metricsPath,
					Scheme:     scheme,
					Enabled:    pointer.BoolPtr(true),
				},
				WorkloadReference: runtimev1alpha1.TypedReference{
					APIVersion: deploymentAPIVersion,
					Kind:       deploymentKind,
				},
			},
		}
		workloadBase = appsv1.Deployment{
			TypeMeta: metav1.TypeMeta{
				Kind:       deploymentKind,
				APIVersion: deploymentAPIVersion,
			},
			ObjectMeta: metav1.ObjectMeta{
				Namespace: ns.Name,
				Labels:    deployLabel,
			},
			Spec: appsv1.DeploymentSpec{
				Selector: &metav1.LabelSelector{
					MatchLabels: deployLabel,
				},
				Template: corev1.PodTemplateSpec{
					ObjectMeta: metav1.ObjectMeta{
						Labels: deployLabel,
					},
					Spec: corev1.PodSpec{
						Containers: []corev1.Container{
							{
								Name:            "container-name",
								Image:           "alpine",
								ImagePullPolicy: corev1.PullNever,
								Command:         []string{"containerCommand"},
								Args:            []string{"containerArguments"},
								Ports: []corev1.ContainerPort{
									{
										ContainerPort: int32(podPort),
									},
								},
							},
						},
					},
				},
			},
		}
	})

	AfterEach(func() {
		// Control-runtime test environment has a bug that can't delete resources like deployment/namespaces
		// We have to use different names to segregate between tests
		logf.Log.Info("[TEST] Clean up resources after an integration test")
	})

	It("Test with deployment as workloadBase without selector", func() {
		testName := "deploy-without-selector"
		By("Create the deployment as the workloadBase")
		workload := workloadBase
		workload.Name = testName + "-workload"
		Expect(k8sClient.Create(ctx, &workload)).ToNot(HaveOccurred())

		By("Create the metrics trait pointing to the workloadBase")
		metricsTrait := metricsTraitBase
		metricsTrait.Name = testName + "-trait"
		metricsTrait.Spec.WorkloadReference.Name = workload.Name
		Expect(k8sClient.Create(ctx, &metricsTrait)).ToNot(HaveOccurred())

		By("Check that we have created the service")
		createdService := corev1.Service{}
		Eventually(
			func() error {
				return k8sClient.Get(ctx,
					types.NamespacedName{Namespace: ns.Name, Name: "oam-" + workload.GetName()},
					&createdService)
			},
			time.Second*10, time.Millisecond*500).Should(BeNil())
		logf.Log.Info("[TEST] Get the created service", "service ports", createdService.Spec.Ports)
		Expect(createdService.GetNamespace()).Should(Equal(namespaceName))
		Expect(createdService.Labels).Should(Equal(GetOAMServiceLabel()))
		Expect(len(createdService.Spec.Ports)).Should(Equal(1))
		Expect(createdService.Spec.Ports[0].Port).Should(BeEquivalentTo(servicePort))
		Expect(createdService.Spec.Selector).Should(Equal(deployLabel))
		By("Check that we have created the serviceMonitor in the pre-defined namespaceName")
		var serviceMonitor monitoringv1.ServiceMonitor
		Eventually(
			func() error {
				return k8sClient.Get(ctx,
					types.NamespacedName{Namespace: ServiceMonitorNSName, Name: metricsTrait.GetName()},
					&serviceMonitor)
			},
			time.Second*5, time.Millisecond*50).Should(BeNil())
		logf.Log.Info("[TEST] Get the created serviceMonitor", "service end ports", serviceMonitor.Spec.Endpoints)
		Expect(serviceMonitor.GetNamespace()).Should(Equal(ServiceMonitorNSName))
		Expect(serviceMonitor.Spec.Selector.MatchLabels).Should(Equal(GetOAMServiceLabel()))
		Expect(serviceMonitor.Spec.Selector.MatchExpressions).Should(BeNil())
		Expect(serviceMonitor.Spec.NamespaceSelector.MatchNames).Should(Equal([]string{metricsTrait.Namespace}))
		Expect(serviceMonitor.Spec.NamespaceSelector.Any).Should(BeFalse())
		Expect(len(serviceMonitor.Spec.Endpoints)).Should(Equal(1))
		Expect(serviceMonitor.Spec.Endpoints[0].Port).Should(BeEmpty())
		Expect(*serviceMonitor.Spec.Endpoints[0].TargetPort).Should(BeEquivalentTo(targetPort))
		Expect(serviceMonitor.Spec.Endpoints[0].Scheme).Should(Equal(scheme))
		Expect(serviceMonitor.Spec.Endpoints[0].Path).Should(Equal(metricsPath))
	})

	It("Test with deployment as workloadBase selector", func() {
		testName := "deploy-with-selector"
		By("Create the deployment as the workloadBase")
		workload := workloadBase.DeepCopy()
		workload.Name = testName + "-workload"
		Expect(k8sClient.Create(ctx, workload)).ToNot(HaveOccurred())

		By("Create the metrics trait pointing to the workloadBase")
		podSelector := map[string]string{"podlabel": "goodboy"}
		metricsTrait := metricsTraitBase
		metricsTrait.Name = testName + "-trait"
		metricsTrait.Spec.WorkloadReference.Name = workload.Name
		metricsTrait.Spec.ScrapeService.TargetSelector = podSelector
		Expect(k8sClient.Create(ctx, &metricsTrait)).ToNot(HaveOccurred())

		By("Check that we have created the service")
		createdService := corev1.Service{}
		Eventually(
			func() error {
				return k8sClient.Get(ctx,
					types.NamespacedName{Namespace: ns.Name, Name: "oam-" + workload.GetName()},
					&createdService)
			},
			time.Second*10, time.Millisecond*500).Should(BeNil())
		logf.Log.Info("[TEST] Get the created service", "service ports", createdService.Spec.Ports)
		Expect(createdService.Labels).Should(Equal(GetOAMServiceLabel()))
		Expect(createdService.Spec.Selector).Should(Equal(deployLabel))
		By("Check that we have created the serviceMonitor in the pre-defined namespaceName")
		var serviceMonitor monitoringv1.ServiceMonitor
		Eventually(
			func() error {
				return k8sClient.Get(ctx,
					types.NamespacedName{Namespace: ServiceMonitorNSName, Name: metricsTrait.GetName()},
					&serviceMonitor)
			},
			time.Second*5, time.Millisecond*50).Should(BeNil())
		logf.Log.Info("[TEST] Get the created serviceMonitor", "service end ports", serviceMonitor.Spec.Endpoints)
		Expect(serviceMonitor.Spec.Selector.MatchLabels).Should(Equal(GetOAMServiceLabel()))
		Expect(serviceMonitor.Spec.Selector.MatchExpressions).Should(BeNil())
		Expect(serviceMonitor.Spec.NamespaceSelector.MatchNames).Should(Equal([]string{metricsTrait.Namespace}))
		Expect(serviceMonitor.Spec.NamespaceSelector.Any).Should(BeFalse())
		Expect(len(serviceMonitor.Spec.Endpoints)).Should(Equal(1))
		Expect(serviceMonitor.Spec.Endpoints[0].Port).Should(BeEmpty())
		Expect(*serviceMonitor.Spec.Endpoints[0].TargetPort).Should(BeEquivalentTo(targetPort))
		Expect(serviceMonitor.Spec.Endpoints[0].Scheme).Should(Equal(scheme))
		Expect(serviceMonitor.Spec.Endpoints[0].Path).Should(Equal(metricsPath))
	})
})
