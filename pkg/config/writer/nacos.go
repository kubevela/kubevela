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
	"os"
	"path/filepath"
	"runtime/debug"
	"strings"

	"cuelang.org/go/cue"
	"github.com/nacos-group/nacos-sdk-go/v2/clients"
	"github.com/nacos-group/nacos-sdk-go/v2/clients/config_client"
	"github.com/nacos-group/nacos-sdk-go/v2/common/constant"
	"github.com/nacos-group/nacos-sdk-go/v2/vo"
	"k8s.io/klog/v2"

	"github.com/kubevela/workflow/pkg/cue/model/value"

	"github.com/oam-dev/kubevela/apis/types"
	icontext "github.com/oam-dev/kubevela/pkg/config/context"
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

// NewNacosClient builds the client write and Read use when NacosData.Client is nil.
// A package variable so the controller tests can hand in a mock per spec; the CLI and
// the controller use the SDK's constructor.
var NewNacosClient func(param vo.NacosClientParam) (config_client.IConfigClient, error) = clients.NewConfigClient

// parseNacosConfig parse the nacos server config
func parseNacosConfig(templateField cue.Value, wc *ExpandedWriterConfig) {
	nacos := templateField.LookupPath(cue.ParsePath("nacos"))
	if nacos.Err() == nil {
		formatValue := nacos.LookupPath(cue.ParsePath("format"))
		format, err := formatValue.String()
		if err != nil {
			klog.Warningf("fail to get the format from the nacos config: %s", err.Error())
		}
		endpointValue := nacos.LookupPath(value.FieldPath("endpoint", "name"))
		endpoint, err := endpointValue.String()
		if err != nil {
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
	formatValue := nacos.LookupPath(cue.ParsePath("format"))
	format, err := formatValue.String()
	if err != nil {
		format = config.Format
	}
	var nacosData NacosData
	if err := value.UnmarshalTo(nacos, &nacosData); err != nil {
		return nil, err
	}
	content := nacos.LookupPath(cue.ParsePath("content"))
	if content.Err() != nil {
		return nil, content.Err()
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

// Read returns the content Nacos holds for the dataId and group of d, through the same
// endpoint config and client construction as the write. An empty string with no error
// means Nacos has no such config. The reconciler calls it once per Config, on the first
// sync, so a legacy config whose content the CLI already published is not published
// again (Nacos keeps a history entry per publish).
func Read(ctx context.Context, d *NacosData, configReader icontext.ReadConfigProvider) (content []byte, err error) {
	defer func() {
		if rec := recover(); rec != nil {
			err = fmt.Errorf("panic when reading the data from nacos:%v", rec)
			debug.PrintStack()
		}
	}()
	cli, closeClient, err := d.client(ctx, configReader)
	if err != nil {
		return nil, err
	}
	defer closeClient()
	got, err := cli.GetConfig(vo.ConfigParam{
		DataId: d.Metadata.DataID,
		Group:  d.Metadata.Group,
	})
	if err != nil {
		return nil, fmt.Errorf("fail to read the config from the nacos server:%w", err)
	}
	return []byte(got), nil
}

// client returns the client to talk to the endpoint of n and the function to release
// it: n.Client when set (the caller owns it, release is a no-op), otherwise one built
// from the endpoint config through NewNacosClient and closed by the release function.
func (n *NacosData) client(ctx context.Context, configReader icontext.ReadConfigProvider) (config_client.IConfigClient, func(), error) {
	if n.Client != nil {
		return n.Client, func() {}, nil
	}
	// the config of the nacos server saving in the default system namespace
	config, err := configReader(ctx, n.Endpoint.Namespace, n.Endpoint.Name)
	if err != nil {
		return nil, nil, fmt.Errorf("fail to read the config of the nacos server:%w", err)
	}
	param := n.clientParam(config)
	nacosClient, err := NewNacosClient(param)
	if err != nil {
		return nil, nil, err
	}
	return nacosClient, nacosClient.CloseClient, nil
}

// clientParam turns the endpoint config (the properties of the config made from the
// nacos-server template) into the SDK's client parameters. The client config is always
// built: the SDK's defaults put its log and cache directories under the process working
// directory, which is the repository for the CLI and / for the controller, so they go
// under the temp directory instead; and the block's appName and namespaceId reach the
// client whether or not the endpoint config has a client section.
func (n *NacosData) clientParam(config map[string]interface{}) vo.NacosClientParam {
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
	options := []constant.ClientOption{
		constant.WithAppName(n.Metadata.AppName),
		constant.WithNamespaceId(n.Metadata.NamespaceID),
		constant.WithLogDir(filepath.Join(os.TempDir(), "nacos", "log")),
		constant.WithCacheDir(filepath.Join(os.TempDir(), "nacos", "cache")),
	}
	// Discover the server endpoint
	if clientConfigs, ok := config["client"]; ok {
		if client, _ := clientConfigs.(map[string]interface{}); client != nil {
			options = append(options,
				constant.WithEndpoint(readString(client, "endpoint")),
				constant.WithUsername(readString(client, "username")),
				constant.WithPassword(readString(client, "password")),
				constant.WithRegionId(readString(client, "regionId")),
				constant.WithOpenKMS(readBool(client, "openKMS")),
				constant.WithAccessKey(readString(client, "accessKey")),
				constant.WithSecretKey(readString(client, "secretKey")),
			)
		}
	}
	nacosParam.ClientConfig = constant.NewClientConfig(options...)
	return nacosParam
}

func (n *NacosData) write(ctx context.Context, configReader icontext.ReadConfigProvider) (err error) {
	defer func() {
		if rec := recover(); rec != nil {
			err = fmt.Errorf("panic when writing the data to nacos:%v", rec)
			debug.PrintStack()
		}
	}()
	cli, closeClient, err := n.client(ctx, configReader)
	if err != nil {
		return err
	}
	defer closeClient()
	_, err = cli.PublishConfig(vo.ConfigParam{
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
