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

package registry

import (
	"context"
	"errors"
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	di "github.com/oam-dev/kubevela/pkg/registry"
	velaerrors "github.com/oam-dev/kubevela/pkg/utils/errors"
)

// stubReader answers from a fixed map and reports anything else as absent.
type stubReader struct {
	files map[string]string
	err   error
}

func (s stubReader) ReadFile(_ context.Context, _, path, _ string) (string, error) {
	if s.err != nil {
		return "", s.err
	}
	if content, ok := s.files[path]; ok {
		return content, nil
	}
	return "", fmt.Errorf("reading %q: %w", path, velaerrors.ErrFileNotFound)
}

func withReader(t *testing.T, r FileReader) {
	t.Helper()
	snapshot := di.Snapshot()
	t.Cleanup(func() { di.Restore(snapshot) })
	di.RegisterAs[FileReader](r)
}

func read(t *testing.T, registry, path string) (*ReadFileReturns, error) {
	t.Helper()
	return ReadFile(context.Background(), &ReadFileParams{
		Params: ReadFileVars{Registry: registry, Path: path},
	})
}

// TestReadFileDistinguishesAbsenceFromFailure is the whole point of the
// soft-fail path: a file that is not there is an ordinary answer a template can
// branch on, while a registry that cannot be reached is still an error.
func TestReadFileDistinguishesAbsenceFromFailure(t *testing.T) {
	t.Run("present file returns content and found", func(t *testing.T) {
		withReader(t, stubReader{files: map[string]string{"conf/eu.yaml": "region: eu"}})

		got, err := read(t, "demo", "conf/eu.yaml")
		require.NoError(t, err)
		assert.Equal(t, "region: eu", got.Returns.Content)
		assert.True(t, got.Returns.Found, "a file that exists must report found")
	})

	t.Run("missing file is not an error", func(t *testing.T) {
		withReader(t, stubReader{files: map[string]string{}})

		got, err := read(t, "demo", "conf/nowhere.yaml")
		require.NoError(t, err, "absence must not fail the resolution")
		require.NotNil(t, got)
		assert.False(t, got.Returns.Found)
		assert.Empty(t, got.Returns.Content, "no content may be invented for a missing file")
	})

	t.Run("transport failure is still an error", func(t *testing.T) {
		withReader(t, stubReader{err: errors.New("dial tcp: connection refused")})

		_, err := read(t, "demo", "conf/eu.yaml")
		require.Error(t, err, "an unreachable registry must not look like an absent file")
		assert.Contains(t, err.Error(), "connection refused")
	})

	t.Run("auth failure is still an error", func(t *testing.T) {
		withReader(t, stubReader{err: errors.New("401 Unauthorized")})

		_, err := read(t, "demo", "conf/eu.yaml")
		require.Error(t, err, "a rejected credential must not look like an absent file")
	})
}

// TestReadFileStillValidatesItsInputs guards the error paths that predate the
// soft-fail change, so widening not-found did not widen anything else.
func TestReadFileStillValidatesItsInputs(t *testing.T) {
	withReader(t, stubReader{files: map[string]string{}})

	_, err := read(t, "", "conf/eu.yaml")
	require.ErrorContains(t, err, "registry is required")

	_, err = read(t, "demo", "")
	require.ErrorContains(t, err, "path is required")
}
