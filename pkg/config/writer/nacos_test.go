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
	"github.com/nacos-group/nacos-sdk-go/v2/clients/config_client"
	"github.com/nacos-group/nacos-sdk-go/v2/vo"
	"github.com/stretchr/testify/require"

	"cuelang.org/go/cue/cuecontext"

	configcontext "github.com/oam-dev/kubevela/pkg/config/context"
	"github.com/oam-dev/kubevela/pkg/cue/script"
	nacosmock "github.com/oam-dev/kubevela/test/mock/nacos"
)

func TestNacosWriter(t *testing.T) {
	r := require.New(t)
	v := cuecontext.New().CompileString(`
	nacos: {
		endpoint: {
			name: "test-nacos-server"
		}
		format: "json"
	}
	`)
	r.Equal(v.Err(), nil)
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

// readConfigFor returns an endpoint reader that answers for the given config name.
func readConfigFor(name string) configcontext.ReadConfigProvider {
	return func(ctx context.Context, namespace, configName string) (map[string]interface{}, error) {
		if configName == name {
			return map[string]interface{}{
				"servers": []interface{}{map[string]interface{}{"ipAddr": "127.0.0.1", "port": float64(8848)}},
			}, nil
		}
		return nil, errors.New("config not found")
	}
}

func TestRead(t *testing.T) {
	r := require.New(t)
	data := &NacosData{NacosConfig: NacosConfig{
		Endpoint: ConfigRef{Name: "nacos", Namespace: "vela-system"},
		Format:   "properties",
		Metadata: NacosConfigMetadata{DataID: "nc-db", Group: "DEFAULT_GROUP"},
	}}

	t.Run("through the caller's client", func(t *testing.T) {
		ctl := gomock.NewController(t)
		mock := nacosmock.NewMockIConfigClient(ctl)
		mock.EXPECT().GetConfig(gomock.Eq(vo.ConfigParam{DataId: "nc-db", Group: "DEFAULT_GROUP"})).Return("mysqlPort = 3306\n", nil)
		data.Client = mock
		got, err := Read(context.TODO(), data, readConfigFor("nacos"))
		r.NoError(err)
		r.Equal("mysqlPort = 3306\n", string(got))
		data.Client = nil
	})

	t.Run("a read error is reported", func(t *testing.T) {
		ctl := gomock.NewController(t)
		mock := nacosmock.NewMockIConfigClient(ctl)
		mock.EXPECT().GetConfig(gomock.Any()).Return("", errors.New("boom"))
		data.Client = mock
		_, err := Read(context.TODO(), data, readConfigFor("nacos"))
		r.Error(err)
		r.Contains(err.Error(), "fail to read the config from the nacos server:boom")
		data.Client = nil
	})

	t.Run("an unreadable endpoint is reported before any client is built", func(t *testing.T) {
		_, err := Read(context.TODO(), data, readConfigFor("other"))
		r.Error(err)
		r.Equal("fail to read the config of the nacos server:config not found", err.Error())
	})
}

func TestNewNacosClientSeam(t *testing.T) {
	r := require.New(t)
	ctl := gomock.NewController(t)
	mock := nacosmock.NewMockIConfigClient(ctl)
	var gotParam vo.NacosClientParam
	previous := NewNacosClient
	NewNacosClient = func(param vo.NacosClientParam) (config_client.IConfigClient, error) {
		gotParam = param
		return mock, nil
	}
	t.Cleanup(func() { NewNacosClient = previous })

	data := &NacosData{
		NacosConfig: NacosConfig{
			Endpoint: ConfigRef{Name: "nacos", Namespace: "vela-system"},
			Format:   "properties",
			Metadata: NacosConfigMetadata{DataID: "nc-db", Group: "DEFAULT_GROUP"},
		},
		Content: []byte("mysqlPort = 3306\n"),
	}
	// write: Client nil, so the seam builds the client from the endpoint config and the
	// client is closed afterwards
	mock.EXPECT().PublishConfig(gomock.Eq(vo.ConfigParam{DataId: "nc-db", Group: "DEFAULT_GROUP", Content: "mysqlPort = 3306\n", Type: "properties"})).Return(true, nil)
	mock.EXPECT().CloseClient()
	r.NoError(data.write(context.TODO(), readConfigFor("nacos")))
	r.Len(gotParam.ServerConfigs, 1)
	r.Equal("127.0.0.1", gotParam.ServerConfigs[0].IpAddr)
	r.Equal(uint64(8848), gotParam.ServerConfigs[0].Port)
	r.Nil(data.Client, "write does not keep the client it built")

	// Read goes through the same seam
	mock.EXPECT().GetConfig(gomock.Any()).Return("", nil)
	mock.EXPECT().CloseClient()
	got, err := Read(context.TODO(), data, readConfigFor("nacos"))
	r.NoError(err)
	r.Empty(got)

	// a seam error surfaces
	NewNacosClient = func(vo.NacosClientParam) (config_client.IConfigClient, error) { return nil, errors.New("no client") }
	r.EqualError(data.write(context.TODO(), readConfigFor("nacos")), "no client")
}
