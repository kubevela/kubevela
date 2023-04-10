/*
 Copyright 2021. The KubeVela Authors.

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

package query

import (
	"context"
	"testing"
	"time"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	batchv1 "k8s.io/api/batch/v1"
	"k8s.io/client-go/rest"
	"k8s.io/utils/pointer"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/envtest"

	"github.com/oam-dev/kubevela/pkg/utils/common"
)

var cfg *rest.Config
var k8sClient client.Client
var testEnv *envtest.Environment
var ctx context.Context

var _ = BeforeSuite(func(done Done) {
	By("bootstrapping test environment")

	testEnv = &envtest.Environment{
		ControlPlaneStartTimeout: time.Minute * 3,
		ControlPlaneStopTimeout:  time.Minute,
		UseExistingCluster:       pointer.Bool(false),
		CRDDirectoryPaths: []string{
			"./testdata/gateway/crds",
			"../../../../charts/vela-core/crds",
			"./testdata/machinelearning.seldon.io_seldondeployments.yaml",
			"./testdata/helm-release-crd.yaml",
		},
	}

	By("start kube test env")
	var err error
	cfg, err = testEnv.Start()
	Expect(err).ShouldNot(HaveOccurred())
	Expect(cfg).ToNot(BeNil())

	By("new kube client")
	cfg.Timeout = time.Minute * 2

	scheme := common.Scheme
	batchv1.AddToScheme(scheme)

	k8sClient, err = client.New(cfg, client.Options{Scheme: scheme})

	Expect(err).Should(BeNil())
	Expect(k8sClient).ToNot(BeNil())

	ctx = context.Background()
	Expect(err).To(BeNil())
	close(done)
}, 240)

var _ = AfterSuite(func() {
	By("tearing down the test environment")
	err := testEnv.Stop()
	Expect(err).ToNot(HaveOccurred())
})

func TestQueryProvider(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "VelaQL Suite")
}
