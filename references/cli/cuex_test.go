package cli

import (
	"bytes"
	cmdutil "github.com/oam-dev/kubevela/pkg/utils/util"
	"github.com/stretchr/testify/require"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const (
	testdataDir = "test-data/cuex"
)

func TestCuexEval(t *testing.T) {
	c := initArgs()
	buffer := bytes.NewBuffer(nil)
	ioStream := cmdutil.IOStreams{In: os.Stdin, Out: buffer, ErrOut: os.Stderr}

	cmd := newCuexEvalCommand(c, ioStream)

	testCases := map[string]struct {
		filepath   string
		expression string
		out        string
		expect     string
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
			`
{
    "label": "app",
    "S": {
        "name": "Postgres",
        "version": "13",
        "label": "app",
        "image": "docker.io/postgres:13"
    }
}
`,
		},
		"yaml": {
			filepath.Join(testdataDir, "foo.cue"),
			"",
			"yaml",
			`
label: app
S:
  name: Postgres
  version: "13"
  label: app
  image: docker.io/postgres:13
`,
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
			cmd.SetArgs([]string{tc.filepath, "-e", tc.expression, "--out", tc.out})
			err := cmd.Execute()
			r.NoError(err)
			r.Equal(trimNextLine(tc.expect), trimNextLine(buffer.String()))
		})
	}
}

func trimNextLine(s string) string {
	return strings.TrimLeft(strings.TrimRight(s, "\n"), "\n")
}
