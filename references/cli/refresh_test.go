package cli

import (
	"os"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestCheckAndUpdateRefreshInterval(t *testing.T) {
	testdir := "testdir-refresh-interval"
	testMaxInterval := time.Second

	err := os.MkdirAll(testdir, 0755)
	assert.NoError(t, err)
	defer os.RemoveAll(testdir)

	r, err := useCacheInsteadRefresh(testdir, testMaxInterval)
	assert.Equal(t, r, false, "should not use cache for tmp file is not created")
	assert.NoError(t, err)

	r, err = useCacheInsteadRefresh(testdir, testMaxInterval)
	assert.Equal(t, r, true, "should use cache for interval is not expired")
	assert.NoError(t, err)

	time.Sleep(2 * testMaxInterval)
	r, err = useCacheInsteadRefresh(testdir, testMaxInterval)
	assert.Equal(t, r, false, "should not use cache for interval is already expired")
	assert.NoError(t, err)
}

func TestWriteAndReadLocalCapHash(t *testing.T) {
	testdir := "testdir-caphash"
	err := os.MkdirAll(testdir, 0755)
	assert.NoError(t, err)
	defer os.RemoveAll(testdir)

	result := readCapDefHashFromLocal(testdir)
	assert.Equal(t, result, map[string]string{}, "capability hash data should be empty")
	fakeHashData := map[string]string{
		"a": "test1",
		"b": "test2",
	}
	err = writeCapDefHashIntoLocal(testdir, fakeHashData)
	assert.NoError(t, err, "write new hash data successfully")
	result = readCapDefHashFromLocal(testdir)
	assert.Equal(t, result, fakeHashData, "read hash data successfully")
}
