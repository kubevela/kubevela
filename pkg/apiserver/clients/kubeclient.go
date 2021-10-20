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
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/oam-dev/kubevela/pkg/multicluster"
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
	var err error
	kubeClient, err = multicluster.GetMulticlusterKubernetesClient()
	if err != nil {
		return nil, err
	}
	return kubeClient, nil
}
