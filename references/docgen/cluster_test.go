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
	"fmt"
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
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/selection"
	"k8s.io/client-go/rest"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/yaml"

	"github.com/kubevela/pkg/util/singleton"

	commontypes "github.com/oam-dev/kubevela/apis/core.oam.dev/common"
	corev1beta1 "github.com/oam-dev/kubevela/apis/core.oam.dev/v1beta1"
	"github.com/oam-dev/kubevela/apis/types"
	"github.com/oam-dev/kubevela/pkg/oam"
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

func newTraitRevision(ns, defName, version, param string, revision int64) *corev1beta1.DefinitionRevision {
	return &corev1beta1.DefinitionRevision{
		ObjectMeta: metav1.ObjectMeta{
			Name:      fmt.Sprintf("%s-v%s", defName, version),
			Namespace: ns,
			Labels:    map[string]string{oam.LabelTraitDefinitionName: defName},
		},
		Spec: corev1beta1.DefinitionRevisionSpec{
			Revision:       revision,
			DefinitionType: commontypes.TraitType,
			TraitDefinition: corev1beta1.TraitDefinition{
				ObjectMeta: metav1.ObjectMeta{Name: defName, Namespace: ns},
				Spec: corev1beta1.TraitDefinitionSpec{
					Version: version,
					Schematic: &commontypes.Schematic{CUE: &commontypes.CUE{
						Template: fmt.Sprintf("parameter: {\n\t%s: string\n}\n", param),
					}},
				},
			},
		},
	}
}

func TestGetCapabilityFromDefinitionVersion(t *testing.T) {
	ctx := context.Background()
	// Parameter extraction builds the cuex compiler, whose kube config loader is
	// config.GetConfigOrDie: without a kubeconfig it would end the test binary.
	// Restore nil afterwards rather than reading the previous value (that runs the loader).
	t.Cleanup(func() { singleton.KubeConfig.Set(nil) })
	singleton.KubeConfig.Set(&rest.Config{Host: "http://127.0.0.1:1"})
	sharedPolicy := &corev1beta1.DefinitionRevision{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "shared-policy-v1.0.0",
			Namespace: "default",
			Labels:    map[string]string{oam.LabelPolicyDefinitionName: "shared"},
		},
		Spec: corev1beta1.DefinitionRevisionSpec{
			Revision:       1,
			DefinitionType: commontypes.PolicyType,
			PolicyDefinition: corev1beta1.PolicyDefinition{
				ObjectMeta: metav1.ObjectMeta{Name: "shared", Namespace: "default"},
				Spec: corev1beta1.PolicyDefinitionSpec{
					Version: "1.0.0",
					Schematic: &commontypes.Schematic{CUE: &commontypes.CUE{
						Template: "parameter: {\n\tpolicyParam: string\n}\n",
					}},
				},
			},
		},
	}
	// Two revisions carrying the same spec.version: the lower revision's name sorts
	// first, so returning the first match instead of the highest revision fails.
	staleDup := newTraitRevision("default", "mytrait", "1.3.0", "staleParam", 3)
	staleDup.Name = "mytrait-v1.3.0-a"
	latestDup := newTraitRevision("default", "mytrait", "1.3.0", "latestParam", 4)
	latestDup.Name = "mytrait-v1.3.0-b"
	cli := fake.NewClientBuilder().WithScheme(common.Scheme).WithObjects(
		newTraitRevision("default", "mytrait", "1.0.0", "oldParam", 1),
		newTraitRevision("default", "mytrait", "1.2.0", "newParam", 2),
		staleDup,
		latestDup,
		newTraitRevision(types.DefaultKubeVelaNS, "systrait", "2.0.0", "sysParam", 1),
		newTraitRevision("default", "shared", "1.0.0", "traitParam", 1),
		sharedPolicy,
	).Build()
	c := common.Args{}
	c.SetClient(cli)

	for _, v := range []string{"1.2.0", "v1.2.0"} {
		capability, err := GetCapabilityFromDefinitionVersion(ctx, c, "default", "mytrait", v)
		require.NoError(t, err, v)
		assert.Contains(t, capability.CueTemplate, "newParam", v)
		assert.NotContains(t, capability.CueTemplate, "oldParam", v)
		assert.Equal(t, types.TypeTrait, capability.Type)
	}

	capability, err := GetCapabilityFromDefinitionVersion(ctx, c, "default", "mytrait", "1.0.0")
	require.NoError(t, err)
	assert.Contains(t, capability.CueTemplate, "oldParam")

	// Duplicate versions resolve to the highest revision.
	capability, err = GetCapabilityFromDefinitionVersion(ctx, c, "default", "mytrait", "1.3.0")
	require.NoError(t, err)
	assert.Contains(t, capability.CueTemplate, "latestParam")
	assert.NotContains(t, capability.CueTemplate, "staleParam")

	_, err = GetCapabilityFromDefinitionVersion(ctx, c, "default", "mytrait", "9.9.9")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "no mytrait with version 9.9.9 found")

	// Only present in vela-system: found through the fallback when ns is default.
	capability, err = GetCapabilityFromDefinitionVersion(ctx, c, "default", "systrait", "2.0.0")
	require.NoError(t, err)
	assert.Contains(t, capability.CueTemplate, "sysParam")

	// No fallback for a namespace other than default.
	_, err = GetCapabilityFromDefinitionVersion(ctx, c, "other", "systrait", "2.0.0")
	require.Error(t, err)

	// The same name and version across kinds is ambiguous.
	_, err = GetCapabilityFromDefinitionVersion(ctx, c, "default", "shared", "1.0.0")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "more than one definition type")

	// The integer revision path keeps working.
	capability, err = GetCapabilityFromDefinitionRevision(ctx, c, "default", "mytrait", 2)
	require.NoError(t, err)
	assert.Contains(t, capability.CueTemplate, "newParam")
}

