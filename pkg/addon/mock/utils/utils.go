package utils

import (
	"context"
	"fmt"
	"strings"

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
    "bucket": "" } } }'
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

	err = yaml.Unmarshal([]byte(registryCmStr), &cm)
	if err != nil {
		return err
	}

	err = k8sClient.Get(ctx, types.NamespacedName{Name: cm.Name, Namespace: cm.Namespace}, &originCm)
	if err != nil && apierrors.IsNotFound(err) {
		err = k8sClient.Create(ctx, &cm)
	} else {
		cm.ResourceVersion = originCm.ResourceVersion
		err = k8sClient.Update(ctx, &cm)
	}
	return err
}
