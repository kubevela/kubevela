package plugins

import (
	"context"
	"fmt"
	"io/ioutil"

	"cuelang.org/go/cue"

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
		Name: "route",
		Type: types.TypeTrait,
		Parameters: []types.Parameter{
			{
				Name:     "domain",
				Required: true,
				Default:  "",
				Type:     cue.StringKind,
			},
		},
	}
	deployment := types.Template{
		Name: "deployment",
		Type: types.TypeWorkload,
		Parameters: []types.Parameter{
			{
				Name:     "name",
				Required: true,
				Type:     cue.StringKind,
				Default:  "",
			},
			{
				Type: cue.ListKind,
				Name: "env",
			},
			{
				Name:     "image",
				Type:     cue.StringKind,
				Default:  "",
				Short:    "i",
				Required: true,
				Usage:    "specify app image",
			},
			{
				Name:    "port",
				Type:    cue.IntKind,
				Short:   "p",
				Default: int64(8080),
				Usage:   "specify port for container",
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

	// Notice!!  DefinitionPath Object is Cluster Scope object
	// which means objects created in other DefinitionNamespace will also affect here.
	It("gettrait", func() {
		traitDefs, err := GetTraitsFromCluster(context.Background(), DefinitionNamespace, k8sClient, definitionDir)
		Expect(err).Should(BeNil())
		logf.Log.Info(fmt.Sprintf("Getting trait definitions %v", traitDefs))
		for i := range traitDefs {
			traitDefs[i].Template = ""
			traitDefs[i].DefinitionPath = ""
		}
		Expect(traitDefs).Should(Equal([]types.Template{route}))
	})
	// Notice!!  DefinitionPath Object is Cluster Scope object
	// which means objects created in other DefinitionNamespace will also affect here.
	It("getworkload", func() {
		workloadDefs, err := GetWorkloadsFromCluster(context.Background(), DefinitionNamespace, k8sClient, definitionDir)
		Expect(err).Should(BeNil())
		logf.Log.Info(fmt.Sprintf("Getting workload definitions  %v", workloadDefs))
		for i := range workloadDefs {
			workloadDefs[i].Template = ""
			workloadDefs[i].DefinitionPath = ""
		}
		Expect(workloadDefs).Should(Equal([]types.Template{deployment}))
	})
	It("getall", func() {
		alldef, err := GetTemplatesFromCluster(context.Background(), DefinitionNamespace, k8sClient, definitionDir)
		Expect(err).Should(BeNil())
		logf.Log.Info(fmt.Sprintf("Getting all definitions %v", alldef))
		for i := range alldef {
			alldef[i].Template = ""
			alldef[i].DefinitionPath = ""
		}
		Expect(alldef).Should(Equal([]types.Template{deployment, route}))
	})
})
