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

package docgen

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"cuelang.org/go/cue"
	"github.com/google/go-cmp/cmp"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/stretchr/testify/assert"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/selection"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/yaml"

	corev1beta1 "github.com/oam-dev/kubevela/apis/core.oam.dev/v1beta1"
	"github.com/oam-dev/kubevela/apis/types"
	"github.com/oam-dev/kubevela/pkg/oam/util"
	"github.com/oam-dev/kubevela/pkg/utils/common"
)

const (
	TestDir        = "testdata"
	DeployName     = "deployments.testapps"
	WebserviceName = "webservice.testapps"
)

var _ = Describe("DefinitionFiles", func() {

	deployment := types.Capability{
		Namespace:   "testdef",
		Name:        DeployName,
		Type:        types.TypeComponentDefinition,
		CrdName:     "deployments.apps",
		Description: "description not defined",
		Category:    types.CUECategory,
		Parameters: []types.Parameter{
			{
				Name:     "image",
				Type:     cue.StringKind,
				Default:  "",
				Short:    "i",
				Required: true,
				Usage:    "Which image would you like to use for your service",
			},
			{
				Name:    "port",
				Type:    cue.IntKind,
				Short:   "p",
				Default: int64(8080),
				Usage:   "Which port do you want customer traffic sent to",
			},
			{
				Type: cue.ListKind,
				Name: "env",
			},
		},
		Labels: map[string]string{"usecase": "forplugintest"},
	}

	websvc := types.Capability{
		Namespace:   "testdef",
		Name:        WebserviceName,
		Type:        types.TypeComponentDefinition,
		Description: "description not defined",
		Category:    types.CUECategory,
		Parameters: []types.Parameter{
			{
				Name:     "image",
				Type:     cue.StringKind,
				Default:  "",
				Short:    "i",
				Required: true,
				Usage:    "Which image would you like to use for your service",
			}, {
				Name:    "port",
				Type:    cue.IntKind,
				Short:   "p",
				Default: int64(6379),
				Usage:   "Which port do you want customer traffic sent to",
			},
			{
				Name: "env", Type: cue.ListKind,
			},
		},
		CrdName: "deployments.apps",
		Labels:  map[string]string{"usecase": "forplugintest"},
	}

	req, _ := labels.NewRequirement("usecase", selection.Equals, []string{"forplugintest"})
	selector := labels.NewSelector().Add(*req)
	// Notice!!  DefinitionPath Object is Cluster Scope object
	// which means objects created in other DefinitionNamespace will also affect here.
	It("getcomponents", func() {
		arg := common.Args{}
		arg.SetClient(k8sClient)
		workloadDefs, _, err := GetComponentsFromCluster(context.Background(), DefinitionNamespace, arg, selector)
		Expect(err).Should(BeNil())
		for i := range workloadDefs {
			// CueTemplate should always be fulfilled, even those whose CueTemplateURI is assigend,
			By("check CueTemplate is fulfilled")
			Expect(workloadDefs[i].CueTemplate).ShouldNot(BeEmpty())
			workloadDefs[i].CueTemplate = ""
		}
		Expect(cmp.Diff(workloadDefs, []types.Capability{deployment, websvc})).Should(BeEquivalentTo(""))
	})
	It("getall", func() {
		arg := common.Args{}
		arg.SetClient(k8sClient)
		alldef, err := GetCapabilitiesFromCluster(context.Background(), DefinitionNamespace, arg, selector)
		Expect(err).Should(BeNil())
		for i := range alldef {
			alldef[i].CueTemplate = ""
		}
		Expect(cmp.Diff(alldef, []types.Capability{deployment, websvc})).Should(BeEquivalentTo(""))
	})
})

