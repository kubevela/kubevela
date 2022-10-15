/*
Copyright 2022 The KubeVela Authors.

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

package e2e_apiserver_test

import (
	"context"
	"fmt"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"

	"github.com/oam-dev/kubevela/apis/types"
	v1 "github.com/oam-dev/kubevela/pkg/apiserver/interfaces/api/dto/v1"
	"github.com/oam-dev/kubevela/pkg/config"
)

var tc = `
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
					"auths": "\(parameter.registry)": {
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
		registry: string
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

var _ = Describe("Test the rest api about the config", func() {
	var templateName = "test-image-registry"
	It("Prepare a template", func() {
		cf := config.NewConfigFactory(k8sClient)
		it, err := cf.ParseTemplate(templateName, []byte(tc))
		Expect(err).Should(BeNil())
		err = cf.CreateOrUpdateConfigTemplate(context.TODO(), types.DefaultKubeVelaNS, it)
		Expect(err).Should(BeNil())
	})

	It("Test creating a config", func() {
		req := v1.CreateConfigRequest{
			Name:        "test-registry",
			Alias:       "Test Registry",
			Description: "This is a demo config",
			Template:    v1.NamespacedName{Name: templateName},
			Properties:  `{"registry": "kubevela.test.com"}`,
		}
		res := post("/configs", req)
		var config v1.Config
		Expect(decodeResponseBody(res, &config)).Should(Succeed())
		Expect(config.Alias).Should(Equal(req.Alias))
		Expect(config.Name).Should(Equal(req.Name))
		Expect(config.Description).Should(Equal(req.Description))
		Expect(config.Template.Name).Should(Equal(templateName))
		Expect(config.Sensitive).Should(BeFalse())
		Expect(config.Secret).Should(BeNil())
		Expect(config.Properties["registry"]).Should(Equal("kubevela.test.com"))

		By("the template is not exist")
		req = v1.CreateConfigRequest{
			Name:        "test-registry",
			Alias:       "Test Registry",
			Description: "This is a demo config",
			Template:    v1.NamespacedName{Name: templateName + "notfound"},
			Properties:  `{"registry": "kubevela.test.com"}`,
		}
		res = post("/configs", req)
		Expect(res.StatusCode).Should(Equal(404))

		By("without the template")

		req = v1.CreateConfigRequest{
			Name:        "test-config",
			Alias:       "Test Config",
			Description: "This is a demo config",
			Properties:  `{"url": "kubevela.test.com"}`,
		}
		res = post("/configs", req)
		config = v1.Config{}
		Expect(decodeResponseBody(res, &config)).Should(Succeed())
		Expect(config.Properties["url"]).Should(Equal("kubevela.test.com"))
	})

	It("Test getting a config", func() {
		res := get(fmt.Sprintf("/configs/%s", "test-registry-not-found"))
		Expect(res.StatusCode).Should(Equal(404))
		res = get(fmt.Sprintf("/configs/%s", "test-registry"))
		var config v1.Config
		Expect(decodeResponseBody(res, &config)).Should(Succeed())
		Expect(config.Alias).Should(Equal("Test Registry"))
		Expect(config.Name).Should(Equal("test-registry"))
		Expect(config.Description).Should(Equal("This is a demo config"))
		Expect(config.Template.Name).Should(Equal(templateName))
		Expect(config.Sensitive).Should(BeFalse())
		Expect(config.Secret).Should(BeNil())
		Expect(config.Properties["registry"]).Should(Equal("kubevela.test.com"))
	})

	It("Test updating a config", func() {
		req := v1.UpdateConfigRequest{
			Alias:       "Test Registry Alias",
			Description: "This is a test config",
			Properties:  `{"registry": "kubevela.test.cn"}`,
		}
		res := put(fmt.Sprintf("/configs/%s", "test-registry"), req)
		var config v1.Config
		Expect(decodeResponseBody(res, &config)).Should(Succeed())
		Expect(config.Alias).Should(Equal("Test Registry Alias"))
		Expect(config.Description).Should(Equal("This is a test config"))
		Expect(config.Template.Name).Should(Equal(templateName))
		Expect(config.Sensitive).Should(BeFalse())
		Expect(config.Secret).Should(BeNil())
		Expect(config.Properties["registry"]).Should(Equal("kubevela.test.cn"))
	})

	It("Test deleting a config", func() {
		res := delete(fmt.Sprintf("/configs/%s", "test-registry"))
		Expect(res.StatusCode).Should(Equal(200))
	})
})
