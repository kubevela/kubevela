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
	"fmt"
	"net/http/httptest"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"github.com/stretchr/testify/assert"
	"helm.sh/helm/v3/pkg/chartutil"
	v1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"sigs.k8s.io/yaml"

	"github.com/oam-dev/kubevela/apis/core.oam.dev/common"
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
		Expect(anno[compDefAnnotation]).Should(BeEquivalentTo("my-comp"))
		Expect(anno[traitDefAnnotation]).Should(BeEquivalentTo("my-trait"))
		Expect(anno[workflowStepDefAnnotation]).Should(BeEquivalentTo("my-wfstep"))
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

		app4 := v1beta1.Application{}
		Expect(yaml.Unmarshal([]byte(testApp4Yaml), &app4)).Should(BeNil())
		Expect(k8sClient.Create(ctx, &app4)).Should(BeNil())

		usedApps, err := checkAddonHasBeenUsed(ctx, k8sClient, "my-addon", addonApp, cfg)
		Expect(err).Should(BeNil())
		Expect(len(usedApps)).Should(BeEquivalentTo(4))
	})

	It("check fetch lagacy addon definitions", func() {
		res := make(map[string]bool)

		server := httptest.NewServer(ossHandler)
		defer server.Close()

		url := server.URL
		cmYaml := strings.ReplaceAll(registryCmYaml, "TEST_SERVER_URL", url)
		cm := v1.ConfigMap{}
		Expect(yaml.Unmarshal([]byte(cmYaml), &cm)).Should(BeNil())
		err := k8sClient.Create(ctx, &cm)
		if apierrors.IsAlreadyExists(err) {
			Expect(k8sClient.Update(ctx, &cm)).To(Succeed())
		} else {
			Expect(err).To(Succeed())
		}

		disableTestAddonApp := v1beta1.Application{}
		Expect(yaml.Unmarshal([]byte(addonDisableTestAppYaml), &disableTestAddonApp)).Should(BeNil())
		Expect(findLegacyAddonDefs(ctx, k8sClient, "test-disable-addon", disableTestAddonApp.GetLabels()[oam.LabelAddonRegistry], cfg, res)).Should(BeNil())
		Expect(len(res)).Should(BeEquivalentTo(2))
	})
})

func TestMerge2Map(t *testing.T) {
	res := make(map[string]bool)
	merge2DefMap(compDefAnnotation, "my-comp1,my-comp2", res)
	merge2DefMap(traitDefAnnotation, "my-trait1,my-trait2", res)
	merge2DefMap(workflowStepDefAnnotation, "my-wfStep1,my-wfStep2", res)
	assert.Equal(t, 6, len(res))
}

func TestUsingAddonInfo(t *testing.T) {
	apps := []v1beta1.Application{
		{ObjectMeta: metav1.ObjectMeta{Namespace: "namespace-1", Name: "app-1"}},
		{ObjectMeta: metav1.ObjectMeta{Namespace: "namespace-2", Name: "app-2"}},
		{ObjectMeta: metav1.ObjectMeta{Namespace: "namespace-1", Name: "app-3"}},
		{ObjectMeta: metav1.ObjectMeta{Namespace: "namespace-3", Name: "app-3"}},
	}
	res := appsDependsOnAddonErrInfo(apps)
	assert.Contains(t, res, "and other 1 more applications. Please delete all of them before removing.")

	apps = []v1beta1.Application{
		{ObjectMeta: metav1.ObjectMeta{Namespace: "namespace-1", Name: "app-1"}},
		{ObjectMeta: metav1.ObjectMeta{Namespace: "namespace-2", Name: "app-2"}},
		{ObjectMeta: metav1.ObjectMeta{Namespace: "namespace-1", Name: "app-3"}},
	}
	res = appsDependsOnAddonErrInfo(apps)
	assert.Contains(t, res, "Please delete all of them before removing.")

	apps = []v1beta1.Application{
		{ObjectMeta: metav1.ObjectMeta{Namespace: "namespace-1", Name: "app-1"}},
	}
	res = appsDependsOnAddonErrInfo(apps)
	assert.Contains(t, res, "this addon is being used by: namespace-1/app-1 applications. Please delete all of them before removing.")

	apps = []v1beta1.Application{
		{ObjectMeta: metav1.ObjectMeta{Namespace: "namespace-1", Name: "app-1"}},
		{ObjectMeta: metav1.ObjectMeta{Namespace: "namespace-2", Name: "app-2"}},
	}
	res = appsDependsOnAddonErrInfo(apps)
	assert.Contains(t, res, ". Please delete all of them before removing.")
}

