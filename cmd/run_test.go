package cmd

import (
	"bytes"
	"github.com/spf13/cobra"
	"testing"
)

func TestHelp(t *testing.T) {
	r := NewRunCmd("")
	r.Execute()
}

func TestWorkloadNotSpecified(t *testing.T) {
	r := RunCmd

	executeCommand(r, "containerized", "frontend", "-p", "1234", "nginx:1.9.4")
}

func executeCommand(root *cobra.Command, args ...string) (output string, err error) {
	_, output, err = executeCommandC(root, args...)
	return output, err
}

func executeCommandC(root *cobra.Command, args ...string) (c *cobra.Command, output string, err error) {
	buf := new(bytes.Buffer)
	root.SetOut(buf)
	root.SetErr(buf)
	root.SetArgs(args)
	c, err = root.ExecuteC()

	return c, buf.String(), err
}