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

package addon

import (
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"sigs.k8s.io/yaml"

	"github.com/oam-dev/kubevela/apis/core.oam.dev/v1beta1"
	velatypes "github.com/oam-dev/kubevela/apis/types"
	"github.com/oam-dev/kubevela/pkg/oam"
	"github.com/oam-dev/kubevela/pkg/oam/util"
)

var _ = Describe("Test definition check", func() {
	var compDef v1beta1.ComponentDefinition
	var traitDef v1beta1.TraitDefinition
	var wfStepDef v1beta1.WorkflowStepDefinition

	BeforeEach(func() {
		compDef = v1beta1.ComponentDefinition{}
		traitDef = v1beta1.TraitDefinition{}
		wfStepDef = v1beta1.WorkflowStepDefinition{}

		Expect(yaml.Unmarshal([]byte(compDefYaml), &compDef)).Should(BeNil())
		Expect(k8sClient.Create(ctx, &compDef)).Should(SatisfyAny(BeNil(), util.AlreadyExistMatcher{}))

		Expect(yaml.Unmarshal([]byte(traitDefYaml), &traitDef)).Should(BeNil())
		Expect(k8sClient.Create(ctx, &traitDef)).Should(SatisfyAny(BeNil(), util.AlreadyExistMatcher{}))

		Expect(yaml.Unmarshal([]byte(wfStepDefYaml), &wfStepDef)).Should(BeNil())
		Expect(k8sClient.Create(ctx, &wfStepDef)).Should(SatisfyAny(BeNil(), util.AlreadyExistMatcher{}))
	})

	It("Test pass def to app annotation", func() {
		c := v1beta1.ComponentDefinition{TypeMeta: metav1.TypeMeta{APIVersion: "core.oam.dev/v1beta1", Kind: "ComponentDefinition"}}
		c.SetName("my-comp")

		t := v1beta1.TraitDefinition{TypeMeta: metav1.TypeMeta{APIVersion: "core.oam.dev/v1beta1", Kind: "TraitDefinition"}}
		t.SetName("my-trait")

		w := v1beta1.WorkflowStepDefinition{TypeMeta: metav1.TypeMeta{APIVersion: "core.oam.dev/v1beta1", Kind: "WorkflowStepDefinition"}}
		w.SetName("my-wfstep")

		var defs []*unstructured.Unstructured
		cDef, err := util.Object2Unstructured(c)
		Expect(err).Should(BeNil())
		defs = append(defs, cDef)
		tDef, err := util.Object2Unstructured(t)
		defs = append(defs, tDef)
		Expect(err).Should(BeNil())
		wDef, err := util.Object2Unstructured(w)
		Expect(err).Should(BeNil())
		defs = append(defs, wDef)

		addonApp := v1beta1.Application{ObjectMeta: metav1.ObjectMeta{Name: "addon-app", Namespace: velatypes.DefaultKubeVelaNS}}
		err = passDefInAppAnnotation(defs, &addonApp)
		Expect(err).Should(BeNil())

		anno := addonApp.GetAnnotations()
		Expect(len(anno)).Should(BeEquivalentTo(3))
		Expect(anno[compDefAnnotation]).Should(BeEquivalentTo(`["my-comp"]`))
		Expect(anno[traitDefAnnotation]).Should(BeEquivalentTo(`["my-trait"]`))
		Expect(anno[workflowStepDefAnnotation]).Should(BeEquivalentTo(`["my-wfstep"]`))
	})

	It("Test checkAddonHasBeenUsed func", func() {
		addonApp := v1beta1.Application{}
		Expect(yaml.Unmarshal([]byte(addonAppYaml), &addonApp)).Should(BeNil())

		app1 := v1beta1.Application{}
		Expect(yaml.Unmarshal([]byte(testApp1Yaml), &app1)).Should(BeNil())
		Expect(k8sClient.Create(ctx, &app1)).Should(BeNil())

		app2 := v1beta1.Application{}
		Expect(yaml.Unmarshal([]byte(testApp2Yaml), &app2)).Should(BeNil())
		Expect(k8sClient.Create(ctx, &app2)).Should(BeNil())

		Expect(k8sClient.Create(ctx, &v1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "test-ns"}}))
		app3 := v1beta1.Application{}
		Expect(yaml.Unmarshal([]byte(testApp3Yaml), &app3)).Should(BeNil())
		Expect(k8sClient.Create(ctx, &app3)).Should(BeNil())

		usedApps, err := checkAddonHasBeenUsed(ctx, k8sClient, "my-addon", addonApp, cfg)
		Expect(err).Should(BeNil())
		Expect(len(usedApps)).Should(BeEquivalentTo(3))
	})

	It("check fetch lagacy addon definitions", func() {
		res := make(map[string]bool)

		server := httptest.NewServer(ossHandler)
		defer server.Close()

		url := server.URL
		cmYaml := strings.ReplaceAll(registryCmYaml, "TEST_SERVER_URL", url)
		cm := v1.ConfigMap{}
		Expect(yaml.Unmarshal([]byte(cmYaml), &cm)).Should(BeNil())
		Expect(k8sClient.Create(ctx, &cm)).Should(BeNil())

		disableTestAddonApp := v1beta1.Application{}
		Expect(yaml.Unmarshal([]byte(addonDisableTestAppYaml), &disableTestAddonApp)).Should(BeNil())
		Expect(findLegacyAddonDefs(ctx, k8sClient, "test-disable-addon", disableTestAddonApp.GetLabels()[oam.LabelAddonRegistry], cfg, res)).Should(BeNil())
		Expect(len(res)).Should(BeEquivalentTo(2))
	})
})

