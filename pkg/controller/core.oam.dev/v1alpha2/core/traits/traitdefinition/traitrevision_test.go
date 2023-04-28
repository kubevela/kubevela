/*
 Copyright 2021. The KubeVela Authors.

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

package traitdefinition

import (
	"context"
	"fmt"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	v1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	"github.com/oam-dev/kubevela/apis/core.oam.dev/common"
	"github.com/oam-dev/kubevela/apis/core.oam.dev/v1beta1"
	"github.com/oam-dev/kubevela/pkg/oam"
	"github.com/oam-dev/kubevela/pkg/oam/testutil"
	"github.com/oam-dev/kubevela/pkg/oam/util"
)

var _ = Describe("Test DefinitionRevision created by TraitDefinition", func() {
	ctx := context.Background()
	namespace := "test-revision"
	var ns v1.Namespace

	BeforeEach(func() {
		ns = v1.Namespace{
			ObjectMeta: metav1.ObjectMeta{
				Name: namespace,
			},
		}
		Expect(k8sClient.Create(ctx, &ns)).Should(SatisfyAny(BeNil(), &util.AlreadyExistMatcher{}))
	})

	Context("Test TraitDefinition", func() {
		It("Test update TraitDefinition", func() {
			tdName := "test-update-traitdef"
			req := reconcile.Request{NamespacedName: client.ObjectKey{Name: tdName, Namespace: namespace}}

			td1 := tdWithNoTemplate.DeepCopy()
			td1.Name = tdName
			td1.Spec.Schematic.CUE.Template = fmt.Sprintf(tdTemplate, "test-v1")
			By("create traitDefinition")
			Expect(k8sClient.Create(ctx, td1)).Should(SatisfyAll(BeNil()))
			testutil.ReconcileRetry(&r, req)

			By("check whether definitionRevision is created")
			tdRevName1 := fmt.Sprintf("%s-v1", tdName)
			var tdRev1 v1beta1.DefinitionRevision
			Eventually(func() error {
				err := k8sClient.Get(ctx, client.ObjectKey{Namespace: namespace, Name: tdRevName1}, &tdRev1)
				return err
			}, 10*time.Second, time.Second).Should(BeNil())

			By("update traitDefinition")
			td := new(v1beta1.TraitDefinition)
			Eventually(func() error {
				err := k8sClient.Get(ctx, client.ObjectKey{Namespace: namespace, Name: tdName}, td)
				if err != nil {
					return err
				}
				td.Spec.Schematic.CUE.Template = fmt.Sprintf(tdTemplate, "test-v2")
				return k8sClient.Update(ctx, td)
			}, 10*time.Second, time.Second).Should(BeNil())

			testutil.ReconcileRetry(&r, req)

			By("check whether a new definitionRevision is created")
			tdRevName2 := fmt.Sprintf("%s-v2", tdName)
			var tdRev2 v1beta1.DefinitionRevision
			Eventually(func() error {
				return k8sClient.Get(ctx, client.ObjectKey{Namespace: namespace, Name: tdRevName2}, &tdRev2)
			}, 10*time.Second, time.Second).Should(BeNil())
		})

		It("Test only update TraitDefinition Labels, Shouldn't create new revision", func() {
			td := tdWithNoTemplate.DeepCopy()
			tdName := "test-td"
			td.Name = tdName
			defKey := client.ObjectKey{Namespace: namespace, Name: tdName}
			req := reconcile.Request{NamespacedName: defKey}
			td.Spec.Schematic.CUE.Template = fmt.Sprintf(tdTemplate, "test")
			Expect(k8sClient.Create(ctx, td)).Should(BeNil())
			testutil.ReconcileRetry(&r, req)

			By("Check revision create by TraitDefinition")
			defRevName := fmt.Sprintf("%s-v1", tdName)
			revKey := client.ObjectKey{Namespace: namespace, Name: defRevName}
			var defRev v1beta1.DefinitionRevision
			Eventually(func() error {
				return k8sClient.Get(ctx, revKey, &defRev)
			}, 10*time.Second, time.Second).Should(BeNil())

			By("Only update TraitDefinition Labels")
			var checkRev v1beta1.TraitDefinition
			Eventually(func() error {
				err := k8sClient.Get(ctx, defKey, &checkRev)
				if err != nil {
					return err
				}
				checkRev.SetLabels(map[string]string{
					"test-label": "test-defRev",
				})
				return k8sClient.Update(ctx, &checkRev)
			}, 10*time.Second, time.Second).Should(BeNil())
			testutil.ReconcileRetry(&r, req)

			newDefRevName := fmt.Sprintf("%s-v2", tdName)
			newRevKey := client.ObjectKey{Namespace: namespace, Name: newDefRevName}
			Expect(k8sClient.Get(ctx, newRevKey, &defRev)).Should(HaveOccurred())
		})
	})

	Context("Test TraitDefinition Controller clean up", func() {
		It("Test clean up definitionRevision", func() {
			var revKey client.ObjectKey
			var defRev v1beta1.DefinitionRevision
			tdName := "test-clean-up"
			revisionNum := 1
			defKey := client.ObjectKey{Namespace: namespace, Name: tdName}
			req := reconcile.Request{NamespacedName: defKey}

			By("create a new TraitDefinition")
			td := tdWithNoTemplate.DeepCopy()
			td.Name = tdName
			td.Spec.Schematic.CUE.Template = fmt.Sprintf(tdTemplate, fmt.Sprintf("test-v%d", revisionNum))
			Expect(k8sClient.Create(ctx, td)).Should(BeNil())

			By("update TraitDefinition")
			checkComp := new(v1beta1.TraitDefinition)
			for i := 0; i < defRevisionLimit+1; i++ {
				Eventually(func() error {
					err := k8sClient.Get(ctx, defKey, checkComp)
					if err != nil {
						return err
					}
					checkComp.Spec.Schematic.CUE.Template = fmt.Sprintf(tdTemplate, fmt.Sprintf("test-v%d", revisionNum))
					return k8sClient.Update(ctx, checkComp)
				}, 10*time.Second, time.Second).Should(BeNil())
				testutil.ReconcileRetry(&r, req)

				revKey = client.ObjectKey{Namespace: namespace, Name: fmt.Sprintf("%s-v%d", tdName, revisionNum)}
				revisionNum++
				var defRev v1beta1.DefinitionRevision
				Eventually(func() error {
					return k8sClient.Get(ctx, revKey, &defRev)
				}, 10*time.Second, time.Second).Should(BeNil())
			}

			By("create new TraitDefinition will remove oldest definitionRevision")
			Eventually(func() error {
				err := k8sClient.Get(ctx, defKey, checkComp)
				if err != nil {
					return err
				}
				checkComp.Spec.Schematic.CUE.Template = fmt.Sprintf(tdTemplate, fmt.Sprintf("test-v%d", revisionNum))
				return k8sClient.Update(ctx, checkComp)
			}, 10*time.Second, time.Second).Should(BeNil())
			testutil.ReconcileRetry(&r, req)

			revKey = client.ObjectKey{Namespace: namespace, Name: fmt.Sprintf("%s-v%d", tdName, revisionNum)}
			revisionNum++
			Eventually(func() error {
				return k8sClient.Get(ctx, revKey, &defRev)
			}, 10*time.Second, time.Second).Should(BeNil())

			deletedRevision := new(v1beta1.DefinitionRevision)
			deleteConfigMap := new(v1.ConfigMap)
			deleteRevKey := types.NamespacedName{Namespace: namespace, Name: tdName + "-v1"}
			deleteCMKey := types.NamespacedName{Namespace: namespace, Name: tdName + "-v1"}
			listOpts := []client.ListOption{
				client.InNamespace(namespace),
				client.MatchingLabels{
					oam.LabelTraitDefinitionName: tdName,
				},
			}
			defRevList := new(v1beta1.DefinitionRevisionList)
			Eventually(func() error {
				err := k8sClient.List(ctx, defRevList, listOpts...)
				if err != nil {
					return err
				}
				if len(defRevList.Items) != defRevisionLimit+1 {
					return fmt.Errorf("error defRevison number wants %d, actually %d", defRevisionLimit+1, len(defRevList.Items))
				}
				err = k8sClient.Get(ctx, deleteRevKey, deletedRevision)
				if err == nil || !apierrors.IsNotFound(err) {
					return fmt.Errorf("haven't clean up the oldest revision")
				}
				err = k8sClient.Get(ctx, deleteCMKey, deleteConfigMap)
				if err == nil || !apierrors.IsNotFound(err) {
					return fmt.Errorf("haven't clean up the oldest configMap")
				}
				return nil
			}, time.Second*30, time.Microsecond*300).Should(BeNil())

			By("update app again will continue to delete the oldest revision")
			Eventually(func() error {
				err := k8sClient.Get(ctx, defKey, checkComp)
				if err != nil {
					return err
				}
				checkComp.Spec.Schematic.CUE.Template = fmt.Sprintf(tdTemplate, fmt.Sprintf("test-v%d", revisionNum))
				return k8sClient.Update(ctx, checkComp)
			}, 10*time.Second, time.Second).Should(BeNil())
			testutil.ReconcileRetry(&r, req)

			revKey = client.ObjectKey{Namespace: namespace, Name: fmt.Sprintf("%s-v%d", tdName, revisionNum)}
			Eventually(func() error {
				return k8sClient.Get(ctx, revKey, &defRev)
			}, 10*time.Second, time.Second).Should(BeNil())

			deleteRevKey = types.NamespacedName{Namespace: namespace, Name: tdName + "-v2"}
			deleteCMKey = types.NamespacedName{Namespace: namespace, Name: tdName + "-v2"}
			Eventually(func() error {
				err := k8sClient.List(ctx, defRevList, listOpts...)
				if err != nil {
					return err
				}
				if len(defRevList.Items) != defRevisionLimit+1 {
					return fmt.Errorf("error defRevison number wants %d, actually %d", defRevisionLimit+1, len(defRevList.Items))
				}
				err = k8sClient.Get(ctx, deleteRevKey, deletedRevision)
				if err == nil || !apierrors.IsNotFound(err) {
					return fmt.Errorf("haven't clean up the oldest revision")
				}
				err = k8sClient.Get(ctx, deleteCMKey, deleteConfigMap)
				if err == nil || !apierrors.IsNotFound(err) {
					return fmt.Errorf("haven't clean up the oldest configMap")
				}
				return nil
			}, time.Second*30, time.Microsecond*300).Should(BeNil())
		})

		It("Test clean up definitionRevision contains definitionRevision with custom name", func() {
			var revKey client.ObjectKey
			var defRev v1beta1.DefinitionRevision
			revisionNames := []string{"1.3.1", "", "1.3.3", "", "prod"}
			tdName := "test-td-with-specify-revision"
			revisionNum := 1
			defKey := client.ObjectKey{Namespace: namespace, Name: tdName}
			req := reconcile.Request{NamespacedName: defKey}

			By("create a new traitDefinition")
			td := tdWithNoTemplate.DeepCopy()
			td.Name = tdName
			td.Spec.Schematic.CUE.Template = fmt.Sprintf(tdTemplate, fmt.Sprintf("test-v%d", revisionNum))
			Expect(k8sClient.Create(ctx, td)).Should(BeNil())
			testutil.ReconcileRetry(&r, req)
			revKey = client.ObjectKey{Namespace: namespace, Name: fmt.Sprintf("%s-v%d", tdName, revisionNum)}
			Eventually(func() error {
				return k8sClient.Get(ctx, revKey, &defRev)
			}, 10*time.Second, time.Second).Should(BeNil())
			Expect(defRev.Spec.Revision).Should(Equal(int64(revisionNum)))

			By("update traitDefinition")
			checkTrait := new(v1beta1.TraitDefinition)
			for _, revisionName := range revisionNames {
				revisionNum++
				Eventually(func() error {
					err := k8sClient.Get(ctx, defKey, checkTrait)
					if err != nil {
						return err
					}
					checkTrait.SetAnnotations(map[string]string{
						oam.AnnotationDefinitionRevisionName: revisionName,
					})
					checkTrait.Spec.Schematic.CUE.Template = fmt.Sprintf(tdTemplate, fmt.Sprintf("test-v%d", revisionNum))
					return k8sClient.Update(ctx, checkTrait)
				}, 10*time.Second, time.Second).Should(BeNil())

				Eventually(func() error {
					testutil.ReconcileOnce(&r, req)
					newTd := new(v1beta1.TraitDefinition)
					err := k8sClient.Get(ctx, req.NamespacedName, newTd)
					if err != nil {
						return err
					}
					if newTd.Status.LatestRevision.Revision != int64(revisionNum) {
						return fmt.Errorf("fail to update status")
					}
					return nil
				}, 15*time.Second, time.Second)

				if len(revisionName) == 0 {
					revKey = client.ObjectKey{Namespace: namespace, Name: fmt.Sprintf("%s-v%d", tdName, revisionNum)}
				} else {
					revKey = client.ObjectKey{Namespace: namespace, Name: fmt.Sprintf("%s-v%s", tdName, revisionName)}
				}

				By("check the definitionRevision is created by controller")
				var defRev v1beta1.DefinitionRevision
				Eventually(func() error {
					return k8sClient.Get(ctx, revKey, &defRev)
				}, 10*time.Second, time.Second).Should(BeNil())

				Expect(defRev.Spec.Revision).Should(Equal(int64(revisionNum)))
			}

			By("create new TraitDefinition will remove oldest definitionRevision")
			revisionNum++
			Eventually(func() error {
				err := k8sClient.Get(ctx, defKey, checkTrait)
				if err != nil {
					return err
				}
				checkTrait.SetAnnotations(map[string]string{
					oam.AnnotationDefinitionRevisionName: "test",
				})
				checkTrait.Spec.Schematic.CUE.Template = fmt.Sprintf(tdTemplate, "test-vtest")
				return k8sClient.Update(ctx, checkTrait)
			}, 10*time.Second, time.Second).Should(BeNil())

			Eventually(func() error {
				testutil.ReconcileOnce(&r, req)
				newTd := new(v1beta1.TraitDefinition)
				err := k8sClient.Get(ctx, req.NamespacedName, newTd)
				if err != nil {
					return err
				}
				if newTd.Status.LatestRevision.Revision != int64(revisionNum) {
					return fmt.Errorf("fail to update status")
				}
				return nil
			}, 15*time.Second, time.Second)

			revKey = client.ObjectKey{Namespace: namespace, Name: fmt.Sprintf("%s-v%s", tdName, "test")}
			Eventually(func() error {
				return k8sClient.Get(ctx, revKey, &defRev)
			}, 10*time.Second, time.Second).Should(BeNil())

			deletedRevision := new(v1beta1.DefinitionRevision)
			deleteConfigMap := new(v1.ConfigMap)
			deleteRevKey := types.NamespacedName{Namespace: namespace, Name: tdName + "-v1"}
			deleteCMKey := types.NamespacedName{Namespace: namespace, Name: tdName + "-v1"}
			listOpts := []client.ListOption{
				client.InNamespace(namespace),
				client.MatchingLabels{
					oam.LabelTraitDefinitionName: tdName,
				},
			}
			defRevList := new(v1beta1.DefinitionRevisionList)
			Eventually(func() error {
				err := k8sClient.List(ctx, defRevList, listOpts...)
				if err != nil {
					return err
				}
				if len(defRevList.Items) != defRevisionLimit+1 {
					return fmt.Errorf("error defRevison number wants %d, actually %d", defRevisionLimit+1, len(defRevList.Items))
				}
				err = k8sClient.Get(ctx, deleteRevKey, deletedRevision)
				if err == nil || !apierrors.IsNotFound(err) {
					return fmt.Errorf("haven't clean up the oldest revision")
				}
				err = k8sClient.Get(ctx, deleteCMKey, deleteConfigMap)
				if err == nil || !apierrors.IsNotFound(err) {
					return fmt.Errorf("haven't clean up the oldest configMap")
				}
				return nil
			}, time.Second*30, time.Microsecond*300).Should(BeNil())

			By("update app again will continue to delete the oldest revision")
			revisionNum++
			Eventually(func() error {
				err := k8sClient.Get(ctx, defKey, checkTrait)
				if err != nil {
					return err
				}
				checkTrait.SetAnnotations(map[string]string{
					oam.AnnotationDefinitionRevisionName: "",
				})
				checkTrait.Spec.Schematic.CUE.Template = fmt.Sprintf(tdTemplate, fmt.Sprintf("test-v%d", revisionNum))
				return k8sClient.Update(ctx, checkTrait)
			}, 10*time.Second, time.Second).Should(BeNil())

			Eventually(func() error {
				testutil.ReconcileOnce(&r, req)
				newTd := new(v1beta1.TraitDefinition)
				err := k8sClient.Get(ctx, req.NamespacedName, newTd)
				if err != nil {
					return err
				}
				if newTd.Status.LatestRevision.Revision != int64(revisionNum) {
					return fmt.Errorf("fail to update status")
				}
				return nil
			}, 15*time.Second, time.Second)

			revKey = client.ObjectKey{Namespace: namespace, Name: fmt.Sprintf("%s-v%d", tdName, revisionNum)}
			Eventually(func() error {
				return k8sClient.Get(ctx, revKey, &defRev)
			}, 10*time.Second, time.Second).Should(BeNil())

			deleteRevKey = types.NamespacedName{Namespace: namespace, Name: tdName + "-v1.3.1"}
			deleteCMKey = types.NamespacedName{Namespace: namespace, Name: tdName + "-v1.3.1"}
			Eventually(func() error {
				err := k8sClient.List(ctx, defRevList, listOpts...)
				if err != nil {
					return err
				}
				if len(defRevList.Items) != defRevisionLimit+1 {
					return fmt.Errorf("error defRevison number wants %d, actually %d", defRevisionLimit+1, len(defRevList.Items))
				}
				err = k8sClient.Get(ctx, deleteRevKey, deletedRevision)
				if err == nil || !apierrors.IsNotFound(err) {
					return fmt.Errorf("haven't clean up the oldest revision")
				}
				err = k8sClient.Get(ctx, deleteCMKey, deleteConfigMap)
				if err == nil || !apierrors.IsNotFound(err) {
					return fmt.Errorf("haven't clean up the oldest configMap")
				}
				return nil
			}, time.Second*30, time.Microsecond*300).Should(BeNil())
		})
	})
})

var tdTemplate = `        
patch: {
	// +patchKey=name
	spec: template: spec: containers: [{
		name:    "%s"
		image:   parameter.image
		command: parameter.cmd
		if parameter["volumes"] != _|_ {
			volumeMounts: [ for v in parameter.volumes {
				{
					mountPath: v.path
					name:      v.name
				}
			}]
		}
	}]
}
parameter: {

	// +usage=Specify the image of sidecar container
	image: string

	// +usage=Specify the commands run in the sidecar
	cmd?: [...string]

	// +usage=Specify the shared volume path
	volumes?: [...{
		name: string
		path: string
	}]
}
`

var tdWithNoTemplate = &v1beta1.TraitDefinition{
	TypeMeta: metav1.TypeMeta{
		Kind:       "TraitDefinition",
		APIVersion: "core.oam.dev/v1beta1",
	},
	ObjectMeta: metav1.ObjectMeta{
		Name:      "test-defrev",
		Namespace: "test-revision",
	},
	Spec: v1beta1.TraitDefinitionSpec{
		Schematic: &common.Schematic{
			CUE: &common.CUE{},
		},
	},
}
