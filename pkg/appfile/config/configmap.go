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

package config

import (
	"context"
	"strings"

	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

const (
	// Splitter is a splitter for configmap name generation
	Splitter = "-"

	// TypeConfigMap defines the type of Configmap
	TypeConfigMap = "configmap"
)

// ToConfigMap will get the data of the store and upload to configmap.
// Serverside Application controller can only use the config in appfile context by configmap.
func ToConfigMap(s Store, name, envName string, configData map[string]string) (*v1.ConfigMap, error) {
	namespace, err := s.Namespace(envName)
	if err != nil {
		return nil, err
	}
	var cm = v1.ConfigMap{
		TypeMeta: metav1.TypeMeta{
			APIVersion: "v1",
			Kind:       "ConfigMap",
		},
	}
	cm.SetName(name)
	cm.SetNamespace(namespace)
	cm.Data = configData
	return &cm, nil
}

// GenConfigMapName is a fixed way to name the configmap name for appfile config
func GenConfigMapName(appName, serviceName, configName string) string {
	return strings.Join([]string{"kubevela", appName, serviceName, configName}, Splitter)
}

var _ Store = &Configmap{}

// Configmap is the configmap implementation of config store
type Configmap struct {
	Client client.Client
}

// GetConfigData will get config data from configmap
func (f *Configmap) GetConfigData(name, envName string) ([]map[string]string, error) {

	namespace, err := f.Namespace(envName)
	if err != nil {
		return nil, err
	}

	var cm v1.ConfigMap
	err = f.Client.Get(context.Background(), client.ObjectKey{Name: name, Namespace: namespace}, &cm)
	if err != nil {
		return nil, err
	}
	var data []map[string]string
	for k, v := range cm.Data {
		data = append(data, EncodeConfigFormat(k, v))
	}
	return data, nil
}

// Namespace returns the namespace of the config store from env
func (f *Configmap) Namespace(envName string) (string, error) {
	// TODO(wonderflow): now we regard env as namespace, it should be fixed when env is store serverside as configmap
	return envName, nil
}

// Type returns the type of the config store
func (Configmap) Type() string {
	return TypeConfigMap
}