func TestDefinitionVersionMatching(t *testing.T) {
	ctx := context.Background()
	// See TestGetCapabilityFromDefinitionVersion: parameter extraction needs a kube config.
	t.Cleanup(func() { singleton.KubeConfig.Set(nil) })
	singleton.KubeConfig.Set(&rest.Config{Host: "http://127.0.0.1:1"})
	// Unversioned revisions are named <def>-v<revision>.
	plainRev1 := newTraitRevision("default", "plain", "", "plainRev1", 1)
	plainRev1.Name = "plain-v1"
	plainRev2 := newTraitRevision("default", "plain", "", "plainRev2", 2)
	plainRev2.Name = "plain-v2"
	// 3.1.0 has a higher revision number than 3.1.2, so the highest version must beat it.
	cli := fake.NewClientBuilder().WithScheme(common.Scheme).WithObjects(
		newTraitRevision("default", "vtrait", "3.3.0", "p330", 1),
		newTraitRevision("default", "vtrait", "3.1.2", "p312", 2),
		newTraitRevision("default", "vtrait", "3.1.0", "p310", 3),
		plainRev1,
		plainRev2,
	).Build()
	c := common.Args{}
	c.SetClient(cli)

	t.Run("partial version matches the highest version with those leading numbers", func(t *testing.T) {
		for want, param := range map[string]string{"3": "p330", "v3": "p330", "3.1": "p312", "v3.1": "p312", "3.1.0": "p310", "3.3": "p330"} {
			capability, err := GetCapabilityFromDefinitionVersion(ctx, c, "default", "vtrait", want)
			require.NoError(t, err, want)
			assert.Contains(t, capability.CueTemplate, param, want)
		}
		for _, want := range []string{"3.0", "3.2", "4", "3.1.1"} {
			_, err := GetCapabilityFromDefinitionVersion(ctx, c, "default", "vtrait", want)
			require.Error(t, err, want)
			assert.Contains(t, err.Error(), "no vtrait with version "+want+" found", want)
		}
	})

	t.Run("matcher", func(t *testing.T) {
		assert.True(t, definitionVersionMatches("3.1.0", "3.1"))
		assert.True(t, definitionVersionMatches("3.1.2", "3.1"))
		assert.False(t, definitionVersionMatches("3.2.0", "3.1"))
		assert.True(t, definitionVersionMatches("v3.2.0", "3"))
		assert.False(t, definitionVersionMatches("30.0.0", "3"))
		assert.True(t, definitionVersionMatches("1.2.3", "v1.2.3"))
		assert.False(t, definitionVersionMatches("1.2.30", "1.2.3"))
		assert.False(t, definitionVersionMatches("", "3"))
	})

	t.Run("version list is highest first and empty without spec.version", func(t *testing.T) {
		revs, err := listDefinitionRevisions(ctx, cli, "default", "vtrait", "")
		require.NoError(t, err)
		assert.Equal(t, []string{"3.3.0", "3.1.2", "3.1.0"}, definitionVersions(revs))
		revs, err = listDefinitionRevisions(ctx, cli, "default", "plain", "")
		require.NoError(t, err)
		assert.Empty(t, definitionVersions(revs))
	})

	show := func(name string, rev int64) (*types.Capability, error) {
		ref := &ParseReference{DefinitionName: name, Remote: &FromCluster{Namespace: "default", Rev: rev}}
		caps, err := ref.getCapabilities(ctx, c)
		if err != nil {
			return nil, err
		}
		return &caps[0], nil
	}

	t.Run("bare number on a versioned definition is its major version", func(t *testing.T) {
		capability, err := show("vtrait", 3)
		require.NoError(t, err)
		assert.Contains(t, capability.CueTemplate, "p330")

		_, err = show("vtrait", 4)
		require.Error(t, err)
		assert.Equal(t, "failed to get revision 4 of capability vtrait: no vtrait with version 4 found in namespace default or vela-system", err.Error())
	})

	t.Run("bare number on an unversioned definition is the revision number", func(t *testing.T) {
		capability, err := show("plain", 2)
		require.NoError(t, err)
		assert.Contains(t, capability.CueTemplate, "plainRev2")

		_, err = show("plain", 9)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "failed to get revision 9 of capability plain")
	})

	t.Run("web version pages and the page --revision opens", func(t *testing.T) {
		ref := &MarkdownReference{ParseReference: ParseReference{Client: cli, I18N: &En}}
		dir := t.TempDir()
		read := func(page string) string {
			data, err := os.ReadFile(filepath.Join(dir, string(types.TypeTrait), page+".md"))
			require.NoError(t, err, page)
			return string(data)
		}

		docs, err := ref.CreateVersionMarkdown(ctx, c, "default", "vtrait", types.TypeTrait, dir, 0, "")
		require.NoError(t, err)
		assert.Equal(t, &VersionDocs{Name: "vtrait", Type: types.TypeTrait, Versions: []string{"3.3.0", "3.1.2", "3.1.0"}}, docs)
		assert.Contains(t, read("vtrait-v3.3.0"), "p330")
		assert.Contains(t, read("vtrait-v3.1.2"), "p312")
		assert.Contains(t, read("vtrait-v3.1.0"), "p310")

		for _, tc := range []struct {
			rev     int64
			version string
			page    string
		}{
			{version: "3.1.2", page: "vtrait-v3.1.2"},
			{version: "3.1", page: "vtrait-v3.1.2"},
			{rev: 3, page: "vtrait-v3.3.0"},
		} {
			docs, err := ref.CreateVersionMarkdown(ctx, c, "default", "vtrait", types.TypeTrait, dir, tc.rev, tc.version)
			require.NoError(t, err, tc)
			assert.Equal(t, tc.page, docs.Page, tc)
		}

		// An unversioned definition gets no version pages; a revision number opens that revision's page.
		docs, err = ref.CreateVersionMarkdown(ctx, c, "default", "plain", types.TypeTrait, dir, 2, "")
		require.NoError(t, err)
		assert.Empty(t, docs.Versions)
		assert.Equal(t, "plain-v2", docs.Page)
		assert.Contains(t, read("plain-v2"), "plainRev2")
		assert.NotContains(t, read("plain-v2"), "plainRev1")

		for _, tc := range []struct {
			name    string
			rev     int64
			version string
			err     string
		}{
			{name: "vtrait", version: "3.2", err: "no vtrait with version 3.2 found"},
			{name: "vtrait", rev: 4, err: "no vtrait with version 4 found"},
			{name: "plain", rev: 9, err: "no plain with revision 9 found"},
			{name: "plain", version: "1.0.0", err: "no plain with version 1.0.0 found"},
		} {
			_, err := ref.CreateVersionMarkdown(ctx, c, "default", tc.name, types.TypeTrait, dir, tc.rev, tc.version)
			require.Error(t, err, tc)
			assert.Contains(t, err.Error(), tc.err, tc)
		}
	})
}

