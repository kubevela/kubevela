package controllers_test

import (
	"context"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"time"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"

	"github.com/ghodss/yaml"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"

	"github.com/crossplane/oam-kubernetes-runtime/apis/core/v1alpha2"
	"github.com/crossplane/oam-kubernetes-runtime/pkg/oam/util"

	appsv1 "k8s.io/api/apps/v1"
	v1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
)

var _ = Describe("Versioning mechanism of components", func() {
	ctx := context.Background()
	namespace := "component-versioning-test"
	ns := corev1.Namespace{
		ObjectMeta: metav1.ObjectMeta{
			Name: namespace,
		},
	}
	componentName := "example-component"

	// to identify different revisions of components
	imageV1 := "wordpress:4.6.1-apache"
	imageV2 := "wordpress:4.6.2-apache"

	var cwV1, cwV2 v1alpha2.ContainerizedWorkload
	var componentV1 v1alpha2.Component
	var appConfig v1alpha2.ApplicationConfiguration

	BeforeEach(func() {
		cwV1 = v1alpha2.ContainerizedWorkload{
			TypeMeta: metav1.TypeMeta{
				Kind:       "ContainerizedWorkload",
				APIVersion: "core.oam.dev/v1alpha2",
			},
			ObjectMeta: metav1.ObjectMeta{
				Namespace: namespace,
			},
			Spec: v1alpha2.ContainerizedWorkloadSpec{
				Containers: []v1alpha2.Container{
					{
						Name:  "wordpress",
						Image: imageV1,
						Ports: []v1alpha2.ContainerPort{
							{
								Name: "wordpress",
								Port: 80,
							},
						},
					},
				},
			},
		}

		cwV2 = v1alpha2.ContainerizedWorkload{
			TypeMeta: metav1.TypeMeta{
				Kind:       "ContainerizedWorkload",
				APIVersion: "core.oam.dev/v1alpha2",
			},
			ObjectMeta: metav1.ObjectMeta{
				Namespace: namespace,
			},
			Spec: v1alpha2.ContainerizedWorkloadSpec{
				Containers: []v1alpha2.Container{
					{
						Name:  "wordpress",
						Image: imageV2,
						Ports: []v1alpha2.ContainerPort{
							{
								Name: "wordpress",
								Port: 80,
							},
						},
					},
				},
			},
		}

		componentV1 = v1alpha2.Component{
			TypeMeta: metav1.TypeMeta{
				APIVersion: "core.oam.dev/v1alpha2",
				Kind:       "Component",
			},
			ObjectMeta: metav1.ObjectMeta{
				Name:      componentName,
				Namespace: namespace,
			},
			Spec: v1alpha2.ComponentSpec{
				Workload: runtime.RawExtension{
					Object: &cwV1,
				},
			},
		}

		appConfig = v1alpha2.ApplicationConfiguration{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "example-appconfig",
				Namespace: namespace,
			},
		}

		logf.Log.Info("Start to run a test, clean up previous resources")
		ns = corev1.Namespace{
			ObjectMeta: metav1.ObjectMeta{
				Name: namespace,
			},
		}
		// delete the namespace with all its resources
		Expect(k8sClient.Delete(ctx, &ns, client.PropagationPolicy(metav1.DeletePropagationForeground))).
			Should(SatisfyAny(BeNil(), &util.NotFoundMatcher{}))
		logf.Log.Info("make sure all the resources are removed")
		objectKey := client.ObjectKey{
			Name: namespace,
		}
		res := &corev1.Namespace{}
		Eventually(
			// gomega has a bug that can't take nil as the actual input, so has to make it a func
			func() error {
				return k8sClient.Get(ctx, objectKey, res)
			},
			time.Second*120, time.Millisecond*500).Should(&util.NotFoundMatcher{})
		Eventually(
			func() error {
				return k8sClient.Create(ctx, &ns)
			},
			time.Second*3, time.Millisecond*300).Should(SatisfyAny(BeNil(), &util.AlreadyExistMatcher{}))
	})

	AfterEach(func() {
		logf.Log.Info("Clean up resources")
		// delete the namespace with all its resources
		Expect(k8sClient.Delete(ctx, &ns, client.PropagationPolicy(metav1.DeletePropagationForeground))).Should(BeNil())
	})

	When("create or update a component", func() {
		It("should create corresponding ControllerRevision", func() {
			By("Create Component v1")
			Expect(k8sClient.Create(ctx, &componentV1)).Should(Succeed())

			cmpV1 := &v1alpha2.Component{}
			By("Get Component v1")
			Expect(k8sClient.Get(ctx, client.ObjectKey{Namespace: namespace, Name: componentName}, cmpV1)).Should(Succeed())

			By("Get Component latest status after ControllerRevision created")
			Eventually(
				func() *v1alpha2.Revision {
					k8sClient.Get(ctx, client.ObjectKey{Namespace: namespace, Name: componentName}, cmpV1)
					return cmpV1.Status.LatestRevision
				},
				time.Second*15, time.Millisecond*500).ShouldNot(BeNil())

			revisionNameV1 := cmpV1.Status.LatestRevision.Name
			By("Get corresponding ControllerRevision of Component v1")
			cr := &appsv1.ControllerRevision{}
			Expect(k8sClient.Get(ctx, client.ObjectKey{Namespace: namespace, Name: revisionNameV1}, cr)).Should(Succeed())
			By("Check revision seq number")
			Expect(cr.Revision).Should(Equal(int64(1)))

			cwV2raw, _ := json.Marshal(cwV2)
			cmpV1.Spec.Workload.Raw = cwV2raw
			By("Update Component into revision v2")
			Expect(k8sClient.Update(ctx, cmpV1)).Should(Succeed())

			cmpV2 := &v1alpha2.Component{}
			By("Get Component v2")
			Expect(k8sClient.Get(ctx, client.ObjectKey{Namespace: namespace, Name: componentName}, cmpV2)).Should(Succeed())

			By("Get Component latest status after ControllerRevision created")
			Eventually(
				func() string {
					k8sClient.Get(ctx, client.ObjectKey{Namespace: namespace, Name: componentName}, cmpV2)
					return cmpV2.Status.LatestRevision.Name
				},
				time.Second*15, time.Millisecond*500).ShouldNot(Equal(revisionNameV1))

			revisionNameV2 := cmpV2.Status.LatestRevision.Name
			crV2 := &appsv1.ControllerRevision{}
			By("Get corresponding ControllerRevision of Component v2")
			Expect(k8sClient.Get(ctx, client.ObjectKey{Namespace: namespace, Name: revisionNameV2}, crV2)).Should(Succeed())
			By("Check revision seq number")
			Expect(crV2.Revision).Should(Equal(int64(2)))

		})
	})

	When("Components have revisionName in AppConfig", func() {
		It("should NOT create NOR update workloads, when update components", func() {
			By("Create Component v1")
			Expect(k8sClient.Create(ctx, &componentV1)).Should(Succeed())

			cmpV1 := &v1alpha2.Component{}
			By("Get Component v1")
			Expect(k8sClient.Get(ctx, client.ObjectKey{Namespace: namespace, Name: componentName}, cmpV1)).Should(Succeed())

			By("Get Component latest status after ControllerRevision created")
			Eventually(
				func() *v1alpha2.Revision {
					k8sClient.Get(ctx, client.ObjectKey{Namespace: namespace, Name: componentName}, cmpV1)
					return cmpV1.Status.LatestRevision
				},
				time.Second*15, time.Millisecond*500).ShouldNot(BeNil())

			revisionNameV1 := cmpV1.Status.LatestRevision.Name

			appConfigWithRevisionName := appConfig
			appConfigWithRevisionName.Spec.Components = append(appConfigWithRevisionName.Spec.Components,
				v1alpha2.ApplicationConfigurationComponent{
					RevisionName: revisionNameV1,
				})
			By("Apply appConfig")
			Expect(k8sClient.Create(ctx, &appConfigWithRevisionName)).Should(Succeed())

			cwWlV1 := v1alpha2.ContainerizedWorkload{}
			By("Check ContainerizedWorkload workload's image field is v1")
			Eventually(
				func() error {
					return k8sClient.Get(ctx, client.ObjectKey{Namespace: namespace, Name: componentName}, &cwWlV1)
				},
				time.Second*15, time.Millisecond*500).Should(BeNil())
			Expect(cwWlV1.Spec.Containers[0].Image).Should(Equal(imageV1))

			cwV2raw, _ := json.Marshal(cwV2)
			cmpV1.Spec.Workload.Raw = cwV2raw
			By("Update Component to revision v2")
			Expect(k8sClient.Update(ctx, cmpV1)).Should(Succeed())

			By("Check ContainerizedWorkload workload's image field is still v1")
			Expect(k8sClient.Get(ctx, client.ObjectKey{Namespace: namespace, Name: componentName}, &cwWlV1)).Should(Succeed())
			Expect(cwWlV1.Spec.Containers[0].Image).Should(Equal(imageV1))
		})
	})

	When("Components have componentName", func() {
		It("should update workloads with new revision of components, when update components", func() {
			By("Create Component v1")
			Expect(k8sClient.Create(ctx, &componentV1)).Should(Succeed())

			cmpV1 := &v1alpha2.Component{}
			By("Get Component latest status after ControllerRevision created")
			Eventually(
				func() *v1alpha2.Revision {
					k8sClient.Get(ctx, client.ObjectKey{Namespace: namespace, Name: componentName}, cmpV1)
					return cmpV1.Status.LatestRevision
				},
				time.Second*30, time.Millisecond*500).ShouldNot(BeNil())

			revisionNameV1 := cmpV1.Status.LatestRevision.Name

			appConfigWithRevisionName := appConfig
			appConfigWithRevisionName.Spec.Components = append(appConfigWithRevisionName.Spec.Components,
				v1alpha2.ApplicationConfigurationComponent{
					ComponentName: componentName,
				})
			By("Apply appConfig")
			Expect(k8sClient.Create(ctx, &appConfigWithRevisionName)).Should(Succeed())

			cwWlV1 := &v1alpha2.ContainerizedWorkload{}
			By("Check ContainerizedWorkload workload's image field is v1")
			Eventually(
				func() error {
					return k8sClient.Get(ctx, client.ObjectKey{Namespace: namespace, Name: componentName}, cwWlV1)
				},
				time.Second*15, time.Millisecond*500).Should(BeNil())
			Expect(cwWlV1.Spec.Containers[0].Image).Should(Equal(imageV1))

			cwV2raw, _ := json.Marshal(cwV2)
			cmpV1.Spec.Workload.Raw = cwV2raw
			By("Update Component to revision v2")
			Expect(k8sClient.Update(ctx, cmpV1)).Should(Succeed())

			By("Check Component has been changed to revision v2")
			By("Get latest Component revision: revision 2")
			cmpV2 := &v1alpha2.Component{}
			Eventually(
				func() string {
					k8sClient.Get(ctx, client.ObjectKey{Namespace: namespace, Name: componentName}, cmpV2)
					return cmpV2.Status.LatestRevision.Name
				},
				time.Second*30, time.Millisecond*500).ShouldNot(Equal(revisionNameV1))

			By("Check ContainerizedWorkload workload's image field has been changed to v2")
			cwWlV2 := &v1alpha2.ContainerizedWorkload{}
			Eventually(func() string {
				k8sClient.Get(ctx, client.ObjectKey{Namespace: namespace, Name: componentName}, cwWlV2)
				return cwWlV2.Spec.Containers[0].Image
			}, time.Second*60, time.Microsecond*500).Should(Equal(imageV2))
		})
	})

	When("Components have componentName and have revision-enabled trait", func() {
		It("should create workloads with name of revision and keep the old revision", func() {

			By("Create trait definition")
			var td v1alpha2.TraitDefinition
			Expect(readYaml("testdata/revision/trait-def.yaml", &td)).Should(BeNil())

			var gtd v1alpha2.TraitDefinition
			if err := k8sClient.Get(ctx, client.ObjectKey{Name: td.Name, Namespace: td.Namespace}, &gtd); err != nil {
				Expect(k8sClient.Create(ctx, &td)).Should(Succeed())
			} else {
				td.ResourceVersion = gtd.ResourceVersion
				Expect(k8sClient.Update(ctx, &td)).Should(Succeed())
			}

			By("Create Component v1")
			var comp1 v1alpha2.Component
			Expect(readYaml("testdata/revision/comp-v1.yaml", &comp1)).Should(BeNil())
			Expect(k8sClient.Create(ctx, &comp1)).Should(Succeed())

			By("Create AppConfig with component")
			var appconfig v1alpha2.ApplicationConfiguration
			Expect(readYaml("testdata/revision/app.yaml", &appconfig)).Should(BeNil())
			Expect(k8sClient.Create(ctx, &appconfig)).Should(Succeed())

			By("Get Component latest status after ControllerRevision created")
			Eventually(
				func() *v1alpha2.Revision {
					k8sClient.Get(ctx, client.ObjectKey{Namespace: namespace, Name: componentName}, &comp1)
					return comp1.Status.LatestRevision
				},
				time.Second*300, time.Millisecond*500).ShouldNot(BeNil())

			revisionNameV1 := comp1.Status.LatestRevision.Name

			By("Workload created with revisionName v1")
			var w1 unstructured.Unstructured
			Eventually(
				func() error {
					w1.SetAPIVersion("example.com/v1")
					w1.SetKind("Bar")
					return k8sClient.Get(ctx, client.ObjectKey{Namespace: namespace, Name: revisionNameV1}, &w1)
				},
				time.Second*60, time.Millisecond*500).Should(BeNil())
			k1, _, _ := unstructured.NestedString(w1.Object, "spec", "key")
			Expect(k1).Should(BeEquivalentTo("v1"), fmt.Sprintf("%v", w1.Object))

			By("Create Component v2")
			var comp2 v1alpha2.Component
			Expect(readYaml("testdata/revision/comp-v2.yaml", &comp2)).Should(BeNil())
			comp2.ResourceVersion = comp1.ResourceVersion
			Expect(k8sClient.Update(ctx, &comp2)).Should(Succeed())

			By("Get Component latest status after ControllerRevision created")
			Eventually(
				func() *v1alpha2.Revision {
					k8sClient.Get(ctx, client.ObjectKey{Namespace: namespace, Name: componentName}, &comp2)
					if comp2.Status.LatestRevision != nil && comp2.Status.LatestRevision.Revision > 1 {
						return comp2.Status.LatestRevision
					}
					return nil
				},
				time.Second*120, time.Millisecond*500).ShouldNot(BeNil())

			revisionNameV2 := comp2.Status.LatestRevision.Name

			By("Workload exist with revisionName v2")
			var w2 unstructured.Unstructured
			Eventually(
				func() error {
					w2.SetAPIVersion("example.com/v1")
					w2.SetKind("Bar")
					return k8sClient.Get(ctx, client.ObjectKey{Namespace: namespace, Name: revisionNameV2}, &w2)
				},
				time.Second*60, time.Millisecond*500).Should(BeNil())
			k2, _, _ := unstructured.NestedString(w2.Object, "spec", "key")
			Expect(k2).Should(BeEquivalentTo("v2"), fmt.Sprintf("%v", w2.Object))

			By("Check AppConfig status")
			Eventually(
				func() string {
					err := k8sClient.Get(ctx, client.ObjectKey{Namespace: namespace, Name: appconfig.Name}, &appconfig)
					if err != nil {
						return ""
					}
					if len(appconfig.Status.Workloads) == 0 {
						return ""
					}
					return appconfig.Status.Workloads[0].ComponentRevisionName
				},
				time.Second*60, time.Millisecond*500).Should(BeEquivalentTo(revisionNameV2))

			Expect(len(appconfig.Status.Workloads)).Should(BeEquivalentTo(1))

			Expect(len(appconfig.Status.HistoryWorkloads)).Should(BeEquivalentTo(1))
			Expect(appconfig.Status.HistoryWorkloads[0].Revision).Should(BeEquivalentTo(revisionNameV1))

			// Clean
			k8sClient.Delete(ctx, &appconfig)
			k8sClient.Delete(ctx, &comp1)
			k8sClient.Delete(ctx, &comp2)
		})
	})

	When("Components have componentName and without revision-enabled trait", func() {
		It("should create workloads with name of component and replace the old revision", func() {

			By("Create trait definition")
			var td v1alpha2.TraitDefinition
			Expect(readYaml("testdata/revision/trait-def-no-revision.yaml", &td)).Should(BeNil())
			var gtd v1alpha2.TraitDefinition
			if err := k8sClient.Get(ctx, client.ObjectKey{Name: td.Name, Namespace: td.Namespace}, &gtd); err != nil {
				Expect(k8sClient.Create(ctx, &td)).Should(Succeed())
			} else {
				td.ResourceVersion = gtd.ResourceVersion
				Expect(k8sClient.Update(ctx, &td)).Should(Succeed())
			}

			By("Create Component v1")
			var comp1 v1alpha2.Component
			Expect(readYaml("testdata/revision/comp-v1.yaml", &comp1)).Should(BeNil())
			Expect(k8sClient.Create(ctx, &comp1)).Should(Succeed())

			By("Create AppConfig with component")
			var appconfig v1alpha2.ApplicationConfiguration
			Expect(readYaml("testdata/revision/app.yaml", &appconfig)).Should(BeNil())
			Expect(k8sClient.Create(ctx, &appconfig)).Should(Succeed())

			By("Workload created with component name")
			var w1 unstructured.Unstructured
			Eventually(
				func() error {
					w1.SetAPIVersion("example.com/v1")
					w1.SetKind("Bar")
					return k8sClient.Get(ctx, client.ObjectKey{Namespace: namespace, Name: componentName}, &w1)
				},
				time.Second*60, time.Millisecond*500).Should(BeNil())

			k1, _, _ := unstructured.NestedString(w1.Object, "spec", "key")
			Expect(k1).Should(BeEquivalentTo("v1"), fmt.Sprintf("%v", w1.Object))

			By("Create Component v2")
			var comp2 v1alpha2.Component
			Expect(readYaml("testdata/revision/comp-v2.yaml", &comp2)).Should(BeNil())
			k8sClient.Get(ctx, client.ObjectKey{Namespace: namespace, Name: componentName}, &comp1)
			comp2.ResourceVersion = comp1.ResourceVersion
			Expect(k8sClient.Update(ctx, &comp2)).Should(Succeed())

			By("Workload exist with revisionName v2")
			var w2 unstructured.Unstructured
			Eventually(
				func() string {
					w2.SetAPIVersion("example.com/v1")
					w2.SetKind("Bar")
					err := k8sClient.Get(ctx, client.ObjectKey{Namespace: namespace, Name: componentName}, &w2)
					if err != nil {
						return ""
					}
					k2, _, _ := unstructured.NestedString(w2.Object, "spec", "key")
					return k2
				},
				time.Second*120, time.Millisecond*500).Should(BeEquivalentTo("v2"))

			By("Check AppConfig status")
			Eventually(
				func() error {
					return k8sClient.Get(ctx, client.ObjectKey{Namespace: namespace, Name: appconfig.Name}, &appconfig)
				},
				time.Second*15, time.Millisecond*500).Should(BeNil())

			Expect(len(appconfig.Status.Workloads)).Should(BeEquivalentTo(1))

			// Clean
			k8sClient.Delete(ctx, &appconfig)
			k8sClient.Delete(ctx, &comp1)
			k8sClient.Delete(ctx, &comp2)
		})
	})
})