func TestMerge2Map(t *testing.T) {
	res := make(map[string]bool)
	err := merge2DefMap(compDefAnnotation, `["my-comp1","my-comp2"]`, res)
	assert.NoError(t, err)
	err = merge2DefMap(traitDefAnnotation, `["my-trait1","my-trait2"]`, res)
	assert.NoError(t, err)
	err = merge2DefMap(workflowStepDefAnnotation, `["my-wfStep1","my-wfStep2"]`, res)
	assert.NoError(t, err)
	assert.Equal(t, 6, len(res))
}

const (
	compDefYaml = `
apiVersion: core.oam.dev/v1beta1
kind: ComponentDefinition
metadata:
   name: my-comp
   namespace: vela-system
`
	traitDefYaml = `
apiVersion: core.oam.dev/v1beta1
kind: TraitDefinition
metadata:
   name: my-trait
   namespace: vela-system
`
	wfStepDefYaml = `
apiVersion: core.oam.dev/v1beta1
kind: WorkflowStepDefinition
metadata:
   name: my-wfstep
   namespace: vela-system
`
)

const (
	addonAppYaml = `
apiVersion: core.oam.dev/v1beta1
kind: Application
metadata:
  labels:
    addons.oam.dev/name: myaddon
    addons.oam.dev/registry: KubeVela
  annotations:
    addon.oam.dev/componentDefinitions: '["my-comp"]'
    addon.oam.dev/traitDefinitions: '["my-trait"]'
    addon.oam.dev/workflowStepDefinitions: '["my-wfstep"]'
  name: addon-myaddon
  namespace: vela-system
spec:
`
	testApp1Yaml = `
apiVersion: core.oam.dev/v1beta1
kind: Application
metadata:
  labels:
  name: app-1
  namespace: default
spec:
  components:
     - name: comp1
       type: my-comp
       traits:
       - type: my-trait
`
	testApp2Yaml = `
apiVersion: core.oam.dev/v1beta1
kind: Application
metadata:
  labels:
  name: app-2
  namespace: default
spec:
  components:
     - name: comp2
       type: webservice
       traits:
       - type: my-trait
`
	testApp3Yaml = `
apiVersion: core.oam.dev/v1beta1
kind: Application
metadata:
  name: app-3
  namespace: test-ns
spec:
  components:
    - name: podinfo
      type: webservice

  workflow:      
    steps:
    - type: my-wfstep
      name: deploy
`
	registryCmYaml = `
apiVersion: v1
data:
  registries: '{ "KubeVela":{ "name": "KubeVela", "oss": { "end_point": "TEST_SERVER_URL",
    "bucket": "", "path": "" } } }'
kind: ConfigMap
metadata:
  name: vela-addon-registry
  namespace: vela-system
`
	addonDisableTestAppYaml = `
apiVersion: core.oam.dev/v1beta1
kind: Application
metadata:
  name: addon-test-disable-addon
  namespace: vela-system
  labels:
    addons.oam.dev/name: test-disable-addon
    addons.oam.dev/registry: KubeVela
spec:
  components:
    - name: podinfo
      type: webservice
`
)