func newPolicyRevision(ns, defName, version, param string, revision int64) *corev1beta1.DefinitionRevision {
	name := fmt.Sprintf("%s-v%s", defName, version)
	if version == "" {
		name = fmt.Sprintf("%s-v%d", defName, revision)
	}
	return &corev1beta1.DefinitionRevision{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: ns,
			Labels:    map[string]string{oam.LabelPolicyDefinitionName: defName},
		},
		Spec: corev1beta1.DefinitionRevisionSpec{
			Revision:       revision,
			DefinitionType: commontypes.PolicyType,
			PolicyDefinition: corev1beta1.PolicyDefinition{
				ObjectMeta: metav1.ObjectMeta{Name: defName, Namespace: ns},
				Spec: corev1beta1.PolicyDefinitionSpec{
					Version: version,
					Schematic: &commontypes.Schematic{CUE: &commontypes.CUE{
						Template: fmt.Sprintf("parameter: {\n\t%s: string\n}\n", param),
					}},
				},
			},
		},
	}
}

// A versioned trait and an unversioned policy share the name "mixed". Their DefinitionRevision
// names do not collide (mixed-v1.0.0 vs mixed-v1), so both exist in one namespace.
func TestRevisionResolutionAcrossDefinitionTypes(t *testing.T) {
	ctx := context.Background()
	// See TestGetCapabilityFromDefinitionVersion: parameter extraction needs a kube config.
	t.Cleanup(func() { singleton.KubeConfig.Set(nil) })
	singleton.KubeConfig.Set(&rest.Config{Host: "http://127.0.0.1:1"})
	// Same object name as the trait revision; renamed as in TestGetCapabilityFromDefinitionVersion.
	sharedPolicy := newPolicyRevision("default", "shared", "1.0.0", "policyParam", 1)
	sharedPolicy.Name = "shared-policy-v1.0.0"
	cli := fake.NewClientBuilder().WithScheme(common.Scheme).WithObjects(
		newTraitRevision("default", "mixed", "1.0.0", "trait100", 1),
		newTraitRevision("default", "mixed", "3.0.0", "trait300", 2),
		newPolicyRevision("default", "mixed", "", "policyRev1", 1),
		newPolicyRevision("default", "mixed", "", "policyRev2", 2),
		newTraitRevision("default", "shared", "1.0.0", "traitParam", 1),
		sharedPolicy,
	).Build()
	c := common.Args{}
	c.SetClient(cli)

	t.Run("bare number is resolved per definition type", func(t *testing.T) {
		show := func(rev int64) (*types.Capability, error) {
			ref := &ParseReference{DefinitionName: "mixed", Remote: &FromCluster{Namespace: "default", Rev: rev}}
			caps, err := ref.getCapabilities(ctx, c)
			if err != nil {
				return nil, err
			}
			return &caps[0], nil
		}
		// Only the trait has major version 3.
		capability, err := show(3)
		require.NoError(t, err)
		assert.Equal(t, types.TypeTrait, capability.Type)
		assert.Contains(t, capability.CueTemplate, "trait300")

		// Only the policy has revision 2; the trait's versions must not turn it into "major version 2".
		capability, err = show(2)
		require.NoError(t, err)
		assert.Equal(t, types.TypePolicy, capability.Type)
		assert.Contains(t, capability.CueTemplate, "policyRev2")

		// Trait version 1.0.0 and policy revision 1 both match.
		_, err = show(1)
		require.Error(t, err)
		assert.Equal(t, "failed to get revision 1 of capability mixed: revision 1 of mixed matches more than one definition type (Policy, Trait)", err.Error())

		_, err = show(5)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "no mixed with revision 5 or version 5 found")
	})

	t.Run("web page is resolved across definition types", func(t *testing.T) {
		ref := &MarkdownReference{ParseReference: ParseReference{Client: cli, I18N: &En}}
		dir := t.TempDir()

		// The site opens mixed as a trait, but revision 2 only exists for the policy.
		docs, err := ref.CreateVersionMarkdown(ctx, c, "default", "mixed", types.TypeTrait, dir, 2, "")
		require.NoError(t, err)
		assert.Equal(t, []string{"3.0.0", "1.0.0"}, docs.Versions)
		assert.Equal(t, "mixed-v2", docs.Page)
		assert.Equal(t, types.TypePolicy, docs.PageType)
		data, err := os.ReadFile(filepath.Join(dir, string(types.TypePolicy), "mixed-v2.md"))
		require.NoError(t, err)
		assert.Contains(t, string(data), "policyRev2")

		docs, err = ref.CreateVersionMarkdown(ctx, c, "default", "mixed", types.TypeTrait, dir, 3, "")
		require.NoError(t, err)
		assert.Equal(t, "mixed-v3.0.0", docs.Page)
		assert.Equal(t, types.TypeTrait, docs.PageType)

		for _, tc := range []struct {
			name    string
			rev     int64
			version string
			err     string
		}{
			{name: "mixed", rev: 1, err: "revision 1 of mixed matches more than one definition type (Policy, Trait)"},
			{name: "shared", version: "1.0.0", err: "version 1.0.0 of shared matches more than one definition type (Policy, Trait)"},
			{name: "shared", rev: 1, err: "revision 1 of shared matches more than one definition type (Policy, Trait)"},
		} {
			_, err := ref.CreateVersionMarkdown(ctx, c, "default", tc.name, types.TypeTrait, dir, tc.rev, tc.version)
			require.Error(t, err, tc)
			assert.Equal(t, tc.err, err.Error(), tc)
		}
	})
}

