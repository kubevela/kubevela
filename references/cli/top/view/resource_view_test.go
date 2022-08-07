package view

import (
	"github.com/stretchr/testify/assert"
	"testing"
)

func TestResourceView(t *testing.T) {
	view := NewResourceView(nil)
	assert.Equal(t, view.Name(), "Resource")
}