var _ = Describe("test GetCapabilityByName", func() {
	var (
		ctx        context.Context
		c          common.Args
		ns         string
		defaultNS  string
		cd1        corev1beta1.ComponentDefinition
		cd2        corev1beta1.ComponentDefinition
		component1 string
		component2 string
	)
	BeforeEach(func() {
		c = common.Args{}
		c.SetClient(k8sClient)
		ctx = context.Background()
		ns = "cluster-test-ns-suffix"
		defaultNS = types.DefaultKubeVelaNS
		component1 = "cd1"
		component2 = "cd2"

		By("create namespace")
		Expect(k8sClient.Create(ctx, &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: ns}})).Should(SatisfyAny(BeNil(), &util.AlreadyExistMatcher{}))
		Expect(k8sClient.Create(ctx, &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: defaultNS}})).Should(SatisfyAny(BeNil(), &util.AlreadyExistMatcher{}))

		By("create ComponentDefinition")
		data, _ := os.ReadFile("testdata/componentDef.yaml")
		yaml.Unmarshal(data, &cd1)
		yaml.Unmarshal(data, &cd2)

		cd1.Namespace = ns
		cd1.Name = component1
		Expect(k8sClient.Create(ctx, &cd1)).Should(SatisfyAny(BeNil(), &util.AlreadyExistMatcher{}))

		cd2.Namespace = defaultNS
		cd2.Name = component2
		Expect(k8sClient.Create(ctx, &cd2)).Should(SatisfyAny(BeNil(), &util.AlreadyExistMatcher{}))
	})

	AfterEach(func() {
		for _, obj := range []client.Object{&cd1, &cd2} {
			key := client.ObjectKeyFromObject(obj)
			Expect(k8sClient.Delete(ctx, obj)).Should(Succeed())
			Eventually(func(g Gomega) {
				g.Expect(k8sClient.Get(ctx, key, obj)).Should(Satisfy(errors.IsNotFound))
			}, 10*time.Second).Should(Succeed())
		}
	})

	It("get capability", func() {
		By("ComponentDefinition is in the current namespace")
		_, err := GetCapabilityByName(ctx, c, component1, ns)
		Expect(err).Should(BeNil())

		By("ComponentDefinition is in the default namespace")
		_, err = GetCapabilityByName(ctx, c, component2, ns)
		Expect(err).Should(BeNil())

		By("capability cloud not be found")
		_, err = GetCapabilityByName(ctx, c, "a-component-definition-not-existed", ns)
		Expect(err).Should(HaveOccurred())
	})
})

var _ = Describe("test GetNamespacedCapabilitiesFromCluster", func() {
	var (
		ctx        context.Context
		c          common.Args
		ns         string
		defaultNS  string
		cd1        corev1beta1.ComponentDefinition
		cd2        corev1beta1.ComponentDefinition
		component1 string
		component2 string
	)
	BeforeEach(func() {
		c = common.Args{}
		c.SetClient(k8sClient)
		ctx = context.Background()
		ns = "cluster-test-ns"
		defaultNS = types.DefaultKubeVelaNS
		component1 = "cd1"
		component2 = "cd2"

		By("create namespace")
		Expect(k8sClient.Create(ctx, &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: ns}})).Should(SatisfyAny(BeNil(), &util.AlreadyExistMatcher{}))
		Expect(k8sClient.Create(ctx, &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: defaultNS}})).Should(SatisfyAny(BeNil(), &util.AlreadyExistMatcher{}))

		By("create ComponentDefinition")
		data, _ := os.ReadFile("testdata/componentDef.yaml")
		yaml.Unmarshal(data, &cd1)
		yaml.Unmarshal(data, &cd2)
		cd1.Namespace = ns
		cd1.Name = component1
		Expect(k8sClient.Create(ctx, &cd1)).Should(SatisfyAny(BeNil(), &util.AlreadyExistMatcher{}))

		cd2.Namespace = defaultNS
		cd2.Name = component2
		Expect(k8sClient.Create(ctx, &cd2)).Should(SatisfyAny(BeNil(), &util.AlreadyExistMatcher{}))

	})

	AfterEach(func() {
		for _, obj := range []client.Object{&cd1, &cd2} {
			key := client.ObjectKeyFromObject(obj)
			Expect(k8sClient.Delete(ctx, obj)).Should(Succeed())
			Eventually(func(g Gomega) {
				g.Expect(k8sClient.Get(ctx, key, obj)).Should(Satisfy(errors.IsNotFound))
			}, 10*time.Second).Should(Succeed())
		}
	})

	It("get namespaced capabilities", func() {
		By("found all capabilities")
		capabilities, err := GetNamespacedCapabilitiesFromCluster(ctx, ns, c, nil)
		Expect(len(capabilities)).Should(Equal(2))
		Expect(err).Should(BeNil())

		By("found two capabilities with a bad namespace")
		capabilities, err = GetNamespacedCapabilitiesFromCluster(ctx, "a-bad-ns", c, nil)
		Expect(len(capabilities)).Should(Equal(1))
		Expect(err).Should(BeNil())
	})
})

