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
	"fmt"
	"reflect"
	"strings"

	"github.com/magiconair/properties"
	"github.com/pelletier/go-toml"
	"gopkg.in/yaml.v2"
	"k8s.io/klog/v2"

	"github.com/kubevela/workflow/pkg/cue/model/value"

	icontext "github.com/oam-dev/kubevela/pkg/config/context"
	"github.com/oam-dev/kubevela/pkg/cue/script"
)

// ExpandedWriterConfig define the supported output ways.
type ExpandedWriterConfig struct {
	Nacos *NacosConfig `json:"nacos"`
}

// ExpandedWriterData the data for the expanded writer
type ExpandedWriterData struct {
	Nacos *NacosData `json:"nacos"`
}

// ConfigRef reference a config secret, it must be system scope.
type ConfigRef struct {
	Name      string `json:"name"`
	Namespace string `json:"namespace"`
}

// ParseExpandedWriterConfig parse the expanded writer config from the template value
func ParseExpandedWriterConfig(template *value.Value) ExpandedWriterConfig {
	var ewc = ExpandedWriterConfig{}
	parseNacosConfig(template, &ewc)
	// parse the other writer configs
	return ewc
}

// RenderForExpandedWriter render the configuration for all expanded writers
func RenderForExpandedWriter(ewc ExpandedWriterConfig, template script.CUE, context icontext.ConfigRenderContext, properties map[string]interface{}) (*ExpandedWriterData, error) {
	var ewd = ExpandedWriterData{}
	var err error
	if ewc.Nacos != nil {
		ewd.Nacos, err = renderNacos(ewc.Nacos, template, context, properties)
		if err != nil {
			return nil, err
		}
		klog.Info("the config render to nacos context successfully")
	}
	return &ewd, nil
}

// Write write the config by the all writers
func Write(ctx context.Context, ewd *ExpandedWriterData, ri icontext.ReadConfigProvider) (list []error) {
	if ewd.Nacos != nil {
		if err := ewd.Nacos.write(ctx, ri); err != nil {
			list = append(list, err)
		} else {
			klog.Info("the config write to the nacos successfully")
		}
	}
	return
}

// encodingOutput support the json、toml、xml、properties and yaml formats.
func encodingOutput(input *value.Value, format string) ([]byte, error) {
	var data = make(map[string]interface{})
	if err := input.UnmarshalTo(&data); err != nil {
		return nil, err
	}
	switch strings.ToLower(format) {
	case "json":
		return json.Marshal(data)
	case "toml":
		return toml.Marshal(data)
	case "properties":
		var kv = map[string]string{}
		if err := convertMap2PropertiesKV("", data, kv); err != nil {
			return nil, err
		}
		return []byte(properties.LoadMap(kv).String()), nil
	default:
		return yaml.Marshal(data)
	}
}

func convertMap2PropertiesKV(last string, input map[string]interface{}, result map[string]string) error {

	interface2str := func(key string, v interface{}, result map[string]string) (string, error) {
		switch t := v.(type) {
		case string:
			return t, nil
		case bool:
			return fmt.Sprintf("%t", t), nil
		case int64, int, int32:
			return fmt.Sprintf("%d", t), nil
		case float64, float32:
			return fmt.Sprintf("%v", t), nil
		case map[string]interface{}:
			if err := convertMap2PropertiesKV(key, t, result); err != nil {
				return "", err
			}
			return "", nil
		default:
			return fmt.Sprintf("%v", t), nil
		}
	}

	for k, v := range input {
		key := k
		if last != "" {
			key = fmt.Sprintf("%s.%s", last, k)
		}
		switch t := v.(type) {
		case string, bool, int64, int, int32, float32, float64, map[string]interface{}:
			v, err := interface2str(key, t, result)
			if err != nil {
				return err
			}
			if v != "" {
				result[key] = v
			}
		case []interface{}, []string, []int64, []float64, []map[string]interface{}:
			var ints []string
			s := reflect.ValueOf(t)
			for i := 0; i < s.Len(); i++ {
				re, err := interface2str(fmt.Sprintf("%s.%d", key, i), s.Index(i).Interface(), result)
				if err != nil {
					return err
				}
				if re != "" {
					ints = append(ints, re)
				}
			}
			if len(ints) > 0 {
				result[key] = strings.Join(ints, ",")
			}
		default:
			return fmt.Errorf("the value type of %s(%T) can not be supported", key, t)
		}
	}
	return nil
}
