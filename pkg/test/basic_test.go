package test

import (
	"os"
	"os/exec"
	"path"
)

var (
	rudrPath, _ = os.Getwd()
)

func Command(name string, arg ...string) *exec.Cmd {
	commandName := path.Join(rudrPath, name)
	return exec.Command(commandName, arg...)
}
