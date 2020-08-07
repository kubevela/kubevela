package test

import (
	"os"
	"os/exec"
	"path"
)

var (
	velaPath, _ = os.Getwd()
)

func Command(name string, arg ...string) *exec.Cmd {
	commandName := path.Join(velaPath, name)
	return exec.Command(commandName, arg...)
}
