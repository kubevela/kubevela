package e2e

import (
	"os"
	"os/exec"
	"path"
	"strings"
	"time"

	"github.com/onsi/ginkgo"
	"github.com/onsi/gomega/gexec"
)

var (
	rudrPath = ""
)

//GetCliBinary is to build rudr binary.
func GetCliBinary() (string, error) {
	// TODO(zzxwill) Need to check before building from scratch every time
	cwd, _ := os.Getwd()
	rudrPath = path.Join(cwd, "..")
	mainPath := path.Join(rudrPath, "../cmd/rudrx/main.go")
	cmd := exec.Command("go", "build", "-o", path.Join(rudrPath, "rudr"), mainPath)

	_, err := cmd.Output()
	return rudrPath, err

}

func Exec(cli string) (string, error) {
	c := strings.Fields(cli)
	commandName := path.Join(rudrPath, c[0])
	command := exec.Command(commandName, c[1:]...)

	var output []byte
	session, err := gexec.Start(command, ginkgo.GinkgoWriter, ginkgo.GinkgoWriter)
	if err != nil {
		return string(output), err
	}
	s := session.Wait(10 * time.Second)
	return string(s.Out.Contents()) + string(s.Err.Contents()), nil

}
