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

package velaql

import (
	"context"
	"math/rand"
	"testing"
	"time"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/rest"
	"k8s.io/utils/pointer"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/envtest"

	"github.com/kubevela/workflow/pkg/cue/packages"

	"github.com/oam-dev/kubevela/pkg/oam/discoverymapper"
	"github.com/oam-dev/kubevela/pkg/utils/common"
)

var cfg *rest.Config
var k8sClient client.Client
var testEnv *envtest.Environment
var viewHandler *ViewHandler
var pod corev1.Pod
var readView corev1.ConfigMap
var applyView corev1.ConfigMap

var _ = BeforeSuite(func(done Done) {
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

	By("new kube client")
	cfg.Timeout = time.Minute * 2
	k8sClient, err = client.New(cfg, client.Options{Scheme: common.Scheme})
	Expect(err).Should(BeNil())
	Expect(k8sClient).ToNot(BeNil())
	By("new kube client success")

	dm, err := discoverymapper.New(cfg)
	Expect(err).To(BeNil())
	pd, err := packages.NewPackageDiscover(cfg)
	Expect(err).To(BeNil())

	viewHandler = NewViewHandler(k8sClient, cfg, dm, pd)
	ctx := context.Background()

	ns := corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "vela-system"}}
	Expect(k8sClient.Create(ctx, &ns)).Should(BeNil())

	Expect(common.ReadYamlToObject("./testdata/example-pod.yaml", &pod)).Should(BeNil())
	Expect(k8sClient.Create(ctx, &pod)).Should(BeNil())

	Expect(common.ReadYamlToObject("./testdata/read-object.yaml", &readView)).Should(BeNil())
	Expect(k8sClient.Create(ctx, &readView)).Should(BeNil())

	Expect(common.ReadYamlToObject("./testdata/apply-object.yaml", &applyView)).Should(BeNil())
	Expect(k8sClient.Create(ctx, &applyView)).Should(BeNil())
	close(done)
}, 240)

var _ = AfterSuite(func() {
	By("tearing down the test environment")
	err := testEnv.Stop()
	Expect(err).ToNot(HaveOccurred())
})

func TestVelaQL(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "VelaQL Suite")
}
