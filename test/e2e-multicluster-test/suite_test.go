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

package e2e_multicluster_test

import (
	"context"
	"testing"
	"time"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/utils/strings/slices"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/config"

	"github.com/kubevela/pkg/multicluster"

	"github.com/oam-dev/kubevela/apis/core.oam.dev/v1beta1"
	"github.com/oam-dev/kubevela/apis/types"
	multicluster2 "github.com/oam-dev/kubevela/pkg/multicluster"
	oamutil "github.com/oam-dev/kubevela/pkg/oam/util"
	"github.com/oam-dev/kubevela/pkg/utils/common"
	"github.com/oam-dev/kubevela/pkg/utils/util"
	"github.com/oam-dev/kubevela/references/cli"
)

const (
	WorkerClusterName           = "cluster-worker"
	WorkerClusterKubeConfigPath = "/tmp/worker.kubeconfig"
)

var (
	k8sClient client.Client
	k8sCli    kubernetes.Interface
)

func execCommand(args ...string) (string, error) {
	ioStream, buf := util.NewTestIOStreams()
	command := cli.NewCommandWithIOStreams(ioStream)
	command.SetArgs(args)
	command.SetOut(buf)
	command.SetErr(buf)
	err := command.Execute()
	return buf.String(), err
}

func TestMulticluster(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "KubeVela MultiCluster Test Suite")
}

var _ = BeforeSuite(func() {
	var err error

	// initialize clients
	options := client.Options{Scheme: common.Scheme}
	config := config.GetConfigOrDie()
	config.Wrap(multicluster.NewTransportWrapper())
	k8sClient, err = client.New(config, options)
	Expect(err).Should(Succeed())
	k8sCli, err = kubernetes.NewForConfig(config)
	Expect(err).Should(Succeed())

	// join worker cluster
	_, err = execCommand("cluster", "join", WorkerClusterKubeConfigPath, "--name", WorkerClusterName)
	Expect(err).Should(Succeed())
	cv, err := multicluster2.GetVersionInfoFromCluster(context.Background(), WorkerClusterName, config)
	Expect(err).Should(Succeed())
	Expect(cv.Minor).Should(Not(BeEquivalentTo("")))
	Expect(cv.Major).Should(BeEquivalentTo("1"))
	ocv := multicluster2.GetVersionInfoFromObject(context.Background(), k8sClient, WorkerClusterName)
	Expect(ocv).Should(BeEquivalentTo(cv))
})

var _ = AfterSuite(func() {
	holdAddons := []string{"addon-terraform", "addon-fluxcd"}
	Eventually(func(g Gomega) {
		apps := &v1beta1.ApplicationList{}
		g.Expect(k8sClient.List(context.Background(), apps)).Should(Succeed())
		for _, app := range apps.Items {
			if slices.Contains(holdAddons, app.Name) {
				continue
			}
			g.Expect(k8sClient.Delete(context.Background(), app.DeepCopy())).Should(Succeed())
		}
	}, 3*time.Minute).Should(Succeed())
	Eventually(func(g Gomega) {
		// Delete terraform and fluxcd in order
		app := &v1beta1.Application{}
		apps := &v1beta1.ApplicationList{}
		for _, addon := range holdAddons {
			g.Expect(k8sClient.Delete(context.Background(), &v1beta1.Application{ObjectMeta: v1.ObjectMeta{Name: addon, Namespace: types.DefaultKubeVelaNS}})).Should(SatisfyAny(Succeed(), oamutil.NotFoundMatcher{}))
			g.Expect(k8sClient.Get(context.Background(), client.ObjectKey{Name: addon, Namespace: types.DefaultKubeVelaNS}, app)).Should(SatisfyAny(Succeed(), oamutil.NotFoundMatcher{}))
		}
		err := k8sClient.List(context.Background(), apps)
		g.Expect(err, nil)
		g.Expect(len(apps.Items)).Should(Equal(0))
	}, 5*time.Minute).Should(Succeed())
	Eventually(func(g Gomega) {
		_, err := execCommand("cluster", "detach", WorkerClusterName)
		g.Expect(err).Should(Succeed())
	}, time.Minute).Should(Succeed())
})
