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
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	oamcomm "github.com/oam-dev/kubevela/apis/core.oam.dev/common"
	"github.com/oam-dev/kubevela/apis/core.oam.dev/v1beta1"
	"github.com/oam-dev/kubevela/pkg/oam"
	"github.com/oam-dev/kubevela/pkg/oam/util"
	"github.com/oam-dev/kubevela/pkg/utils/common"
)

func createNamespace(ctx context.Context, namespaceName string) corev1.Namespace {
	ns := corev1.Namespace{
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
	return ns
}

func createServiceAccount(ctx context.Context, ns, name string) {
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

func applyApp(ctx context.Context, namespaceName, source string, app *v1beta1.Application) {
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
			k8sClient.Get(ctx, client.ObjectKey{Namespace: namespaceName, Name: newApp.Name}, app)
			if app.Status.LatestRevision != nil {
				return app.Status.LatestRevision
			}
			return nil
		},
		time.Second*30, time.Millisecond*500).ShouldNot(BeNil())
}

func updateApp(ctx context.Context, namespaceName, target string, app *v1beta1.Application) {
	By("Update the application to target spec during rolling")
	var targetApp v1beta1.Application
	Expect(common.ReadYamlToObject("testdata/app/"+target, &targetApp)).Should(BeNil())

	Eventually(
		func() error {
			k8sClient.Get(ctx, client.ObjectKey{Namespace: namespaceName, Name: app.Name}, app)
			app.Spec = targetApp.Spec
			return k8sClient.Update(ctx, app)
		}, time.Second*5, time.Millisecond*500).Should(Succeed())
}

func verifyApplicationPhase(ctx context.Context, ns, appName string, expected oamcomm.ApplicationPhase) {
	var testApp v1beta1.Application
	Eventually(func() error {
		err := k8sClient.Get(ctx, client.ObjectKey{Namespace: ns, Name: appName}, &testApp)
		if err != nil {
			return err
		}
		if testApp.Status.Phase != expected {
			return fmt.Errorf("application status wants %s, actually %s", expected, testApp.Status.Phase)
		}
		return nil
	}, 120*time.Second, time.Second).Should(BeNil())
}

