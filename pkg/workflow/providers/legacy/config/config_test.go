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
	"strings"
	"testing"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"k8s.io/apimachinery/pkg/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/rest"
	"k8s.io/utils/ptr"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/envtest"

	"github.com/oam-dev/kubevela/pkg/config"
	oamprovidertypes "github.com/oam-dev/kubevela/pkg/workflow/providers/types"
)

var cfg *rest.Config
var k8sClient client.Client
var testEnv *envtest.Environment
var scheme = runtime.NewScheme()
var factory config.Factory

func TestProvider(t *testing.T) {
	RegisterFailHandler(Fail)

	RunSpecs(t, "Test Config Suite")
}

var _ = BeforeSuite(func() {
	By("Bootstrapping test environment")
	testEnv = &envtest.Environment{
		ControlPlaneStartTimeout: time.Minute,
		ControlPlaneStopTimeout:  time.Minute,
		UseExistingCluster:       ptr.To(false),
	}
	var err error
	cfg, err = testEnv.Start()
	Expect(err).ToNot(HaveOccurred())
	Expect(cfg).ToNot(BeNil())
	Expect(clientgoscheme.AddToScheme(scheme)).Should(BeNil())
	// +kubebuilder:scaffold:scheme
	By("Create the k8s client")
	k8sClient, err = client.New(cfg, client.Options{Scheme: scheme})
	Expect(err).ToNot(HaveOccurred())
	Expect(k8sClient).ToNot(BeNil())

	factory = config.NewConfigFactory(k8sClient)
})

var _ = AfterSuite(func() {
	By("Tearing down the test environment")
	err := testEnv.Stop()
	Expect(err).ToNot(HaveOccurred())
})

var _ = Describe("Test the config provider", func() {

	It("test creating a config", func() {
		ctx := context.Background()
		params := &CreateParams{
			Params: CreateConfigProperties{
				Name:      "hub-kubevela",
				Namespace: "default",
				Template:  "default/test-image-registry",
				Config: map[string]interface{}{
					"registry": "ghcr.io/kubevela",
				},
			},
			RuntimeParams: oamprovidertypes.RuntimeParams{
				ConfigFactory: factory,
			},
		}
		_, err := CreateConfig(ctx, params)
		Expect(strings.Contains(err.Error(), "the template does not exist")).Should(BeTrue())

		template, err := factory.ParseTemplate(ctx, "test-image-registry", []byte(templateContent))
		Expect(err).ToNot(HaveOccurred())
		Expect(factory.CreateOrUpdateConfigTemplate(ctx, "default", template)).ToNot(HaveOccurred())

		_, err = CreateConfig(ctx, params)
		Expect(err).ToNot(HaveOccurred())
	})

	It("test creating a config without the template", func() {
		params := &CreateParams{
			Params: CreateConfigProperties{
				Name:      "www-kubevela",
				Namespace: "default",
				Config: map[string]interface{}{
					"url": "kubevela.net",
				},
			},
			RuntimeParams: oamprovidertypes.RuntimeParams{
				ConfigFactory: factory,
			},
		}
		_, err := CreateConfig(context.Background(), params)
		Expect(err).ToNot(HaveOccurred())
	})

	It("test listing the config", func() {
		ctx := context.Background()
		res, err := ListConfig(ctx, &oamprovidertypes.OAMParams[ListVars]{
			Params: ListVars{
				Namespace: "default",
				Template:  "test-image-registry",
			},
			RuntimeParams: oamprovidertypes.RuntimeParams{
				ConfigFactory: factory,
			},
		})
		Expect(err).ToNot(HaveOccurred())
		contents := res.Configs
		Expect(len(contents)).To(Equal(1))
		Expect(contents[0]["config"].(map[string]interface{})["registry"]).To(Equal("ghcr.io/kubevela"))
	})

	It("test reading the config", func() {
		ctx := context.Background()
		res, err := ReadConfig(ctx, &oamprovidertypes.OAMParams[config.NamespacedName]{
			Params: config.NamespacedName{
				Namespace: "default",
				Name:      "hub-kubevela",
			},
			RuntimeParams: oamprovidertypes.RuntimeParams{
				ConfigFactory: factory,
			},
		})
		Expect(err).ToNot(HaveOccurred())
		Expect(res.Config["registry"]).To(Equal("ghcr.io/kubevela"))
	})

	It("test deleting the config", func() {
		ctx := context.Background()
		_, err := DeleteConfig(ctx, &oamprovidertypes.OAMParams[config.NamespacedName]{
			Params: config.NamespacedName{
				Namespace: "default",
				Name:      "hub-kubevela",
			},
			RuntimeParams: oamprovidertypes.RuntimeParams{
				ConfigFactory: factory,
			},
		})
		Expect(err).ToNot(HaveOccurred())

		configs, err := factory.ListConfigs(context.Background(), "default", "", "", false)
		Expect(err).ToNot(HaveOccurred())
		Expect(len(configs)).To(Equal(1))
		Expect(configs[0].Properties["url"]).To(Equal("kubevela.net"))
	})
})

var templateContent = `
import (
	"encoding/base64"
	"encoding/json"
	"strconv"
)

metadata: {
	name:        "image-registry"
	alias:       "Image Registry"
	scope:       "project"
	description: "Config information to authenticate image registry"
	sensitive:   false
}

template: {
	output: {
		apiVersion: "v1"
		kind:       "Secret"
		metadata: {
			name:      context.name
			namespace: context.namespace
			labels: {
				"config.oam.dev/catalog": "velacore-config"
				"config.oam.dev/type":    "image-registry"
			}
		}
		if parameter.auth != _|_ {
			type: "kubernetes.io/dockerconfigjson"
		}
		if parameter.auth == _|_ {
			type: "Opaque"
		}
		stringData: {
			if parameter.auth != _|_ && parameter.auth.username != _|_ {
				".dockerconfigjson": json.Marshal({
					"auths": (parameter.registry): {
						"username": parameter.auth.username
						"password": parameter.auth.password
						if parameter.auth.email != _|_ {
							"email": parameter.auth.email
						}
						"auth": base64.Encode(null, (parameter.auth.username + ":" + parameter.auth.password))
					}
				})
			}
			if parameter.insecure != _|_ {
				"insecure-skip-verify": strconv.FormatBool(parameter.insecure)
			}
			if parameter.useHTTP != _|_ {
				"protocol-use-http": strconv.FormatBool(parameter.useHTTP)
			}
		}
	}

	parameter: {
		// +usage=Image registry FQDN, such as: index.docker.io
		registry: *"index.docker.io" | string
		// +usage=Authenticate the image registry
		auth?: {
			// +usage=Private Image registry username
			username: string
			// +usage=Private Image registry password
			password: string
			// +usage=Private Image registry email
			email?: string
		}
		// +usage=For the registry server that uses the self-signed certificate
		insecure?: bool
		// +usage=For the registry server that uses the HTTP protocol
		useHTTP?: bool
	}
}
`
