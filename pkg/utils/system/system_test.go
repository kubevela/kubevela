package system

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestCreateIfNotExist(t *testing.T) {
	testDir := "TestCreateIfNotExist"
	defer os.RemoveAll(testDir)

	normalCreate := filepath.Join(testDir, "normalCase")
	_, err := CreateIfNotExist(normalCreate)
	assert.NoError(t, err)
	fi, err := os.Stat(normalCreate)
	assert.NoError(t, err)
	assert.Equal(t, true, fi.IsDir())

	normalNestCreate := filepath.Join(testDir, "nested", "normalCase")
	_, err = CreateIfNotExist(normalNestCreate)
	assert.NoError(t, err)
	fi, err = os.Stat(normalNestCreate)
	assert.NoError(t, err)
	assert.Equal(t, true, fi.IsDir())
}
