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
	"encoding/json"
	"fmt"
	"strings"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	utilfeature "k8s.io/apiserver/pkg/util/feature"
	"sigs.k8s.io/controller-runtime/pkg/client"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/utils/pointer"

	"github.com/oam-dev/kubevela/apis/core.oam.dev/v1beta1"
	"github.com/oam-dev/kubevela/pkg/features"
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
			modifiedDeploy.Spec.ProgressDeadlineSeconds = pointer.Int32(20)
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

		It("Test apply resource already exists", func() {
			ctx := context.Background()
			app := &v1beta1.Application{ObjectMeta: metav1.ObjectMeta{Name: "test-app", Namespace: "default"}}
			By("Test create resource already exists but has no application owner")
			cm1 := &corev1.ConfigMap{ObjectMeta: metav1.ObjectMeta{Name: "test-resource-exists", Namespace: "default"}}
			Expect(rawClient.Create(ctx, cm1)).Should(Succeed())
			Expect(rawClient.Get(ctx, client.ObjectKeyFromObject(cm1), &corev1.ConfigMap{})).Should(Succeed())
			obj1, err := runtime.DefaultUnstructuredConverter.ToUnstructured(cm1)
			Expect(err).Should(Succeed())
			u1 := &unstructured.Unstructured{Object: obj1}
			u1.SetGroupVersionKind(schema.GroupVersionKind{Version: "v1", Kind: "ConfigMap"})
			Expect(k8sApplicator.Apply(ctx, u1, MustBeControlledByApp(app))).Should(Satisfy(func(err error) bool {
				return err != nil && strings.Contains(err.Error(), "exists but not managed by any application now")
			}))
			Expect(rawClient.Delete(ctx, cm1)).Should(Succeed())
			By("Test create resource already exists but owned by other application")
			cm2 := &corev1.ConfigMap{ObjectMeta: metav1.ObjectMeta{Name: "test-resource-exists-owned-by-others", Namespace: "default"}}
			oamutil.AddLabels(cm2, map[string]string{
				oam.LabelAppName:      "other",
				oam.LabelAppNamespace: "default",
			})
			Expect(rawClient.Create(ctx, cm2)).Should(Succeed())
			Expect(rawClient.Get(ctx, client.ObjectKeyFromObject(cm2), &corev1.ConfigMap{})).Should(Succeed())
			obj2, err := runtime.DefaultUnstructuredConverter.ToUnstructured(cm2)
			u2 := &unstructured.Unstructured{Object: obj2}
			u2.SetGroupVersionKind(schema.GroupVersionKind{Version: "v1", Kind: "ConfigMap"})
			Expect(err).Should(Succeed())
			Expect(k8sApplicator.Apply(ctx, u2, MustBeControlledByApp(app))).Should(Satisfy(func(err error) bool {
				return err != nil && strings.Contains(err.Error(), "is managed by other application")
			}))
			Expect(rawClient.Delete(ctx, cm2)).Should(Succeed())
		})

		It("Test apply resources with external modifier", func() {
			deploy.SetGroupVersionKind(appsv1.SchemeGroupVersion.WithKind("Deployment"))
			originalDeploy := deploy.DeepCopy()
			bs, err := json.Marshal(deploy)
			Expect(err).Should(Succeed())
			deploy.SetAnnotations(map[string]string{oam.AnnotationLastAppliedConfig: string(bs)})
			modifiedDeploy := deploy.DeepCopy()
			modifiedDeploy.Spec.Template.Spec.Containers = append(modifiedDeploy.Spec.Template.Spec.Containers, corev1.Container{
				Name:  "added-by-external-modifier",
				Image: "busybox",
			})
			Expect(rawClient.Update(ctx, modifiedDeploy)).Should(Succeed())

			By("Test patch")
			Expect(utilfeature.DefaultMutableFeatureGate.Set(fmt.Sprintf("%s=false", features.ApplyResourceByUpdate))).Should(Succeed())
			Expect(rawClient.Get(ctx, client.ObjectKeyFromObject(deploy), deploy)).Should(Succeed())
			copy1 := originalDeploy.DeepCopy()
			copy1.SetResourceVersion(deploy.ResourceVersion)
			Expect(k8sApplicator.Apply(ctx, copy1)).Should(Succeed())
			Expect(rawClient.Get(ctx, client.ObjectKeyFromObject(deploy), deploy)).Should(Succeed())
			Expect(len(deploy.Spec.Template.Spec.Containers)).Should(Equal(2))

			By("Test update")
			Expect(utilfeature.DefaultMutableFeatureGate.Set(fmt.Sprintf("%s=true", features.ApplyResourceByUpdate))).Should(Succeed())
			Expect(rawClient.Get(ctx, client.ObjectKeyFromObject(deploy), deploy)).Should(Succeed())
			copy2 := originalDeploy.DeepCopy()
			copy2.SetResourceVersion(deploy.ResourceVersion)
			Expect(k8sApplicator.Apply(ctx, copy2)).Should(Succeed())
			Expect(rawClient.Get(ctx, client.ObjectKeyFromObject(deploy), deploy)).Should(Succeed())
			Expect(len(deploy.Spec.Template.Spec.Containers)).Should(Equal(1))

			Expect(utilfeature.DefaultMutableFeatureGate.Set(fmt.Sprintf("%s=false", features.ApplyResourceByUpdate))).Should(Succeed())
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
