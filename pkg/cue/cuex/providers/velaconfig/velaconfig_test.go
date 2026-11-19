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

package velaconfig

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/require"

	di "github.com/oam-dev/kubevela/pkg/registry"
)

type stubReader struct {
	result  *ReadResult
	err     error
	gotNS   string
	gotName string
}

func (s *stubReader) ReadConfig(_ context.Context, namespace, name string) (*ReadResult, error) {
	s.gotNS, s.gotName = namespace, name
	return s.result, s.err
}

func withReader(t *testing.T, r Reader) {
	t.Helper()
	snapshot := di.Snapshot()
	t.Cleanup(func() { di.Restore(snapshot) })
	di.RegisterAs[Reader](r)
}

func withNoReader(t *testing.T) {
	t.Helper()
	snapshot := di.Snapshot()
	t.Cleanup(func() { di.Restore(snapshot) })
	di.Restore(di.RegistrySnapshot{})
}

func read(vars ReadVars) (*ReadReturns, error) {
	return Read(context.Background(), &ReadParams{Params: vars})
}

func TestReadRequiresAName(t *testing.T) {
	withReader(t, &stubReader{result: &ReadResult{}})
	_, err := read(ReadVars{})
	require.ErrorContains(t, err, "name is required")
}

func TestReadDefaultsTheNamespace(t *testing.T) {
	stub := &stubReader{result: &ReadResult{Properties: map[string]interface{}{"a": 1}}}
	withReader(t, stub)

	_, err := read(ReadVars{Name: "db"})
	require.NoError(t, err)
	require.Equal(t, DefaultNamespace, stub.gotNS,
		"a Config with no namespace is looked for where the platform keeps them")
	require.Equal(t, "db", stub.gotName)

	_, err = read(ReadVars{Name: "db", Namespace: "team-a"})
	require.NoError(t, err)
	require.Equal(t, "team-a", stub.gotNS, "an explicit namespace is honoured")
}

// A missing Config, or one marked sensitive, must fail the source loudly.
// Resolving to an empty set of properties would be cached, and the emptiness
// would then look like data.
func TestReadReturnsTheReaderError(t *testing.T) {
	withReader(t, &stubReader{err: errors.New("the config is sensitive")})
	_, err := read(ReadVars{Name: "db"})
	require.Error(t, err)
	require.Contains(t, err.Error(), "the config is sensitive")
	require.Contains(t, err.Error(), `config "db"`, "the message names what it could not read")
}

// Without a registered reader the provider must say so rather than resolve to
// nothing, since nothing is indistinguishable from an empty Config.
func TestReadWithoutARegisteredReader(t *testing.T) {
	withNoReader(t)
	_, err := read(ReadVars{Name: "db"})
	require.ErrorContains(t, err, "no config reader is registered")
}

// A template ranges over outputs unconditionally, so an absent map has to
// present as empty rather than null.
func TestReadNormalisesAbsentOutputs(t *testing.T) {
	withReader(t, &stubReader{result: &ReadResult{
		Properties: map[string]interface{}{"host": "db.internal"},
		Template:   TemplateRef{Name: "db-template", Namespace: "vela-system"},
		Output:     ObjectRef{APIVersion: "v1", Kind: "Secret", Name: "db-conn"},
	}})

	got, err := read(ReadVars{Name: "db"})
	require.NoError(t, err)
	require.NotNil(t, got.Returns.Outputs)
	require.Empty(t, got.Returns.Outputs)
	require.Equal(t, "db-conn", got.Returns.Output.Name)
	require.Equal(t, "db-template", got.Returns.Template.Name)
	require.Equal(t, map[string]interface{}{"host": "db.internal"}, got.Returns.Properties)
}

// Outputs are keyed by the name the template gave them, never positionally:
// pkg/config builds its reference list by ranging a Go map, so a position can
// silently start meaning something else when the Config is updated.
func TestReadKeepsOutputsKeyedByName(t *testing.T) {
	withReader(t, &stubReader{result: &ReadResult{
		Outputs: map[string]ObjectRef{
			"ca": {APIVersion: "v1", Kind: "ConfigMap", Name: "ca-bundle"},
		},
	}})
	got, err := read(ReadVars{Name: "db"})
	require.NoError(t, err)
	require.Equal(t, "ca-bundle", got.Returns.Outputs["ca"].Name)
}

// The package has to register under the name templates reference, or every
// definition importing it fails with "builtin package undefined".
func TestPackageIsRegisteredUnderTheNameTemplatesUse(t *testing.T) {
	require.Equal(t, "velaconfig", ProviderName)
	require.NotNil(t, Package)
	require.NotEmpty(t, template, "the embedded cue is what declares #Read to a template")
}
