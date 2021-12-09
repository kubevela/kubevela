package addon

import (
	"testing"

	"gotest.tools/assert"
)

func TestPathWithParent(t *testing.T) {
	testCases := []struct {
		readPath       string
		parentPath     string
		actualReadPath string
	}{
		{
			readPath:       "example",
			parentPath:     "experimental",
			actualReadPath: "experimental/example",
		},
		{
			readPath:       "example/",
			parentPath:     "experimental",
			actualReadPath: "experimental/example/",
		},
	}
	for _, tc := range testCases {
		res := pathWithParent(tc.readPath, tc.parentPath)
		assert.Equal(t, res, tc.actualReadPath)
	}
}
