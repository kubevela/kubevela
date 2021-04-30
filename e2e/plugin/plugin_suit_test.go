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

package plugin

import (
	"context"
	"io/ioutil"
	"os"
	"testing"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"k8s.io/apimachinery/pkg/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/config"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/yaml"

	core "github.com/oam-dev/kubevela/apis/core.oam.dev"
	"github.com/oam-dev/kubevela/apis/core.oam.dev/v1beta1"
	"github.com/oam-dev/kubevela/pkg/oam/util"
)

var k8sClient client.Client
var scheme = runtime.NewScheme()
var ctx = context.Background()
var app v1beta1.Application
var testShowCdDef v1beta1.ComponentDefinition
var testShowTdDef v1beta1.TraitDefinition
var testCdDef v1beta1.ComponentDefinition
var testCdDefWithHelm v1beta1.ComponentDefinition
var testTdDef v1beta1.TraitDefinition

func TestKubectlPlugin(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Kubectl Plugin Suite")
}

var _ = BeforeSuite(func(done Done) {
	err := clientgoscheme.AddToScheme(scheme)
	Expect(err).Should(BeNil())
	err = core.AddToScheme(scheme)
	Expect(err).Should(BeNil())

	By("Setting up kubernetes client")
	k8sClient, err = client.New(config.GetConfigOrDie(), client.Options{Scheme: scheme})
	if err != nil {
		logf.Log.Error(err, "failed to create k8sClient")
		Fail("setup failed")
	}

	err = os.MkdirAll("definitions", os.ModePerm)
	Expect(err).NotTo(HaveOccurred())
	err = ioutil.WriteFile("definitions/webservice.yaml", []byte(componentDef), 0644)
	Expect(err).NotTo(HaveOccurred())
	err = ioutil.WriteFile("definitions/ingress.yaml", []byte(traitDef), 0644)
	Expect(err).NotTo(HaveOccurred())

	By("apply test definitions")
	Expect(yaml.Unmarshal([]byte(componentDef), &testCdDef)).Should(BeNil())
	err = k8sClient.Create(ctx, &testCdDef)
	Expect(err).Should(SatisfyAny(BeNil(), &util.AlreadyExistMatcher{}))

	Expect(yaml.Unmarshal([]byte(componentDefWithHelm), &testCdDefWithHelm)).Should(BeNil())
	err = k8sClient.Create(ctx, &testCdDefWithHelm)
	Expect(err).Should(SatisfyAny(BeNil(), &util.AlreadyExistMatcher{}))

	Expect(yaml.Unmarshal([]byte(traitDef), &testTdDef)).Should(BeNil())
	err = k8sClient.Create(ctx, &testTdDef)
	Expect(err).Should(SatisfyAny(BeNil(), &util.AlreadyExistMatcher{}))

	Expect(yaml.Unmarshal([]byte(testShowComponentDef), &testShowCdDef)).Should(BeNil())
	err = k8sClient.Create(ctx, &testShowCdDef)
	Expect(err).Should(SatisfyAny(BeNil(), &util.AlreadyExistMatcher{}))

	Expect(yaml.Unmarshal([]byte(testShowTraitDef), &testShowTdDef)).Should(BeNil())
	err = k8sClient.Create(ctx, &testShowTdDef)
	Expect(err).Should(SatisfyAny(BeNil(), &util.AlreadyExistMatcher{}))

	By("apply test application")
	Expect(yaml.Unmarshal([]byte(application), &app)).Should(BeNil())
	err = k8sClient.Create(ctx, &app)
	Expect(err).Should(SatisfyAny(BeNil(), &util.AlreadyExistMatcher{}))

	close(done)
}, 300)

var _ = AfterSuite(func() {
	By("delete application and definitions")
	Expect(k8sClient.Delete(ctx, &app)).Should(BeNil())
	Expect(k8sClient.Delete(ctx, &testCdDef)).Should(BeNil())
	Expect(k8sClient.Delete(ctx, &testCdDefWithHelm)).Should(BeNil())
	Expect(k8sClient.Delete(ctx, &testTdDef)).Should(BeNil())
	Expect(k8sClient.Delete(ctx, &testShowCdDef)).Should(BeNil())
	Expect(k8sClient.Delete(ctx, &testShowTdDef)).Should(BeNil())
})
