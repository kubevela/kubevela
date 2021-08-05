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

package envbinding

import (
	"context"
	"fmt"
	"math/rand"
	"path/filepath"
	"strconv"
	"testing"
	"time"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"k8s.io/client-go/rest"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/envtest"

	"github.com/oam-dev/kubevela/pkg/cue/packages"
	"github.com/oam-dev/kubevela/pkg/oam/discoverymapper"
	"github.com/oam-dev/kubevela/pkg/utils/common"
)

var cfg *rest.Config
var k8sClient client.Client
var testEnv *envtest.Environment
var controllerDone context.CancelFunc
var r Reconciler

func TestEnvBinding(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "EnvBinding Suite")
}

var _ = BeforeSuite(func(done Done) {
	By("Bootstrapping test environment")
	useExistCluster := false
	testEnv = &envtest.Environment{
		ControlPlaneStartTimeout: time.Minute,
		ControlPlaneStopTimeout:  time.Minute,
		CRDDirectoryPaths: []string{
			filepath.Join("../../../../..", "charts/vela-core/crds"), // this has all the required CRDs,
			"./testdata/crds",
		},
		UseExistingCluster: &useExistCluster,
	}
	var err error
	cfg, err = testEnv.Start()
	Expect(err).ToNot(HaveOccurred())
	Expect(cfg).ToNot(BeNil())

	By("Create the k8s client")
	k8sClient, err = client.New(cfg, client.Options{Scheme: common.Scheme})
	Expect(err).ToNot(HaveOccurred())
	Expect(k8sClient).ToNot(BeNil())

	By("Starting the controller in the background")
	mgr, err := ctrl.NewManager(cfg, ctrl.Options{
		Scheme:             common.Scheme,
		MetricsBindAddress: "0",
		Port:               48081,
	})
	Expect(err).ToNot(HaveOccurred())
	dm, err := discoverymapper.New(mgr.GetConfig())
	Expect(err).ToNot(HaveOccurred())
	_, err = dm.Refresh()
	Expect(err).ToNot(HaveOccurred())
	pd, err := packages.NewPackageDiscover(cfg)
	Expect(err).ToNot(HaveOccurred())

	r = Reconciler{
		Client: mgr.GetClient(),
		Scheme: mgr.GetScheme(),
		dm:     dm,
		pd:     pd,
	}
	Expect(r.SetupWithManager(mgr)).ToNot(HaveOccurred())
	var ctx context.Context
	ctx, controllerDone = context.WithCancel(context.Background())
	go func() {
		defer GinkgoRecover()
		Expect(mgr.Start(ctx)).ToNot(HaveOccurred())
	}()

	close(done)
}, 120)

var _ = AfterSuite(func() {
	By("Stop the controller")
	controllerDone()

	By("Tearing down the test environment")
	err := testEnv.Stop()
	Expect(err).ToNot(HaveOccurred())
})

func randomNamespaceName(basic string) string {
	return fmt.Sprintf("%s-%s", basic, strconv.FormatInt(rand.Int63(), 16))
}
