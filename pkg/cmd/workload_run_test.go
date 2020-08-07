package cmd

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestGetWorkloadName(t *testing.T) {
	cases := [][]string{
		[]string{"root/.vela/definitions/containerizedworkloads.core.oam.dev.cue", "containerizedworkloads.core.oam.dev"},
		[]string{"root/.vela/definitions/containerizedworkloads.core.oam.dev", "containerizedworkloads.core.oam.dev"},
		[]string{"", ""},
		[]string{"containerizedworkloads.core.oam.dev.cue", "containerizedworkloads.core.oam.dev"},
		[]string{"containerizedworkloads.core.oam.dev", "containerizedworkloads.core.oam.dev"},
	}

	for i, c := range cases {
		t.Run(fmt.Sprintf("Get workloadName case: %d", i), func(t *testing.T) {
			assert.Equal(t, c[1], getWorkloadName(c[0]))
		})
	}
}
