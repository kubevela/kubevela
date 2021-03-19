package application

import (
	"context"
	"encoding/json"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"k8s.io/apimachinery/pkg/runtime"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/yaml"

	"github.com/oam-dev/kubevela/apis/core.oam.dev/v1alpha2"
	"github.com/oam-dev/kubevela/apis/standard.oam.dev/v1alpha1"
	"github.com/oam-dev/kubevela/pkg/oam"
	"github.com/oam-dev/kubevela/pkg/oam/util"
)

var _ = Describe("test generate revision ", func() {
	var appRevision1, appRevision2 v1alpha2.ApplicationRevision
	var app v1alpha2.Application
	cd := v1alpha2.ComponentDefinition{}
	webCompDef := v1alpha2.ComponentDefinition{}
	wd := v1alpha2.WorkloadDefinition{}
	td := v1alpha2.TraitDefinition{}
	sd := v1alpha2.ScopeDefinition{}

	BeforeEach(func() {
		ctx := context.Background()

		componentDefJson, _ := yaml.YAMLToJSON([]byte(componentDefYaml))
		Expect(json.Unmarshal(componentDefJson, &cd)).Should(BeNil())
		Expect(k8sClient.Create(ctx, cd.DeepCopy())).Should(SatisfyAny(BeNil(), &util.AlreadyExistMatcher{}))

		traitDefJson, _ := yaml.YAMLToJSON([]byte(TraitDefYaml))
		Expect(json.Unmarshal(traitDefJson, &td)).Should(BeNil())
		Expect(k8sClient.Create(ctx, td.DeepCopy())).Should(SatisfyAny(BeNil(), &util.AlreadyExistMatcher{}))

		scopeDefJson, _ := yaml.YAMLToJSON([]byte(scopeDefYaml))
		Expect(json.Unmarshal(scopeDefJson, &sd)).Should(BeNil())
		Expect(k8sClient.Create(ctx, sd.DeepCopy())).Should(SatisfyAny(BeNil(), &util.AlreadyExistMatcher{}))

		webserverCDJson, _ := yaml.YAMLToJSON([]byte(webComponentDefYaml))
		Expect(json.Unmarshal(webserverCDJson, &webCompDef)).Should(BeNil())
		Expect(k8sClient.Create(ctx, webCompDef.DeepCopy())).Should(SatisfyAny(BeNil(), &util.AlreadyExistMatcher{}))

		workloadDefJson, _ := yaml.YAMLToJSON([]byte(workloadDefYaml))
		Expect(json.Unmarshal(workloadDefJson, &wd)).Should(BeNil())
		Expect(k8sClient.Create(ctx, wd.DeepCopy())).Should(SatisfyAny(BeNil(), &util.AlreadyExistMatcher{}))

		app = v1alpha2.Application{
			TypeMeta: metav1.TypeMeta{
				Kind:       "Application",
				APIVersion: "core.oam.dev/v1alpha2",
			},
			Spec: v1alpha2.ApplicationSpec{
				Components: []v1alpha2.ApplicationComponent{
					{
						WorkloadType: "webservice",
						Name:         "express-server",
						Scopes:       map[string]string{"healthscopes.core.oam.dev": "myapp-default-health"},
						Settings: runtime.RawExtension{
							Raw: []byte(`{"image": "oamdev/testapp:v1", "cmd": ["node", "server.js"]}`),
						},
						Traits: []v1alpha2.ApplicationTrait{
							{
								Name: "route",
								Properties: runtime.RawExtension{
									Raw: []byte(`{"domain": "example.com", "http":{"/": 8080}}`),
								},
							},
						},
					},
				},
			},
		}

		appRevision1 = v1alpha2.ApplicationRevision{
			ObjectMeta: metav1.ObjectMeta{
				Name: "appRevision1",
			},
			Spec: v1alpha2.ApplicationRevisionSpec{
				ComponentDefinitions: make(map[string]v1alpha2.ComponentDefinition),
				WorkloadDefinitions:  make(map[string]v1alpha2.WorkloadDefinition),
				TraitDefinitions:     make(map[string]v1alpha2.TraitDefinition),
				ScopeDefinitions:     make(map[string]v1alpha2.ScopeDefinition),
			},
		}
		appRevision1.Spec.Application = app

		appRevision1.Spec.ComponentDefinitions[cd.Name] = cd

		appRevision1.Spec.ComponentDefinitions[webCompDef.Name] = webCompDef

		appRevision1.Spec.WorkloadDefinitions[wd.Name] = wd

		appRevision1.Spec.TraitDefinitions[td.Name] = td

		appRevision1.Spec.ScopeDefinitions[sd.Name] = sd

		appRevision2 = *appRevision1.DeepCopy()
		appRevision2.Name = "appRevision2"

	})

	It("Test same app revisions should produce same hash and equal", func() {
		appHash1, err := ComputeAppRevisionHash(&appRevision1)
		Expect(err).Should(Succeed())

		appHash2, err := ComputeAppRevisionHash(&appRevision2)
		Expect(err).Should(Succeed())

		Expect(appHash1).Should(Equal(appHash2))

		Expect(DeepEqualRevision(&appRevision1, &appRevision2)).Should(BeTrue())
	})

	It("Test app revisions with same spec should produce same hash and equal regardless of other fields", func() {
		// add an annotation to workload Definition
		wd.SetAnnotations(map[string]string{oam.AnnotationRollingComponent: "true"})
		appRevision2.Spec.WorkloadDefinitions[wd.Name] = wd
		// add status to td
		td.SetConditions(v1alpha1.NewPositiveCondition("Test"))
		appRevision2.Spec.TraitDefinitions[td.Name] = td
		// change the cd meta
		cd.ClusterName = "testCluster"
		appRevision2.Spec.ComponentDefinitions[cd.Name] = cd

		// that should not change the hashvalue
		appHash1, err := ComputeAppRevisionHash(&appRevision1)
		Expect(err).Should(Succeed())
		appHash2, err := ComputeAppRevisionHash(&appRevision2)
		Expect(err).Should(Succeed())
		Expect(appHash1).Should(Equal(appHash2))
		// and compare
		Expect(DeepEqualRevision(&appRevision1, &appRevision2)).Should(BeTrue())
	})

	It("Test app revisions with different spec should produce different hash and not equal", func() {
		// change td spec
		td.Spec.AppliesToWorkloads = append(td.Spec.AppliesToWorkloads, "allWorkload")
		appRevision2.Spec.TraitDefinitions[td.Name] = td

		// that should not change the hashvalue
		appHash1, err := ComputeAppRevisionHash(&appRevision1)
		Expect(err).Should(Succeed())
		appHash2, err := ComputeAppRevisionHash(&appRevision2)
		Expect(err).Should(Succeed())
		Expect(appHash1).ShouldNot(Equal(appHash2))
		// and compare
		Expect(DeepEqualRevision(&appRevision1, &appRevision2)).ShouldNot(BeTrue())

	})

})
