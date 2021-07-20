package providers

import (
	"testing"

	"github.com/bmizerany/assert"
)

func TestProvers(t *testing.T) {
	p := NewProviders()
	p.Register("test", map[string]Handler{
		"foo":   nil,
		"crazy": nil,
	})

	_, found := p.GetHandler("test", "foo")
	assert.Equal(t, found, true)
	_, found = p.GetHandler("test", "crazy")
	assert.Equal(t, found, true)
	_, found = p.GetHandler("staging", "crazy")
	assert.Equal(t, found, false)
	_, found = p.GetHandler("test", "fly")
	assert.Equal(t, found, false)
}
