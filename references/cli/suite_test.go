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

package cli

import (
	"context"
	"math/rand"
	"testing"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/utils/pointer"
	"sigs.k8s.io/controller-runtime/pkg/envtest"

	"github.com/oam-dev/kubevela/apis/types"
	"github.com/oam-dev/kubevela/pkg/utils/common"
)

func TestCli(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Cli Suite")
}

var testEnv *envtest.Environment

var _ = BeforeSuite(func() {
	rand.Seed(time.Now().UnixNano())
	By("bootstrapping test environment")

	testEnv = &envtest.Environment{
		ControlPlaneStartTimeout: time.Minute * 3,
		ControlPlaneStopTimeout:  time.Minute,
		UseExistingCluster:       pointer.Bool(false),
		CRDDirectoryPaths:        []string{"../../charts/vela-core/crds"},
	}

	By("start kube test env")
	var err error
	cfg, err = testEnv.Start()
	Expect(err).ShouldNot(HaveOccurred())
	Expect(cfg).ToNot(BeNil())
	common.SetConfig(cfg)

	By("new clients")
	cfg.Timeout = time.Minute * 2
	cli = common.DynamicClient()
	Expect(cli).ToNot(BeNil())
	dm = common.DiscoveryMapper()
	Expect(dm).ToNot(BeNil())
	pd = common.PackageDiscover()
	Expect(pd).ToNot(BeNil())

	By("new namespace")
	err = cli.Create(context.TODO(), &corev1.Namespace{
		ObjectMeta: v1.ObjectMeta{Name: types.DefaultKubeVelaNS},
	})
	Expect(err).Should(BeNil())
})

var _ = AfterSuite(func() {
	if testEnv != nil {
		err := testEnv.Stop()
		Expect(err).Should(BeNil())
	}
})