func TestIsAddonDir(t *testing.T) {
	var isAddonDir bool
	var err error
	var meta *Meta
	var metaYaml []byte

	// Non-existent dir
	isAddonDir, err = IsAddonDir("non-existent-dir")
	assert.Equal(t, isAddonDir, false)
	assert.Error(t, err)

	// Not a directory (a file)
	isAddonDir, err = IsAddonDir(filepath.Join("testdata", "local", "metadata.yaml"))
	assert.Equal(t, isAddonDir, false)
	assert.Contains(t, err.Error(), "not a directory")

	// No metadata.yaml
	isAddonDir, err = IsAddonDir(".")
	assert.Equal(t, isAddonDir, false)
	assert.Contains(t, err.Error(), "exists in directory")

	// Empty metadata.yaml
	err = os.MkdirAll(filepath.Join("testdata", "testaddon"), 0700)
	assert.NoError(t, err)
	defer func() {
		os.RemoveAll(filepath.Join("testdata", "testaddon"))
	}()
	err = os.WriteFile(filepath.Join("testdata", "testaddon", MetadataFileName), []byte{}, 0644)
	assert.NoError(t, err)
	isAddonDir, err = IsAddonDir(filepath.Join("testdata", "testaddon"))
	assert.Equal(t, isAddonDir, false)
	assert.Contains(t, err.Error(), "missing")

	// Empty addon name
	meta = &Meta{}
	metaYaml, err = yaml.Marshal(meta)
	assert.NoError(t, err)
	err = os.WriteFile(filepath.Join("testdata", "testaddon", MetadataFileName), metaYaml, 0644)
	assert.NoError(t, err)
	isAddonDir, err = IsAddonDir(filepath.Join("testdata", "testaddon"))
	assert.Equal(t, isAddonDir, false)
	assert.Contains(t, err.Error(), "addon name is empty")

	// Empty addon version
	meta = &Meta{
		Name: "name",
	}
	metaYaml, err = yaml.Marshal(meta)
	assert.NoError(t, err)
	err = os.WriteFile(filepath.Join("testdata", "testaddon", MetadataFileName), metaYaml, 0644)
	assert.NoError(t, err)
	isAddonDir, err = IsAddonDir(filepath.Join("testdata", "testaddon"))
	assert.Equal(t, isAddonDir, false)
	assert.Contains(t, err.Error(), "addon version is empty")

	// No metadata.yaml
	meta = &Meta{
		Name:    "name",
		Version: "1.0.0",
	}
	metaYaml, err = yaml.Marshal(meta)
	assert.NoError(t, err)
	err = os.WriteFile(filepath.Join("testdata", "testaddon", MetadataFileName), metaYaml, 0644)
	assert.NoError(t, err)
	isAddonDir, err = IsAddonDir(filepath.Join("testdata", "testaddon"))
	assert.Equal(t, isAddonDir, false)
	assert.Contains(t, err.Error(), "exists in directory")

	// Empty template.yaml
	err = os.WriteFile(filepath.Join("testdata", "testaddon", TemplateFileName), []byte{}, 0644)
	assert.NoError(t, err)
	isAddonDir, err = IsAddonDir(filepath.Join("testdata", "testaddon"))
	assert.Equal(t, isAddonDir, false)
	assert.Contains(t, err.Error(), "missing")

	// Empty template.cue
	err = os.WriteFile(filepath.Join("testdata", "testaddon", AppTemplateCueFileName), []byte{}, 0644)
	assert.NoError(t, err)
	isAddonDir, err = IsAddonDir(filepath.Join("testdata", "testaddon"))
	assert.Equal(t, isAddonDir, false)
	assert.Contains(t, err.Error(), renderOutputCuePath)

	// Pass all checks
	cmd := InitCmd{
		Path:      filepath.Join("testdata", "testaddon2"),
		AddonName: "testaddon2",
	}
	err = cmd.CreateScaffold()
	assert.NoError(t, err)
	defer func() {
		_ = os.RemoveAll(filepath.Join("testdata", "testaddon2"))
	}()
	isAddonDir, err = IsAddonDir(filepath.Join("testdata", "testaddon2"))
	assert.Equal(t, isAddonDir, true)
	assert.NoError(t, err)
}

