package plugins

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"cuelang.org/go/cue"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/selection"
	logf "sigs.k8s.io/controller-runtime/pkg/log"

	"github.com/oam-dev/kubevela/apis/types"
)

const (
	TestDir        = "testdata"
	RouteName      = "routes.test"
	DeployName     = "deployments.testapps"
	WebserviceName = "webservice.testapps"
)

var _ = Describe("DefinitionFiles", func() {
	route := types.Capability{
		Name: RouteName,
		Type: types.TypeTrait,
		Parameters: []types.Parameter{
			{
				Name:     "domain",
				Required: true,
				Default:  "",
				Type:     cue.StringKind,
			},
		},
		Description: "description not defined",
		CrdName:     "routes.standard.oam.dev",
		CrdInfo: &types.CRDInfo{
			APIVersion: "standard.oam.dev/v1alpha1",
			Kind:       "Route",
		},
	}

	deployment := types.Capability{
		Name:        DeployName,
		Type:        types.TypeWorkload,
		CrdName:     "deployments.apps",
		Description: "description not defined",
		Parameters: []types.Parameter{
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
				Usage:    "Which image would you like to use for your service",
			},
			{
				Name:    "port",
				Type:    cue.IntKind,
				Short:   "p",
				Default: int64(8080),
				Usage:   "Which port do you want customer traffic sent to",
			},
		},
		CrdInfo: &types.CRDInfo{
			APIVersion: "apps/v1",
			Kind:       "Deployment",
		},
	}

	websvc := types.Capability{
		Name:        WebserviceName,
		Type:        types.TypeWorkload,
		Description: "description not defined",
		Parameters: []types.Parameter{{
			Name: "env", Type: cue.ListKind,
		}, {
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
		}},
		CrdName: "deployments.apps",
		CrdInfo: &types.CRDInfo{
			APIVersion: "apps/v1",
			Kind:       "Deployment",
		},
	}

	req, _ := labels.NewRequirement("usecase", selection.Equals, []string{"forplugintest"})
	selector := labels.NewSelector().Add(*req)

	// Notice!!  DefinitionPath Object is Cluster Scope object
	// which means objects created in other DefinitionNamespace will also affect here.
	It("gettrait", func() {
		traitDefs, _, err := GetTraitsFromCluster(context.Background(), DefinitionNamespace, types.Args{Config: cfg, Schema: scheme}, definitionDir, selector)
		Expect(err).Should(BeNil())
		logf.Log.Info(fmt.Sprintf("Getting trait definitions %v", traitDefs))
		for i := range traitDefs {
			// CueTemplate should always be fulfilled, even those whose CueTemplateURI is assigend,
			By("check CueTemplate is fulfilled")
			Expect(traitDefs[i].CueTemplate).ShouldNot(BeEmpty())
			traitDefs[i].CueTemplate = ""
			traitDefs[i].DefinitionPath = ""
		}
		Expect(traitDefs).Should(Equal([]types.Capability{route}))
	})

	// Notice!!  DefinitionPath Object is Cluster Scope object
	// which means objects created in other DefinitionNamespace will also affect here.
	It("getworkload", func() {
		workloadDefs, _, err := GetWorkloadsFromCluster(context.Background(), DefinitionNamespace, types.Args{Config: cfg, Schema: scheme}, definitionDir, selector)
		Expect(err).Should(BeNil())
		logf.Log.Info(fmt.Sprintf("Getting workload definitions  %v", workloadDefs))
		for i := range workloadDefs {
			// CueTemplate should always be fulfilled, even those whose CueTemplateURI is assigend,
			By("check CueTemplate is fulfilled")
			Expect(workloadDefs[i].CueTemplate).ShouldNot(BeEmpty())
			workloadDefs[i].CueTemplate = ""
			workloadDefs[i].DefinitionPath = ""
		}
		Expect(workloadDefs).Should(Equal([]types.Capability{deployment, websvc}))
	})
	It("getall", func() {
		alldef, err := GetCapabilitiesFromCluster(context.Background(), DefinitionNamespace, types.Args{Config: cfg, Schema: scheme}, definitionDir, selector)
		Expect(err).Should(BeNil())
		logf.Log.Info(fmt.Sprintf("Getting all definitions %v", alldef))
		for i := range alldef {
			alldef[i].CueTemplate = ""
			alldef[i].DefinitionPath = ""
		}
		Expect(alldef).Should(Equal([]types.Capability{deployment, websvc, route}))
	})
	It("SyncDefinitionsToLocal", func() {
		localDefinitionDir := "testdata/capabilities"
		if _, err := os.Stat(localDefinitionDir); err != nil && os.IsNotExist(err) {
			os.MkdirAll(localDefinitionDir, 0750)
		}
		syncedTemplates, _, err := SyncDefinitionsToLocal(context.Background(),
			types.Args{Config: cfg, Schema: scheme}, localDefinitionDir)

		var containRoute, containDeploy, containWebservice bool
		for _, t := range syncedTemplates {
			switch t.Name {
			case RouteName:
				containRoute = true
			case DeployName:
				containDeploy = true
			case WebserviceName:
				containWebservice = true
			}
		}
		Expect(containRoute).Should(Equal(true))
		Expect(containDeploy).Should(Equal(true))
		Expect(containWebservice).Should(Equal(true))
		Expect(err).Should(BeNil())
		_, err = os.Stat(filepath.Join(localDefinitionDir, "workloads", DeployName))
		Expect(err).Should(BeNil())
		_, err = os.Stat(filepath.Join(localDefinitionDir, "workloads", WebserviceName))
		Expect(err).Should(BeNil())
		_, err = os.Stat(filepath.Join(localDefinitionDir, "traits", RouteName))
		Expect(err).Should(BeNil())
		if _, err := os.Stat(localDefinitionDir); err == nil {
			os.RemoveAll(localDefinitionDir)
		}
	})
	It("SyncDefinitionToLocal", func() {
		localDefinitionDir := "testdata/capabilities"
		if _, err := os.Stat(localDefinitionDir); err != nil && os.IsNotExist(err) {
			os.MkdirAll(localDefinitionDir, 0750)
		}
		template, err := SyncDefinitionToLocal(context.Background(),
			types.Args{Config: cfg, Schema: scheme}, localDefinitionDir, RouteName)
		Expect(err).Should(BeNil())
		Expect(template.Name).Should(Equal(RouteName))
		_, err = os.Stat(filepath.Join(localDefinitionDir, fmt.Sprintf("%s.cue", RouteName)))
		Expect(err).Should(BeNil())
		if _, err := os.Stat(localDefinitionDir); err == nil {
			os.RemoveAll(localDefinitionDir)
		}
	})
})
