package cmd

import (
	"testing"

	"github.com/spf13/pflag"
	"github.com/stretchr/testify/assert"

	cmdutil "github.com/cloud-native-application/rudrx/pkg/cmd/util"
)

func TestNewOptionsCommand(t *testing.T) {
	expectedOutput := `The following options can be passed to any command:
   -t, --testFlag string   this is a test flag

`
	iostream, _, outPut, _ := cmdutil.NewTestIOStreams()
	flags := pflag.NewFlagSet("global", pflag.ContinueOnError)
	flags.StringP("testFlag", "t", "", "this is a test flag")
	cmd := NewOptionsCommand(flags, iostream)

	assert.NoError(t, cmd.Execute(), "run rudrx options expected no error")
	assert.Equal(t, expectedOutput, outPut.String(), "rudrx options expected output\n %s\n got\n%s",
		outPut.String())
}
