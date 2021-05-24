package plugins

import (
	"context"
	"github.com/stretchr/testify/assert"
	"path/filepath"
	"testing"
)

func TestRegistry(t *testing.T) {
	testAddon := "init-container"
	regName := "testReg"
	localPath, err := filepath.Abs("../../e2e/plugin/testdata")
	assert.Nil(t, err)

	cases := map[string]struct {
		url       string
		expectReg Registry
	}{
		"github registry": {
			url:       "https://github.com/oam-dev/catalog/tree/master/registry",
			expectReg: GithubRegistry{},
		},
		"local registry": {
			url:       "file://" + localPath,
			expectReg: LocalRegistry{},
		},
	}

	for _, c := range cases {
		registry, err := NewRegistry(context.Background(), "", regName, c.url)
		assert.NoError(t, err, regName)
		assert.IsType(t, c.expectReg, registry, regName)

		caps, err := registry.ListCaps()
		assert.NoError(t, err, regName)
		assert.NotEmpty(t, caps, regName)

		capability, data, err := registry.GetCap(testAddon)
		assert.NoError(t, err, regName)
		assert.NotNil(t, capability, testAddon)
		assert.NotNil(t, data, testAddon)
	}
}
