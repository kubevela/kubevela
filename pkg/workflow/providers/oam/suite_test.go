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

package oam

import (
	"context"
	"encoding/json"
	"math/rand"
	"testing"
	"time"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	v1 "k8s.io/api/core/v1"
	v12 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/rest"
	"k8s.io/utils/pointer"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/envtest"

	"github.com/oam-dev/kubevela/apis/core.oam.dev/common"
	"github.com/oam-dev/kubevela/apis/core.oam.dev/v1beta1"
	pkgcommon "github.com/oam-dev/kubevela/pkg/utils/common"
)

var cfg *rest.Config
var k8sClient client.Client
var testEnv *envtest.Environment

func TestUtils(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Utils Suite")
}

var _ = BeforeSuite(func(done Done) {
	rand.Seed(time.Now().UnixNano())
	By("bootstrapping test environment for utils test")

	testEnv = &envtest.Environment{
		ControlPlaneStartTimeout: time.Minute * 3,
		ControlPlaneStopTimeout:  time.Minute,
		UseExistingCluster:       pointer.BoolPtr(false),
		CRDDirectoryPaths:        []string{"./testdata"},
	}

	By("start kube test env")
	var err error
	cfg, err = testEnv.Start()
	Expect(err).ShouldNot(HaveOccurred())
	Expect(cfg).ToNot(BeNil())

	By("new kube client")
	cfg.Timeout = time.Minute * 2
	k8sClient, err = client.New(cfg, client.Options{Scheme: pkgcommon.Scheme})
	Expect(err).Should(Succeed())
	close(done)
}, 240)

var _ = AfterSuite(func() {
	By("tearing down the test environment")
	err := testEnv.Stop()
	Expect(err).ToNot(HaveOccurred())
})

var _ = Describe("Test load dynamic components", func() {
	It("Test load dynamic components", func() {
		By("Create objects")
		Expect(k8sClient.Create(context.Background(), &v1.Namespace{ObjectMeta: v12.ObjectMeta{Name: "test"}})).Should(Succeed())
		for _, obj := range []client.Object{&v1.ConfigMap{
			ObjectMeta: v12.ObjectMeta{
				Name:      "dynamic",
				Namespace: "test",
			},
		}, &v1.Service{
			ObjectMeta: v12.ObjectMeta{
				Name:       "dynamic",
				Namespace:  "test",
				Generation: int64(5),
			},
			Spec: v1.ServiceSpec{
				ClusterIP: "10.0.0.254",
				Ports:     []v1.ServicePort{{Port: 80}},
			},
		}, &v1.ConfigMap{
			ObjectMeta: v12.ObjectMeta{
				Name:      "by-label-1",
				Namespace: "test",
				Labels:    map[string]string{"key": "value"},
			},
		}, &v1.ConfigMap{
			ObjectMeta: v12.ObjectMeta{
				Name:      "by-label-2",
				Namespace: "test",
				Labels:    map[string]string{"key": "value"},
			},
		}} {
			Expect(k8sClient.Create(context.Background(), obj)).Should(Succeed())
		}
		testcases := map[string]struct {
			Input     *common.ApplicationComponent
			Output    *common.ApplicationComponent
			Error     string
			IsService bool
		}{
			"normal": {
				Input: &common.ApplicationComponent{
					Type:       "ref-objects",
					Properties: &runtime.RawExtension{Raw: []byte(`{"objects":[{"apiVersion":"v1","kind":"ConfigMap","name":"dynamic"}]}`)},
				},
				Output: &common.ApplicationComponent{
					Type:       "ref-objects",
					Properties: &runtime.RawExtension{Raw: []byte(`{"objects":[{"apiVersion":"v1","kind":"ConfigMap","metadata":{"name":"dynamic","namespace":"test"}}]}`)},
				},
			},
			"bad-properties": {
				Input: &common.ApplicationComponent{
					Type:       "ref-objects",
					Properties: &runtime.RawExtension{Raw: []byte(`{bad}`)},
				},
				Error: "invalid properties for ref-objects",
			},
			"name-and-selector-both-set": {
				Input: &common.ApplicationComponent{
					Type:       "ref-objects",
					Properties: &runtime.RawExtension{Raw: []byte(`{"objects":[{"apiVersion":"v1","kind":"ConfigMap","name":"dynamic","selector":{"key":"value"}}]}`)},
				},
				Error: "invalid properties for ref-objects, name and selector cannot be both set",
			},
			"empty-ref-object-name": {
				Input: &common.ApplicationComponent{
					Type:       "ref-objects",
					Name:       "dynamic",
					Properties: &runtime.RawExtension{Raw: []byte(`{"objects":[{"apiVersion":"v1","kind":"ConfigMap"}]}`)},
				},
				Output: &common.ApplicationComponent{
					Type:       "ref-objects",
					Name:       "dynamic",
					Properties: &runtime.RawExtension{Raw: []byte(`{"objects":[{"apiVersion":"v1","kind":"ConfigMap","metadata":{"name":"dynamic","namespace":"test"}}]}`)},
				},
			},
			"cannot-find-ref-object": {
				Input: &common.ApplicationComponent{
					Type:       "ref-objects",
					Properties: &runtime.RawExtension{Raw: []byte(`{"objects":[{"apiVersion":"v1","kind":"ConfigMap","name":"static"}]}`)},
				},
				Error: "failed to load ref object",
			},
			"modify-service": {
				Input: &common.ApplicationComponent{
					Type:       "ref-objects",
					Properties: &runtime.RawExtension{Raw: []byte(`{"objects":[{"apiVersion":"v1","kind":"Service","name":"dynamic"}]}`)},
				},
				IsService: true,
			},
			"by-labels": {
				Input: &common.ApplicationComponent{
					Type:       "ref-objects",
					Name:       "dynamic",
					Properties: &runtime.RawExtension{Raw: []byte(`{"objects":[{"apiVersion":"v1","kind":"ConfigMap","selector":{"key":"value"}}]}`)},
				},
				Output: &common.ApplicationComponent{
					Type:       "ref-objects",
					Name:       "dynamic",
					Properties: &runtime.RawExtension{Raw: []byte(`{"objects":[{"apiVersion":"v1","kind":"ConfigMap","metadata":{"labels":{"key":"value"},"name":"by-label-1","namespace":"test"}},{"apiVersion":"v1","kind":"ConfigMap","metadata":{"labels":{"key":"value"},"name":"by-label-2","namespace":"test"}}]}`)},
				},
			},
		}
		p := provider{app: &v1beta1.Application{}, cli: k8sClient}
		p.app.SetNamespace("test")
		for name, tt := range testcases {
			By("Test " + name)
			output, err := p.loadDynamicComponent(tt.Input)
			if tt.Error != "" {
				Expect(err).ShouldNot(BeNil())
				Expect(err.Error()).Should(ContainSubstring(tt.Error))
			} else {
				Expect(err).Should(Succeed())
				if tt.IsService {
					svc := &v1.Service{}
					Expect(json.Unmarshal(output.Properties.Raw, svc)).Should(Succeed())
					Expect(svc.Spec.ClusterIP).Should(BeEmpty())
				} else {
					Expect(output).Should(Equal(tt.Output))
				}
			}
		}
	})
})
