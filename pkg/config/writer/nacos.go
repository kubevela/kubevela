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
	"fmt"
	"runtime/debug"
	"strings"

	"k8s.io/klog/v2"

	"github.com/nacos-group/nacos-sdk-go/v2/clients"
	"github.com/nacos-group/nacos-sdk-go/v2/clients/config_client"
	"github.com/nacos-group/nacos-sdk-go/v2/common/constant"
	"github.com/nacos-group/nacos-sdk-go/v2/vo"

	"github.com/kubevela/workflow/pkg/cue/model/value"

	"github.com/oam-dev/kubevela/apis/types"
	icontext "github.com/oam-dev/kubevela/pkg/config/context"
	"github.com/oam-dev/kubevela/pkg/cue"
	"github.com/oam-dev/kubevela/pkg/cue/script"
)

// NacosConfig defines the nacos output
type NacosConfig struct {
	Endpoint ConfigRef `json:"endpoint"`
	// Format defines the format in which Data will be output.
	Format   string              `json:"format"`
	Metadata NacosConfigMetadata `json:"metadata"`
}

// NacosConfigMetadata the metadata of the nacos config
type NacosConfigMetadata struct {
	DataID      string `json:"dataId"`
	Group       string `json:"group"`
	NamespaceID string `json:"namespaceId"`
	AppName     string `json:"appName,omitempty"`
	Tenant      string `json:"tenant,omitempty"`
	Tag         string `json:"tag,omitempty"`
}

// NacosData merge the nacos endpoint config and the rendered data
type NacosData struct {
	NacosConfig
	Content []byte                      `json:"-"`
	Client  config_client.IConfigClient `json:"-"`
}

// parseNacosConfig parse the nacos server config
func parseNacosConfig(templateField *value.Value, wc *ExpandedWriterConfig) {
	nacos, _ := templateField.LookupValue("nacos")
	if nacos != nil {
		format, err := nacos.GetString("format")
		if err != nil && !cue.IsFieldNotExist(err) {
			klog.Warningf("fail to get the format from the nacos config: %s", err.Error())
		}
		endpoint, err := nacos.GetString("endpoint", "name")
		if err != nil && !cue.IsFieldNotExist(err) {
			klog.Warningf("fail to get the endpoint name from the nacos config: %s", err.Error())
		}
		wc.Nacos = &NacosConfig{
			Format: format,
			Endpoint: ConfigRef{
				Name: endpoint,
			},
		}
	}
}

func renderNacos(config *NacosConfig, template script.CUE, context icontext.ConfigRenderContext, properties map[string]interface{}) (*NacosData, error) {
	nacos, err := template.RunAndOutput(context, properties, "template", "nacos")
	if err != nil {
		return nil, err
	}
	format, err := nacos.GetString("format")
	if err != nil {
		format = config.Format
	}
	var nacosData NacosData
	if err := nacos.UnmarshalTo(&nacosData); err != nil {
		return nil, err
	}
	content, err := nacos.LookupValue("content")
	if err != nil {
		return nil, err
	}

	out, err := encodingOutput(content, format)
	if err != nil {
		return nil, err
	}
	nacosData.Content = out
	if nacosData.Endpoint.Namespace == "" {
		nacosData.Endpoint.Namespace = types.DefaultKubeVelaNS
	}

	return &nacosData, nil
}

func (n *NacosData) write(ctx context.Context, configReader icontext.ReadConfigProvider) (err error) {
	defer func() {
		if rec := recover(); rec != nil {
			err = fmt.Errorf("panic when writing the data to nacos:%v", rec)
			debug.PrintStack()
		}
	}()
	// the config of the nacos server saving in the default system namespace
	config, err := configReader(ctx, n.Endpoint.Namespace, n.Endpoint.Name)
	if err != nil {
		return fmt.Errorf("fail to read the config of the nacos server:%w", err)
	}
	readString := func(data map[string]interface{}, key string) string {
		if v, ok := data[key]; ok {
			str, _ := v.(string)
			return str
		}
		return ""
	}
	readUint64 := func(data map[string]interface{}, key string) uint64 {
		if v, ok := data[key]; ok {
			vu, _ := v.(float64)
			return uint64(vu)
		}
		return 0
	}
	readBool := func(data map[string]interface{}, key string) bool {
		if v, ok := data[key]; ok {
			vu, _ := v.(bool)
			return vu
		}
		return false
	}
	var nacosParam vo.NacosClientParam
	if serverConfigs, ok := config["servers"]; ok {
		var servers []constant.ServerConfig
		serverEndpoints, _ := serverConfigs.([]interface{})
		for _, s := range serverEndpoints {
			sm, ok := s.(map[string]interface{})
			if ok && sm != nil {
				servers = append(servers, constant.ServerConfig{
					IpAddr:   readString(sm, "ipAddr"),
					Port:     readUint64(sm, "port"),
					GrpcPort: readUint64(sm, "grpcPort"),
				})
			}
		}
		nacosParam.ServerConfigs = servers
	}
	// Discover the server endpoint
	if clientConfigs, ok := config["client"]; ok {
		client, _ := clientConfigs.(map[string]interface{})
		if client != nil {
			nacosParam.ClientConfig = constant.NewClientConfig(
				constant.WithEndpoint(readString(client, "endpoint")),
				constant.WithAppName(n.Metadata.AppName),
				constant.WithNamespaceId(n.Metadata.NamespaceID),
				constant.WithUsername(readString(client, "username")),
				constant.WithPassword(readString(client, "password")),
				constant.WithRegionId(readString(client, "regionId")),
				constant.WithOpenKMS(readBool(client, "openKMS")),
				constant.WithAccessKey(readString(client, "accessKey")),
				constant.WithSecretKey(readString(client, "secretKey")),
			)
		}
	}
	// The mock client creates on the outer.
	if n.Client == nil {
		nacosClient, err := clients.NewConfigClient(nacosParam)
		if err != nil {
			return err
		}
		defer nacosClient.CloseClient()
		n.Client = nacosClient
	}
	_, err = n.Client.PublishConfig(vo.ConfigParam{
		DataId:  n.Metadata.DataID,
		Group:   n.Metadata.Group,
		Content: string(n.Content),
		AppName: n.Metadata.AppName,
		Tag:     n.Metadata.Tag,
		Type:    strings.ToLower(n.Format),
	})
	if err != nil {
		return fmt.Errorf("fail to publish the config to the nacos server:%w", err)
	}
	return nil
}
