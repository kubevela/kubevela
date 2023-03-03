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

package componentdefinition

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"time"

	"github.com/getkin/kin-openapi/openapi3"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	v1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	"sigs.k8s.io/yaml"

	"github.com/oam-dev/kubevela/apis/core.oam.dev/common"
	"github.com/oam-dev/kubevela/apis/core.oam.dev/v1beta1"
	apistypes "github.com/oam-dev/kubevela/apis/types"
	coredef "github.com/oam-dev/kubevela/pkg/controller/core.oam.dev/v1alpha2/core"
	"github.com/oam-dev/kubevela/pkg/oam"
	"github.com/oam-dev/kubevela/pkg/oam/testutil"
	"github.com/oam-dev/kubevela/pkg/oam/util"
)

var _ = Describe("Test DefinitionRevision created by ComponentDefinition", func() {
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

	Context("Test ComponentDefinition", func() {
		It("Test only create one ComponentDefinition", func() {
			cdName := "test-defrev-job"
			req := reconcile.Request{NamespacedName: client.ObjectKey{Name: cdName, Namespace: namespace}}

			cd := cdWithNoTemplate.DeepCopy()
			cd.Name = cdName
			cd.Spec.Schematic.CUE.Template = fmt.Sprintf(cdTemplate, "test")
			By("create componentDefinition")
			Expect(k8sClient.Create(ctx, cd)).Should(SatisfyAll(BeNil()))
			testutil.ReconcileRetry(&r, req)

			By("check whether DefinitionRevision is created")
			cdRevName := fmt.Sprintf("%s-v1", cdName)
			var cdRev v1beta1.DefinitionRevision
			Eventually(func() error {
				return k8sClient.Get(ctx, client.ObjectKey{Namespace: namespace, Name: cdRevName}, &cdRev)
			}, 10*time.Second, time.Second).Should(BeNil())

			var cm v1.ConfigMap
			schemeCMName := fmt.Sprintf("component-schema-%s", cdName)
			Eventually(func() error {
				return k8sClient.Get(ctx, client.ObjectKey{Namespace: namespace, Name: schemeCMName}, &cm)
			}, 10*time.Second, time.Second).Should(BeNil())

			schemaStr := cm.Data[apistypes.OpenapiV3JSONSchema]
			var schema openapi3.Schema
			Expect(json.Unmarshal([]byte(schemaStr), &schema)).Should(BeNil())
			Expect(len(schema.Properties)).Should(Equal(4))
		})

		It("Test creating the webservice ComponentDefinition", func() {
			cdName := "test-webservice"
			req := reconcile.Request{NamespacedName: client.ObjectKey{Name: cdName, Namespace: namespace}}

			content, err := os.ReadFile("./test-data/webservice-cd.yaml")
			Expect(err).Should(BeNil())
			var cd v1beta1.ComponentDefinition
			Expect(yaml.Unmarshal(content, &cd)).Should(BeNil())
			cd.Name = cdName
			cd.Namespace = namespace

			By("create componentDefinition")
			Expect(k8sClient.Create(ctx, &cd)).Should(SatisfyAll(BeNil()))
			testutil.ReconcileRetry(&r, req)

			schemeCMName := fmt.Sprintf("component-schema-%s", cdName)

			var cdGet v1beta1.ComponentDefinition
			Eventually(func() bool {
				if err := k8sClient.Get(ctx, client.ObjectKey{Namespace: namespace, Name: cdName}, &cdGet); err == nil {
					return len(cdGet.Status.Conditions) > 0 && cdGet.Status.Conditions[0].Status == v1.ConditionTrue && cdGet.Status.ConfigMapRef == schemeCMName
				}
				return false
			}, 10*time.Second, time.Second).Should(BeTrue())

			By("check whether DefinitionRevision is created")
			cdRevName := fmt.Sprintf("%s-v1", cdName)
			var cdRev v1beta1.DefinitionRevision
			Eventually(func() error {
				return k8sClient.Get(ctx, client.ObjectKey{Namespace: namespace, Name: cdRevName}, &cdRev)
			}, 10*time.Second, time.Second).Should(BeNil())

			var cm v1.ConfigMap

			Eventually(func() error {
				return k8sClient.Get(ctx, client.ObjectKey{Namespace: namespace, Name: schemeCMName}, &cm)
			}, 10*time.Second, time.Second).Should(BeNil())

			schemaStr := cm.Data[apistypes.OpenapiV3JSONSchema]
			var schema openapi3.Schema
			Expect(json.Unmarshal([]byte(schemaStr), &schema)).Should(BeNil())
			Expect(len(schema.Required)).Should(Equal(3))
		})

		It("Test update ComponentDefinition", func() {
			cdName := "test-update-componentdef"
			req := reconcile.Request{NamespacedName: client.ObjectKey{Name: cdName, Namespace: namespace}}

			cd1 := cdWithNoTemplate.DeepCopy()
			cd1.Name = cdName
			cd1.Spec.Schematic.CUE.Template = fmt.Sprintf(cdTemplate, "test-v1")
			By("create componentDefinition")
			Expect(k8sClient.Create(ctx, cd1)).Should(SatisfyAll(BeNil()))
			testutil.ReconcileRetry(&r, req)

			By("check whether definitionRevision is created")
			cdRevName1 := fmt.Sprintf("%s-v1", cdName)
			var cdRev1 v1beta1.DefinitionRevision
			Eventually(func() error {
				return k8sClient.Get(ctx, client.ObjectKey{Namespace: namespace, Name: cdRevName1}, &cdRev1)
			}, 10*time.Second, time.Second).Should(BeNil())

			By("update componentDefinition")
			cd := new(v1beta1.ComponentDefinition)
			Eventually(func() error {
				err := k8sClient.Get(ctx, client.ObjectKey{Namespace: namespace, Name: cdName}, cd)
				if err != nil {
					return err
				}
				cd.Spec.Schematic.CUE.Template = fmt.Sprintf(cdTemplate, "test-v2")
				return k8sClient.Update(ctx, cd)
			}, 10*time.Second, time.Second).Should(BeNil())

			testutil.ReconcileRetry(&r, req)

			By("check whether a new definitionRevision is created")
			cdRevName2 := fmt.Sprintf("%s-v2", cdName)
			var cdRev2 v1beta1.DefinitionRevision
			Eventually(func() error {
				return k8sClient.Get(ctx, client.ObjectKey{Namespace: namespace, Name: cdRevName2}, &cdRev2)
			}, 10*time.Second, time.Second).Should(BeNil())

		})
	})

	Context("Test DefinitionRevision created by ComponentDefinition", func() {

		It("Test different ComponentDefinition with same Spec, Should have the same hash value", func() {
			cd1 := cdWithNoTemplate.DeepCopy()
			cd1.Name = "test-cd1"
			cd1.Spec.Schematic.CUE.Template = fmt.Sprintf(cdTemplate, "test-defrev")
			cd2 := cdWithNoTemplate.DeepCopy()
			cd2.Name = "test-cd2"
			cd2.Spec.Schematic.CUE.Template = fmt.Sprintf(cdTemplate, "test-defrev")

			defRev1, _, err := coredef.GenerateDefinitionRevision(ctx, r.Client, cd1)
			Expect(err).Should(BeNil())
			defRev2, _, err := coredef.GenerateDefinitionRevision(ctx, r.Client, cd2)
			Expect(err).Should(BeNil())
			Expect(defRev1.Spec.RevisionHash).Should(Equal(defRev2.Spec.RevisionHash))
		})

		It("Test only update ComponentDefinition Labels, Shouldn't create new revision", func() {
			cd := cdWithNoTemplate.DeepCopy()
			cdName := "test-cd"
			cd.Name = cdName
			defKey := client.ObjectKey{Namespace: namespace, Name: cdName}
			req := reconcile.Request{NamespacedName: defKey}
			cd.Spec.Schematic.CUE.Template = fmt.Sprintf(cdTemplate, "test")
			Expect(k8sClient.Create(ctx, cd)).Should(BeNil())
			testutil.ReconcileRetry(&r, req)

			By("Check revision create by ComponentDefinition")
			defRevName := fmt.Sprintf("%s-v1", cdName)
			revKey := client.ObjectKey{Namespace: namespace, Name: defRevName}
			var defRev v1beta1.DefinitionRevision
			Eventually(func() error {
				return k8sClient.Get(ctx, revKey, &defRev)
			}, 10*time.Second, time.Second).Should(BeNil())

			By("Only update componentDefinition Labels")
			var checkRev v1beta1.ComponentDefinition
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

			newDefRevName := fmt.Sprintf("%s-v2", cdName)
			newRevKey := client.ObjectKey{Namespace: namespace, Name: newDefRevName}
			Expect(k8sClient.Get(ctx, newRevKey, &defRev)).Should(HaveOccurred())
		})

	})

	Context("Test create DefinitionRevision with the specified name", func() {
		It("Test specified DefinitionRevision name in annotation", func() {
			cdName := "test-specified-defrev-name"
			req := reconcile.Request{NamespacedName: client.ObjectKey{Name: cdName, Namespace: namespace}}

			cd := cdWithNoTemplate.DeepCopy()
			cd.Name = cdName
			cd.Spec.Schematic.CUE.Template = fmt.Sprintf(cdTemplate, "test")
			cd.SetAnnotations(map[string]string{
				oam.AnnotationDefinitionRevisionName: "1.1.3",
			})
			By("create componentDefinition")
			Expect(k8sClient.Create(ctx, cd)).Should(SatisfyAll(BeNil()))
			testutil.ReconcileRetry(&r, req)

			By("check whether DefinitionRevision is created")
			cdRevName := fmt.Sprintf("%s-v1.1.3", cdName)
			var cdRev v1beta1.DefinitionRevision
			Eventually(func() error {
				return k8sClient.Get(ctx, client.ObjectKey{Namespace: namespace, Name: cdRevName}, &cdRev)
			}, 10*time.Second, time.Second).Should(BeNil())

			By("check the DefinitionRevision's RevisionNum")
			Expect(cdRev.Spec.Revision).Should(Equal(int64(1)))
		})
	})

	Context("Test ComponentDefinition Controller clean up", func() {
		It("Test clean up definitionRevision", func() {
			var revKey client.ObjectKey
			var defRev v1beta1.DefinitionRevision
			cdName := "test-clean-up"
			revisionNum := 1
			defKey := client.ObjectKey{Namespace: namespace, Name: cdName}
			req := reconcile.Request{NamespacedName: defKey}

			By("create a new componentDefinition")
			cd := cdWithNoTemplate.DeepCopy()
			cd.Name = cdName
			cd.Spec.Schematic.CUE.Template = fmt.Sprintf(cdTemplate, fmt.Sprintf("test-v%d", revisionNum))
			Expect(k8sClient.Create(ctx, cd)).Should(BeNil())

			By("update componentDefinition")
			checkComp := new(v1beta1.ComponentDefinition)
			for i := 0; i < defRevisionLimit+1; i++ {
				Eventually(func() error {
					err := k8sClient.Get(ctx, defKey, checkComp)
					if err != nil {
						return err
					}
					checkComp.Spec.Schematic.CUE.Template = fmt.Sprintf(cdTemplate, fmt.Sprintf("test-v%d", revisionNum))
					return k8sClient.Update(ctx, checkComp)
				}, 10*time.Second, time.Second).Should(BeNil())
				testutil.ReconcileRetry(&r, req)

				revKey = client.ObjectKey{Namespace: namespace, Name: fmt.Sprintf("%s-v%d", cdName, revisionNum)}
				revisionNum++
				var defRev v1beta1.DefinitionRevision
				Eventually(func() error {
					return k8sClient.Get(ctx, revKey, &defRev)
				}, 10*time.Second, time.Second).Should(BeNil())
			}

			By("create new componentDefinition will remove oldest definitionRevision")
			Eventually(func() error {
				err := k8sClient.Get(ctx, defKey, checkComp)
				if err != nil {
					return err
				}
				checkComp.Spec.Schematic.CUE.Template = fmt.Sprintf(cdTemplate, fmt.Sprintf("test-v%d", revisionNum))
				return k8sClient.Update(ctx, checkComp)
			}, 10*time.Second, time.Second).Should(BeNil())
			testutil.ReconcileRetry(&r, req)

			revKey = client.ObjectKey{Namespace: namespace, Name: fmt.Sprintf("%s-v%d", cdName, revisionNum)}
			revisionNum++
			Eventually(func() error {
				return k8sClient.Get(ctx, revKey, &defRev)
			}, 10*time.Second, time.Second).Should(BeNil())

			deletedRevision := new(v1beta1.DefinitionRevision)
			deleteConfigMap := new(v1.ConfigMap)
			deleteRevKey := types.NamespacedName{Namespace: namespace, Name: cdName + "-v1"}
			deleteCMKey := types.NamespacedName{Namespace: namespace, Name: cdName + "-v1"}
			listOpts := []client.ListOption{
				client.InNamespace(namespace),
				client.MatchingLabels{
					oam.LabelComponentDefinitionName: cdName,
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
				checkComp.Spec.Schematic.CUE.Template = fmt.Sprintf(cdTemplate, fmt.Sprintf("test-v%d", revisionNum))
				return k8sClient.Update(ctx, checkComp)
			}, 10*time.Second, time.Second).Should(BeNil())
			testutil.ReconcileRetry(&r, req)

			revKey = client.ObjectKey{Namespace: namespace, Name: fmt.Sprintf("%s-v%d", cdName, revisionNum)}
			Eventually(func() error {
				return k8sClient.Get(ctx, revKey, &defRev)
			}, 10*time.Second, time.Second).Should(BeNil())

			deleteRevKey = types.NamespacedName{Namespace: namespace, Name: cdName + "-v2"}
			deleteCMKey = types.NamespacedName{Namespace: namespace, Name: cdName + "-v2"}
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
			cdName := "test-cd-with-specify-revision"
			revisionNum := 1
			defKey := client.ObjectKey{Namespace: namespace, Name: cdName}
			req := reconcile.Request{NamespacedName: defKey}

			By("create a new componentDefinition")
			cd := cdWithNoTemplate.DeepCopy()
			cd.Name = cdName
			cd.Spec.Schematic.CUE.Template = fmt.Sprintf(cdTemplate, fmt.Sprintf("test-v%d", revisionNum))
			Expect(k8sClient.Create(ctx, cd)).Should(BeNil())
			testutil.ReconcileRetry(&r, req)
			revKey = client.ObjectKey{Namespace: namespace, Name: fmt.Sprintf("%s-v%d", cdName, revisionNum)}
			Eventually(func() error {
				return k8sClient.Get(ctx, revKey, &defRev)
			}, 10*time.Second, time.Second).Should(BeNil())
			Expect(defRev.Spec.Revision).Should(Equal(int64(revisionNum)))

			By("update componentDefinition")
			for _, revisionName := range revisionNames {
				revisionNum++
				Eventually(func() error {
					oldComp := cd.DeepCopy()
					err := k8sClient.Get(ctx, defKey, oldComp)
					if err != nil {
						return err
					}
					oldComp.SetAnnotations(map[string]string{
						oam.AnnotationDefinitionRevisionName: revisionName,
					})
					oldComp.Spec.Schematic.CUE.Template = fmt.Sprintf(cdTemplate, fmt.Sprintf("test-v%d", revisionNum))
					return k8sClient.Update(ctx, oldComp)
				}, 10*time.Second, time.Second).Should(BeNil())

				Eventually(func() error {
					testutil.ReconcileOnce(&r, req)
					newCd := new(v1beta1.ComponentDefinition)
					err := k8sClient.Get(ctx, req.NamespacedName, newCd)
					if err != nil {
						return err
					}
					if newCd.Status.LatestRevision.Revision != int64(revisionNum) {
						return fmt.Errorf("fail to update status")
					}
					return nil
				}, 15*time.Second, time.Second)

				if len(revisionName) == 0 {
					revKey = client.ObjectKey{Namespace: namespace, Name: fmt.Sprintf("%s-v%d", cdName, revisionNum)}
				} else {
					revKey = client.ObjectKey{Namespace: namespace, Name: fmt.Sprintf("%s-v%s", cdName, revisionName)}
				}

				By("check the definitionRevision is created by controller")
				var defRev v1beta1.DefinitionRevision
				Eventually(func() error {
					return k8sClient.Get(ctx, revKey, &defRev)
				}, 10*time.Second, time.Second).Should(BeNil())

				Expect(defRev.Spec.Revision).Should(Equal(int64(revisionNum)))
			}

			By("create new componentDefinition will remove oldest definitionRevision")
			revisionNum++
			Eventually(func() error {
				oldComp := cd.DeepCopy()
				err := k8sClient.Get(ctx, defKey, oldComp)
				if err != nil {
					return err
				}
				oldComp.SetAnnotations(map[string]string{
					oam.AnnotationDefinitionRevisionName: "test",
				})
				oldComp.Spec.Schematic.CUE.Template = fmt.Sprintf(cdTemplate, "test-vtest")
				return k8sClient.Update(ctx, oldComp)
			}, 10*time.Second, time.Second).Should(BeNil())

			Eventually(func() error {
				testutil.ReconcileOnce(&r, req)
				newCd := new(v1beta1.ComponentDefinition)
				err := k8sClient.Get(ctx, req.NamespacedName, newCd)
				if err != nil {
					return err
				}
				if newCd.Status.LatestRevision.Revision != int64(revisionNum) {
					return fmt.Errorf("fail to update status")
				}
				return nil
			}, 15*time.Second, time.Second)

			revKey = client.ObjectKey{Namespace: namespace, Name: fmt.Sprintf("%s-v%s", cdName, "test")}
			Eventually(func() error {
				return k8sClient.Get(ctx, revKey, &defRev)
			}, 10*time.Second, time.Second).Should(BeNil())

			deletedRevision := new(v1beta1.DefinitionRevision)
			deleteConfigMap := new(v1.ConfigMap)
			deleteRevKey := types.NamespacedName{Namespace: namespace, Name: cdName + "-v1"}
			deleteCMKey := types.NamespacedName{Namespace: namespace, Name: cdName + "-v1"}
			listOpts := []client.ListOption{
				client.InNamespace(namespace),
				client.MatchingLabels{
					oam.LabelComponentDefinitionName: cdName,
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
				oldComp := cd.DeepCopy()
				err := k8sClient.Get(ctx, defKey, oldComp)
				if err != nil {
					return err
				}
				oldComp.SetAnnotations(map[string]string{
					oam.AnnotationDefinitionRevisionName: "",
				})
				oldComp.Spec.Schematic.CUE.Template = fmt.Sprintf(cdTemplate, fmt.Sprintf("test-v%d", revisionNum))
				return k8sClient.Update(ctx, oldComp)
			}, 10*time.Second, time.Second).Should(BeNil())

			Eventually(func() error {
				testutil.ReconcileOnce(&r, req)
				newCd := new(v1beta1.ComponentDefinition)
				err := k8sClient.Get(ctx, req.NamespacedName, newCd)
				if err != nil {
					return err
				}
				if newCd.Status.LatestRevision.Revision != int64(revisionNum) {
					return fmt.Errorf("fail to update status")
				}
				return nil
			}, 15*time.Second, time.Second)

			revKey = client.ObjectKey{Namespace: namespace, Name: fmt.Sprintf("%s-v%d", cdName, revisionNum)}
			Eventually(func() error {
				return k8sClient.Get(ctx, revKey, &defRev)
			}, 10*time.Second, time.Second).Should(BeNil())

			deleteRevKey = types.NamespacedName{Namespace: namespace, Name: cdName + "-v1.3.1"}
			deleteCMKey = types.NamespacedName{Namespace: namespace, Name: cdName + "-v1.3.1"}
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

var cdTemplate = `        
output: {
	apiVersion: "batch/v1"
	kind:       "Job"
	spec: {
		parallelism: parameter.count
		completions: parameter.count
		template: spec: {
			restartPolicy: parameter.restart
			containers: [{
				name:  "%s"
				image: parameter.image

				if parameter["cmd"] != _|_ {
					command: parameter.cmd
				}
			}]
		}
	}
}
parameter: {
	// +usage=Specify number of tasks to run in parallel
	// +short=c
	count: *1 | int

	// +usage=Which image would you like to use for your service
	// +short=i
	image: string

	// +usage=Define the job restart policy, the value can only be Never or OnFailure. By default, it's Never.
	restart: *"Never" | string

	// +usage=Commands to run in the container
	cmd?: [...string]
}
`

var cdWithNoTemplate = &v1beta1.ComponentDefinition{
	TypeMeta: metav1.TypeMeta{
		Kind:       "ComponentDefinition",
		APIVersion: "core.oam.dev/v1beta1",
	},
	ObjectMeta: metav1.ObjectMeta{
		Name:      "test-defrev",
		Namespace: "test-revision",
	},
	Spec: v1beta1.ComponentDefinitionSpec{
		Workload: common.WorkloadTypeDescriptor{
			Definition: common.WorkloadGVK{
				APIVersion: "batch/v1",
				Kind:       "Job",
			},
		},
		Schematic: &common.Schematic{
			CUE: &common.CUE{},
		},
	},
}
