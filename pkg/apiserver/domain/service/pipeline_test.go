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

package service

import (
	"github.com/oam-dev/kubevela/pkg/apiserver/infrastructure/datastore"
	"github.com/oam-dev/kubevela/pkg/oam/util"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
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
		Expect(err).Should(SatisfyAny(BeNil(), &util.AlreadyExistMatcher{}))
		pipelineService = NewTestPipelineService(ds, k8sClient, cfg).(*pipelineServiceImpl)
		pipelineRunService = pipelineService.PipelineRunService.(*pipelineRunServiceImpl)
		contextService = pipelineService.ContextService.(*contextServiceImpl)
		projectService = pipelineService.ProjectService.(*projectServiceImpl)
	})

})
