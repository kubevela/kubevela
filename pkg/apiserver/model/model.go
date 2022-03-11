/*
Copyright 2021 The KubeVela Authors.

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

package model

import (
	"encoding/json"
	"fmt"
	"reflect"
	"time"

	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/yaml"

	"github.com/oam-dev/kubevela/pkg/apiserver/log"
)

var tableNamePrefix = "vela_"

var registeredModels = map[string]Interface{}

// Interface model interface
type Interface interface {
	TableName() string
}

// RegisterModel register model
func RegisterModel(models ...Interface) {
	for _, model := range models {
		if _, exist := registeredModels[model.TableName()]; exist {
			panic(fmt.Errorf("model table name %s conflict", model.TableName()))
		}
		registeredModels[model.TableName()] = model
	}
}

// JSONStruct json struct, same with runtime.RawExtension
type JSONStruct map[string]interface{}

// NewJSONStruct new json struct from runtime.RawExtension
func NewJSONStruct(raw *runtime.RawExtension) (*JSONStruct, error) {
	if raw == nil || raw.Raw == nil {
		return nil, nil
	}
	var data JSONStruct
	err := json.Unmarshal(raw.Raw, &data)
	if err != nil {
		return nil, fmt.Errorf("parse raw data failure %w", err)
	}
	return &data, nil
}

// NewJSONStructByString new json struct from string
func NewJSONStructByString(source string) (*JSONStruct, error) {
	if source == "" {
		return nil, nil
	}
	var data JSONStruct
	err := json.Unmarshal([]byte(source), &data)
	if err != nil {
		return nil, fmt.Errorf("parse raw data failure %w", err)
	}
	return &data, nil
}

// NewJSONStructByStruct new json struct from struct object
func NewJSONStructByStruct(object interface{}) (*JSONStruct, error) {
	if object == nil {
		return nil, nil
	}
	var data JSONStruct
	out, err := yaml.Marshal(object)
	if err != nil {
		return nil, fmt.Errorf("marshal object data failure %w", err)
	}
	if err := yaml.Unmarshal(out, &data); err != nil {
		return nil, fmt.Errorf("unmarshal object data failure %w", err)
	}
	return &data, nil
}

// JSON Encoded as a JSON string
func (j *JSONStruct) JSON() string {
	b, err := json.Marshal(j)
	if err != nil {
		log.Logger.Errorf("json marshal failure %s", err.Error())
	}
	return string(b)
}

// RawExtension Encoded as a RawExtension
func (j *JSONStruct) RawExtension() *runtime.RawExtension {
	yamlByte, err := yaml.Marshal(j)
	if err != nil {
		log.Logger.Errorf("yaml marshal failure %s", err.Error())
		return nil
	}
	b, err := yaml.YAMLToJSON(yamlByte)
	if err != nil {
		log.Logger.Errorf("yaml to json failure %s", err.Error())
		return nil
	}
	return &runtime.RawExtension{Raw: b}
}

// BaseModel common model
type BaseModel struct {
	CreateTime time.Time `json:"createTime"`
	UpdateTime time.Time `json:"updateTime"`
}

// SetCreateTime set create time
func (m *BaseModel) SetCreateTime(time time.Time) {
	m.CreateTime = time
}

// SetUpdateTime set update time
func (m *BaseModel) SetUpdateTime(time time.Time) {
	m.UpdateTime = time
}

func deepCopy(src interface{}) interface{} {
	dst := reflect.New(reflect.TypeOf(src).Elem())

	val := reflect.ValueOf(src).Elem()
	nVal := dst.Elem()
	for i := 0; i < val.NumField(); i++ {
		nvField := nVal.Field(i)
		nvField.Set(val.Field(i))
	}

	return dst.Interface()
}
