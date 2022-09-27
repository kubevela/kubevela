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
	"encoding/json"
	"encoding/xml"
	"fmt"
	"strings"

	"github.com/magiconair/properties"
	"github.com/pelletier/go-toml"
	"gopkg.in/yaml.v2"

	"github.com/kubevela/workflow/pkg/cue/model/value"

	"github.com/oam-dev/kubevela/pkg/cue/script"
	icontext "github.com/oam-dev/kubevela/pkg/integration/context"
)

// ExpandedWriterConfig define the supported output ways.
type ExpandedWriterConfig struct {
	Nacos *NacosConfig `json:"nacos"`
}

// ExpandedWriterData the data for the expanded writer
type ExpandedWriterData struct {
	Nacos *NacosData `json:"nacos"`
}

// IntegrationRef reference a integration secret, it must be system scope.
type IntegrationRef struct {
	Name string `json:"name"`
}

// ParseExpandedWriterConfig parse the expanded writer config from the template value
func ParseExpandedWriterConfig(template *value.Value) ExpandedWriterConfig {
	var ewc = ExpandedWriterConfig{}
	parseNacosConfig(template, &ewc)
	// parse the other writer configs
	return ewc
}

// RenderForExpandedWriter render the configuration for all expanded writers
func RenderForExpandedWriter(ewc ExpandedWriterConfig, template script.CUE, context icontext.IntegrationRenderContext, properties map[string]interface{}) (*ExpandedWriterData, error) {
	var ewd = ExpandedWriterData{}
	var err error
	if ewc.Nacos != nil {
		ewd.Nacos, err = renderNacos(ewc.Nacos, template, context, properties)
		if err != nil {
			return nil, err
		}
	}
	return &ewd, nil
}

// Write write the integration by the all writers
func Write(ctx context.Context, ewd *ExpandedWriterData, ri icontext.ReadIntegrationProvider) (list []error) {
	if ewd.Nacos != nil {
		if err := ewd.Nacos.write(ctx, ri); err != nil {
			list = append(list, err)
		}
	}
	return
}

// encodingOutput support the json、toml、xml、properties and yaml formats.
func encodingOutput(input *value.Value, format string) ([]byte, error) {
	var data = make(map[string]interface{})
	if err := input.UnmarshalTo(data); err != nil {
		return nil, err
	}
	switch strings.ToLower(format) {
	case "json":
		return json.Marshal(data)
	case "toml":
		return toml.Marshal(data)
	case "xml":
		return xml.Marshal(data)
	case "properties":
		var kv = map[string]string{}
		if err := convertMap2KV("", data, kv); err != nil {
			return nil, err
		}
		return []byte(properties.LoadMap(kv).String()), nil
	default:
		return yaml.Marshal(data)
	}
}

func convertMap2KV(last string, input map[string]interface{}, result map[string]string) error {
	for k, v := range input {
		key := k
		if last != "" {
			key = fmt.Sprintf("%s.%s", last, k)
		}
		switch t := v.(type) {
		case string:
			result[key] = t
		case bool:
			result[key] = fmt.Sprintf("%t", t)
		case int64, int, int32:
			result[key] = fmt.Sprintf("%d", t)
		case float64, float32:
			result[key] = fmt.Sprintf("%v", t)
		case map[string]interface{}:
			if err := convertMap2KV(key, t, result); err != nil {
				return err
			}
		default:
			return fmt.Errorf("the value type of %s can not be supported", key)
		}
	}
	return nil
}
