/*
Copyright 2023 The KubeVela Authors.

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

package config

import (
	"context"
	"testing"
	"time"

	"github.com/kubevela/workflow/pkg/mock"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"k8s.io/client-go/rest"
	"k8s.io/utils/ptr"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/envtest"

	configv1alpha1 "github.com/oam-dev/kubevela/apis/config.oam.dev/v1alpha1"
	"github.com/oam-dev/kubevela/pkg/config"
	"github.com/oam-dev/kubevela/pkg/utils/common"
	oamprovidertypes "github.com/oam-dev/kubevela/pkg/workflow/providers/types"
)

var cfg *rest.Config
var k8sClient client.Client
var testEnv *envtest.Environment
var factory config.Factory

func TestProvider(t *testing.T) {
	RegisterFailHandler(Fail)

	RunSpecs(t, "Test Legacy Config Suite")
}

var _ = BeforeSuite(func() {
	By("Bootstrapping test environment")
	testEnv = &envtest.Environment{
		ControlPlaneStartTimeout: time.Minute,
		ControlPlaneStopTimeout:  time.Minute,
		UseExistingCluster:       ptr.To(false),
		CRDDirectoryPaths:        []string{"../../../../../charts/vela-core/crds"},
	}
	var err error
	cfg, err = testEnv.Start()
	Expect(err).ToNot(HaveOccurred())
	Expect(cfg).ToNot(BeNil())
	// the shared scheme: the migration library the adopt-first step uses reads core.oam.dev
	// objects (the distribution Application) as well as config.oam.dev ones
	By("Create the k8s client")
	k8sClient, err = client.New(cfg, client.Options{Scheme: common.Scheme})
	Expect(err).ToNot(HaveOccurred())
	Expect(k8sClient).ToNot(BeNil())

	factory = config.NewConfigFactory(k8sClient)
})

var _ = AfterSuite(func() {
	By("Tearing down the test environment")
	err := testEnv.Stop()
	Expect(err).ToNot(HaveOccurred())
})

// The behaviour is pkg/workflow/providers/config's and is tested there; these specs pin
// that the legacy names reach it with the legacy parameter shape.
var _ = Describe("Test the legacy config provider", func() {
	ctx := context.Background()
	runtimeParams := func(act *mock.Action) oamprovidertypes.RuntimeParams {
		return oamprovidertypes.RuntimeParams{ConfigFactory: factory, KubeClient: k8sClient, Action: act}
	}

	It("GetProviders returns expected hyphenated keys", func() {
		providers := GetProviders()
		Expect(providers).To(HaveKey("create-config"))
		Expect(providers).To(HaveKey("read-config"))
		Expect(providers).To(HaveKey("list-config"))
		Expect(providers).To(HaveKey("delete-config"))
		Expect(providers).To(HaveLen(4))
	})

	It("create-config writes a Config CR and waits for it", func() {
		template, err := factory.ParseTemplate(ctx, "test-plain", []byte(templateContent))
		Expect(err).ToNot(HaveOccurred())
		Expect(factory.CreateOrUpdateConfigTemplate(ctx, "default", template)).ToNot(HaveOccurred())

		act := &mock.Action{}
		_, err = CreateConfig(ctx, &CreateParams{
			Params: CreateConfigProperties{
				Name: "legacy-step", Namespace: "default", Template: "default/test-plain",
				Config: map[string]interface{}{"value": "v"},
			},
			RuntimeParams: runtimeParams(act),
		})
		Expect(err).ToNot(HaveOccurred())
		var got configv1alpha1.Config
		Expect(k8sClient.Get(ctx, client.ObjectKey{Namespace: "default", Name: "legacy-step"}, &got)).Should(Succeed())
		Expect(string(got.Spec.Properties.Raw)).Should(MatchJSON(`{"value":"v"}`))
		Expect(act.Phase).Should(Equal("Wait"))
	})

	It("read-config waits on the unrendered Config, list-config and delete-config reach the factory", func() {
		act := &mock.Action{}
		res, err := ReadConfig(ctx, &oamprovidertypes.OAMParams[config.NamespacedName]{
			Params:        config.NamespacedName{Namespace: "default", Name: "legacy-step"},
			RuntimeParams: runtimeParams(act),
		})
		Expect(err).ToNot(HaveOccurred())
		Expect(res.Config).Should(BeNil())
		Expect(act.Phase).Should(Equal("Wait"))

		list, err := ListConfig(ctx, &oamprovidertypes.OAMParams[ListVars]{
			Params:        ListVars{Namespace: "default", Template: "test-plain"},
			RuntimeParams: runtimeParams(&mock.Action{}),
		})
		Expect(err).ToNot(HaveOccurred())
		Expect(list.Configs).Should(BeEmpty(), "no controller materialised a Secret, so nothing is listed yet")

		_, err = DeleteConfig(ctx, &oamprovidertypes.OAMParams[config.NamespacedName]{
			Params:        config.NamespacedName{Namespace: "default", Name: "legacy-step"},
			RuntimeParams: runtimeParams(&mock.Action{}),
		})
		Expect(err).ToNot(HaveOccurred())
		exists, err := config.ConfigCRExists(ctx, k8sClient, "default", "legacy-step")
		Expect(err).ToNot(HaveOccurred())
		Expect(exists).Should(BeFalse())
	})
})

var templateContent = `
metadata: {
	name:        "test-plain"
	alias:       "Plain"
	scope:       "project"
	description: "a plain template"
	sensitive:   false
}

template: {
	output: {
		apiVersion: "v1"
		kind:       "Secret"
		metadata: {
			name:      context.name
			namespace: context.namespace
		}
		stringData: value: parameter.value
	}
	parameter: {
		value: string
	}
}
`
