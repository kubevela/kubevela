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

package utils

import (
	"path/filepath"
	"testing"
	"time"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/rest"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/envtest"

	"github.com/kubevela/workflow/pkg/cue/packages"

	oamCore "github.com/oam-dev/kubevela/apis/core.oam.dev"
)

var cfg *rest.Config
var k8sClient client.Client
var testEnv *envtest.Environment
var pd *packages.PackageDiscover

func TestCapability(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Capability Suite")
}

var _ = BeforeSuite(func(done Done) {
	By("Bootstrapping test environment")
	useExistCluster := false
	testEnv = &envtest.Environment{
		ControlPlaneStartTimeout: time.Minute,
		ControlPlaneStopTimeout:  time.Minute,
		CRDDirectoryPaths: []string{
			filepath.Join("../../..", "charts/vela-core/crds"), // this has all the required CRDs,
		},
		UseExistingCluster: &useExistCluster,
	}
	var err error
	cfg, err = testEnv.Start()
	Expect(err).ToNot(HaveOccurred())
	Expect(cfg).ToNot(BeNil())

	err = oamCore.AddToScheme(scheme.Scheme)
	Expect(err).NotTo(HaveOccurred())

	By("Create the k8s client")
	k8sClient, err = client.New(cfg, client.Options{Scheme: scheme.Scheme})
	Expect(err).ToNot(HaveOccurred())
	Expect(k8sClient).ToNot(BeNil())

	pd, err = packages.NewPackageDiscover(cfg)
	Expect(err).ToNot(HaveOccurred())

	close(done)
}, 60)

var _ = AfterSuite(func() {
	By("Tearing down the test environment")
	err := testEnv.Stop()
	Expect(err).ToNot(HaveOccurred())
})
