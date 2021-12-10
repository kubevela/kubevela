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

package resourcekeeper

import (
	"path/filepath"
	"testing"
	"time"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"k8s.io/utils/pointer"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/envtest"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"

	"github.com/oam-dev/kubevela/pkg/utils/common"
)

var testEnv *envtest.Environment
var testClient client.Client

func TestResourceKeeper(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "ResourceKeeper Suite")
}

var _ = BeforeSuite(func(done Done) {
	By("Bootstrapping test environment")
	testEnv = &envtest.Environment{
		ControlPlaneStartTimeout: time.Minute,
		ControlPlaneStopTimeout:  time.Minute,
		CRDDirectoryPaths: []string{
			filepath.Join("../..", "charts/vela-core/crds"), // this has all the required CRDs,
		},
		UseExistingCluster:    pointer.Bool(false),
		ErrorIfCRDPathMissing: true,
	}
	var err error
	cfg, err := testEnv.Start()
	Expect(err).ShouldNot(HaveOccurred())
	Expect(cfg).ShouldNot(BeNil())

	logf.SetLogger(zap.New(zap.UseDevMode(true), zap.WriteTo(GinkgoWriter)))

	testClient, err = client.New(cfg, client.Options{Scheme: common.Scheme})
	Expect(err).ShouldNot(HaveOccurred())
	Expect(testClient).ShouldNot(BeNil())

	close(done)
}, 300)

var _ = AfterSuite(func() {
	Expect(testEnv.Stop()).Should(Succeed())
})
