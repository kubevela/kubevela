package controllers_test

import (
	"context"
	"time"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"

	"github.com/crossplane/crossplane-runtime/apis/core/v1alpha1"
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

var (
	varInt32_60 int32 = 60
)

var _ = Describe("HealthScope", func() {
	ctx := context.Background()
	namespace := "health-scope-test"
	trueVar := true
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

	It("Test an application config with health scope", func() {
		healthScopeName := "example-health-scope"
		// create health scope definition
		sd := v1alpha2.ScopeDefinition{
			ObjectMeta: metav1.ObjectMeta{
				Name: "healthscope.core.oam.dev",
			},
			Spec: v1alpha2.ScopeDefinitionSpec{
				AllowComponentOverlap: true,
				WorkloadRefsPath:      "spec.workloadRefs",
				Reference: v1alpha2.DefinitionReference{
					Name: "healthscope.core.oam.dev",
				},
			},
		}
		logf.Log.Info("Creating health scope definition")
		Expect(k8sClient.Create(ctx, &sd)).Should(SatisfyAny(BeNil(), &util.AlreadyExistMatcher{}))

		// create health scope.
		hs := v1alpha2.HealthScope{
			ObjectMeta: metav1.ObjectMeta{
				Name:      healthScopeName,
				Namespace: namespace,
			},
			Spec: v1alpha2.HealthScopeSpec{
				ProbeTimeout:       &varInt32_60,
				WorkloadReferences: []v1alpha1.TypedReference{},
			},
		}
		logf.Log.Info("Creating health scope")
		Expect(k8sClient.Create(ctx, &hs)).Should(SatisfyAny(BeNil(), &util.AlreadyExistMatcher{}))
		By("Check empty health scope is healthy")
		Eventually(func() v1alpha2.HealthStatus {
			k8sClient.Get(ctx, client.ObjectKey{Namespace: namespace, Name: healthScopeName}, &hs)
			return hs.Status.ScopeHealthCondition.HealthStatus
		}, time.Second*30, time.Millisecond*500).Should(Equal(v1alpha2.StatusHealthy))

		label := map[string]string{"workload": "containerized-workload"}
		// create a workload definition
		wd := v1alpha2.WorkloadDefinition{
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
		logf.Log.Info("Creating workload definition")
		// For some reason, WorkloadDefinition is created as a Cluster scope object
		Expect(k8sClient.Create(ctx, &wd)).Should(SatisfyAny(BeNil(), &util.AlreadyExistMatcher{}))
		// create a workload CR
		wl := v1alpha2.ContainerizedWorkload{
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
					},
				},
			},
		}
		// reflect workload gvk from scheme
		gvks, _, _ := scheme.ObjectKinds(&wl)
		wl.APIVersion = gvks[0].GroupVersion().String()
		wl.Kind = gvks[0].Kind
		// Create a component definition
		componentName := "example-component"
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
						Name:       "instance-name",
						Required:   &trueVar,
						FieldPaths: []string{"metadata.name"},
					},
					{
						Name:       "image",
						Required:   &falseVar,
						FieldPaths: []string{"spec.containers[0].image"},
					},
				},
			},
		}
		logf.Log.Info("Creating component", "Name", comp.Name, "Namespace", comp.Namespace)
		Expect(k8sClient.Create(ctx, &comp)).Should(BeNil())
		// Create application configuration
		workloadInstanceName1 := "example-appconfig-healthscope-a"
		workloadInstanceName2 := "example-appconfig-healthscope-b"
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
								Name:  "instance-name",
								Value: intstr.IntOrString{StrVal: workloadInstanceName1, Type: intstr.String},
							},
							{
								Name:  "image",
								Value: intstr.IntOrString{StrVal: imageName, Type: intstr.String},
							},
						},
						Scopes: []v1alpha2.ComponentScope{
							{
								ScopeReference: v1alpha1.TypedReference{
									APIVersion: gvks[0].GroupVersion().String(),
									Kind:       v1alpha2.HealthScopeGroupVersionKind.Kind,
									Name:       healthScopeName,
								},
							},
						},
					},
					{
						ComponentName: componentName,
						ParameterValues: []v1alpha2.ComponentParameterValue{
							{
								Name:  "instance-name",
								Value: intstr.IntOrString{StrVal: workloadInstanceName2, Type: intstr.String},
							},
							{
								Name:  "image",
								Value: intstr.IntOrString{StrVal: imageName, Type: intstr.String},
							},
						},
						Scopes: []v1alpha2.ComponentScope{
							{
								ScopeReference: v1alpha1.TypedReference{
									APIVersion: gvks[0].GroupVersion().String(),
									Kind:       v1alpha2.HealthScopeGroupVersionKind.Kind,
									Name:       healthScopeName,
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
		By("Checking deployment-a is created")
		objectKey := client.ObjectKey{
			Name:      workloadInstanceName1,
			Namespace: namespace,
		}
		deploy := &appsv1.Deployment{}
		logf.Log.Info("Checking on deployment", "Key", objectKey)
		Eventually(
			func() error {
				return k8sClient.Get(ctx, objectKey, deploy)
			},
			time.Second*15, time.Millisecond*500).Should(BeNil())

		// Verify all components declared in AppConfig are created
		By("Checking deployment-b is created")
		objectKey2 := client.ObjectKey{
			Name:      workloadInstanceName2,
			Namespace: namespace,
		}
		deploy2 := &appsv1.Deployment{}
		logf.Log.Info("Checking on deployment", "Key", objectKey2)
		Eventually(
			func() error {
				return k8sClient.Get(ctx, objectKey2, deploy2)
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

		healthScopeObject := client.ObjectKey{
			Name:      healthScopeName,
			Namespace: namespace,
		}
		healthScope := &v1alpha2.HealthScope{}
		By("Verify health scope")
		Eventually(
			func() v1alpha2.ScopeHealthCondition {
				*healthScope = v1alpha2.HealthScope{}
				k8sClient.Get(ctx, healthScopeObject, healthScope)
				logf.Log.Info("Checking on health scope",
					"len(WorkloadReferences)",
					len(healthScope.Spec.WorkloadReferences),
					"health",
					healthScope.Status.ScopeHealthCondition)
				return healthScope.Status.ScopeHealthCondition
			},
			time.Second*120, time.Second*5).Should(Equal(v1alpha2.ScopeHealthCondition{
			HealthStatus:     v1alpha2.StatusHealthy,
			Total:            int64(2),
			HealthyWorkloads: int64(2),
		}))
	})
})
