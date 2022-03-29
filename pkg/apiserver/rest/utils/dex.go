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

package utils

import (
	"context"
	"encoding/json"

	v1 "k8s.io/api/core/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/oam-dev/kubevela/apis/types"
)

// GetDexConnectors returns the dex connectors for Dex connector controller
func GetDexConnectors(ctx context.Context, k8sClient client.Client) (map[string]interface{}, error) {
	secrets := &v1.SecretList{}
	if err := k8sClient.List(ctx, secrets, client.InNamespace(types.DefaultKubeVelaNS),
		client.MatchingLabels{types.LabelConfigType: "config-dex-connector"}); err != nil {
		return nil, err
	}
	connectors := make([]map[string]interface{}, len(secrets.Items))
	for i, s := range secrets.Items {
		var data map[string]interface{}
		key := s.Labels[types.LabelConfigSubType]
		err := json.Unmarshal(s.Data[key], &data)
		if err != nil {
			return nil, err
		}
		connectors[i] = map[string]interface{}{
			"type":   s.Labels[types.LabelConfigSubType],
			"id":     s.Name,
			"name":   s.Name,
			"config": data,
		}
	}

	return map[string]interface{}{
		"connectors": connectors,
	}, nil
}
