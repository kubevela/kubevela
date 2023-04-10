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

package provider

import (
	"context"
	"strings"
	"testing"
	"time"

	monitorContext "github.com/kubevela/pkg/monitor/context"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"k8s.io/apimachinery/pkg/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/rest"
	"k8s.io/utils/pointer"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/envtest"

	wfContext "github.com/kubevela/workflow/pkg/context"
	"github.com/kubevela/workflow/pkg/cue/model/value"

	"github.com/oam-dev/kubevela/pkg/config"
)

var cfg *rest.Config
var k8sClient client.Client
var testEnv *envtest.Environment
var scheme = runtime.NewScheme()
var p *provider

func TestProvider(t *testing.T) {
	RegisterFailHandler(Fail)

	RunSpecs(t, "Test Config Suite")
}

var _ = BeforeSuite(func(done Done) {
	By("Bootstrapping test environment")
	testEnv = &envtest.Environment{
		ControlPlaneStartTimeout: time.Minute,
		ControlPlaneStopTimeout:  time.Minute,
		UseExistingCluster:       pointer.Bool(false),
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

	p = &provider{
		factory: config.NewConfigFactory(k8sClient),
	}
	close(done)
}, 120)

var _ = AfterSuite(func() {
	By("Tearing down the test environment")
	err := testEnv.Stop()
	Expect(err).ToNot(HaveOccurred())
})

var _ = Describe("Test the config provider", func() {

	It("test creating a config", func() {
		mCtx := monitorContext.NewTraceContext(context.Background(), "")
		v, err := value.NewValue(`
		name: "hub-kubevela"
		namespace: "default"
		template: "default/test-image-registry"
		config: {
			registry: "hub.kubevela.net"
		}
		`, nil, "")
		Expect(err).ToNot(HaveOccurred())
		err = p.Create(mCtx, new(wfContext.WorkflowContext), v, nil)
		Expect(strings.Contains(err.Error(), "the template does not exist")).Should(BeTrue())

		template, err := p.factory.ParseTemplate("test-image-registry", []byte(templateContent))
		Expect(err).ToNot(HaveOccurred())
		Expect(p.factory.CreateOrUpdateConfigTemplate(context.TODO(), "default", template)).ToNot(HaveOccurred())

		Expect(p.Create(mCtx, new(wfContext.WorkflowContext), v, nil)).ToNot(HaveOccurred())
	})

	It("test creating a config without the template", func() {
		mCtx := monitorContext.NewTraceContext(context.Background(), "")
		v, err := value.NewValue(`
		name: "www-kubevela"
		namespace: "default"
		config: {
			url: "kubevela.net"
		}
		`, nil, "")
		Expect(err).ToNot(HaveOccurred())
		Expect(p.Create(mCtx, new(wfContext.WorkflowContext), v, nil)).ToNot(HaveOccurred())
	})

	It("test listing the config", func() {
		mCtx := monitorContext.NewTraceContext(context.Background(), "")
		v, err := value.NewValue(`
		namespace: "default"
		template: "test-image-registry"
		`, nil, "")
		Expect(err).ToNot(HaveOccurred())
		Expect(p.List(mCtx, new(wfContext.WorkflowContext), v, nil)).ToNot(HaveOccurred())
		configs, err := v.LookupValue("configs")
		Expect(err).ToNot(HaveOccurred())
		var contents []map[string]interface{}
		Expect(configs.UnmarshalTo(&contents)).ToNot(HaveOccurred())
		Expect(len(contents)).To(Equal(1))
		Expect(contents[0]["config"].(map[string]interface{})["registry"]).To(Equal("hub.kubevela.net"))
	})

	It("test reading the config", func() {
		mCtx := monitorContext.NewTraceContext(context.Background(), "")
		v, err := value.NewValue(`
		name: "hub-kubevela"
		namespace: "default"
		`, nil, "")
		Expect(err).ToNot(HaveOccurred())
		Expect(p.Read(mCtx, new(wfContext.WorkflowContext), v, nil)).ToNot(HaveOccurred())
		registry, err := v.GetString("config", "registry")
		Expect(err).ToNot(HaveOccurred())
		Expect(registry).To(Equal("hub.kubevela.net"))
	})

	It("test deleting the config", func() {
		mCtx := monitorContext.NewTraceContext(context.Background(), "")
		v, err := value.NewValue(`
		name: "hub-kubevela"
		namespace: "default"
		`, nil, "")
		Expect(err).ToNot(HaveOccurred())
		Expect(p.Delete(mCtx, new(wfContext.WorkflowContext), v, nil)).ToNot(HaveOccurred())

		configs, err := p.factory.ListConfigs(context.Background(), "default", "", "", false)
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