var _ = Describe("test GetCapabilityFromDefinitionRevision", func() {
	var (
		ctx context.Context
		c   common.Args
	)

	BeforeEach(func() {
		c = common.Args{}
		c.SetClient(k8sClient)
		ctx = context.Background()

		By("create namespace")
		Expect(k8sClient.Create(ctx, &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "rev-test-custom-ns"}})).Should(SatisfyAny(BeNil(), &util.AlreadyExistMatcher{}))
		Expect(k8sClient.Create(ctx, &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "rev-test-ns"}})).Should(SatisfyAny(BeNil(), &util.AlreadyExistMatcher{}))

		// Load test DefinitionRevisions files into client
		dir := filepath.Join("..", "..", "pkg", "definition", "testdata")
		testFiles, err := os.ReadDir(dir)
		Expect(err).Should(Succeed())
		for _, file := range testFiles {
			if !strings.HasSuffix(file.Name(), ".yaml") {
				continue
			}
			content, err := os.ReadFile(filepath.Join(dir, file.Name()))
			Expect(err).Should(Succeed())
			def := &corev1beta1.DefinitionRevision{}
			err = yaml.Unmarshal(content, def)
			Expect(err).Should(Succeed())
			client, err := c.GetClient()
			Expect(err).Should(Succeed())
			err = client.Create(context.TODO(), def)
			Expect(err).Should(SatisfyAny(BeNil(), &util.AlreadyExistMatcher{}))
		}
	})

	It("non-existent defrev", func() {
		_, err := GetCapabilityFromDefinitionRevision(ctx, c, "rev-test-custom-ns", "not-a-name", 0)
		Expect(err).ShouldNot(Succeed())
	})

	It("component type", func() {
		_, err := GetCapabilityFromDefinitionRevision(ctx, c, "rev-test-ns", "webservice", 0)
		Expect(err).Should(Succeed())
	})

	It("trait type", func() {
		_, err := GetCapabilityFromDefinitionRevision(ctx, c, "rev-test-custom-ns", "affinity", 0)
		Expect(err).Should(Succeed())
	})
})

// `vela def show` must tell an author where a source can be consumed, and say why
// when that is narrower than everywhere.
//
// Two restrictions, from different causes. The context one is a consequence of
// what the template reads - a source keyed on a field that only exists during a
// component render cannot resolve in a workflow step - and can only be changed by
// changing the reads. consumableFrom is a deliberate choice by the definition's
// author. They compose, so the note has to read correctly with either alone or
// both together.
//
// The context branch cannot be reached through a real SourceDefinition today:
// the cache-key rules permit only universally-available fields, so a template
// reading a surface-specific one is refused at `vela def apply` before this could
// run. It is tested here directly because it is the branch that starts mattering
// the day that changes, and because a note that reads wrongly is the kind of
// thing nobody notices until a user quotes it back.
func reasonBySurface(surfaces []types.SourceSurface) map[string]string {
	out := map[string]string{}
	for _, s := range surfaces {
		out[s.Name] = s.Reason
	}
	return out
}

func consumableBySurface(surfaces []types.SourceSurface) map[string]bool {
	out := map[string]bool{}
	for _, s := range surfaces {
		out[s.Name] = s.Consumable
	}
	return out
}