func readYaml(path string, object runtime.Object) error {
	data, err := ioutil.ReadFile(path)
	if err != nil {
		return err
	}
	return yaml.Unmarshal(data, object)
}

var _ = Describe("Component revision", func() {
	ctx := context.Background()
	apiVersion := "core.oam.dev/v1alpha2"
	namespace := "default"
	componentName := "revision-component"
	appConfigName := "revision-app"
	workload := v1.Deployment{
		TypeMeta:   metav1.TypeMeta{APIVersion: "apps/v1", Kind: "Deployment"},
		ObjectMeta: metav1.ObjectMeta{Namespace: namespace},
		Spec: v1.DeploymentSpec{
			Selector: &metav1.LabelSelector{
				MatchLabels: map[string]string{
					"app": "nginx",
				},
			},
			Template: corev1.PodTemplateSpec{
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{
						{Name: "nginx",
							Image: "nginx:1.9.4"},
					},
				},
				ObjectMeta: metav1.ObjectMeta{Labels: map[string]string{"app": "nginx"}},
			},
		},
	}
	component := v1alpha2.Component{
		TypeMeta: metav1.TypeMeta{
			APIVersion: apiVersion,
			Kind:       "Component",
		},
		ObjectMeta: metav1.ObjectMeta{Name: componentName, Namespace: namespace},
		Spec: v1alpha2.ComponentSpec{
			Workload: runtime.RawExtension{Object: workload.DeepCopyObject()},
		},
	}

	TraitDefinition := v1alpha2.TraitDefinition{
		TypeMeta: metav1.TypeMeta{
			APIVersion: apiVersion,
			Kind:       "TraitDefinition",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      "manualscalertraits2.core.oam.dev",
			Namespace: namespace,
		},
		Spec: v1alpha2.TraitDefinitionSpec{
			RevisionEnabled: true,
			Reference: v1alpha2.DefinitionReference{
				Name: "manualscalertraits.core.oam.dev",
			},
			WorkloadRefPath: "spec.workloadRef",
		},
	}

	appConfig := v1alpha2.ApplicationConfiguration{
		TypeMeta: metav1.TypeMeta{
			APIVersion: apiVersion,
			Kind:       "ApplicationConfiguration",
		},
		ObjectMeta: metav1.ObjectMeta{Name: appConfigName, Namespace: namespace},
		Spec: v1alpha2.ApplicationConfigurationSpec{
			Components: []v1alpha2.ApplicationConfigurationComponent{{
				ComponentName: componentName},
			},
		},
	}

	workloadObjKey := client.ObjectKey{Name: componentName, Namespace: namespace}
	appConfigObjKey := client.ObjectKey{Name: appConfigName, Namespace: namespace}

	trait := v1alpha2.ManualScalerTrait{
		TypeMeta: metav1.TypeMeta{
			APIVersion: apiVersion,
			Kind:       "ManualScalerTrait",
		},
		ObjectMeta: metav1.ObjectMeta{Name: appConfigName, Namespace: namespace},
		Spec: v1alpha2.ManualScalerTraitSpec{
			ReplicaCount: 2,
		},
	}

	Context("Attach a revision-enable trait the first time, workload should not be recreated", func() {
		It("should create Component and ApplicationConfiguration", func() {
			By("submit ApplicationConfiguration")
			Expect(k8sClient.Create(ctx, &component)).Should(Succeed())
			Expect(k8sClient.Create(ctx, &appConfig)).Should(Succeed())

			By("check workload")
			var deploy v1.Deployment
			Eventually(
				func() error {
					return k8sClient.Get(ctx, workloadObjKey, &deploy)
				},
				time.Second*15, time.Millisecond*500).Should(BeNil())

			By("apply new ApplicationConfiguration with a revision enabled trait")
			Expect(k8sClient.Create(ctx, &TraitDefinition)).Should(Succeed())
			Expect(k8sClient.Get(ctx, appConfigObjKey, &appConfig)).Should(Succeed())
			appConfig.Spec.Components[0].Traits = []v1alpha2.ComponentTrait{{Trait: runtime.RawExtension{Object: trait.DeepCopyObject()}}}
			Expect(k8sClient.Update(ctx, &appConfig)).Should(Succeed())

			By("check current workload exists")
			time.Sleep(3 * time.Second)
			var currentDeploy v1.Deployment
			Expect(k8sClient.Get(ctx, workloadObjKey, &currentDeploy)).Should(BeNil())

			By("check version 1 workload doesn't exist")
			var v1Deploy v1.Deployment
			workloadObjKey := client.ObjectKey{Name: componentName + "-v1", Namespace: namespace}
			Expect(k8sClient.Get(ctx, workloadObjKey, &v1Deploy)).Should(SatisfyAny(&util.NotFoundMatcher{}))
		})
	})

	AfterEach(func() {
		k8sClient.Delete(ctx, &appConfig)
		k8sClient.Delete(ctx, &component)
	})
})
