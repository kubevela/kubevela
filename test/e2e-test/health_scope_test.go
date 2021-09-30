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
	"strings"
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

	"github.com/oam-dev/kubevela/apis/core.oam.dev/common"
	"github.com/oam-dev/kubevela/apis/core.oam.dev/v1alpha2"
	"github.com/oam-dev/kubevela/apis/core.oam.dev/v1beta1"
	"github.com/oam-dev/kubevela/pkg/oam/util"
	utilcommon "github.com/oam-dev/kubevela/pkg/utils/common"
)

var (
	varInt32_60 int32 = 60
)

var _ = Describe("HealthScope", func() {
	ctx := context.Background()
	var namespace string
	trueVar := true
	falseVar := false
	var ns corev1.Namespace
	BeforeEach(func() {
		namespace = randomNamespaceName("health-scope-test")
		ns = corev1.Namespace{
			ObjectMeta: metav1.ObjectMeta{
				Name: namespace,
			},
		}
		Expect(k8sClient.Create(ctx, &ns)).Should(Succeed())

		// create health scope definition
		sd := v1alpha2.ScopeDefinition{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "healthscope.core.oam.dev",
				Namespace: "vela-system",
			},
			Spec: v1alpha2.ScopeDefinitionSpec{
				AllowComponentOverlap: true,
				WorkloadRefsPath:      "spec.workloadRefs",
				Reference: common.DefinitionReference{
					Name: "healthscope.core.oam.dev",
				},
			},
		}
		logf.Log.Info("Creating health scope definition")
		Expect(k8sClient.Create(ctx, &sd)).Should(SatisfyAny(BeNil(), &util.AlreadyExistMatcher{}))
	})
	AfterEach(func() {
		logf.Log.Info("Clean up resources")
		Expect(k8sClient.DeleteAllOf(ctx, &v1alpha2.HealthScope{}, client.InNamespace(namespace))).Should(BeNil())
		// delete the namespace with all its resources
		Expect(k8sClient.Delete(ctx, &ns, client.PropagationPolicy(metav1.DeletePropagationForeground))).Should(BeNil())
	})

	It("Test an application config with health scope", func() {
		healthScopeName := "example-health-scope"
		// create health scope.
		hs := v1alpha2.HealthScope{
			ObjectMeta: metav1.ObjectMeta{
				Name:      healthScopeName,
				Namespace: namespace,
			},
			Spec: v1alpha2.HealthScopeSpec{
				ProbeTimeout:       &varInt32_60,
				WorkloadReferences: []corev1.ObjectReference{},
			},
		}
		Expect(k8sClient.Create(ctx, &hs)).Should(SatisfyAny(BeNil(), &util.AlreadyExistMatcher{}))

		By("Check empty health scope is healthy")
		Eventually(func() v1alpha2.HealthStatus {
			k8sClient.Get(ctx, client.ObjectKey{Namespace: namespace, Name: healthScopeName}, &hs)
			return hs.Status.ScopeHealthCondition.HealthStatus
		}, time.Second*30, time.Millisecond*500).Should(Equal(v1alpha2.StatusHealthy))

		label := map[string]string{"workload": "deployment-workload"}
		wd := v1alpha2.WorkloadDefinition{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "deployments.apps",
				Namespace: namespace,
				Labels:    label,
			},
			Spec: v1alpha2.WorkloadDefinitionSpec{
				Reference: common.DefinitionReference{
					Name: "deployments.apps",
				},
			},
		}

		logf.Log.Info("Creating workload definition")
		// For some reason, WorkloadDefinition is created as a Cluster scope object
		Expect(k8sClient.Create(ctx, &wd)).Should(SatisfyAny(BeNil(), &util.AlreadyExistMatcher{}))

		workloadName := "example-deployment-workload"
		wl := appsv1.Deployment{
			TypeMeta: metav1.TypeMeta{
				APIVersion: "apps/v1",
				Kind:       "Deployment",
			},
			ObjectMeta: metav1.ObjectMeta{
				Namespace: namespace,
				Labels:    label,
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
								Image: "wordpress:php7.2",
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

		By("check component successfully created")
		Eventually(
			func() error {
				return k8sClient.Get(ctx, client.ObjectKey{Name: componentName, Namespace: comp.Namespace}, &comp)
			},
			time.Second*5, time.Millisecond*100).Should(BeNil())

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
								ScopeReference: corev1.ObjectReference{
									APIVersion: "core.oam.dev/v1alpha2",
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
								ScopeReference: corev1.ObjectReference{
									APIVersion: "core.oam.dev/v1alpha2",
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

		healthScopeObject := client.ObjectKey{
			Name:      healthScopeName,
			Namespace: namespace,
		}
		healthScope := &v1alpha2.HealthScope{}
		By("Verify health scope")
		Eventually(
			func() v1alpha2.ScopeHealthCondition {
				RequestReconcileNow(ctx, &appConfig)
				*healthScope = v1alpha2.HealthScope{}
				k8sClient.Get(ctx, healthScopeObject, healthScope)
				logf.Log.Info("Checking on health scope",
					"len(WorkloadReferences)",
					len(healthScope.Spec.WorkloadReferences),
					"health",
					healthScope.Status.ScopeHealthCondition)
				return healthScope.Status.ScopeHealthCondition
			},
			time.Second*150, time.Second*5).Should(Equal(v1alpha2.ScopeHealthCondition{
			HealthStatus:     v1alpha2.StatusHealthy,
			Total:            int64(2),
			HealthyWorkloads: int64(2),
		}))
	})

	It("Test an application with health policy", func() {
		By("Apply a healthy application")
		var newApp v1beta1.Application
		var healthyAppName, unhealthyAppName string
		Expect(utilcommon.ReadYamlToObject("testdata/app/app_healthscope.yaml", &newApp)).Should(BeNil())
		newApp.Namespace = namespace
		Eventually(func() error {
			return k8sClient.Create(ctx, newApp.DeepCopy())
		}, 10*time.Second, 500*time.Millisecond).Should(Succeed())

		healthyAppName = newApp.Name
		By("Get Application latest status")
		Eventually(
			func() *common.Revision {
				var app v1beta1.Application
				k8sClient.Get(ctx, client.ObjectKey{Namespace: namespace, Name: healthyAppName}, &app)
				if app.Status.LatestRevision != nil {
					return app.Status.LatestRevision
				}
				return nil
			},
			time.Second*30, time.Millisecond*500).ShouldNot(BeNil())

		By("Apply an unhealthy application")
		newApp = v1beta1.Application{}
		Expect(utilcommon.ReadYamlToObject("testdata/app/app_healthscope_unhealthy.yaml", &newApp)).Should(BeNil())
		newApp.Namespace = namespace
		Eventually(func() error {
			return k8sClient.Create(ctx, newApp.DeepCopy())
		}, 10*time.Second, 500*time.Millisecond).Should(Succeed())

		unhealthyAppName = newApp.Name
		By("Get Application latest status")
		Eventually(
			func() *common.Revision {
				var app v1beta1.Application
				k8sClient.Get(ctx, client.ObjectKey{Namespace: namespace, Name: unhealthyAppName}, &app)
				if app.Status.LatestRevision != nil {
					return app.Status.LatestRevision
				}
				return nil
			},
			time.Second*30, time.Millisecond*500).ShouldNot(BeNil())

		By("Verify the healthy health scope")
		healthScopeObject := client.ObjectKey{
			Name:      "app-healthscope",
			Namespace: namespace,
		}

		healthScope := &v1alpha2.HealthScope{}
		Expect(k8sClient.Get(ctx, healthScopeObject, healthScope)).Should(Succeed())

		Eventually(
			func() v1alpha2.ScopeHealthCondition {
				*healthScope = v1alpha2.HealthScope{}
				k8sClient.Get(ctx, healthScopeObject, healthScope)
				return healthScope.Status.ScopeHealthCondition
			},
			time.Second*60, time.Millisecond*500).Should(Equal(v1alpha2.ScopeHealthCondition{
			HealthStatus:     v1alpha2.StatusHealthy,
			Total:            int64(2),
			HealthyWorkloads: int64(2),
		}))

		By("Verify the healthy application status")
		Eventually(func() error {
			healthyApp := &v1beta1.Application{}
			if err := k8sClient.Get(ctx, client.ObjectKey{Namespace: namespace, Name: healthyAppName}, healthyApp); err != nil {
				return err
			}
			appCompStatuses := healthyApp.Status.Services
			if len(appCompStatuses) != 2 {
				return fmt.Errorf("expect 2 comp statuses, but got %d", len(appCompStatuses))
			}
			compSts1 := appCompStatuses[0]
			if !compSts1.Healthy || !strings.Contains(compSts1.Message, "Ready:1/1") {
				return fmt.Errorf("expect healthy comp, but %v is unhealthy, msg: %q", compSts1.Name, compSts1.Message)
			}
			if len(compSts1.Traits) != 1 {
				return fmt.Errorf("expect 2 traits statuses, but got %d", len(compSts1.Traits))
			}
			Expect(compSts1.Traits[0].Message).Should(ContainSubstring("No loadBalancer found"))

			return nil
		}, time.Second*30, time.Millisecond*500).Should(Succeed())

		By("Verify the unhealthy health scope")
		healthScopeObject = client.ObjectKey{
			Name:      "app-healthscope-unhealthy",
			Namespace: namespace,
		}

		healthScope = &v1alpha2.HealthScope{}
		Expect(k8sClient.Get(ctx, healthScopeObject, healthScope)).Should(Succeed())

		Eventually(
			func() v1alpha2.ScopeHealthCondition {
				*healthScope = v1alpha2.HealthScope{}
				k8sClient.Get(ctx, healthScopeObject, healthScope)
				return healthScope.Status.ScopeHealthCondition
			},
			time.Second*60, time.Millisecond*500).Should(Equal(v1alpha2.ScopeHealthCondition{
			HealthStatus:       v1alpha2.StatusUnhealthy,
			Total:              int64(2),
			UnhealthyWorkloads: int64(1),
			HealthyWorkloads:   int64(1),
		}))

		By("Verify the unhealthy application status")
		Eventually(func() error {
			unhealthyApp := &v1beta1.Application{}
			if err := k8sClient.Get(ctx, client.ObjectKey{Namespace: namespace, Name: healthyAppName}, unhealthyApp); err != nil {
				return err
			}
			appCompStatuses := unhealthyApp.Status.Services
			if len(appCompStatuses) != 2 {
				return fmt.Errorf("expect 2 comp statuses, but got %d", len(appCompStatuses))
			}
			for _, cSts := range appCompStatuses {
				if cSts.Name == "my-server-unhealthy" {
					unhealthyCompSts := cSts
					if unhealthyCompSts.Healthy || !strings.Contains(unhealthyCompSts.Message, "Ready:0/1") {
						return fmt.Errorf("expect unhealthy comp, but %s is unhealthy, msg: %q", unhealthyCompSts.Name, unhealthyCompSts.Message)
					}
				}
			}
			return nil
		}, time.Second*30, time.Millisecond*500).Should(Succeed())
	})
})