func verifyApplicationDelaySuspendExpected(ctx context.Context, ns, appName, suspendStep, nextStep, duration string) {
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

func verifyWorkloadRunningExpected(ctx context.Context, namespaceName, workloadName string, replicas int32, image string) {
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

var _ = Describe("Application Normal tests", func() {
	ctx := context.Background()
	var namespaceName string
	var ns corev1.Namespace
	var app *v1beta1.Application

	BeforeEach(func() {
		By("Start to run a test, clean up previous resources")
		namespaceName = "app-normal-e2e-test" + "-" + strconv.FormatInt(rand.Int63(), 16)
		ns = createNamespace(ctx, namespaceName)
		app = &v1beta1.Application{}
	})

	AfterEach(func() {
		By("Clean up resources after a test")
		k8sClient.Delete(ctx, app)
		By(fmt.Sprintf("Delete the entire namespaceName %s", ns.Name))
		// delete the namespaceName with all its resources
		Expect(k8sClient.Delete(ctx, &ns, client.PropagationPolicy(metav1.DeletePropagationBackground))).Should(BeNil())
	})

	It("Test app created normally", func() {
		applyApp(ctx, namespaceName, "app1.yaml", app)
		By("Apply the application rollout go directly to the target")
		verifyWorkloadRunningExpected(ctx, namespaceName, "myweb", 1, "stefanprodan/podinfo:4.0.3")

		By("Update app with trait")
		updateApp(ctx, namespaceName, "app2.yaml", app)
		By("Apply the application rollout go directly to the target")
		verifyWorkloadRunningExpected(ctx, namespaceName, "myweb", 2, "stefanprodan/podinfo:4.0.3")

		By("Update app with trait updated")
		updateApp(ctx, namespaceName, "app3.yaml", app)
		By("Apply the application rollout go directly to the target")
		verifyWorkloadRunningExpected(ctx, namespaceName, "myweb", 3, "stefanprodan/podinfo:4.0.3")

		By("Update app with trait and workload image updated")
		updateApp(ctx, namespaceName, "app4.yaml", app)
		By("Apply the application rollout go directly to the target")
		verifyWorkloadRunningExpected(ctx, namespaceName, "myweb", 1, "stefanprodan/podinfo:5.0.2")
	})

	It("Test app have component with multiple same type traits", func() {
		traitDef := new(v1beta1.TraitDefinition)
		Expect(common.ReadYamlToObject("testdata/app/trait_config.yaml", traitDef)).Should(BeNil())
		traitDef.Namespace = namespaceName
		Expect(k8sClient.Create(ctx, traitDef)).Should(BeNil())

		By("apply application")
		applyApp(ctx, namespaceName, "app7.yaml", app)
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

	It("Test app failed after retries", func() {
		By("Apply an application")
		var newApp v1beta1.Application
		Expect(common.ReadYamlToObject("testdata/app/app10.yaml", &newApp)).Should(BeNil())
		newApp.Namespace = namespaceName
		Expect(k8sClient.Create(ctx, &newApp)).Should(BeNil())

		By("check application status")
		verifyApplicationPhase(ctx, newApp.Namespace, newApp.Name, oamcomm.ApplicationWorkflowFailed)
	})

	It("Test app with notification and custom if", func() {
		By("Apply an application")
		var newApp v1beta1.Application
		Expect(common.ReadYamlToObject("testdata/app/app12.yaml", &newApp)).Should(BeNil())
		newApp.Namespace = namespaceName
		Expect(k8sClient.Create(ctx, &newApp)).Should(BeNil())

		By("check application status")
		verifyWorkloadRunningExpected(ctx, namespaceName, "comp-custom-if", 1, "crccheck/hello-world")
	})

	It("Test wait suspend", func() {
		By("Apply wait suspend application")
		var newApp v1beta1.Application
		Expect(common.ReadYamlToObject("testdata/app/app_wait_suspend.yaml", &newApp)).Should(BeNil())
		newApp.Namespace = namespaceName
		Expect(k8sClient.Create(ctx, &newApp)).Should(BeNil())

		By("check application suspend duration")
		verifyApplicationDelaySuspendExpected(ctx, newApp.Namespace, newApp.Name, "suspend-test", "apply-wait-suspend-comp", "30s")
	})

	It("Test app with ServiceAccount", func() {
		By("Creating a ServiceAccount")
		const saName = "app-service-account"
		createServiceAccount(ctx, namespaceName, saName)

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
					Resources: []string{"deployments", "controllerrevisions"},
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
		annotations[oam.AnnotationApplicationServiceAccountName] = saName
		newApp.SetAnnotations(annotations)
		Expect(k8sClient.Create(ctx, &newApp)).Should(BeNil())

		By("Checking an application status")
		verifyWorkloadRunningExpected(ctx, namespaceName, "myweb", 1, "stefanprodan/podinfo:4.0.3")

		Expect(k8sClient.Delete(ctx, &newApp)).Should(Succeed())
		Eventually(func(g Gomega) {
			g.Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(&newApp), &newApp)).Should(Satisfy(errors.IsNotFound))
		}, 15*time.Second).Should(Succeed())
	})

	It("Test app with ServiceAccount which has no permission for the component", func() {
		By("Creating a ServiceAccount")
		const saName = "dummy-service-account"
		createServiceAccount(ctx, namespaceName, saName)

		By("Creating an application")
		var newApp v1beta1.Application
		Expect(common.ReadYamlToObject("testdata/app/app11.yaml", &newApp)).Should(BeNil())
		newApp.Namespace = namespaceName
		annotations := newApp.GetAnnotations()
		annotations[oam.AnnotationApplicationServiceAccountName] = saName
		newApp.SetAnnotations(annotations)
		Expect(k8sClient.Create(ctx, &newApp)).Should(BeNil())

		By("Checking an application status")
		verifyApplicationPhase(ctx, newApp.Namespace, newApp.Name, oamcomm.ApplicationWorkflowFailed)
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
		annotations[oam.AnnotationApplicationServiceAccountName] = saName
		newApp.SetAnnotations(annotations)
		Expect(k8sClient.Create(ctx, &newApp)).Should(BeNil())

		By("Checking an application status")
		verifyApplicationPhase(ctx, newApp.Namespace, newApp.Name, oamcomm.ApplicationWorkflowFailed)
	})

	It("Test app with replication policy", func() {
		By("Apply replica-webservice definition")
		var compDef v1beta1.ComponentDefinition
		Expect(common.ReadYamlToObject("testdata/definition/replica-webservice.yaml", &compDef)).Should(BeNil())
		Eventually(func() error {
			return k8sClient.Create(ctx, compDef.DeepCopy())
		}, 10*time.Second, 500*time.Millisecond).Should(SatisfyAny(util.AlreadyExistMatcher{}, BeNil()))

		By("Creating an application")
		applyApp(ctx, namespaceName, "app_replication.yaml", app)

		By("Checking the replication & application status")
		verifyWorkloadRunningExpected(ctx, namespaceName, "hello-rep-beijing", 1, "crccheck/hello-world")
		verifyWorkloadRunningExpected(ctx, namespaceName, "hello-rep-hangzhou", 1, "crccheck/hello-world")
		By("Checking the origin component are not be dispatched")
		var workload v1.Deployment
		err := k8sClient.Get(ctx, client.ObjectKey{Namespace: namespaceName, Name: "hello-rep"}, &workload)
		Expect(err).Should(SatisfyAny(&util.NotFoundMatcher{}))

		By("Checking the component not replicated & application status")
		verifyWorkloadRunningExpected(ctx, namespaceName, "hello-no-rep", 1, "crccheck/hello-world")

		var svc corev1.Service
		By("Verify Service running as expected")
		verifySeriveDispatched := func(svcName string) {
			Eventually(
				func() error {
					return k8sClient.Get(ctx, client.ObjectKey{Namespace: namespaceName, Name: svcName}, &svc)
				},
				time.Second*120, time.Millisecond*500).Should(BeNil())
		}
		verifySeriveDispatched("hello-rep-beijing")
		verifySeriveDispatched("hello-rep-hangzhou")

		By("Checking the services not replicated & application status")
		verifyWorkloadRunningExpected(ctx, namespaceName, "hello-no-rep", 1, "crccheck/hello-world")

	})
})
