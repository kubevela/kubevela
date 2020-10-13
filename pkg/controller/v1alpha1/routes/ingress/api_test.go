package ingress

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestGetRouteIngress(t *testing.T) {
	_, err := GetRouteIngress("nginx")
	assert.NoError(t, err)
	_, err = GetRouteIngress("")
	assert.NoError(t, err)
	_, err = GetRouteIngress("istio")
	assert.EqualError(t, err, "unknow route ingress provider 'istio', only 'nginx' is supported now")
}
