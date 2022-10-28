package service

import (
	"context"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"

	"github.com/oam-dev/kubevela/pkg/apiserver/infrastructure/datastore"
	"github.com/oam-dev/kubevela/pkg/oam/util"
)

var (
	defaultNamespace   = "project-default-ns1-test"
	pipelineService    *pipelineServiceImpl
	pipelineRunService *pipelineRunServiceImpl
	contextService     *contextServiceImpl
	projectService     *projectServiceImpl
)
var _ = Describe("Test pipeline service functions", func() {
	BeforeEach(func() {
		ds, err := NewDatastore(datastore.Config{Type: "kubeapi", Database: "target-test-kubevela"})
		Expect(ds).ToNot(BeNil())
		Expect(err).Should(BeNil())
		var ns = corev1.Namespace{}
		ns.Name = defaultNamespace
		err = k8sClient.Create(context.TODO(), &ns)
		Expect(err).Should(SatisfyAny(BeNil(), &util.AlreadyExistMatcher{}))
		pipelineService = NewTestPipelineService(ds, k8sClient, cfg).(*pipelineServiceImpl)
		pipelineRunService = pipelineService.PipelineRunService.(*pipelineRunServiceImpl)
		contextService = pipelineService.ContextService.(*contextServiceImpl)
		projectService = pipelineService.ProjectService.(*projectServiceImpl)

		pp, err := projectService.ListProjects(context.TODO(), 0, 0)
		Expect(err).Should(BeNil())
		// reset all projects
		for _, p := range pp.Projects {
			_ = projectService.DeleteProject(context.TODO(), p.Name)
		}
	})

})
