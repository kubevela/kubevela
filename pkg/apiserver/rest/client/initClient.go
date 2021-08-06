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

package client

import (
	k8sruntime "k8s.io/apimachinery/pkg/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/config"

	oamcore "github.com/oam-dev/kubevela/apis/core.oam.dev"
)

// NewK8sClient init a local k8s client which add oamcore scheme
func NewK8sClient() (client.Client, error) {
	conf, err := config.GetConfig()
	if err != nil {
		return nil, err
	}
	scheme := k8sruntime.NewScheme()
	if err := clientgoscheme.AddToScheme(scheme); err != nil {
		return nil, err
	}
	if err := oamcore.AddToScheme(scheme); err != nil {
		return nil, err
	}

	k8sClient, err := client.New(conf, client.Options{Scheme: scheme})
	if err != nil {
		return nil, err
	}
	return k8sClient, nil
}
