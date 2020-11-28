package controllers_test

import (
	"context"
	"fmt"
	"time"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/util/intstr"
	utilpointer "k8s.io/utils/pointer"
	"sigs.k8s.io/controller-runtime/pkg/client"
	logf "sigs.k8s.io/controller-runtime/pkg/log"

	"github.com/crossplane/oam-kubernetes-runtime/apis/core/v1alpha2"
	"github.com/crossplane/oam-kubernetes-runtime/pkg/oam"
	"github.com/crossplane/oam-kubernetes-runtime/pkg/oam/util"
)

var _ = Describe("ContainerizedWorkload", func() {
	ctx := context.TODO()
	var namespace, fakeLabelKey, fakeAnnotationKey, componentName, workloadInstanceName, imageName string
	var replica int32
	var ns corev1.Namespace
	var wd v1alpha2.WorkloadDefinition
	var label map[string]string
	var annotations map[string]string
	var wl v1alpha2.ContainerizedWorkload
	var comp v1alpha2.Component
	var appConfig v1alpha2.ApplicationConfiguration
	var mts v1alpha2.ManualScalerTrait

	BeforeEach(func() {
		// init the strings
		namespace = "containerized-workload-test"
		fakeLabelKey = "workload"
		fakeAnnotationKey = "kubectl.kubernetes.io/last-applied-configuration"
		componentName = "example-component"
		workloadInstanceName = "example-appconfig-workload"
		imageName = "wordpress:php7.2"

		logf.Log.Info("Start to run a test, clean up previous resources")
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
			// gomega has a bug that can't take nil as the actual input, so has to make it a func
			func() error {
				return k8sClient.Get(ctx, objectKey, res)
			},
			time.Second*120, time.Millisecond*500).Should(&util.NotFoundMatcher{})
		// recreate it
		Eventually(
			func() error {
				return k8sClient.Create(ctx, &ns)
			},
			time.Second*3, time.Millisecond*300).Should(SatisfyAny(BeNil(), &util.AlreadyExistMatcher{}))
		// create a workload definition
		label = map[string]string{fakeLabelKey: "containerized-workload"}
		wd = v1alpha2.WorkloadDefinition{
			ObjectMeta: metav1.ObjectMeta{
				Name:   "containerizedworkloads.core.oam.dev",
				Labels: label,
			},
			Spec: v1alpha2.WorkloadDefinitionSpec{
				Reference: v1alpha2.DefinitionReference{
					Name: "containerizedworkloads.core.oam.dev",
				},
				ChildResourceKinds: []v1alpha2.ChildResourceKind{
					{
						APIVersion: corev1.SchemeGroupVersion.String(),
						Kind:       util.KindService,
					},
					{
						APIVersion: appsv1.SchemeGroupVersion.String(),
						Kind:       util.KindDeployment,
					},
				},
			},
		}
		// create a workload CR
		configFileValue := "testValue"
		wl = v1alpha2.ContainerizedWorkload{
			ObjectMeta: metav1.ObjectMeta{
				Namespace: namespace,
				Labels:    label,
			},
			Spec: v1alpha2.ContainerizedWorkloadSpec{
				Containers: []v1alpha2.Container{
					{
						Name:  "wordpress",
						Image: "wordpress:4.6.1-apache",
						Ports: []v1alpha2.ContainerPort{
							{
								Name: "wordpress",
								Port: 80,
							},
						},
						ConfigFiles: []v1alpha2.ContainerConfigFile{
							{
								Path:  "/test/path/config",
								Value: &configFileValue,
							},
						},
					},
				},
			},
		}
		// reflect workload gvk from scheme
		gvks, _, _ := scheme.ObjectKinds(&wl)
		wl.APIVersion = gvks[0].GroupVersion().String()
		wl.Kind = gvks[0].Kind
		// Create a component definition
		comp = v1alpha2.Component{
			ObjectMeta: metav1.ObjectMeta{
				Name:      componentName,
				Namespace: namespace,
				Labels:    label,
			},
			Spec: v1alpha2.ComponentSpec{
				Workload: runtime.RawExtension{
					Object: &wl,
				},
				Parameters: []v1alpha2.ComponentParameter{
					{
						Name:       "instance-name",
						Required:   utilpointer.BoolPtr(true),
						FieldPaths: []string{"metadata.name"},
					},
					{
						Name:       "image",
						Required:   utilpointer.BoolPtr(false),
						FieldPaths: []string{"spec.containers[0].image"},
					},
				},
			},
		}
		// Create a manualscaler trait CR
		replica = 3
		mts = v1alpha2.ManualScalerTrait{
			ObjectMeta: metav1.ObjectMeta{
				Namespace: namespace,
				Name:      "sample-manualscaler-trait",
				Labels:    label,
			},
			Spec: v1alpha2.ManualScalerTraitSpec{
				ReplicaCount: replica,
			},
		}
		// reflect trait gvk from scheme
		gvks, _, _ = scheme.ObjectKinds(&mts)
		mts.APIVersion = gvks[0].GroupVersion().String()
		mts.Kind = gvks[0].Kind
		// Create application configuration
		workloadInstanceName := "example-appconfig-workload"
		imageName := "wordpress:php7.2"
		annotations = map[string]string{fakeAnnotationKey: "fake-annotation-key-item"}
		appConfig = v1alpha2.ApplicationConfiguration{
			ObjectMeta: metav1.ObjectMeta{
				Name:        "example-appconfig",
				Namespace:   namespace,
				Labels:      label,
				Annotations: annotations,
			},
			Spec: v1alpha2.ApplicationConfigurationSpec{
				Components: []v1alpha2.ApplicationConfigurationComponent{
					{
						ComponentName: componentName,
						ParameterValues: []v1alpha2.ComponentParameterValue{
							{
								Name:  "instance-name",
								Value: intstr.IntOrString{StrVal: workloadInstanceName, Type: intstr.String},
							},
							{
								Name:  "image",
								Value: intstr.IntOrString{StrVal: imageName, Type: intstr.String},
							},
						},
						Traits: []v1alpha2.ComponentTrait{
							{
								Trait: runtime.RawExtension{
									Object: &mts,
								},
							},
						},
					},
				},
			},
		}
	})
	AfterEach(func() {
		logf.Log.Info("Clean up resources")
		// delete the namespace with all its resources
		Expect(k8sClient.Delete(ctx, &ns, client.PropagationPolicy(metav1.DeletePropagationForeground))).Should(BeNil())
	})

	It("apply an application config", func() {
		logf.Log.Info("Creating workload definition")
		// For some reason, WorkloadDefinition is created as a Cluster scope object
		Expect(k8sClient.Create(ctx, &wd)).Should(SatisfyAny(BeNil(), &util.AlreadyExistMatcher{}))
		logf.Log.Info("Creating component", "Name", comp.Name, "Namespace", comp.Namespace)
		Expect(k8sClient.Create(ctx, &comp)).Should(BeNil())
		logf.Log.Info("Creating application config", "Name", appConfig.Name, "Namespace", appConfig.Namespace)
		Expect(k8sClient.Create(ctx, &appConfig)).Should(BeNil())
		// Verification
		By("Checking containerizedworkload is created")
		cw := &v1alpha2.ContainerizedWorkload{}
		Eventually(func() error {
			return k8sClient.Get(ctx, client.ObjectKey{Name: workloadInstanceName, Namespace: namespace}, cw)
		}, time.Second*15, time.Millisecond*500).Should(BeNil())

		By("Checking ManuelScalerTrait is created")
		Eventually(func() error {
			return k8sClient.Get(ctx, client.ObjectKey{Name: mts.Name, Namespace: namespace}, &mts)
		}, time.Second*15, time.Millisecond*500).Should(BeNil())

		By("Checking labels")
		cwLabels := cw.GetLabels()
		Expect(cwLabels).Should(SatisfyAll(
			HaveKey(fakeLabelKey), // propogated from appConfig
			HaveKey(oam.LabelAppComponent),
			HaveKey(oam.LabelAppComponentRevision),
			HaveKey(oam.LabelAppName),
			HaveKey(oam.LabelOAMResourceType)))

		cwLabels = mts.GetLabels()
		Expect(cwLabels).Should(SatisfyAll(
			HaveKey(fakeLabelKey), // propogated from appConfig
			HaveKey(oam.LabelAppComponent),
			HaveKey(oam.LabelAppComponentRevision),
			HaveKey(oam.LabelAppName),
			HaveKey(oam.LabelOAMResourceType)))

		By("Checking ConfigMap is created")
		cmObjectKey := client.ObjectKey{
			Namespace: namespace,
			Name:      fmt.Sprintf("%s-%s-3972676475", workloadInstanceName, "wordpress"),
		}
		configMap := &corev1.ConfigMap{}
		logf.Log.Info("Checking on configMap", "Key", cmObjectKey)
		Eventually(
			func() error {
				return k8sClient.Get(ctx, cmObjectKey, configMap)
			},
			time.Second*15, time.Millisecond*500).Should(BeNil())

		By("Checking deployment is created")
		objectKey := client.ObjectKey{
			Name:      workloadInstanceName,
			Namespace: namespace,
		}
		deploy := &appsv1.Deployment{}
		logf.Log.Info("Checking on deployment", "Key", objectKey)
		Eventually(
			func() error {
				return k8sClient.Get(ctx, objectKey, deploy)
			},
			time.Second*15, time.Millisecond*500).Should(BeNil())

		By("Verify that the parameter substitute works")
		Expect(deploy.Spec.Template.Spec.Containers[0].Image).Should(Equal(imageName))

		// Verification
		By("Checking service is created")
		service := &corev1.Service{}
		logf.Log.Info("Checking on service", "Key", objectKey)
		Eventually(
			func() error {
				return k8sClient.Get(ctx, objectKey, service)
			},
			time.Second*15, time.Millisecond*500).Should(BeNil())

		By("Verify deployment scaled according to the manualScaler trait")
		Eventually(
			func() int32 {
				k8sClient.Get(ctx, objectKey, deploy)
				return deploy.Status.Replicas
			},
			time.Second*60, time.Second*5).Should(BeEquivalentTo(replica))
		Expect(*deploy.Spec.Replicas).Should(BeEquivalentTo(replica))
		By("Verify pod annotations should not contain kubectl.kubernetes.io/last-applied-configuration")
		Eventually(
			func() bool {
				k8sClient.Get(ctx, objectKey, deploy)
				annotations := deploy.Spec.Template.Annotations
				if _, ok := annotations[fakeAnnotationKey]; ok {
					return false
				}
				return true
			},
			time.Second*15, time.Millisecond*500).Should(BeTrue())
	})

	It("checking appConfig status changed outside of the controller loop is preserved", func() {
		logf.Log.Info("Creating workload definition")
		// For some reason, WorkloadDefinition is created as a Cluster scope object
		Expect(k8sClient.Create(ctx, &wd)).Should(SatisfyAny(BeNil(), &util.AlreadyExistMatcher{}))
		logf.Log.Info("Creating component", "Name", comp.Name, "Namespace", comp.Namespace)
		Expect(k8sClient.Create(ctx, &comp)).Should(BeNil())
		logf.Log.Info("Creating application config", "Name", appConfig.Name, "Namespace", appConfig.Namespace)
		Expect(k8sClient.Create(ctx, &appConfig)).Should(BeNil())
		By("Verify deployment scaled according to the manualScaler trait")
		objectKey := client.ObjectKey{
			Name:      workloadInstanceName,
			Namespace: namespace,
		}
		deploy := &appsv1.Deployment{}
		Eventually(
			func() int32 {
				k8sClient.Get(ctx, objectKey, deploy)
				return deploy.Status.Replicas
			},
			time.Second*60, time.Second*5).Should(BeEquivalentTo(replica))
		Expect(*deploy.Spec.Replicas).Should(BeEquivalentTo(replica))
		// Verify that the status is set
		By("Add applicationConfiguration status")
		var appConfigWithStatus v1alpha2.ApplicationConfiguration
		appKey := client.ObjectKey{
			Name:      appConfig.Name,
			Namespace: namespace,
		}
		Expect(k8sClient.Get(ctx, appKey, &appConfigWithStatus)).ShouldNot(HaveOccurred())
		logf.Log.Info("get appConfig status", "workload status", appConfigWithStatus.Status.Workloads[0])
		appConfigWithStatus.Status.Workloads[0].Status = "ready"
		appConfigWithStatus.Status.Workloads[0].Traits[0].Status = "running"
		Expect(k8sClient.Status().Update(ctx, &appConfigWithStatus)).ShouldNot(HaveOccurred())
		By("Checking appConfig status is updated")
		appConfigWithStatus = v1alpha2.ApplicationConfiguration{}
		Expect(k8sClient.Get(ctx, appKey, &appConfigWithStatus)).ShouldNot(HaveOccurred())
		Expect(appConfigWithStatus.Status.Workloads[0].Status).Should(Equal("ready"))
		Expect(appConfigWithStatus.Status.Workloads[0].Traits[0].Status).Should(BeEquivalentTo("running"))
		By("Checking appConfig status is updated again")
		appConfigWithStatus = v1alpha2.ApplicationConfiguration{}
		Expect(k8sClient.Get(ctx, appKey, &appConfigWithStatus)).ShouldNot(HaveOccurred())
		Expect(appConfigWithStatus.Status.Workloads[0].Status).Should(Equal("ready"))
		Expect(appConfigWithStatus.Status.Workloads[0].Traits[0].Status).Should(BeEquivalentTo("running"))
		By("change the appConfig, scale down the replicate")
		mts.Spec.ReplicaCount = 1
		appConfigWithStatus.Spec.Components[0].Traits[0].Trait.Raw = util.JSONMarshal(mts)
		Expect(k8sClient.Update(ctx, &appConfigWithStatus)).ShouldNot(HaveOccurred())
		By("Verify deployment scaled according to the manualScaler trait")
		Eventually(
			func() int32 {
				k8sClient.Get(ctx, objectKey, deploy)
				return deploy.Status.Replicas
			},
			time.Second*120, time.Second*5).Should(BeEquivalentTo(1))
		By("Checking appConfig status is preserved through reconcile")
		appConfigWithStatus = v1alpha2.ApplicationConfiguration{}
		Expect(k8sClient.Get(ctx, appKey, &appConfigWithStatus)).ShouldNot(HaveOccurred())
		Expect(appConfigWithStatus.Status.Workloads[0].Status).Should(Equal("ready"))
		Expect(appConfigWithStatus.Status.Workloads[0].Traits[0].Status).Should(BeEquivalentTo("running"))
	})
})
