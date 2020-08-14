package builtin

import (
	"testing"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"

	"github.com/ghodss/yaml"
	"github.com/stretchr/testify/assert"

	"github.com/cloud-native-application/rudrx/pkg/builtin/traitdefinition"
	"github.com/cloud-native-application/rudrx/pkg/builtin/workloaddefinition"
)

func TestValidYaml(t *testing.T) {
	cases := map[string]string{
		"scale":         traitdefinition.ManualScaler,
		"rollout":       traitdefinition.SimpleRollout,
		"containerized": workloaddefinition.ContainerizedWorkload,
		"deployment":    workloaddefinition.Deployment,
	}
	for name, val := range cases {
		data := unstructured.Unstructured{}
		assert.NoError(t, yaml.Unmarshal([]byte(val), &data), name)
	}
}
