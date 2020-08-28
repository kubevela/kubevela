package traitdefinition

import (
	"testing"

	"github.com/crossplane/oam-kubernetes-runtime/apis/core/v1alpha2"

	"github.com/ghodss/yaml"
	"github.com/stretchr/testify/assert"
)

func TestValidTraitDefinition(t *testing.T) {
	cases := map[string]string{
		"scale":   ManualScaler,
		"rollout": SimpleRollout,
	}
	for name, val := range cases {
		data := v1alpha2.TraitDefinition{}
		assert.NoError(t, yaml.Unmarshal([]byte(val), &data), name)
	}
}
