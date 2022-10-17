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

package writer

import (
	"context"
	"errors"
	"testing"

	"github.com/golang/mock/gomock"
	"github.com/nacos-group/nacos-sdk-go/v2/vo"
	"github.com/stretchr/testify/require"

	"github.com/kubevela/workflow/pkg/cue/model/value"

	configcontext "github.com/oam-dev/kubevela/pkg/config/context"
	"github.com/oam-dev/kubevela/pkg/cue/script"
	nacosmock "github.com/oam-dev/kubevela/test/mock/nacos"
)

func TestNacosWriter(t *testing.T) {
	r := require.New(t)
	v, err := value.NewValue(`
	nacos: {
		endpoint: {
			name: "test-nacos-server"
		}
		format: "json"
	}
	`, nil, "")
	r.Equal(err, nil)
	ewc := &ExpandedWriterConfig{}
	parseNacosConfig(v, ewc)
	r.Equal(ewc.Nacos.Endpoint.Name, "test-nacos-server")
	r.Equal(ewc.Nacos.Format, "json")
	renderContext := configcontext.ConfigRenderContext{Name: "nacos-config", Namespace: "vela"}
	data, err := renderNacos(ewc.Nacos, script.CUE(`
	template: {
		nacos: {
			// The endpoint can not references the parameter.
			endpoint: {
				// Users must create a config base the nacos-server template firstly.
				name: "test-nacos-server"
			}
			format: parameter.contentType
	
			// could references the parameter
			metadata: {
				dataId: parameter.dataId
				group:  parameter.group
				if parameter.appName != _|_ {
					appName: parameter.appName
				}
				if parameter.namespaceId != _|_ {
					namespaceId: parameter.namespaceId
				}
				if parameter.tenant != _|_ {
					tenant: parameter.tenant
				}
				if parameter.tag != _|_ {
					tag: parameter.tag
				}
			}
			content: parameter.content
		}
		parameter: {
			// +usage=Configuration ID
			dataId: string
			// +usage=Configuration group
			group: *"DEFAULT_GROUP" | string
			// +usage=The configuration content.
			content: {
				...
			}
			contentType: *"json" | "yaml" | "properties" | "toml"
			// +usage=The app name of the configuration
			appName?: string
			// +usage=The namespaceId of the configuration
			namespaceId?: string
			// +usage=The tenant, corresponding to the namespace ID field of Nacos
			tenant?: string
			// +usage=The tag of the configuration
			tag?: string
		}
	}
	`), renderContext, map[string]interface{}{
		"dataId": "hello",
		"content": map[string]interface{}{
			"c1": 1,
		},
		"contentType": "properties",
		"appName":     "appName",
		"namespaceId": "namespaceId",
		"tenant":      "tenant",
		"tag":         "tag",
	})
	r.Equal(err, nil)

	ctl := gomock.NewController(t)
	nacosClient := nacosmock.NewMockIConfigClient(ctl)
	data.Client = nacosClient
	nacosClient.EXPECT().PublishConfig(gomock.Eq(vo.ConfigParam{
		DataId:  "hello",
		Group:   "DEFAULT_GROUP",
		Content: "c1 = 1\n",
		Tag:     "tag",
		AppName: "appName",
		Type:    "properties",
	})).Return(true, nil)

	err = data.write(context.TODO(), func(ctx context.Context, namespace, name string) (map[string]interface{}, error) {
		if name == "test-nacos-server" {
			return map[string]interface{}{
				"servers": []interface{}{
					map[string]interface{}{
						"ipAddr": "127.0.0.1",
						"port":   8849,
					},
				},
				"client": map[string]interface{}{
					"endpoint":  "",
					"username":  "",
					"password":  "",
					"accessKey": "accessKey",
					"secretKey": "secretKey",
				},
			}, nil
		}
		return nil, errors.New("config not found")
	})
	r.Equal(err, nil)
}
