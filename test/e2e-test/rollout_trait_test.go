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
	"time"

	v1 "k8s.io/api/apps/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/oam-dev/kubevela/apis/standard.oam.dev/v1alpha1"
	"github.com/oam-dev/kubevela/pkg/oam/util"
	"github.com/oam-dev/kubevela/pkg/utils/common"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"

	corev1 "k8s.io/api/core/v1"

	"github.com/oam-dev/kubevela/apis/core.oam.dev/v1beta1"
)

var _ = Describe("rollout related e2e-test,rollout trait test", func() {
	ctx := context.Background()
	var namespaceName, componentName, compRevName string
	var ns corev1.Namespace
	var app v1beta1.Application
	var rollout v1alpha1.Rollout
	var targerDeploy, sourceDeploy v1.Deployment
	var err error

	createAllDef := func() {
		By("install all related definition")
		var cd v1beta1.ComponentDefinition
		Expect(common.ReadYamlToObject("testdata/rollout/deployment/webServiceDefinition.yaml", &cd)).Should(BeNil())
		// create the componentDefinition if not exist
		cd.Namespace = namespaceName
		Eventually(
			func() error {
				return k8sClient.Create(ctx, &cd)
			},
			time.Second*3, time.Millisecond*300).Should(SatisfyAny(BeNil(), &util.AlreadyExistMatcher{}))
		var td v1beta1.TraitDefinition
		Expect(common.ReadYamlToObject("testdata/rollout/deployment/rolloutDefinition.yaml", &td)).Should(BeNil())
		td.Namespace = namespaceName
		Eventually(func() error {
			return k8sClient.Create(ctx, &td)
		},
			time.Second*3, time.Millisecond*300).Should(SatisfyAny(BeNil(), &util.AlreadyExistMatcher{}))
	}

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

	verifySuccess := func(componentRevision string) {
		By("check rollout status have succeed")
		Eventually(func() error {
			rolloutKey := types.NamespacedName{Namespace: namespaceName, Name: componentName}
			if err := k8sClient.Get(ctx, rolloutKey, &rollout); err != nil {
				return err
			}
			if rollout.Spec.TargetRevisionName != componentRevision {
				return fmt.Errorf("rollout have not point to right targetRevision")
			}
			if rollout.Status.RollingState != v1alpha1.RolloutSucceedState {
				return fmt.Errorf("error rollout status state %s", rollout.Status.RollingState)
			}
			compRevName = rollout.Spec.TargetRevisionName
			deployKey := types.NamespacedName{Namespace: namespaceName, Name: compRevName}
			if err := k8sClient.Get(ctx, deployKey, &targerDeploy); err != nil {
				return err
			}
			if *targerDeploy.Spec.Replicas != *rollout.Spec.RolloutPlan.TargetSize {
				return fmt.Errorf("targetDeploy replicas missMatch %d, %d", targerDeploy.Spec.Replicas, rollout.Spec.RolloutPlan.TargetSize)
			}
			if targerDeploy.Status.UpdatedReplicas != *targerDeploy.Spec.Replicas {
				return fmt.Errorf("update not finish")
			}
			if rollout.Status.LastSourceRevision == "" {
				return nil
			}
			deployKey = types.NamespacedName{Namespace: namespaceName, Name: rollout.Status.LastSourceRevision}
			if err := k8sClient.Get(ctx, deployKey, &sourceDeploy); err == nil || !apierrors.IsNotFound(err) {
				return fmt.Errorf("source deploy still exist")
			}
			return nil
		}, time.Second*180, 300*time.Millisecond).Should(BeNil())
	}

	BeforeEach(func() {
		By("Start to run a test, init whole env")
		namespaceName = randomNamespaceName("rollout-trait-e2e-test")
		createNamespace()
		createAllDef()
		componentName = "express-server"
	})

	AfterEach(func() {
		By("Clean up resources after a test")
		Eventually(func() error {
			err := k8sClient.Delete(ctx, &app)
			if err == nil || apierrors.IsNotFound(err) {
				return nil
			}
			return err
		}, 15*time.Second, 300*time.Microsecond).Should(BeNil())
		By(fmt.Sprintf("Delete the entire namespaceName %s", ns.Name))
		// delete the namespaceName with all its resources
		Expect(k8sClient.Delete(ctx, &ns, client.PropagationPolicy(metav1.DeletePropagationBackground))).Should(BeNil())
	})

	It("rollout as a trait whole process e2e-test", func() {
		By("first scale operation")
		Expect(common.ReadYamlToObject("testdata/rollout/deployment/application.yaml", &app)).Should(BeNil())
		app.Namespace = namespaceName
		Expect(k8sClient.Create(ctx, &app)).Should(BeNil())
		verifySuccess("express-server-v1")
		appKey := types.NamespacedName{Namespace: namespaceName, Name: app.Name}
		checkApp := &v1beta1.Application{}
		By("update application upgrade to v2")
		Eventually(func() error {
			if err = k8sClient.Get(ctx, appKey, checkApp); err != nil {
				return err
			}
			checkApp.Spec.Components[0].Properties.Raw = []byte(`{"image":"stefanprodan/podinfo:4.0.3","cpu":"0.1"}`)
			if err = k8sClient.Update(ctx, checkApp); err != nil {
				return err
			}
			return nil
		}, 30*time.Second, 300*time.Millisecond).Should(BeNil())
		verifySuccess("express-server-v2")
		By("update application upgrade to v3")
		Eventually(func() error {
			if err = k8sClient.Get(ctx, appKey, checkApp); err != nil {
				return err
			}
			checkApp.Spec.Components[0].Properties.Raw = []byte(`{"image":"stefanprodan/podinfo:4.0.3","cpu":"0.2"}`)
			if err = k8sClient.Update(ctx, checkApp); err != nil {
				return err
			}
			return nil
		}, 30*time.Second, 300*time.Millisecond).Should(BeNil())
		verifySuccess("express-server-v3")
		By("roll back to v2")
		time.Sleep(30 * time.Second)
		Eventually(func() error {
			if err = k8sClient.Get(ctx, appKey, checkApp); err != nil {
				return err
			}
			checkApp.Spec.Components[0].Traits[0].Properties.Raw = []byte(`{"targetRevision":"express-server-v2"}`)
			if err = k8sClient.Update(ctx, checkApp); err != nil {
				return err
			}
			return nil
		}, 30*time.Second, 300*time.Millisecond).Should(BeNil())
		verifySuccess("express-server-v2")
		By("modify targetSize to scale")
		Eventually(func() error {
			if err = k8sClient.Get(ctx, appKey, checkApp); err != nil {
				return err
			}
			checkApp.Spec.Components[0].Traits[0].Properties.Raw =
				[]byte(`{"targetRevision":"express-server-v2","targetSize":7,"firstBatchReplicas":1,"secondBatchReplicas":1}`)
			if err = k8sClient.Update(ctx, checkApp); err != nil {
				return err
			}
			return nil
		}, 30*time.Second, 300*time.Millisecond).Should(BeNil())
		time.Sleep(12 * time.Second)
		verifySuccess("express-server-v2")
		By("update application upgrade to v4")
		Eventually(func() error {
			if err = k8sClient.Get(ctx, appKey, checkApp); err != nil {
				return err
			}
			checkApp.Spec.Components[0].Properties.Raw = []byte(`{"image":"stefanprodan/podinfo:4.0.3","cpu":"0.3"}`)
			checkApp.Spec.Components[0].Traits[0].Properties.Raw =
				[]byte(`{"firstBatchReplicas":3,"secondBatchReplicas":4,"targetSize":7}`)
			if err = k8sClient.Update(ctx, checkApp); err != nil {
				return err
			}
			return nil
		}, 30*time.Second, 300*time.Millisecond).Should(BeNil())
		verifySuccess("express-server-v4")
	})
})
