package view

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestResourceView(t *testing.T) {
	view := NewResourceView(nil)
	assert.Equal(t, view.Name(), "Resource")
}