func TestSourceSurfaces(t *testing.T) {
	// Reads nothing surface-specific, declares nothing: available everywhere.
	t.Run("unrestricted", func(t *testing.T) {
		surfaces := sourceSurfaces(`
$internal: {key: "s", keyInputs: []}
schema: {v: string}
output: {v: "x"}
`)
		assert.Equal(t, map[string]bool{
			"components": true, "traits": true, "workflow steps": true, "policies": true,
		}, consumableBySurface(surfaces))
		for _, sfc := range surfaces {
			assert.Empty(t, sfc.Reason, "a consumable surface needs no reason")
		}
	})

	t.Run("restricted by consumableFrom", func(t *testing.T) {
		surfaces := sourceSurfaces(`
$internal: {key: "s", keyInputs: []}
consumableFrom: ["component", "trait"]
schema: {v: string}
output: {v: "x"}
`)
		// Every surface is listed either way - the unreachable ones are the point.
		assert.Equal(t, map[string]bool{
			"components": true, "traits": true, "workflow steps": false, "policies": false,
		}, consumableBySurface(surfaces))
		assert.Equal(t, map[string]string{
			"components": "", "traits": "",
			"workflow steps": "not in consumableFrom", "policies": "not in consumableFrom",
		}, reasonBySurface(surfaces))
	})

	// consumableFrom naming every surface is not a restriction, and must not be
	// reported as one.
	t.Run("consumableFrom that restricts nothing is not reported", func(t *testing.T) {
		surfaces := sourceSurfaces(`
$internal: {key: "s", keyInputs: []}
consumableFrom: ["component", "trait", "workflowstep", "policy-rendered"]
schema: {v: string}
output: {v: "x"}
`)
		for _, sfc := range surfaces {
			assert.True(t, sfc.Consumable, sfc.Name)
			assert.Empty(t, sfc.Reason, sfc.Name)
		}
	})

	// The case the caller-identity keyed fields were added for: a source that
	// resolves per consuming component, and so cannot resolve where no component
	// is being rendered. The reason names the field, because "not consumable" on
	// its own leaves the author guessing which read caused it.
	t.Run("restricted by the context it reads", func(t *testing.T) {
		surfaces := sourceSurfaces(`
$internal: {key: "s-\(context.componentName)", keyInputs: ["componentName"]}
schema: {v: string}
output: {v: context.componentName}
`)
		assert.Equal(t, map[string]bool{
			"components": true, "traits": true, "workflow steps": false, "policies": false,
		}, consumableBySurface(surfaces))
		assert.Equal(t, map[string]string{
			"components": "", "traits": "",
			"workflow steps": "reads context.componentName",
			"policies":       "reads context.componentName",
		}, reasonBySurface(surfaces))
	})

	// Both causes at once are joined, because they are independent.
	t.Run("restricted by context and consumableFrom together", func(t *testing.T) {
		surfaces := sourceSurfaces(`
$internal: {key: "s-\(context.componentName)", keyInputs: ["componentName"]}
consumableFrom: ["component"]
schema: {v: string}
output: {v: context.componentName}
`)
		reasons := reasonBySurface(surfaces)
		assert.Empty(t, reasons["components"])
		assert.Equal(t, "not in consumableFrom", reasons["traits"])
		assert.Equal(t, "reads context.componentName; not in consumableFrom", reasons["workflow steps"])
	})

	// A template whose key cannot be inferred must not produce a half-answer: no
	// surfaces rather than a wrong list. Every other extractor here fails open the
	// same way.
	t.Run("an unparseable template yields nothing", func(t *testing.T) {
		assert.Empty(t, sourceSurfaces(`this is not CUE {{{`))
	})
}

// The generated $internal: block is where the cache key lives, and `vela def show`
// reads it from there.
//
// The extractor has to read it from there. Pointed anywhere else it prints
// nothing at all, quietly costing an operator the one field that correlates a
// source with its cache entry. This pins it.
func TestInternalCacheFields(t *testing.T) {
	fields := internalCacheFields(`
$internal: {
	key: "platform-registry-\(context.cluster)"
	keyInputs: ["cluster", "namespace"]
}
storage: {storageTTL: "30m"}
`)
	byName := map[string]string{}
	for _, f := range fields {
		byName[f.Name] = f.Value
	}
	assert.Equal(t, `platform-registry-\(context.cluster)`, byName["key"])
	// Rendered as the context reads an author writes, not as the CUE list the
	// generator stores.
	assert.Equal(t, "context.cluster, context.namespace", byName["keyInputs"])

	t.Run("no context reads says so rather than printing an empty cell", func(t *testing.T) {
		fields := internalCacheFields(`$internal: {key: "s", keyInputs: []}`)
		byName := map[string]string{}
		for _, f := range fields {
			byName[f.Name] = f.Value
		}
		assert.Equal(t, "(none - one cache entry for the whole cluster)", byName["keyInputs"])
	})
}
