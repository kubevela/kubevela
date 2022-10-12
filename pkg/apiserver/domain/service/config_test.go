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

package service

import (
	"context"
	"errors"

	terraformapi "github.com/oam-dev/terraform-controller/api/v1beta1"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	apitypes "k8s.io/apimachinery/pkg/types"

	"github.com/oam-dev/kubevela/apis/types"
	"github.com/oam-dev/kubevela/pkg/apiserver/infrastructure/datastore"
	v1 "github.com/oam-dev/kubevela/pkg/apiserver/interfaces/api/dto/v1"
	"github.com/oam-dev/kubevela/pkg/apiserver/utils/bcode"
	"github.com/oam-dev/kubevela/pkg/config"
	"github.com/oam-dev/kubevela/pkg/cue/script"
)

var alibabaTerraformTemplate = `
import "strings"

metadata: {
	name:        "terraform-provider-alibaba"
	alias:       "Alibaba Terraform Provider"
	sensitive:   true
	scope:       "system"
	description: "Terraform Provider for Alibaba Cloud"
}

template: {
	outputs: {
		"provider": {
			apiVersion: "terraform.core.oam.dev/v1beta1"
			kind:       "Provider"
			metadata: {
				name:      parameter.name
				namespace: context.namespace
				labels:    l
			}
			spec: {
				provider: "alibaba"
				region:   parameter.ALICLOUD_REGION
				credentials: {
					source: "Secret"
					secretRef: {
						namespace: "vela-system"
						name:      context.name
						key:       "credentials"
					}
				}
			}
		}
	}

	output: {
		apiVersion: "v1"
		kind:       "Secret"
		metadata: {
			name:      context.name
			namespace: context.namespace
		}
		type: "Opaque"
		stringData: credentials: strings.Join([creds1, creds2], "\n")
	}

	creds1: "accessKeyID: " + parameter.ALICLOUD_ACCESS_KEY
	creds2: "accessKeySecret: " + parameter.ALICLOUD_SECRET_KEY

	l: {
		"config.oam.dev/catalog":  "velacore-config"
		"config.oam.dev/type":     "terraform-provider"
		"config.oam.dev/provider": "terraform-alibaba"
	}

	parameter: {
		//+usage=The name of Terraform Provider for Alibaba Cloud
		name: *"default" | string
		//+usage=Get ALICLOUD_ACCESS_KEY per this guide https://help.aliyun.com/knowledge_detail/38738.html
		ALICLOUD_ACCESS_KEY: string
		//+usage=Get ALICLOUD_SECRET_KEY per this guide https://help.aliyun.com/knowledge_detail/38738.html
		ALICLOUD_SECRET_KEY: string
		//+usage=Get ALICLOUD_REGION by picking one RegionId from Alibaba Cloud region list https://www.alibabacloud.com/help/doc-detail/72379.htm
		ALICLOUD_REGION: string
	}
}
`

var _ = Describe("Test config service", func() {
	var factory config.Factory
	var ds datastore.DataStore
	var configService *configServiceImpl
	var projectService ProjectService
	BeforeEach(func() {
		factory = config.NewConfigFactory(k8sClient)
		Expect(factory).ToNot(BeNil())
		var err error
		ds, err = NewDatastore(datastore.Config{Type: "kubeapi", Database: "config-test-kubevela"})
		Expect(ds).ToNot(BeNil())
		Expect(err).Should(BeNil())
		projectService = NewTestProjectService(ds, k8sClient)
		configService = &configServiceImpl{
			KubeClient:     k8sClient,
			ProjectService: projectService,
			Factory:        factory,
		}
	})
	It("Test apply terraform template", func() {
		tem, err := factory.ParseTemplate("alibaba-provider", []byte(alibabaTerraformTemplate))
		Expect(err).To(BeNil())
		Expect(factory.ApplyTemplate(context.Background(), types.DefaultKubeVelaNS, tem)).To(BeNil())
	})
	It("Test detail the template", func() {
		detail, err := configService.GetTemplate(context.TODO(), config.NamespacedName{Name: "alibaba-provider"})
		Expect(err).To(BeNil())
		Expect(len(detail.UISchema)).To(Equal(4))
	})
	It("Test apply a new config", func() {
		_, err := configService.CreateConfig(context.TODO(), "", v1.CreateConfigRequest{
			Name:        "alibaba-test-error-properties",
			Alias:       "Alibaba Cloud",
			Description: "This is a terraform provider",
			Template: v1.NamespacedName{
				Name: "alibaba-provider",
			},
			Properties: `{}`,
		})
		Expect(err).ToNot(BeNil())
		var paramErr = &script.ParameterError{}
		Expect(errors.As(err, &paramErr)).To(Equal(true))
		Expect(paramErr.Name).To(Equal("ALICLOUD_ACCESS_KEY"))
		Expect(paramErr.Message).To(Equal("This parameter is required"))

		config, err := configService.CreateConfig(context.TODO(), "", v1.CreateConfigRequest{
			Name:        "alibaba-test",
			Alias:       "Alibaba Cloud",
			Description: "This is a terraform provider",
			Template: v1.NamespacedName{
				Name: "alibaba-provider",
			},
			Properties: `{"ALICLOUD_ACCESS_KEY":"test", "ALICLOUD_SECRET_KEY": "test", "ALICLOUD_REGION": "test", "name": "test"}`,
		})
		Expect(err).To(BeNil())

		Expect(config.Sensitive).To(Equal(true))
		Expect(config.Description).To(Equal("This is a terraform provider"))

		var provider terraformapi.Provider
		Expect(k8sClient.Get(context.TODO(), apitypes.NamespacedName{
			Namespace: types.DefaultKubeVelaNS,
			Name:      "test",
		}, &provider)).To(BeNil())
	})

	It("Test list configs", func() {
		list, err := configService.ListConfigs(context.TODO(), "", "alibaba-provider", false)
		Expect(err).To(BeNil())
		Expect(len(list)).To(Equal(1))

		_, err = projectService.CreateProject(context.TODO(), v1.CreateProjectRequest{Name: "mysql-project"})
		Expect(err).To(BeNil())

		// can not share the config that is the system scope
		list, err = configService.ListConfigs(context.TODO(), "mysql-project", "alibaba-provider", false)
		Expect(err).To(BeNil())
		Expect(len(list)).To(Equal(0))

		list, err = configService.ListConfigs(context.TODO(), "", "not-found", false)
		Expect(err).To(BeNil())
		Expect(len(list)).To(Equal(0))
	})

	It("Test detail a config", func() {
		_, err := configService.GetConfig(context.TODO(), "", "alibaba-test")
		Expect(err).To(Equal(bcode.ErrSensitiveConfig))
	})

	It("Test delete a config", func() {
		Expect(configService.DeleteConfig(context.TODO(), "", "alibaba-test")).To(BeNil())
		var list terraformapi.ProviderList
		Expect(k8sClient.List(context.TODO(), &list)).To(BeNil())
		Expect(len(list.Items)).To(Equal(0))
	})
})
