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
	"math/rand"
	"strconv"
	"time"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	v1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	oamcomm "github.com/oam-dev/kubevela/apis/core.oam.dev/common"
	"github.com/oam-dev/kubevela/apis/core.oam.dev/v1beta1"
	"github.com/oam-dev/kubevela/pkg/oam"
	"github.com/oam-dev/kubevela/pkg/oam/util"
	"github.com/oam-dev/kubevela/pkg/utils/common"
)

var _ = Describe("Application Normal tests", func() {
	ctx := context.Background()
	var namespaceName string
	var ns corev1.Namespace
	var app v1beta1.Application

	createNamespace := func() {
		ns = corev1.Namespace{
			ObjectMeta: metav1.ObjectMeta{
				Name: namespaceName,
			},
		}
		// delete the namespaceName with all its resources
		Eventually(
			func() error {
				return k8sClient.Delete(ctx, &ns, client.PropagationPolicy(metav1.DeletePropagationForeground))
			},
			time.Second*120, time.Millisecond*500).Should(SatisfyAny(BeNil(), &util.NotFoundMatcher{}))
		By("make sure all the resources are removed")
		objectKey := client.ObjectKey{
			Name: namespaceName,
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

	createServiceAccount := func(ns, name string) {
		sa := corev1.ServiceAccount{
			ObjectMeta: metav1.ObjectMeta{
				Namespace: ns,
				Name:      name,
			},
		}
		Eventually(
			func() error {
				return k8sClient.Create(ctx, &sa)
			},
			time.Second*3, time.Millisecond*300).Should(SatisfyAny(BeNil(), &util.AlreadyExistMatcher{}))
	}

	applyApp := func(source string) {
		By("Apply an application")
		var newApp v1beta1.Application
		Expect(common.ReadYamlToObject("testdata/app/"+source, &newApp)).Should(BeNil())
		newApp.Namespace = namespaceName
		Eventually(func() error {
			return k8sClient.Create(ctx, newApp.DeepCopy())
		}, 10*time.Second, 500*time.Millisecond).Should(Succeed())

		By("Get Application latest status")
		Eventually(
			func() *oamcomm.Revision {
				k8sClient.Get(ctx, client.ObjectKey{Namespace: namespaceName, Name: newApp.Name}, &app)
				if app.Status.LatestRevision != nil {
					return app.Status.LatestRevision
				}
				return nil
			},
			time.Second*30, time.Millisecond*500).ShouldNot(BeNil())
	}

	updateApp := func(target string) {
		By("Update the application to target spec during rolling")
		var targetApp v1beta1.Application
		Expect(common.ReadYamlToObject("testdata/app/"+target, &targetApp)).Should(BeNil())

		Eventually(
			func() error {
				k8sClient.Get(ctx, client.ObjectKey{Namespace: namespaceName, Name: app.Name}, &app)
				app.Spec = targetApp.Spec
				return k8sClient.Update(ctx, &app)
			}, time.Second*5, time.Millisecond*500).Should(Succeed())
	}

	verifyApplicationWorkflowSuspending := func(ns, appName string) {
		var testApp v1beta1.Application
		Eventually(func() error {
			err := k8sClient.Get(ctx, client.ObjectKey{Namespace: ns, Name: appName}, &testApp)
			if err != nil {
				return err
			}
			if testApp.Status.Phase != oamcomm.ApplicationWorkflowSuspending {
				return fmt.Errorf("application status wants %s, actually %s", oamcomm.ApplicationWorkflowSuspending, testApp.Status.Phase)
			}
			return nil
		}, 120*time.Second, time.Second).Should(BeNil())
	}

	verifyApplicationDelaySuspendExpected := func(ns, appName, suspendStep, nextStep, duration string) {
		var testApp v1beta1.Application
		Eventually(func() error {
			waitDuration, err := time.ParseDuration(duration)
			if err != nil {
				return err
			}

			err = k8sClient.Get(ctx, client.ObjectKey{Namespace: ns, Name: appName}, &testApp)
			if err != nil {
				return err
			}

			if testApp.Status.Workflow == nil {
				return fmt.Errorf("application wait to start workflow")
			}

			if testApp.Status.Workflow.Finished {
				var suspendStartTime, nextStepStartTime metav1.Time
				var sFlag, nFlag bool

				for _, wfStatus := range testApp.Status.Workflow.Steps {
					if wfStatus.Name == suspendStep {
						suspendStartTime = wfStatus.FirstExecuteTime
						sFlag = true
						continue
					}

					if wfStatus.Name == nextStep {
						nextStepStartTime = wfStatus.FirstExecuteTime
						nFlag = true
					}
				}

				if !sFlag {
					return fmt.Errorf("application can not find suspend step: %s", suspendStep)
				}

				if !nFlag {
					return fmt.Errorf("application can not find next step: %s", nextStep)
				}

				dd := nextStepStartTime.Sub(suspendStartTime.Time)
				if waitDuration > dd {
					return fmt.Errorf("application suspend wait duration wants more than %s, actually %s", duration, dd.String())
				}

				return nil
			}
			return fmt.Errorf("application status workflow finished wants true, actually false")
		}, 120*time.Second, time.Second).Should(BeNil())
	}

	verifyWorkloadRunningExpected := func(workloadName string, replicas int32, image string) {
		var workload v1.Deployment
		By("Verify Workload running as expected")
		Eventually(
			func() error {
				if err := k8sClient.Get(ctx, client.ObjectKey{Namespace: namespaceName, Name: workloadName}, &workload); err != nil {
					return err
				}
				if workload.Status.ReadyReplicas != replicas {
					return fmt.Errorf("expect replicas %v != real %v", replicas, workload.Status.ReadyReplicas)
				}
				if workload.Spec.Template.Spec.Containers[0].Image != image {
					return fmt.Errorf("expect replicas %v != real %v", image, workload.Spec.Template.Spec.Containers[0].Image)
				}
				return nil
			},
			time.Second*60, time.Millisecond*500).Should(BeNil())
	}

	verifyComponentRevision := func(compName string, revisionNum int64) {
		By("Verify Component revision")
		expectCompRevName := fmt.Sprintf("%s-v%d", compName, revisionNum)
		Eventually(
			func() error {
				gotCR := &v1.ControllerRevision{}
				if err := k8sClient.Get(ctx, client.ObjectKey{Namespace: namespaceName, Name: expectCompRevName}, gotCR); err != nil {
					return err
				}
				if gotCR.Revision != revisionNum {
					return fmt.Errorf("expect revision %d != real %d", revisionNum, gotCR.Revision)
				}
				return nil
			},
			time.Second*10, time.Millisecond*500).Should(BeNil())
	}

	BeforeEach(func() {
		By("Start to run a test, clean up previous resources")
		namespaceName = "app-normal-e2e-test" + "-" + strconv.FormatInt(rand.Int63(), 16)
		createNamespace()
	})

	AfterEach(func() {
		By("Clean up resources after a test")
		k8sClient.Delete(ctx, &app)
		By(fmt.Sprintf("Delete the entire namespaceName %s", ns.Name))
		// delete the namespaceName with all its resources
		Expect(k8sClient.Delete(ctx, &ns, client.PropagationPolicy(metav1.DeletePropagationBackground))).Should(BeNil())
	})

	It("Test app created normally", func() {
		applyApp("app1.yaml")
		By("Apply the application rollout go directly to the target")
		verifyWorkloadRunningExpected("myweb", 1, "stefanprodan/podinfo:4.0.3")
		verifyComponentRevision("myweb", 1)

		By("Update app with trait")
		updateApp("app2.yaml")
		By("Apply the application rollout go directly to the target")
		verifyWorkloadRunningExpected("myweb", 2, "stefanprodan/podinfo:4.0.3")
		verifyComponentRevision("myweb", 2)

		By("Update app with trait updated")
		updateApp("app3.yaml")
		By("Apply the application rollout go directly to the target")
		verifyWorkloadRunningExpected("myweb", 3, "stefanprodan/podinfo:4.0.3")
		verifyComponentRevision("myweb", 3)

		By("Update app with trait and workload image updated")
		updateApp("app4.yaml")
		By("Apply the application rollout go directly to the target")
		verifyWorkloadRunningExpected("myweb", 1, "stefanprodan/podinfo:5.0.2")
		verifyComponentRevision("myweb", 4)
	})

	It("Test app have component with multiple same type traits", func() {
		traitDef := new(v1beta1.TraitDefinition)
		Expect(common.ReadYamlToObject("testdata/app/trait_config.yaml", traitDef)).Should(BeNil())
		traitDef.Namespace = namespaceName
		Expect(k8sClient.Create(ctx, traitDef)).Should(BeNil())

		By("apply application")
		applyApp("app7.yaml")
		appName := "test-worker"

		By("check application status")
		testApp := new(v1beta1.Application)
		Eventually(func() error {
			err := k8sClient.Get(ctx, client.ObjectKey{Namespace: namespaceName, Name: appName}, testApp)
			if err != nil {
				return err
			}
			if len(testApp.Status.Services) != 1 {
				return fmt.Errorf("error ComponentStatus number wants %d, actually %d", 1, len(testApp.Status.Services))
			}
			if len(testApp.Status.Services[0].Traits) != 2 {
				return fmt.Errorf("error TraitStatus number wants %d, actually %d", 2, len(testApp.Status.Services[0].Traits))
			}
			return nil
		}, 5*time.Second).Should(BeNil())

		By("check trait status")
		Expect(testApp.Status.Services[0].Traits[0].Message).Should(Equal("configMap:app-file-html"))
		Expect(testApp.Status.Services[0].Traits[1].Message).Should(Equal("secret:app-env-config"))
	})

	It("Test app have rollout-template false annotation", func() {
		By("Apply an application")
		var newApp v1beta1.Application
		Expect(common.ReadYamlToObject("testdata/app/app5.yaml", &newApp)).Should(BeNil())
		newApp.Namespace = namespaceName
		Expect(k8sClient.Create(ctx, &newApp)).ShouldNot(BeNil())
	})

	It("Test app have components with same name", func() {
		By("Apply an application")
		var newApp v1beta1.Application
		Expect(common.ReadYamlToObject("testdata/app/app8.yaml", &newApp)).Should(BeNil())
		newApp.Namespace = namespaceName
		Expect(k8sClient.Create(ctx, &newApp)).ShouldNot(BeNil())
	})

	It("Test two app have component with same name", func() {
		By("Apply an application")
		var firstApp v1beta1.Application
		Expect(common.ReadYamlToObject("testdata/app/app9.yaml", &firstApp)).Should(BeNil())
		firstApp.Namespace = namespaceName
		firstApp.Name = "first-app"
		Expect(k8sClient.Create(ctx, &firstApp)).Should(BeNil())

		time.Sleep(time.Second)
		var secondApp v1beta1.Application
		Expect(common.ReadYamlToObject("testdata/app/app9.yaml", &secondApp)).Should(BeNil())
		secondApp.Namespace = namespaceName
		secondApp.Name = "second-app"
		Expect(k8sClient.Create(ctx, &secondApp)).ShouldNot(BeNil())
	})

	It("Test app failed after retries", func() {
		By("Apply an application")
		var newApp v1beta1.Application
		Expect(common.ReadYamlToObject("testdata/app/app10.yaml", &newApp)).Should(BeNil())
		newApp.Namespace = namespaceName
		Expect(k8sClient.Create(ctx, &newApp)).Should(BeNil())

		By("check application status")
		verifyApplicationWorkflowSuspending(newApp.Namespace, newApp.Name)
	})

	It("Test wait suspend", func() {
		By("Apply wait suspend application")
		var newApp v1beta1.Application
		Expect(common.ReadYamlToObject("testdata/app/app_wait_suspend.yaml", &newApp)).Should(BeNil())
		newApp.Namespace = namespaceName
		Expect(k8sClient.Create(ctx, &newApp)).Should(BeNil())

		By("check application suspend duration")
		verifyApplicationDelaySuspendExpected(newApp.Namespace, newApp.Name, "suspend-test", "apply-wait-suspend-comp", "30s")
	})

	It("Test app with ServiceAccount", func() {
		By("Creating a ServiceAccount")
		const saName = "app-service-account"
		createServiceAccount(namespaceName, saName)

		By("Creating Role and RoleBinding")
		const roleName = "worker"
		role := rbacv1.Role{
			ObjectMeta: metav1.ObjectMeta{
				Namespace: namespaceName,
				Name:      roleName,
			},
			Rules: []rbacv1.PolicyRule{
				{
					Verbs:     []string{rbacv1.VerbAll},
					APIGroups: []string{"apps"},
					Resources: []string{"deployments"},
				},
			},
		}
		Expect(k8sClient.Create(ctx, &role)).Should(BeNil())

		roleBinding := rbacv1.RoleBinding{
			ObjectMeta: metav1.ObjectMeta{
				Namespace: namespaceName,
				Name:      roleName + "-binding",
			},
			Subjects: []rbacv1.Subject{
				{
					Kind:      "ServiceAccount",
					Name:      saName,
					Namespace: namespaceName,
				},
			},
			RoleRef: rbacv1.RoleRef{
				APIGroup: rbacv1.GroupName,
				Kind:     "Role",
				Name:     roleName,
			},
		}
		Expect(k8sClient.Create(ctx, &roleBinding)).Should(BeNil())

		By("Creating an application")
		var newApp v1beta1.Application
		Expect(common.ReadYamlToObject("testdata/app/app11.yaml", &newApp)).Should(BeNil())
		newApp.Namespace = namespaceName
		annotations := newApp.GetAnnotations()
		annotations[oam.AnnotationServiceAccountName] = saName
		newApp.SetAnnotations(annotations)
		Expect(k8sClient.Create(ctx, &newApp)).Should(BeNil())

		By("Checking an application status")
		verifyWorkloadRunningExpected("myweb", 1, "stefanprodan/podinfo:4.0.3")
		verifyComponentRevision("myweb", 1)
	})

	It("Test app with ServiceAccount which has no permission for the component", func() {
		By("Creating a ServiceAccount")
		const saName = "dummy-service-account"
		createServiceAccount(namespaceName, saName)

		By("Creating an application")
		var newApp v1beta1.Application
		Expect(common.ReadYamlToObject("testdata/app/app11.yaml", &newApp)).Should(BeNil())
		newApp.Namespace = namespaceName
		annotations := newApp.GetAnnotations()
		annotations[oam.AnnotationServiceAccountName] = saName
		newApp.SetAnnotations(annotations)
		Expect(k8sClient.Create(ctx, &newApp)).Should(BeNil())

		By("Checking an application status")
		verifyApplicationWorkflowSuspending(newApp.Namespace, newApp.Name)
	})

	It("Test app with non-existence ServiceAccount", func() {
		By("Ensuring that given service account doesn't exists")
		const saName = "not-existing-service-account"
		sa := corev1.ServiceAccount{
			ObjectMeta: metav1.ObjectMeta{
				Namespace: namespaceName,
				Name:      saName,
			},
		}
		Eventually(
			func() error {
				return k8sClient.Delete(ctx, &sa)
			},
			time.Second*3, time.Millisecond*300).Should(SatisfyAny(BeNil(), &util.NotFoundMatcher{}))

		By("Creating an application")
		var newApp v1beta1.Application
		Expect(common.ReadYamlToObject("testdata/app/app11.yaml", &newApp)).Should(BeNil())
		newApp.Namespace = namespaceName
		annotations := newApp.GetAnnotations()
		annotations[oam.AnnotationServiceAccountName] = saName
		newApp.SetAnnotations(annotations)
		Expect(k8sClient.Create(ctx, &newApp)).Should(BeNil())

		By("Checking an application status")
		verifyApplicationWorkflowSuspending(newApp.Namespace, newApp.Name)
	})
})
