package plugins

import (
	"context"
	"fmt"
	"io/ioutil"

	"github.com/cloud-native-application/rudrx/api/types"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/ghodss/yaml"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"

	corev1 "k8s.io/api/core/v1"
	logf "sigs.k8s.io/controller-runtime/pkg/log"

	"github.com/crossplane/oam-kubernetes-runtime/apis/core/v1alpha2"
	"github.com/crossplane/oam-kubernetes-runtime/pkg/oam/util"
)

var _ = Describe("DefinitionFiles", func() {
	ctx := context.Background()
	route := types.Template{
		Name:  "routes.extend.oam.dev",
		Type:  types.TypeTrait,
		Alias: "route",
		Object: map[string]interface{}{
			"apiVersion": "extend.oam.dev/v1alpha2",
			"kind":       "Route",
		},
		Parameters: []types.Parameter{
			{
				Name:       "domain",
				Short:      "d",
				Required:   true,
				FieldPaths: []string{"spec.domain"},
			},
		},
	}
	deployment := types.Template{
		Name:  "deployments.testapps",
		Type:  types.TypeWorkload,
		Alias: "deployment",
		Object: map[string]interface{}{
			"apiVersion": "core.oam.dev/v1alpha2",
			"kind":       "Deployment",
		},
		Parameters: []types.Parameter{
			{
				Name:       "image",
				Short:      "i",
				Required:   true,
				FieldPaths: []string{"spec.containers[0].image"},
			},
		},
	}

	BeforeEach(func() {
		Expect(k8sClient.Create(ctx, &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: DefinitionNamespace}})).Should(SatisfyAny(BeNil(), &util.AlreadyExistMatcher{}))

		traitdata, err := ioutil.ReadFile("testdata/traitDef.yaml")
		Expect(err).Should(BeNil())
		var td v1alpha2.TraitDefinition
		Expect(yaml.Unmarshal(traitdata, &td)).Should(BeNil())

		td.Namespace = DefinitionNamespace
		logf.Log.Info("Creating trait definition", "data", td)
		Expect(k8sClient.Create(ctx, &td)).Should(SatisfyAny(BeNil(), &util.AlreadyExistMatcher{}))

		workloaddata, err := ioutil.ReadFile("testdata/workloadDef.yaml")
		Expect(err).Should(BeNil())
		var wd v1alpha2.WorkloadDefinition
		Expect(yaml.Unmarshal(workloaddata, &wd)).Should(BeNil())

		wd.Namespace = DefinitionNamespace
		logf.Log.Info("Creating workload definition", "data", wd)
		Expect(k8sClient.Create(ctx, &wd)).Should(SatisfyAny(BeNil(), &util.AlreadyExistMatcher{}))

	})

	// Notice!!  Definition Object is Cluster Scope object
	// which means objects created in other DefinitionNamespace will also affect here.
	It("gettrait", func() {
		traitDefs, err := GetTraitsFromCluster(context.Background(), DefinitionNamespace, k8sClient)
		Expect(err).Should(BeNil())
		logf.Log.Info(fmt.Sprintf("Getting trait definitions %v", traitDefs))

		Expect(traitDefs).Should(Equal([]types.Template{route}))
	})
	// Notice!!  Definition Object is Cluster Scope object
	// which means objects created in other DefinitionNamespace will also affect here.
	It("getworkload", func() {
		workloadDefs, err := GetWorkloadsFromCluster(context.Background(), DefinitionNamespace, k8sClient)
		Expect(err).Should(BeNil())
		logf.Log.Info(fmt.Sprintf("Getting workload definitions  %v", workloadDefs))

		Expect(workloadDefs).Should(Equal([]types.Template{deployment}))
	})
	It("getall", func() {
		alldef, err := GetTemplatesFromCluster(context.Background(), DefinitionNamespace, k8sClient)
		Expect(err).Should(BeNil())
		logf.Log.Info(fmt.Sprintf("Getting all definitions %v", alldef))

		Expect(alldef).Should(Equal([]types.Template{deployment, route}))
	})
})
