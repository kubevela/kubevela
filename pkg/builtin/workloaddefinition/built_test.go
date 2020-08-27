package workloaddefinition

import (
	"testing"

	"github.com/crossplane/oam-kubernetes-runtime/apis/core/v1alpha2"

	"github.com/ghodss/yaml"
	"github.com/stretchr/testify/assert"
)

func TestValidWorkloadDefinition(t *testing.T) {
	cases := map[string]string{
		"containerized": ContainerizedWorkload,
		"deployment":    Deployment,
	}
	for name, val := range cases {
		data := v1alpha2.WorkloadDefinition{}
		assert.NoError(t, yaml.Unmarshal([]byte(val), &data), name)
	}
}
