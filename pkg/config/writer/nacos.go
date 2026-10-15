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

	"github.com/kubevela/workflow/pkg/cue/model/value"

	"github.com/oam-dev/kubevela/apis/types"
	icontext "github.com/oam-dev/kubevela/pkg/config/context"
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

// The two sections of an endpoint config (the properties of a config made from the
// nacos-server template) the client is built from: the server addresses, and the
// discovery endpoint with the credentials. EndpointHash projects the same keys.
const (
	EndpointServersKey = "servers"
	EndpointClientKey  = "client"
)

// NewNacosClient builds the client write and Read use when NacosData.Client is nil.
// A package variable so the controller tests can hand in a mock per spec; the CLI and
// the controller use the SDK's constructor.
var NewNacosClient func(param vo.NacosClientParam) (config_client.IConfigClient, error) = clients.NewConfigClient

// renderNacos reads the nacos block out of the rendered template value: the one the
// Secret render produced from the CueX compile of the script with the context and the
// parameters merged in. One render serves the Secret, the outputs and this block, so a
// template that imports a vela package or calls a provider behaves the same for all
// three and its providers run once. An empty group becomes the SDK's default group, so
// the hash, the status, the read and the publish all name the group Nacos uses.
func renderNacos(config *NacosConfig, rendered cue.Value) (*NacosData, error) {
	nacos := rendered.LookupPath(cue.ParsePath("template.nacos"))
	if !nacos.Exists() {
		return nil, fmt.Errorf("the rendered template has no nacos block")
	}
	if err := nacos.Err(); err != nil {
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
	if nacosData.Metadata.Group == "" {
		nacosData.Metadata.Group = constant.DEFAULT_GROUP
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
	return n.NewClient(config)
}

// NewClient builds the SDK client for the endpoint config (the properties of the config
// made from the nacos-server template) and returns it with the function that closes it.
// A caller that reads and publishes in one go builds the client once and hands it to
// both through n.Client: every construction re-initialises the SDK's global logger and
// opens a log writer that is never closed, so one per sync is the most the controller
// should pay.
func (n *NacosData) NewClient(endpointConfig map[string]interface{}) (config_client.IConfigClient, func(), error) {
	nacosClient, err := NewNacosClient(n.clientParam(endpointConfig))
	if err != nil {
		return nil, nil, fmt.Errorf("fail to build the nacos client for the endpoint %s/%s: %w", n.Endpoint.Namespace, n.Endpoint.Name, err)
	}
	return nacosClient, nacosClient.CloseClient, nil
}

// clientParam turns the endpoint config (the properties of the config made from the
// nacos-server template) into the SDK's client parameters. The client config is always
// built: the SDK's defaults put its log and cache directories under the process working
// directory, which is the repository for the CLI and / for the controller, so they go
// under the temp directory instead, the cache in a directory per endpoint (namespace and
// name as two components, each within the file name limit on its own) because the SDK
// keys its entries by dataId, group and namespaceId alone; and the block's appName
// and namespaceId reach the client whether or not the endpoint config has a client
// section. The SDK's snapshot fallback is off: with it on, a read while the server is
// unreachable returns the snapshot the SDK wrote after its last successful read, with no
// error, and the controller would record stale content as current and never retry. (The
// SDK's failover files are a different mechanism: an operator has to write them into the
// cache directory by hand, and nothing here does.)
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
	if serverConfigs, ok := config[EndpointServersKey]; ok {
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
		constant.WithCacheDir(filepath.Join(os.TempDir(), "nacos", "cache", n.Endpoint.Namespace, n.Endpoint.Name)),
		constant.WithDisableUseSnapShot(true),
	}
	// Discover the server endpoint
	if clientConfigs, ok := config[EndpointClientKey]; ok {
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
