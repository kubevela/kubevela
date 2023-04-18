package cli

import (
	"bytes"
	"path/filepath"
	"testing"

	"github.com/kubevela/pkg/util/stringtools"
	"github.com/stretchr/testify/require"
)

const (
	testdataDir = "test-data/cuex"
)

func TestCuexEval(t *testing.T) {
	buffer := bytes.NewBuffer(nil)
	cmd := NewCueXEvalCommand()
	cmd.SetOut(buffer)

	testCases := map[string]struct {
		filepath string
		path     string
		format   string
		expect   string
	}{
		"normal evaluate": {
			filepath.Join(testdataDir, "foo.cue"),
			"S.name",
			"",
			"\"Postgres\"",
		},
		"json": {
			filepath.Join(testdataDir, "foo.cue"),
			"",
			"json",
			`{"label":"app","S":{"name":"Postgres","version":"13","label":"app","image":"docker.io/postgres:13"}}`,
		},
		"yaml": {
			filepath.Join(testdataDir, "foo.cue"),
			"",
			"yaml",
			`
				S:
				  image: docker.io/postgres:13
				  label: app
				  name: Postgres
				  version: "13"
				label: app`,
		},
		"http": {
			filepath.Join(testdataDir, "httpget.cue"),
			"statusCode",
			"",
			"200",
		},
	}

	for name, tc := range testCases {
		t.Run(name, func(t *testing.T) {
			buffer.Truncate(0)
			r := require.New(t)
			cmd.SetArgs([]string{"-f", tc.filepath, "-p", tc.path, "-o", tc.format})
			err := cmd.Execute()
			r.NoError(err)
			r.Equal(stringtools.TrimLeadingIndent(tc.expect), stringtools.TrimLeadingIndent(buffer.String()))
		})
	}
}
