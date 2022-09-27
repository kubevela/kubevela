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

	"k8s.io/klog"

	"github.com/nacos-group/nacos-sdk-go/v2/clients"
	"github.com/nacos-group/nacos-sdk-go/v2/common/constant"
	"github.com/nacos-group/nacos-sdk-go/v2/vo"

	"github.com/kubevela/workflow/pkg/cue/model/value"

	"github.com/oam-dev/kubevela/apis/types"
	"github.com/oam-dev/kubevela/pkg/cue"
	"github.com/oam-dev/kubevela/pkg/cue/script"
	icontext "github.com/oam-dev/kubevela/pkg/integration/context"
)

// NacosConfig defines the nacos output
type NacosConfig struct {
	Endpoint IntegrationRef `json:"endpoint"`
	// Format defines the format in which Data will be output.
	Format string `json:"format"`
}

// NacosData merge the nacos endpoint config and the rendered data
type NacosData struct {
	NacosConfig
	Data []byte
}

// parseNacosConfig parse the nacos server config
func parseNacosConfig(template *value.Value, wc *ExpandedWriterConfig) {
	nacos, _ := template.LookupValue("nacos")
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
			Endpoint: IntegrationRef{
				Name: endpoint,
			},
		}
	}
}

func renderNacos(config *NacosConfig, template script.CUE, context icontext.IntegrationRenderContext, properties map[string]interface{}) (*NacosData, error) {
	nacos, err := template.RunAndOutput(context, properties, "nacos")
	if err != nil {
		return nil, err
	}
	out, err := encodingOutput(nacos, config.Format)
	if err != nil {
		return nil, err
	}
	return &NacosData{Data: out, NacosConfig: *config}, nil
}

func (n *NacosData) write(ctx context.Context, integrationReader icontext.ReadIntegrationProvider) (err error) {
	defer func() {
		if rec := recover(); rec != nil {
			err = fmt.Errorf("panic when writing the data to nacos")
			debug.PrintStack()
		}
	}()
	// the integration of the nacos server saving in the default system namespace
	config, err := integrationReader(ctx, types.DefaultKubeVelaNS, n.Endpoint.Name)
	if err != nil {
		return fmt.Errorf("fail to read the integration secret of the nacos server:%w", err)
	}
	nacosClient, err := clients.NewConfigClient(vo.NacosClientParam{
		ClientConfig: constant.NewClientConfig(
			constant.WithEndpoint(config["endpoint"].(string)),
			constant.WithAppName(config["appName"].(string)),
			constant.WithNamespaceId(config["namespaceId"].(string)),
			constant.WithUsername(config["username"].(string)),
			constant.WithPassword(config["password"].(string)),
		),
	})
	if err != nil {
		return err
	}
	_, err = nacosClient.PublishConfig(vo.ConfigParam{
		DataId:  config["dataId"].(string),
		Group:   config["group"].(string),
		Content: string(n.Data),
		AppName: config["appName"].(string),
		Type:    n.Format,
	})
	if err != nil {
		return fmt.Errorf("fail to publish the integration to the nacos server:%w", err)
	}
	return nil
}
