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

package clients

import (
	k8sruntime "k8s.io/apimachinery/pkg/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/config"

	oamcore "github.com/oam-dev/kubevela/apis/core.oam.dev"
)

var kubeClient client.Client

// SetKubeClient for test
func SetKubeClient(c client.Client) {
	kubeClient = c
}

// GetKubeClient create and return kube runtime client
func GetKubeClient() (client.Client, error) {
	if kubeClient != nil {
		return kubeClient, nil
	}
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
