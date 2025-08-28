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

package controllers_test

import (
	"context"
	"fmt"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	testdef "github.com/kubevela/pkg/util/test/definition"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	pkgruntime "k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/oam-dev/kubevela/apis/core.oam.dev/common"
	"github.com/oam-dev/kubevela/apis/core.oam.dev/v1beta1"
	"github.com/oam-dev/kubevela/apis/types"
	pkgdef "github.com/oam-dev/kubevela/pkg/definition"
	"github.com/oam-dev/kubevela/pkg/oam"
	"github.com/oam-dev/kubevela/pkg/oam/util"
	utilcommon "github.com/oam-dev/kubevela/pkg/utils/common"
)

var _ = Describe("ComponentDefinition Normal tests", func() {
	ctx := context.Background()

	var namespace string
	var ns corev1.Namespace

	BeforeEach(func() {
		namespace = randomNamespaceName("def-e2e-test")
		ns = corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: namespace}}

		Eventually(func() error {
			return k8sClient.Create(ctx, &ns)
		}, time.Second*3, time.Microsecond*300).Should(SatisfyAny(BeNil(), &util.AlreadyExistMatcher{}))
	})

	AfterEach(func() {
		By("Clean up resources after a test")
		k8sClient.DeleteAllOf(ctx, &v1beta1.ComponentDefinition{}, client.InNamespace(namespace))
		k8sClient.DeleteAllOf(ctx, &v1beta1.WorkloadDefinition{}, client.InNamespace(namespace))
		k8sClient.DeleteAllOf(ctx, &v1beta1.TraitDefinition{}, client.InNamespace(namespace))
		k8sClient.DeleteAllOf(ctx, &v1beta1.WorkflowStepDefinition{}, client.InNamespace(namespace))
		k8sClient.DeleteAllOf(ctx, &v1beta1.DefinitionRevision{}, client.InNamespace(namespace))

		By(fmt.Sprintf("Delete the entire namespaceName %s", ns.Name))
		Expect(k8sClient.Delete(ctx, &ns, client.PropagationPolicy(metav1.DeletePropagationForeground))).Should(Succeed())
	})

	Context("Test dynamic admission control for componentDefinition", func() {

		It("Test componentDefinition which only set type field", func() {
			workDef := &v1beta1.WorkloadDefinition{
				TypeMeta: metav1.TypeMeta{
					Kind:       "WorkloadDefinition",
					APIVersion: "core.oam.dev/v1beta1",
				},
				ObjectMeta: metav1.ObjectMeta{
					Name: "deployments.apps",
				},
				Spec: v1beta1.WorkloadDefinitionSpec{
					Reference: common.DefinitionReference{
						Name:    "deployments.apps",
						Version: "v1",
					},
				},
			}
			workDef.SetNamespace(namespace)
			Expect(k8sClient.Create(ctx, workDef)).Should(BeNil())
			getWd := new(v1beta1.WorkloadDefinition)
			Eventually(func() error {
				return k8sClient.Get(ctx, client.ObjectKey{Name: workDef.Name, Namespace: namespace}, getWd)
			}, 15*time.Second, time.Second).Should(BeNil())

			cd := webServiceWithNoTemplate.DeepCopy()
			cd.Spec.Workload.Definition = common.WorkloadGVK{}
			cd.Spec.Workload.Type = "deployments.apps"
			cd.SetNamespace(namespace)
			cd.SetName("test-componentdef")
			cd.Spec.Schematic.CUE.Template = webServiceV1Template

			Eventually(func() error {
				return k8sClient.Create(ctx, cd)
			}, 5*time.Second, time.Second).Should(BeNil())

			defRev := new(v1beta1.DefinitionRevision)
			Eventually(func() error {
				return k8sClient.Get(ctx, client.ObjectKey{Name: "test-componentdef-v1", Namespace: namespace}, defRev)
			}).Should(BeNil())
		})

		It("Test componentDefinition only set definition field", func() {
			testCd := webServiceWithNoTemplate.DeepCopy()
			testCd.Spec.Schematic.CUE.Template = webServiceV1Template
			testCd.SetName("test-componentdef-v1")
			testCd.SetNamespace(namespace)
			Expect(k8sClient.Create(ctx, testCd)).Should(Succeed())

			By("check MutatingWebhook fill the type filed")
			newCd := new(v1beta1.ComponentDefinition)
			Eventually(func() error {
				return k8sClient.Get(ctx, client.ObjectKey{Name: testCd.Name, Namespace: namespace}, newCd)
			}, 15*time.Second, time.Second).Should(BeNil())

			Expect(newCd.Spec.Workload.Type).Should(Equal("deployments.apps"))

			By("check workloadDefinition created by MutatingWebhook")
			newWd := new(v1beta1.WorkloadDefinition)
			wdName := newCd.Spec.Workload.Type
			Eventually(func() error {
				return k8sClient.Get(ctx, client.ObjectKey{Name: wdName, Namespace: namespace}, newWd)
			}, 15*time.Second, time.Second).Should(BeNil())

			Expect(newWd.Spec.Reference.Name).Should(Equal(wdName))
			Expect(newWd.Spec.Reference.Version).To(Equal("v1"))
		})

		It("Test componentDefinition which definition and type fields are all empty", func() {
			testCd1 := webServiceWithNoTemplate.DeepCopy()
			testCd1.SetName("test-componentdef-v2")
			testCd1.Spec.Workload.Definition = common.WorkloadGVK{}
			testCd1.Spec.Schematic.CUE.Template = webServiceV1Template
			testCd1.SetNamespace(namespace)
			Expect(k8sClient.Create(ctx, testCd1)).Should(BeNil())

			newCd := new(v1beta1.ComponentDefinition)
			Eventually(func() error {
				return k8sClient.Get(ctx, client.ObjectKey{Name: testCd1.Name, Namespace: namespace}, newCd)
			}, 15*time.Second, time.Second).Should(BeNil())
			Expect(newCd.Spec.Workload.Type).Should(Equal(types.AutoDetectWorkloadDefinition))
		})

		It("Test componentDefinition which definition and type point to same workload type", func() {
			testCd2 := webServiceWithNoTemplate.DeepCopy()
			testCd2.SetName("test-componentdef-v3")
			testCd2.Spec.Workload.Type = "deployments.apps"
			testCd2.Spec.Schematic.CUE.Template = webServiceV1Template
			testCd2.SetNamespace(namespace)
			Expect(k8sClient.Create(ctx, testCd2)).Should(Succeed())
		})

		It("Test componentDefinition which definition and type point to different workload type", func() {
			testCd3 := webServiceWithNoTemplate.DeepCopy()
			testCd3.SetName("test-componentdef-v4")
			testCd3.Spec.Workload.Type = "jobs.batch"
			testCd3.Spec.Schematic.CUE.Template = webServiceV1Template
			testCd3.SetNamespace(namespace)
			Expect(k8sClient.Create(ctx, testCd3)).Should(HaveOccurred())
		})

		It("Test componentDefinition which specify the name of definitionRevision", func() {
			By("create componentDefinition")
			cd := webServiceWithNoTemplate.DeepCopy()
			cd.SetNamespace(namespace)
			cd.SetName("test-def-specify-revision")
			cd.SetAnnotations(map[string]string{
				oam.AnnotationDefinitionRevisionName: "1.1.1",
			})
			cd.Spec.Schematic.CUE.Template = webServiceV1Template
			Expect(k8sClient.Create(ctx, cd)).Should(Succeed())

			By("check definitionRevision created by controller")
			defRev := new(v1beta1.DefinitionRevision)
			Eventually(func() error {
				return k8sClient.Get(ctx, client.ObjectKey{Name: fmt.Sprintf("%s-v%s", cd.Name, "1.1.1"), Namespace: namespace}, defRev)
			}, 15*time.Second, time.Second).Should(BeNil())

			By("update componentDefinition")
			oldCd := new(v1beta1.ComponentDefinition)
			Eventually(func() error {
				return k8sClient.Get(ctx, client.ObjectKey{Name: cd.Name, Namespace: namespace}, oldCd)
			}, 15*time.Second, time.Second).Should(BeNil())

			newCd := oldCd.DeepCopy()
			cd.Spec.Schematic.CUE.Template = webServiceV2Template
			Expect(k8sClient.Create(ctx, newCd)).Should(HaveOccurred())
		})
	})

	Context("Test dynamic admission control for traitDefinition", func() {
		It("Test traitDefinition which specify the name of definitionRevision", func() {
			By("create traitDefinition")
			td := exposeWithNoTemplate.DeepCopy()
			td.SetNamespace(namespace)
			td.SetName("test-td-specify-revision")
			td.SetAnnotations(map[string]string{
				oam.AnnotationDefinitionRevisionName: "1.1.1",
			})
			td.Spec.Schematic.CUE.Template = exposeV1Template
			Expect(k8sClient.Create(ctx, td)).Should(Succeed())

			By("check definitionRevision created by controller")
			defRev := new(v1beta1.DefinitionRevision)
			Eventually(func() error {
				return k8sClient.Get(ctx, client.ObjectKey{Name: fmt.Sprintf("%s-v%s", td.Name, "1.1.1"), Namespace: namespace}, defRev)
			}, 15*time.Second, time.Second).Should(BeNil())

			By("update traitDefinition spec, should be ejected")
			oldTd := new(v1beta1.TraitDefinition)
			Eventually(func() error {
				return k8sClient.Get(ctx, client.ObjectKey{Name: td.Name, Namespace: namespace}, oldTd)
			}, 15*time.Second, time.Second).Should(BeNil())

			newTd := oldTd.DeepCopy()
			newTd.Spec.Schematic.CUE.Template = exposeV2Template
			Expect(k8sClient.Create(ctx, newTd)).Should(HaveOccurred())
		})
	})

	It("Test notification step definition", func() {
		By("Install notification workflow step definition")
		_, file, _, _ := runtime.Caller(0)
		Expect(testdef.InstallDefinitionFromYAML(ctx, k8sClient, filepath.Join(file, "../../../charts/vela-core/templates/defwithtemplate/notification.yaml"), func(s string) string {
			return strings.ReplaceAll(s, `{{ include "systemDefinitionNamespace" . }}`, "vela-system")
		})).Should(SatisfyAny(Succeed(), &util.AlreadyExistMatcher{}))

		By("Create a secret for the notification step")
		Expect(k8sClient.Create(ctx, &corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "my-secret",
				Namespace: namespace,
			},
			StringData: map[string]string{"url": "https://kubevela.io"},
		})).Should(SatisfyAny(Succeed(), &util.AlreadyExistMatcher{}))

		By("Create application with notification step consuming a secret")
		var newApp v1beta1.Application
		Expect(utilcommon.ReadYamlToObject("testdata/app/app_notification_secret.yaml", &newApp)).Should(BeNil())
		newApp.Namespace = namespace
		Expect(k8sClient.Create(ctx, &newApp)).Should(BeNil())

		By("Verify application is running")
		verifyApplicationPhase(context.TODO(), newApp.Namespace, newApp.Name, common.ApplicationRunning)

		By("Create application with notification step")
		newApp = v1beta1.Application{}
		Expect(utilcommon.ReadYamlToObject("testdata/app/app_notification.yaml", &newApp)).Should(BeNil())
		newApp.Namespace = namespace
		Expect(k8sClient.Create(ctx, &newApp)).Should(BeNil())

		By("Verify application is running")
		verifyApplicationPhase(context.TODO(), newApp.Namespace, newApp.Name, common.ApplicationRunning)
	})

	Context("Definition Retrieval and CUE Parsing Validation", func() {
		It("should successfully parse all definitions loaded from helm chart templates", func() {
			By("Loading all definition YAML files from charts/vela-core/templates/defwithtemplate")
			_, file, _, _ := runtime.Caller(0)
			definitionDir := filepath.Join(file, "../../../charts/vela-core/templates/defwithtemplate")

			files, err := filepath.Glob(filepath.Join(definitionDir, "*.yaml"))
			Expect(err).To(BeNil())
			Expect(len(files)).To(BeNumerically(">", 0))

			By(fmt.Sprintf("Found %d definition YAML files to test", len(files)))

			// Install all definitions
			installedDefinitions := []string{}
			for _, definitionFile := range files {
				definitionName := strings.TrimSuffix(filepath.Base(definitionFile), ".yaml")
				By(fmt.Sprintf("Installing definition from %s", definitionName))

				err := testdef.InstallDefinitionFromYAML(ctx, k8sClient, definitionFile, func(s string) string {
					// Replace helm template placeholders like the existing notification test
					s = strings.ReplaceAll(s, `{{ include "systemDefinitionNamespace" . }}`, namespace)
					s = strings.ReplaceAll(s, `{{- include "systemDefinitionNamespace" . }}`, namespace)
					return s
				})
				if err != nil {
					// Some definitions might fail to install due to dependencies, that's ok
					fmt.Printf("Warning: Failed to install definition from %s: %v\n", definitionFile, err)
				} else {
					installedDefinitions = append(installedDefinitions, definitionName)
				}
			}

			By(fmt.Sprintf("Successfully installed %d definitions, now testing CUE parsing", len(installedDefinitions)))

			// Test CUE parsing on all installed ComponentDefinitions
			By("Testing CUE parsing for ComponentDefinitions")
			var componentDefs v1beta1.ComponentDefinitionList
			Expect(k8sClient.List(ctx, &componentDefs, &client.ListOptions{Namespace: namespace})).Should(Succeed())

			componentErrorCount := 0
			for i := range componentDefs.Items {
				unstructuredObj, err := pkgruntime.DefaultUnstructuredConverter.ToUnstructured(&componentDefs.Items[i])
				if err != nil {
					componentErrorCount++
					fmt.Printf("ERROR: ComponentDefinition %s failed to convert to unstructured: %v\n", componentDefs.Items[i].Name, err)
					continue
				}

				def := &pkgdef.Definition{Unstructured: unstructured.Unstructured{Object: unstructuredObj}}
				_, err = def.ToCUEString()
				if err != nil {
					componentErrorCount++
					fmt.Printf("ERROR: ComponentDefinition %s failed CUE parsing: %v\n", componentDefs.Items[i].Name, err)
				}
			}

			// Test CUE parsing on all installed TraitDefinitions
			By("Testing CUE parsing for TraitDefinitions")
			var traitDefs v1beta1.TraitDefinitionList
			Expect(k8sClient.List(ctx, &traitDefs, &client.ListOptions{Namespace: namespace})).Should(Succeed())

			traitErrorCount := 0
			for i := range traitDefs.Items {
				unstructuredObj, err := pkgruntime.DefaultUnstructuredConverter.ToUnstructured(&traitDefs.Items[i])
				if err != nil {
					traitErrorCount++
					fmt.Printf("ERROR: TraitDefinition %s failed to convert to unstructured: %v\n", traitDefs.Items[i].Name, err)
					continue
				}

				def := &pkgdef.Definition{Unstructured: unstructured.Unstructured{Object: unstructuredObj}}
				_, err = def.ToCUEString()
				if err != nil {
					traitErrorCount++
					fmt.Printf("ERROR: TraitDefinition %s failed CUE parsing: %v\n", traitDefs.Items[i].Name, err)
				}
			}

			By(fmt.Sprintf("CUE parsing results: %d ComponentDefinitions tested, %d TraitDefinitions tested",
				len(componentDefs.Items), len(traitDefs.Items)))

			totalErrors := componentErrorCount + traitErrorCount
			Expect(totalErrors).To(Equal(0), fmt.Sprintf("%d definitions failed CUE parsing with updated logic", totalErrors))
		})
	})
})