// spec.version reaches page file names and sidebar links, so a malformed one must not
// write outside the docs directory.
func TestVersionPagesRejectUnsafeVersions(t *testing.T) {
	ctx := context.Background()
	// See TestGetCapabilityFromDefinitionVersion: parameter extraction needs a kube config.
	t.Cleanup(func() { singleton.KubeConfig.Set(nil) })
	singleton.KubeConfig.Set(&rest.Config{Host: "http://127.0.0.1:1"})
	const evilVersion = "../../../../pwn"
	evil := newTraitRevision("default", "evil", evilVersion, "evilParam", 2)
	evil.Name = "evil-v2"
	cli := fake.NewClientBuilder().WithScheme(common.Scheme).WithObjects(
		newTraitRevision("default", "evil", "1.0.0", "goodParam", 1),
		evil,
	).Build()
	c := common.Args{}
	c.SetClient(cli)
	ref := &MarkdownReference{ParseReference: ParseReference{Client: cli, I18N: &En}}

	base := t.TempDir()
	dir := filepath.Join(base, "docs")
	// Unchecked, dir/trait/evil-v../../../../pwn.md cleans to base/pwn.md.
	escaped := filepath.Join(base, "pwn.md")

	docs, err := ref.CreateVersionMarkdown(ctx, c, "default", "evil", types.TypeTrait, dir, 0, "")
	require.NoError(t, err)
	// The unsafe version gets no page, so the sidebar built from Versions links no such page.
	assert.Equal(t, []string{"1.0.0"}, docs.Versions)
	_, err = os.Stat(filepath.Join(dir, string(types.TypeTrait), "evil-v1.0.0.md"))
	require.NoError(t, err)
	_, err = os.Stat(escaped)
	assert.True(t, os.IsNotExist(err), "page written outside the docs directory")

	// Selecting the unsafe version directly is refused too.
	_, err = ref.CreateVersionMarkdown(ctx, c, "default", "evil", types.TypeTrait, dir, 0, evilVersion)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "is not a valid docs page name")
	_, err = os.Stat(escaped)
	assert.True(t, os.IsNotExist(err), "page written outside the docs directory")

	for v, safe := range map[string]bool{"1.2.3": true, "1.2.3-beta.1+build.5": true, "12": true, "../x": false, "a/b": false, `a\b`: false, "": false} {
		assert.Equal(t, safe, isSafePageVersion(v), v)
	}
}
