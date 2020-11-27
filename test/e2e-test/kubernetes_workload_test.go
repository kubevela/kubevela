package controllers_test

import (
	"context"
	"time"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/util/intstr"
	"sigs.k8s.io/controller-runtime/pkg/client"
	logf "sigs.k8s.io/controller-runtime/pkg/log"

	"github.com/crossplane/oam-kubernetes-runtime/apis/core/v1alpha2"
	"github.com/crossplane/oam-kubernetes-runtime/pkg/oam/util"
)

var _ = Describe("Test kubernetes native workloads", func() {
	ctx := context.Background()
	namespace := "kubernetes-workload-test"
	falseVar := false
	ns := corev1.Namespace{
		ObjectMeta: metav1.ObjectMeta{
			Name: namespace,
		},
	}
	BeforeEach(func() {
		logf.Log.Info("Start to run a test, clean up previous resources")
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

	})
	AfterEach(func() {
		logf.Log.Info("Clean up resources")
		// delete the namespace with all its resources
		Expect(k8sClient.Delete(ctx, &ns, client.PropagationPolicy(metav1.DeletePropagationForeground))).Should(BeNil())
	})

	It("use deployment workload", func() {
		label := map[string]string{"workload": "deployment"}
		// create a workload definition for
		wd := v1alpha2.WorkloadDefinition{
			ObjectMeta: metav1.ObjectMeta{
				Name:   "deployments.apps",
				Labels: label,
			},
			Spec: v1alpha2.WorkloadDefinitionSpec{
				Reference: v1alpha2.DefinitionReference{
					Name: "deployments.apps",
				},
			},
		}
		logf.Log.Info("Creating workload definition for deployment")
		// For some reason, WorkloadDefinition is created as a Cluster scope object
		Expect(k8sClient.Create(ctx, &wd)).Should(SatisfyAny(BeNil(), &util.AlreadyExistMatcher{}))
		// create a workload CR
		workloadName := "example-deployment-workload"
		wl := appsv1.Deployment{
			ObjectMeta: metav1.ObjectMeta{
				Namespace: namespace,
				Name:      workloadName,
			},
			Spec: appsv1.DeploymentSpec{
				Selector: &metav1.LabelSelector{
					MatchLabels: label,
				},
				Template: corev1.PodTemplateSpec{
					ObjectMeta: metav1.ObjectMeta{
						Namespace: namespace,
						Labels:    label,
					},
					Spec: corev1.PodSpec{
						Containers: []corev1.Container{
							{
								Name:  "wordpress",
								Image: "wordpress:4.6.1-apache",
								Ports: []corev1.ContainerPort{
									{
										Name:          "wordpress",
										HostPort:      80,
										ContainerPort: 8080,
									},
								},
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
		componentName := "example-deployment-workload"
		comp := v1alpha2.Component{
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
						Name:       "image",
						Required:   &falseVar,
						FieldPaths: []string{"spec.template.spec.containers[0].image"},
					},
				},
			},
		}
		logf.Log.Info("Creating component", "Name", comp.Name, "Namespace", comp.Namespace)
		Expect(k8sClient.Create(ctx, &comp)).Should(BeNil())
		// Create a manualscaler trait CR
		var replica int32 = 5
		mts := v1alpha2.ManualScalerTrait{
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
		imageName := "wordpress:php7.2"
		appConfig := v1alpha2.ApplicationConfiguration{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "example-appconfig",
				Namespace: namespace,
				Labels:    label,
			},
			Spec: v1alpha2.ApplicationConfigurationSpec{
				Components: []v1alpha2.ApplicationConfigurationComponent{
					{
						ComponentName: componentName,
						ParameterValues: []v1alpha2.ComponentParameterValue{
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
		logf.Log.Info("Creating application config", "Name", appConfig.Name, "Namespace", appConfig.Namespace)
		Expect(k8sClient.Create(ctx, &appConfig)).Should(BeNil())
		// Verification
		By("Checking deployment is created")
		objectKey := client.ObjectKey{
			Name:      workloadName,
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

		By("Verify deployment scaled according to the manualScaler trait")
		Eventually(
			func() int32 {
				k8sClient.Get(ctx, objectKey, deploy)
				return deploy.Status.Replicas
			},
			time.Second*60, time.Second*5).Should(BeEquivalentTo(replica))
		Expect(*deploy.Spec.Replicas).Should(BeEquivalentTo(replica))
	})
})
