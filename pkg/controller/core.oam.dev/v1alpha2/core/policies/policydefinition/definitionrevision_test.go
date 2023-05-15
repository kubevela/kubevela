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

package policydefinition

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

var _ = Describe("Test DefinitionRevision created by PolicyDefinition", func() {
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

	Context("Test PolicyDefinition", func() {
		It("Test update PolicyDefinition", func() {
			defName := "test-def"
			req := reconcile.Request{NamespacedName: client.ObjectKey{Name: defName, Namespace: namespace}}

			def1 := defWithNoTemplate.DeepCopy()
			def1.Name = defName
			def1.Spec.Schematic.CUE.Template = fmt.Sprintf(defTemplate, "test-v1")
			By("create policyDefinition")
			Expect(k8sClient.Create(ctx, def1)).Should(SatisfyAll(BeNil()))
			testutil.ReconcileRetry(&r, req)

			By("check whether definitionRevision is created")
			defRevName1 := fmt.Sprintf("%s-v1", defName)
			var defRev1 v1beta1.DefinitionRevision
			Eventually(func() error {
				err := k8sClient.Get(ctx, client.ObjectKey{Namespace: namespace, Name: defRevName1}, &defRev1)
				return err
			}, 10*time.Second, time.Second).Should(BeNil())

			By("update policyDefinition")
			def := new(v1beta1.PolicyDefinition)
			Eventually(func() error {
				err := k8sClient.Get(ctx, client.ObjectKey{Namespace: namespace, Name: defName}, def)
				if err != nil {
					return err
				}
				def.Spec.Schematic.CUE.Template = fmt.Sprintf(defTemplate, "test-v2")
				return k8sClient.Update(ctx, def)
			}, 10*time.Second, time.Second).Should(BeNil())

			testutil.ReconcileRetry(&r, req)

			By("check whether a new definitionRevision is created")
			tdRevName2 := fmt.Sprintf("%s-v2", defName)
			var tdRev2 v1beta1.DefinitionRevision
			Eventually(func() error {
				return k8sClient.Get(ctx, client.ObjectKey{Namespace: namespace, Name: tdRevName2}, &tdRev2)
			}, 10*time.Second, time.Second).Should(BeNil())
		})

		It("Test only update PolicyDefinition Labels, Shouldn't create new revision", func() {
			def := defWithNoTemplate.DeepCopy()
			defName := "test-def-update"
			def.Name = defName
			defKey := client.ObjectKey{Namespace: namespace, Name: defName}
			req := reconcile.Request{NamespacedName: defKey}
			def.Spec.Schematic.CUE.Template = fmt.Sprintf(defTemplate, "test")
			Expect(k8sClient.Create(ctx, def)).Should(BeNil())
			testutil.ReconcileRetry(&r, req)

			By("Check revision create by PolicyDefinition")
			defRevName := fmt.Sprintf("%s-v1", defName)
			revKey := client.ObjectKey{Namespace: namespace, Name: defRevName}
			var defRev v1beta1.DefinitionRevision
			Eventually(func() error {
				return k8sClient.Get(ctx, revKey, &defRev)
			}, 10*time.Second, time.Second).Should(BeNil())

			By("Only update PolicyDefinition Labels")
			var checkRev v1beta1.PolicyDefinition
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

			newDefRevName := fmt.Sprintf("%s-v2", defName)
			newRevKey := client.ObjectKey{Namespace: namespace, Name: newDefRevName}
			Expect(k8sClient.Get(ctx, newRevKey, &defRev)).Should(HaveOccurred())
		})
	})

	Context("Test PolicyDefinition Controller clean up", func() {
		It("Test clean up definitionRevision", func() {
			var revKey client.ObjectKey
			var defRev v1beta1.DefinitionRevision
			defName := "test-clean-up"
			revisionNum := 1
			defKey := client.ObjectKey{Namespace: namespace, Name: defName}
			req := reconcile.Request{NamespacedName: defKey}

			By("create a new PolicyDefinition")
			def := defWithNoTemplate.DeepCopy()
			def.Name = defName
			def.Spec.Schematic.CUE.Template = fmt.Sprintf(defTemplate, fmt.Sprintf("test-v%d", revisionNum))
			Expect(k8sClient.Create(ctx, def)).Should(BeNil())

			By("update PolicyDefinition")
			checkRev := new(v1beta1.PolicyDefinition)
			for i := 0; i < defRevisionLimit+1; i++ {
				Eventually(func() error {
					err := k8sClient.Get(ctx, defKey, checkRev)
					if err != nil {
						return err
					}
					checkRev.Spec.Schematic.CUE.Template = fmt.Sprintf(defTemplate, fmt.Sprintf("test-v%d", revisionNum))
					return k8sClient.Update(ctx, checkRev)
				}, 10*time.Second, time.Second).Should(BeNil())
				testutil.ReconcileRetry(&r, req)

				revKey = client.ObjectKey{Namespace: namespace, Name: fmt.Sprintf("%s-v%d", defName, revisionNum)}
				revisionNum++
				var defRev v1beta1.DefinitionRevision
				Eventually(func() error {
					return k8sClient.Get(ctx, revKey, &defRev)
				}, 10*time.Second, time.Second).Should(BeNil())
			}

			By("create new PolicyDefinition will remove oldest definitionRevision")
			Eventually(func() error {
				err := k8sClient.Get(ctx, defKey, checkRev)
				if err != nil {
					return err
				}
				checkRev.Spec.Schematic.CUE.Template = fmt.Sprintf(defTemplate, fmt.Sprintf("test-v%d", revisionNum))
				return k8sClient.Update(ctx, checkRev)
			}, 10*time.Second, time.Second).Should(BeNil())
			testutil.ReconcileRetry(&r, req)

			revKey = client.ObjectKey{Namespace: namespace, Name: fmt.Sprintf("%s-v%d", defName, revisionNum)}
			revisionNum++
			Eventually(func() error {
				return k8sClient.Get(ctx, revKey, &defRev)
			}, 10*time.Second, time.Second).Should(BeNil())

			deletedRevision := new(v1beta1.DefinitionRevision)
			deleteRevKey := types.NamespacedName{Namespace: namespace, Name: defName + "-v1"}
			listOpts := []client.ListOption{
				client.InNamespace(namespace),
				client.MatchingLabels{
					oam.LabelPolicyDefinitionName: defName,
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
				return nil
			}, time.Second*30, time.Microsecond*300).Should(BeNil())

			By("update app again will continue to delete the oldest revision")
			Eventually(func() error {
				err := k8sClient.Get(ctx, defKey, checkRev)
				if err != nil {
					return err
				}
				checkRev.Spec.Schematic.CUE.Template = fmt.Sprintf(defTemplate, fmt.Sprintf("test-v%d", revisionNum))
				return k8sClient.Update(ctx, checkRev)
			}, 10*time.Second, time.Second).Should(BeNil())
			testutil.ReconcileRetry(&r, req)

			revKey = client.ObjectKey{Namespace: namespace, Name: fmt.Sprintf("%s-v%d", defName, revisionNum)}
			Eventually(func() error {
				return k8sClient.Get(ctx, revKey, &defRev)
			}, 10*time.Second, time.Second).Should(BeNil())

			deleteRevKey = types.NamespacedName{Namespace: namespace, Name: defName + "-v2"}
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
				return nil
			}, time.Second*30, time.Microsecond*300).Should(BeNil())
		})
	})
})

var defTemplate = `        
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

var defWithNoTemplate = &v1beta1.PolicyDefinition{
	TypeMeta: metav1.TypeMeta{
		Kind:       "PolicyDefinition",
		APIVersion: "core.oam.dev/v1beta1",
	},
	ObjectMeta: metav1.ObjectMeta{
		Name:      "test-defrev",
		Namespace: "test-revision",
	},
	Spec: v1beta1.PolicyDefinitionSpec{
		Schematic: &common.Schematic{
			CUE: &common.CUE{},
		},
	},
}
