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

package utils

import (
	"context"
	"fmt"
	"strings"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/yaml"

	apierrors "k8s.io/apimachinery/pkg/api/errors"

	"github.com/oam-dev/kubevela/pkg/utils/common"
)

var (
	// Port is mock server's exposed port
	Port         = 9098
	velaRegistry = `
apiVersion: v1
data:
  registries: '{ "KubeVela":{ "name": "KubeVela", "oss": { "end_point": "http://REGISTRY_ADDR",
    "bucket": "" } }, "Test-Helm":{ "name": "Test-Helm", "helm": { "name":"", "password":"", "url": "http://HELM_ADDR"} } }'
kind: ConfigMap
metadata:
  name: vela-addon-registry
  namespace: vela-system
`
)

// ApplyMockServerConfig config mock server as addon registry
func ApplyMockServerConfig() error {
	args := common.Args{Schema: common.Scheme}
	k8sClient, err := args.GetClient()
	if err != nil {
		return err
	}
	ctx := context.Background()
	originCm := v1.ConfigMap{}
	cm := v1.ConfigMap{}

	registryCmStr := strings.ReplaceAll(velaRegistry, "REGISTRY_ADDR", fmt.Sprintf("127.0.0.1:%d", Port))
	registryCmStr = strings.ReplaceAll(registryCmStr, "HELM_ADDR", fmt.Sprintf("127.0.0.1:%d/helm", Port))

	err = yaml.Unmarshal([]byte(registryCmStr), &cm)
	if err != nil {
		return err
	}

	otherRegistry := cm.DeepCopy()

	err = k8sClient.Get(ctx, types.NamespacedName{Name: cm.Name, Namespace: cm.Namespace}, &originCm)
	if err != nil && apierrors.IsNotFound(err) {
		if err = k8sClient.Create(ctx, &cm); err != nil {
			return err
		}
	} else {
		cm.ResourceVersion = originCm.ResourceVersion
		if err = k8sClient.Update(ctx, &cm); err != nil {
			return err
		}
	}
	if err := k8sClient.Create(ctx, &v1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "test-vela"}}); err != nil {
		if !apierrors.IsAlreadyExists(err) {
			return err
		}
	}
	otherRegistry.SetNamespace("test-vela")
	if err := k8sClient.Create(ctx, otherRegistry); err != nil {
		if !apierrors.IsAlreadyExists(err) {
			return err
		}
	}
	return nil
}
