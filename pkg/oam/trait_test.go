package oam

import (
	"testing"

	"gotest.tools/assert"
)

func TestParse(t *testing.T) {
	assert.Equal(t, "containerizedworkloads.core.oam.dev", Parse("core.oam.dev/v1alpha2.ContainerizedWorkload"))
	assert.Equal(t, "containerizedworkloads.core.oam.dev", Parse("containerizedworkloads.core.oam.dev"))
}
