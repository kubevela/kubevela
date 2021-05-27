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

package appdeployment

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/oam-dev/kubevela/apis/core.oam.dev/v1beta1"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/scale/scheme"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/envtest"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"
)

var decoder *admission.Decoder
var testScheme = runtime.NewScheme()
var testEnv *envtest.Environment
var cfg *rest.Config
var k8sClient client.Client
var handler *ValidatingHandler

func TestAppDeployment(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "AppDeployment Suite")
}

var _ = BeforeSuite(func(done Done) {
	var err error
	var yamlPath string
	if _, set := os.LookupEnv("COMPATIBILITY_TEST"); set {
		yamlPath = "../../../../../test/compatibility-test/testdata"
	} else {
		yamlPath = filepath.Join("../../../../..", "charts", "vela-core", "crds")
	}
	testEnv = &envtest.Environment{
		CRDDirectoryPaths: []string{yamlPath},
	}

	cfg, err = testEnv.Start()
	Expect(err).ToNot(HaveOccurred())
	Expect(cfg).ToNot(BeNil())

	err = v1beta1.SchemeBuilder.AddToScheme(testScheme)
	Expect(err).NotTo(HaveOccurred())
	err = scheme.AddToScheme(testScheme)
	Expect(err).NotTo(HaveOccurred())

	decoder, err = admission.NewDecoder(testScheme)
	Expect(err).Should(BeNil())
	handler = &ValidatingHandler{}
	Expect(handler.InjectDecoder(decoder)).Should(BeNil())

	k8sClient, err = client.New(cfg, client.Options{Scheme: testScheme})
	Expect(err).ToNot(HaveOccurred())
	Expect(k8sClient).ToNot(BeNil())
	Expect(handler.InjectClient(k8sClient)).Should(BeNil())

	close(done)
}, 60)

var _ = AfterSuite(func() {
	By("tearing down the test environment")
	err := testEnv.Stop()
	Expect(err).ToNot(HaveOccurred())
})
