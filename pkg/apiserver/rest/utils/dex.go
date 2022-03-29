package utils

import (
	"context"
	"encoding/json"

	v1 "k8s.io/api/core/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/oam-dev/kubevela/apis/types"
)

//type Connectors struct {
//	Type   string            `json:"type"`
//	ID     string            `json:"id"`
//	Name   string            `json:"name"`
//	Config map[string][]byte `json:"config"`
//}

func GetDexConnectors(ctx context.Context, k8sClient client.Client) (map[string]interface{}, error) {
	secrets := &v1.SecretList{}
	if err := k8sClient.List(ctx, secrets, client.InNamespace(types.DefaultKubeVelaNS),
		client.MatchingLabels{types.LabelConfigType: "config-dex-connector"}); err != nil {
		return nil, err
	}
	connectors := make([]map[string]interface{}, len(secrets.Items))
	for _, s := range secrets.Items {
		var data map[string]interface{}
		key := s.Labels[types.LabelConfigSubType]
		err := json.Unmarshal(s.Data[key], &data)
		if err != nil {
			return nil, err
		}
		connectors = append(connectors, map[string]interface{}{
			"type":   s.Labels[types.LabelConfigSubType],
			"id":     s.Name,
			"name":   s.Name,
			"config": data,
		})
	}

	return map[string]interface{}{
		"connectors": connectors,
	}, nil
}
