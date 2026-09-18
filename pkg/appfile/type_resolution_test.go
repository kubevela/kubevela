/*
Copyright 2021 The KubeVela Authors.

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

package appfile

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/stretchr/testify/require"

	"github.com/oam-dev/kubevela/pkg/module/naming"
)

// --- parseTypeRef ---

func TestParseTypeRef_Form1(t *testing.T) {
	form, mod, av, name, err := parseTypeRef("bucket")
	require.NoError(t, err)
	assert.Equal(t, 1, form)
	assert.Equal(t, "", mod)
	assert.Equal(t, "", av)
	assert.Equal(t, "bucket", name)
}

func TestParseTypeRef_Form2(t *testing.T) {
	form, mod, av, name, err := parseTypeRef("v1/bucket")
	require.NoError(t, err)
	assert.Equal(t, 2, form)
	assert.Equal(t, "", mod)
	assert.Equal(t, "v1", av)
	assert.Equal(t, "bucket", name)
}

func TestParseTypeRef_Form2_v1beta1(t *testing.T) {
	form, _, av, name, err := parseTypeRef("v1beta1/rds")
	require.NoError(t, err)
	assert.Equal(t, 2, form)
	assert.Equal(t, "v1beta1", av)
	assert.Equal(t, "rds", name)
}

func TestParseTypeRef_Form3(t *testing.T) {
	form, mod, av, name, err := parseTypeRef("s3/v1/bucket")
	require.NoError(t, err)
	assert.Equal(t, 3, form)
	assert.Equal(t, "s3", mod)
	assert.Equal(t, "v1", av)
	assert.Equal(t, "bucket", name)
}

func TestParseTypeRef_InvalidTwoSegment(t *testing.T) {
	_, _, _, _, err := parseTypeRef("s3/bucket")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "is not a valid API version")
}

func TestParseTypeRef_TooManySegments(t *testing.T) {
	_, _, _, _, err := parseTypeRef("a/b/c/d")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "3 segments")
}

func TestParseTypeRef_Empty(t *testing.T) {
	form, _, _, name, err := parseTypeRef("")
	require.NoError(t, err)
	assert.Equal(t, 1, form)
	assert.Equal(t, "", name)
}

func TestParseTypeRef_SingleChar(t *testing.T) {
	form, _, _, name, err := parseTypeRef("x")
	require.NoError(t, err)
	assert.Equal(t, 1, form)
	assert.Equal(t, "x", name)
}

func TestParseTypeRef_HyphensInName(t *testing.T) {
	form, mod, av, name, err := parseTypeRef("aws-s3/v2/my-bucket")
	require.NoError(t, err)
	assert.Equal(t, 3, form)
	assert.Equal(t, "aws-s3", mod)
	assert.Equal(t, "v2", av)
	assert.Equal(t, "my-bucket", name)
}

// API version segment must follow the Kubernetes stability convention ^v\d+(alpha\d+|beta\d+)?$.
// Values like v1.2, v1.0, or "latest" must be rejected.
func TestParseTypeRef_InvalidAPIVersion_Dot(t *testing.T) {
	for _, tc := range []string{"v1.2/bucket", "v1.0/widget", "latest/bucket"} {
		_, _, _, _, err := parseTypeRef(tc)
		require.Errorf(t, err, "expected error for %q", tc)
		assert.Contains(t, err.Error(), "is not a valid API version", "input: %q", tc)
	}
}

// Segments must be slash-separated; a dash used instead of a slash (e.g. s3/v1-bucket
// when the intent was s3/v1/bucket) must be rejected as an invalid two-segment type.
func TestParseTypeRef_SlashSeparatedOnly(t *testing.T) {
	_, _, _, _, err := parseTypeRef("s3/v1-bucket")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "is not a valid API version")
}

func TestParseTypeRef_HyphenOnlyName_IsForm1(t *testing.T) {
	form, _, _, name, err := parseTypeRef("my-component")
	require.NoError(t, err)
	assert.Equal(t, 1, form)
	assert.Equal(t, "my-component", name)
}

// --- resolveForm3 (no cluster call) ---

func TestResolveForm3_Simple(t *testing.T) {
	got := resolveForm3("s3", "v1", "bucket")
	assert.Equal(t, "s3-v1-bucket", got)
}

func TestResolveForm3_LongName_Truncated(t *testing.T) {
	mod := strings.Repeat("m", 100)
	av := strings.Repeat("a", 100)
	name := strings.Repeat("n", 100)
	got := resolveForm3(mod, av, name)
	assert.LessOrEqual(t, len(got), naming.MaxObjectNameLen)
	assert.Equal(t, '-', rune(got[len(got)-9]))
}

func TestResolveForm3_SameLongNamesGetDistinctResults(t *testing.T) {
	a := resolveForm3(strings.Repeat("x", 200), "v1", "alpha")
	b := resolveForm3(strings.Repeat("x", 200), "v1", "beta")
	assert.NotEqual(t, a, b)
}