func TestMakeChart(t *testing.T) {
	var err error

	// Not a addon dir
	err = MakeChartCompatible(".", true)
	assert.Contains(t, err.Error(), "not an addon dir")

	// Valid addon dir
	cmd := InitCmd{
		Path:      filepath.Join("testdata", "testaddon"),
		AddonName: "testaddon",
	}
	err = cmd.CreateScaffold()
	assert.NoError(t, err)
	defer func() {
		_ = os.RemoveAll(filepath.Join("testdata", "testaddon"))
	}()
	err = MakeChartCompatible(filepath.Join("testdata", "testaddon"), true)
	assert.NoError(t, err)
	isChartDir, err := chartutil.IsChartDir(filepath.Join("testdata", "testaddon"))
	assert.NoError(t, err)
	assert.Equal(t, isChartDir, true)

	// Already a chart dir
	err = MakeChartCompatible(filepath.Join("testdata", "testaddon"), false)
	assert.NoError(t, err)
	isChartDir, err = chartutil.IsChartDir(filepath.Join("testdata", "testaddon"))
	assert.NoError(t, err)
	assert.Equal(t, isChartDir, true)
}

func TestCheckObjectBindingComponent(t *testing.T) {
	existingBindingDef := unstructured.Unstructured{}
	existingBindingDef.SetAnnotations(map[string]string{oam.AnnotationAddonDefinitionBondCompKey: "kustomize"})

	emptyAnnoDef := unstructured.Unstructured{}
	emptyAnnoDef.SetAnnotations(map[string]string{"test": "onlyForTest"})

	legacyAnnoDef := unstructured.Unstructured{}
	legacyAnnoDef.SetAnnotations(map[string]string{oam.AnnotationIgnoreWithoutCompKey: "kustomize"})
	testCases := map[string]struct {
		object unstructured.Unstructured
		app    v1beta1.Application
		res    bool
	}{
		"bindingExist": {object: existingBindingDef,
			app: v1beta1.Application{Spec: v1beta1.ApplicationSpec{Components: []common.ApplicationComponent{{Name: "kustomize"}}}},
			res: true},
		"NotExisting": {object: existingBindingDef,
			app: v1beta1.Application{Spec: v1beta1.ApplicationSpec{Components: []common.ApplicationComponent{{Name: "helm"}}}},
			res: false},
		"NoBidingAnnotation": {object: emptyAnnoDef,
			app: v1beta1.Application{Spec: v1beta1.ApplicationSpec{Components: []common.ApplicationComponent{{Name: "kustomize"}}}},
			res: true},
		"EmptyApp": {object: existingBindingDef,
			app: v1beta1.Application{Spec: v1beta1.ApplicationSpec{Components: []common.ApplicationComponent{}}},
			res: false},
		"LegacyApp": {object: legacyAnnoDef,
			app: v1beta1.Application{Spec: v1beta1.ApplicationSpec{Components: []common.ApplicationComponent{{Name: "kustomize"}}}},
			res: true,
		},
		"LegacyAppWithoutComp": {object: legacyAnnoDef,
			app: v1beta1.Application{Spec: v1beta1.ApplicationSpec{Components: []common.ApplicationComponent{{}}}},
			res: false,
		},
	}
	for _, s := range testCases {
		result := checkBondComponentExist(s.object, s.app)
		assert.Equal(t, result, s.res)
	}
}

