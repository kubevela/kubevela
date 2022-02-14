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

package apply

import (
	"context"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/utils/pointer"

	"github.com/oam-dev/kubevela/pkg/oam"
	oamutil "github.com/oam-dev/kubevela/pkg/oam/util"
)

var _ = Describe("Test apply", func() {
	var (
		int32_3   = int32(3)
		int32_5   = int32(5)
		ctx       = context.Background()
		deploy    *appsv1.Deployment
		deployKey = types.NamespacedName{
			Name:      "testdeploy",
			Namespace: ns,
		}
	)

	BeforeEach(func() {
		deploy = basicTestDeployment()
		Expect(k8sApplicator.Apply(ctx, deploy)).Should(Succeed())
	})

	AfterEach(func() {
		Expect(rawClient.Delete(ctx, deploy)).Should(SatisfyAny(Succeed(), &oamutil.NotFoundMatcher{}))
	})

	Context("Test apply resources", func() {
		It("Test apply core resources", func() {
			deploy = basicTestDeployment()
			By("Set normal & array field")
			deploy.Spec.Replicas = &int32_3
			deploy.Spec.Template.Spec.Volumes = []corev1.Volume{{Name: "test"}}
			Expect(k8sApplicator.Apply(ctx, deploy)).Should(Succeed())
			resultDeploy := basicTestDeployment()
			Expect(rawClient.Get(ctx, deployKey, resultDeploy)).Should(Succeed())
			Expect(*resultDeploy.Spec.Replicas).Should(Equal(int32_3))
			Expect(len(resultDeploy.Spec.Template.Spec.Volumes)).Should(Equal(1))

			deploy = basicTestDeployment()
			By("Override normal & array field")
			deploy.Spec.Replicas = &int32_5
			deploy.Spec.Template.Spec.Volumes = []corev1.Volume{{Name: "test"}, {Name: "test2"}}
			Expect(k8sApplicator.Apply(ctx, deploy)).Should(Succeed())
			resultDeploy = basicTestDeployment()
			Expect(rawClient.Get(ctx, deployKey, resultDeploy)).Should(Succeed())
			Expect(*resultDeploy.Spec.Replicas).Should(Equal(int32_5))
			Expect(len(resultDeploy.Spec.Template.Spec.Volumes)).Should(Equal(2))

			deploy = basicTestDeployment()
			By("Unset normal & array field")
			deploy.Spec.Replicas = nil
			deploy.Spec.Template.Spec.Volumes = nil
			Expect(k8sApplicator.Apply(ctx, deploy)).Should(Succeed())
			resultDeploy = basicTestDeployment()
			Expect(rawClient.Get(ctx, deployKey, resultDeploy)).Should(Succeed())
			By("Unsetted fields shoulde be removed or set to default value")
			Expect(*resultDeploy.Spec.Replicas).Should(Equal(int32(1)))
			Expect(len(resultDeploy.Spec.Template.Spec.Volumes)).Should(Equal(0))

			deployUpdate := basicTestDeployment()
			deployUpdate.Name = deploy.Name + "-no-update"
			Expect(k8sApplicator.Apply(ctx, deployUpdate, DisableUpdateAnnotation())).Should(Succeed())
			Expect(len(deployUpdate.Annotations[oam.AnnotationLastAppliedConfig])).Should(Equal(0))

			deployUpdate = basicTestDeployment()
			deployUpdate.Spec.Replicas = &int32_3
			deployUpdate.Spec.Template.Spec.Volumes = []corev1.Volume{{Name: "test"}}
			Expect(k8sApplicator.Apply(ctx, deployUpdate)).Should(Succeed())
			resultDeploy = basicTestDeployment()
			resultDeploy.Name = deploy.Name + "-no-update"
			Expect(rawClient.Get(ctx, deployKey, resultDeploy)).Should(Succeed())
			Expect(*resultDeploy.Spec.Replicas).Should(Equal(int32_3))
			Expect(len(resultDeploy.Spec.Template.Spec.Volumes)).Should(Equal(1))
			Expect(rawClient.Delete(ctx, deployUpdate)).Should(SatisfyAny(Succeed(), &oamutil.NotFoundMatcher{}))
		})

		It("Test multiple appliers", func() {
			deploy = basicTestDeployment()
			originalDeploy := deploy.DeepCopy()
			Expect(k8sApplicator.Apply(ctx, deploy)).Should(Succeed())

			modifiedDeploy := &appsv1.Deployment{}
			modifiedDeploy.SetGroupVersionKind(deploy.GroupVersionKind())
			Expect(rawClient.Get(ctx, deployKey, modifiedDeploy)).Should(Succeed())
			By("Other applier changed the deployment")
			modifiedDeploy.Spec.MinReadySeconds = 10
			modifiedDeploy.Spec.ProgressDeadlineSeconds = pointer.Int32Ptr(20)
			modifiedDeploy.Spec.Template.Spec.Volumes = []corev1.Volume{{Name: "test"}}
			Expect(rawClient.Update(ctx, modifiedDeploy)).Should(Succeed())

			By("Original applier apply again")
			Expect(k8sApplicator.Apply(ctx, originalDeploy)).Should(Succeed())
			resultDeploy := basicTestDeployment()
			Expect(rawClient.Get(ctx, deployKey, resultDeploy)).Should(Succeed())

			By("Check the changes from other applier are not effected")
			Expect(resultDeploy.Spec.MinReadySeconds).Should(Equal(int32(10)))
			Expect(*resultDeploy.Spec.ProgressDeadlineSeconds).Should(Equal(int32(20)))
			Expect(len(resultDeploy.Spec.Template.Spec.Volumes)).Should(Equal(1))
		})

	})
})

func basicTestDeployment() *appsv1.Deployment {
	return &appsv1.Deployment{
		TypeMeta: metav1.TypeMeta{
			Kind:       "Deployment",
			APIVersion: "apps/v1",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      "testdeploy",
			Namespace: ns,
		},
		Spec: appsv1.DeploymentSpec{
			// Replicas: x  // normal field with default value
			Selector: &metav1.LabelSelector{
				MatchLabels: map[string]string{
					"app": "nginx",
				},
			},
			Template: corev1.PodTemplateSpec{
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{ // array field
						{
							Name:  "nginx",
							Image: "nginx:1.9.4", // normal field without default value
						},
					},
				},
				ObjectMeta: metav1.ObjectMeta{Labels: map[string]string{"app": "nginx"}},
			},
		},
	}
}
