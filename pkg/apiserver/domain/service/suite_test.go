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
	"context"
	"fmt"
	"math/rand"
	"os"
	"strconv"
	"testing"
	"time"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	k8stypes "k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/rest"
	"k8s.io/utils/pointer"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/envtest"
	"sigs.k8s.io/yaml"

	"github.com/oam-dev/kubevela/apis/core.oam.dev/v1beta1"
	"github.com/oam-dev/kubevela/apis/types"
	"github.com/oam-dev/kubevela/pkg/apiserver/infrastructure/clients"
	"github.com/oam-dev/kubevela/pkg/apiserver/infrastructure/datastore"
	"github.com/oam-dev/kubevela/pkg/apiserver/infrastructure/datastore/kubeapi"
	"github.com/oam-dev/kubevela/pkg/apiserver/infrastructure/datastore/mongodb"
	"github.com/oam-dev/kubevela/pkg/utils/common"
)

var cfg *rest.Config
var k8sClient client.Client
var testEnv *envtest.Environment

func TestService(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Service Suite")
}

var _ = BeforeSuite(func(done Done) {
	rand.Seed(time.Now().UnixNano())
	By("bootstrapping test environment")

	testEnv = &envtest.Environment{
		ControlPlaneStartTimeout: time.Minute * 3,
		ControlPlaneStopTimeout:  time.Minute,
		UseExistingCluster:       pointer.BoolPtr(false),
		CRDDirectoryPaths:        []string{"../../../../charts/vela-core/crds"},
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
	clients.SetKubeClient(k8sClient)
	Expect(err).Should(BeNil())

	initDefinitions(k8sClient)

	close(done)
}, 240)

var _ = AfterSuite(func() {
	By("tearing down the test environment")
	err := testEnv.Stop()
	Expect(err).ToNot(HaveOccurred())
})

func NewDatastore(cfg datastore.Config) (ds datastore.DataStore, err error) {
	switch cfg.Type {
	case "mongodb":
		ds, err = mongodb.New(context.Background(), cfg)
		if err != nil {
			return nil, fmt.Errorf("create mongodb datastore instance failure %w", err)
		}
	case "kubeapi":
		ds, err = kubeapi.New(context.Background(), cfg)
		if err != nil {
			return nil, fmt.Errorf("create mongodb datastore instance failure %w", err)
		}
	default:
		return nil, fmt.Errorf("not support datastore type %s", cfg.Type)
	}
	return ds, nil
}

func randomNamespaceName(basic string) string {
	return fmt.Sprintf("%s-%s", basic, strconv.FormatInt(rand.Int63(), 16))
}

func initDefinitions(k8sClient client.Client) {
	var namespace corev1.Namespace
	err := k8sClient.Get(context.TODO(), k8stypes.NamespacedName{Name: types.DefaultKubeVelaNS}, &namespace)
	if apierrors.IsNotFound(err) {
		err := k8sClient.Create(context.TODO(), &corev1.Namespace{
			ObjectMeta: metav1.ObjectMeta{
				Name: types.DefaultKubeVelaNS,
			},
		})
		Expect(err).Should(BeNil())
	} else {
		Expect(err).Should(BeNil())
	}
	webservice, err := os.ReadFile("./testdata/webservice.yaml")
	Expect(err).Should(BeNil())
	var cd v1beta1.ComponentDefinition
	err = yaml.Unmarshal(webservice, &cd)
	Expect(err).Should(BeNil())
	Expect(k8sClient.Create(context.TODO(), &cd))

	scaler, err := os.ReadFile("./testdata/scaler.yaml")
	Expect(err).Should(BeNil())
	var td v1beta1.TraitDefinition
	err = yaml.Unmarshal(scaler, &td)
	Expect(err).Should(BeNil())
	Expect(k8sClient.Create(context.TODO(), &td))

	deploy, err := os.ReadFile("./testdata/deploy.yaml")
	Expect(err).Should(BeNil())
	var wsd v1beta1.WorkflowStepDefinition
	err = yaml.Unmarshal(deploy, &wsd)
	Expect(err).Should(BeNil())
	Expect(k8sClient.Create(context.TODO(), &wsd))
}
