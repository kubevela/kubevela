/*
 Copyright 2023 The KubeVela Authors.

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

package docgen

import (
	"bytes"
	"context"
	"io"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGenerateProvidersMarkdown(t *testing.T) {
	// depends on cuegen testdata
	path := "../cuegen/generators/provider/testdata"

	src, err := os.ReadFile(filepath.Join(path, "valid.cue"))
	require.NoError(t, err)

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	got := bytes.NewBuffer(nil)
	err = GenerateProvidersMarkdown(ctx, []io.Reader{bytes.NewBuffer(src)}, got)
	require.NoError(t, err)

	expected, err := os.ReadFile(filepath.Join(path, "valid.md"))
	require.NoError(t, err)

	assert.Equal(t, string(expected), got.String())
}