func TestFilterDependencyRegistries(t *testing.T) {
	testCases := []struct {
		registries []Registry
		index      int
		res        []Registry
		origin     []Registry
	}{
		{
			registries: []Registry{{Name: "r1"}, {Name: "r2"}, {Name: "r3"}},
			index:      0,
			res:        []Registry{{Name: "r2"}, {Name: "r3"}},
			origin:     []Registry{{Name: "r1"}, {Name: "r2"}, {Name: "r3"}},
		},
		{
			registries: []Registry{{Name: "r1"}, {Name: "r2"}, {Name: "r3"}},
			index:      1,
			res:        []Registry{{Name: "r1"}, {Name: "r3"}},
			origin:     []Registry{{Name: "r1"}, {Name: "r2"}, {Name: "r3"}},
		},
		{
			registries: []Registry{{Name: "r1"}, {Name: "r2"}, {Name: "r3"}},
			index:      2,
			res:        []Registry{{Name: "r1"}, {Name: "r2"}},
			origin:     []Registry{{Name: "r1"}, {Name: "r2"}, {Name: "r3"}},
		},
		{
			registries: []Registry{{Name: "r1"}, {Name: "r2"}, {Name: "r3"}},
			index:      3,
			res:        []Registry{{Name: "r1"}, {Name: "r2"}, {Name: "r3"}},
			origin:     []Registry{{Name: "r1"}, {Name: "r2"}, {Name: "r3"}},
		},
		{
			registries: []Registry{{Name: "r1"}, {Name: "r2"}, {Name: "r3"}},
			index:      -1,
			res:        []Registry{{Name: "r1"}, {Name: "r2"}, {Name: "r3"}},
			origin:     []Registry{{Name: "r1"}, {Name: "r2"}, {Name: "r3"}},
		},
		{
			registries: []Registry{},
			index:      0,
			res:        []Registry{},
			origin:     []Registry{},
		},
	}
	for _, testCase := range testCases {
		res := FilterDependencyRegistries(testCase.index, testCase.registries)
		assert.Equal(t, res, testCase.res)
		assert.Equal(t, testCase.registries, testCase.origin)
	}
}

func TestCheckAddonPackageValid(t *testing.T) {
	testCases := []struct {
		testCase Meta
		err      error
	}{{
		testCase: Meta{},
		err:      fmt.Errorf("the addon package doesn't have `metadata.yaml`"),
	}, {
		testCase: Meta{Version: "v1.4.0"},
		err:      fmt.Errorf("`matadata.yaml` must define the name of addon"),
	}, {
		testCase: Meta{Name: "test-addon"},
		err:      fmt.Errorf("`matadata.yaml` must define the version of addon"),
	}, {
		testCase: Meta{Name: "test-addon", Version: "1.4.5"},
		err:      nil,
	},
	}
	for _, testCase := range testCases {
		err := validateAddonPackage(&InstallPackage{Meta: testCase.testCase})
		assert.Equal(t, reflect.DeepEqual(err, testCase.err), true)
	}
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
    addon.oam.dev/componentDefinitions: "my-comp"
    addon.oam.dev/traitDefinitions: "my-trait"
    addon.oam.dev/workflowStepDefinitions: "my-wfstep"
    addon.oam.dev/policyDefinitions: "my-policy"
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
	testApp4Yaml = `
apiVersion: core.oam.dev/v1beta1
kind: Application
metadata:
  name: app-4
  namespace: test-ns
spec:
  components:
    - name: podinfo
      type: webservice

  policies:
    - type: my-policy
      name: topology
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
