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
	"time"

	"cuelang.org/go/cue"
	"github.com/google/go-cmp/cmp"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
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
