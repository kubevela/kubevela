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
	cwd, _ := os.Getwd()
	rudrPath = path.Join(cwd, "..")
	mainPath := path.Join(rudrPath, "../cmd/vela/main.go")
	cmd := exec.Command("go", "build", "-o", path.Join(rudrPath, "vela"), mainPath)

	_, err := cmd.Output()
	return rudrPath, err
}

func Exec(cli string) (string, error) {
	var output []byte
	session, err := AsyncExec(cli)
	if err != nil {
		return string(output), err
	}
	s := session.Wait(10 * time.Second)
	return string(s.Out.Contents()) + string(s.Err.Contents()), nil
}

func AsyncExec(cli string) (*gexec.Session, error) {
	c := strings.Fields(cli)
	commandName := path.Join(rudrPath, c[0])
	command := exec.Command(commandName, c[1:]...)
	session, err := gexec.Start(command, ginkgo.GinkgoWriter, ginkgo.GinkgoWriter)
	return session, err
}
