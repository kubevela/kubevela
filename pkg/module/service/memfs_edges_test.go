/*
Copyright 2026 The KubeVela Authors.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package service

import (
	"io"
	"io/fs"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestMapFS_RejectsInvalidPaths(t *testing.T) {
	m := MapFS{"v1/bucket.cue": []byte("x")}
	for _, name := range []string{"../escape", "/abs/path", "v1//bucket.cue", ""} {
		_, err := m.ReadFile(name)
		assert.ErrorIs(t, err, fs.ErrInvalid, "ReadFile(%q)", name)
		_, err = m.ReadDir(name)
		assert.ErrorIs(t, err, fs.ErrInvalid, "ReadDir(%q)", name)
		_, err = m.Open(name)
		assert.ErrorIs(t, err, fs.ErrInvalid, "Open(%q)", name)
	}
}

func TestMapFS_ReadDirSkipsAKeyThatIsOnlyADirectory(t *testing.T) {
	// A key ending in "/" names no file under the directory, so the directory
	// is as good as absent.
	m := MapFS{"v1/": []byte{}}
	_, err := m.ReadDir("v1")
	assert.ErrorIs(t, err, fs.ErrNotExist)

	entries, err := m.ReadDir(".")
	require.NoError(t, err)
	require.Len(t, entries, 1)
	assert.True(t, entries[0].IsDir())
}

func TestMapFS_OpenServesAReadOnlyFile(t *testing.T) {
	m := MapFS{"v1/bucket.cue": []byte("bucket: {}")}

	_, err := m.Open("v1/missing.cue")
	assert.ErrorIs(t, err, fs.ErrNotExist)

	f, err := m.Open("v1/bucket.cue")
	require.NoError(t, err)
	t.Cleanup(func() { assert.NoError(t, f.Close()) })

	info, err := f.Stat()
	require.NoError(t, err)
	assert.Equal(t, "bucket.cue", info.Name(), "Stat names the file, not its path")
	assert.EqualValues(t, len("bucket: {}"), info.Size())
	assert.False(t, info.IsDir())
	assert.Equal(t, fs.FileMode(0), info.Mode())
	assert.True(t, info.ModTime().IsZero())
	assert.Nil(t, info.Sys())

	data, err := io.ReadAll(f)
	require.NoError(t, err)
	assert.Equal(t, "bucket: {}", string(data))
	n, err := f.Read(make([]byte, 4))
	assert.Equal(t, 0, n)
	assert.ErrorIs(t, err, io.EOF, "a drained file reports EOF, not more bytes")
}

func TestMapFS_DirEntriesDescribeThemselves(t *testing.T) {
	m := MapFS{"v1/bucket.cue": []byte("x"), "_module.cue": []byte("y")}
	entries, err := m.ReadDir(".")
	require.NoError(t, err)
	require.Len(t, entries, 2)

	byName := map[string]fs.DirEntry{}
	for _, e := range entries {
		byName[e.Name()] = e
	}
	dir, file := byName["v1"], byName["_module.cue"]
	require.NotNil(t, dir)
	require.NotNil(t, file)

	assert.Equal(t, fs.ModeDir, dir.Type())
	assert.Equal(t, fs.FileMode(0), file.Type())

	dirInfo, err := dir.Info()
	require.NoError(t, err)
	assert.True(t, dirInfo.IsDir())
	assert.Equal(t, fs.ModeDir, dirInfo.Mode())
	fileInfo, err := file.Info()
	require.NoError(t, err)
	assert.False(t, fileInfo.IsDir())
	assert.Equal(t, "_module.cue", fileInfo.Name())
}
