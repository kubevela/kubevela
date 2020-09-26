package plugins

import (
	"context"
	"fmt"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"

	"cuelang.org/go/cue"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/selection"
	logf "sigs.k8s.io/controller-runtime/pkg/log"

	"github.com/oam-dev/kubevela/api/types"
)

var _ = Describe("DefinitionFiles", func() {

	route := types.Capability{
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
		CrdName: "routes.test",
	}

	deployment := types.Capability{
		Name:    "deployment",
		Type:    types.TypeWorkload,
		CrdName: "deployments.testapps",
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

	websvc := types.Capability{
		Name:           "webservice",
		Type:           types.TypeWorkload,
		CueTemplateURI: "https://raw.githubusercontent.com/oam-dev/kubevela/master/vela-templates/web-service.cue",
		Parameters: []types.Parameter{
			{
				Name:     "name",
				Required: true,
				Default:  "",
				Type:     cue.StringKind,
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
				Default: int64(6379),
				Usage:   "specify port for container",
			},
		},
		CrdName: "webservice.testapps",
	}

	req, _ := labels.NewRequirement("usecase", selection.Equals, []string{"forplugintest"})
	selector := labels.NewSelector().Add(*req)

	// Notice!!  DefinitionPath Object is Cluster Scope object
	// which means objects created in other DefinitionNamespace will also affect here.
	It("gettrait", func() {
		traitDefs, err := GetTraitsFromCluster(context.Background(), DefinitionNamespace, k8sClient, definitionDir, selector)
		Expect(err).Should(BeNil())
		logf.Log.Info(fmt.Sprintf("Getting trait definitions %v", traitDefs))
		for i := range traitDefs {
			traitDefs[i].CueTemplate = ""
			traitDefs[i].DefinitionPath = ""
		}
		Expect(traitDefs).Should(Equal([]types.Capability{route}))
	})

	// Notice!!  DefinitionPath Object is Cluster Scope object
	// which means objects created in other DefinitionNamespace will also affect here.
	It("getworkload", func() {
		workloadDefs, err := GetWorkloadsFromCluster(context.Background(), DefinitionNamespace, k8sClient, definitionDir, selector)
		Expect(err).Should(BeNil())
		logf.Log.Info(fmt.Sprintf("Getting workload definitions  %v", workloadDefs))
		for i := range workloadDefs {
			workloadDefs[i].CueTemplate = ""
			workloadDefs[i].DefinitionPath = ""
		}
		Expect(workloadDefs).Should(Equal([]types.Capability{deployment, websvc}))
	})
	It("getall", func() {
		alldef, err := GetCapabilitiesFromCluster(context.Background(), DefinitionNamespace, k8sClient, definitionDir, selector)
		Expect(err).Should(BeNil())
		logf.Log.Info(fmt.Sprintf("Getting all definitions %v", alldef))
		for i := range alldef {
			alldef[i].CueTemplate = ""
			alldef[i].DefinitionPath = ""
		}
		Expect(alldef).Should(Equal([]types.Capability{deployment, websvc, route}))
	})

})
