package e2e

import (
	"os"
	"os/exec"
	"path"
	"strings"
	"time"

	"github.com/onsi/ginkgo"
	"github.com/onsi/gomega"
	"github.com/onsi/gomega/gexec"
)

var rudrPath = GetCliBinary()

//GetCliBinary is to build kubevela binary.
func GetCliBinary() string {
	cwd, _ := os.Getwd()
	return path.Join(cwd, "../..", "./bin")
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

func LongTimeExec(cli string, timeout time.Duration) (string, error) {
	var output []byte
	session, err := AsyncExec(cli)
	if err != nil {
		return string(output), err
	}
	s := session.Wait(timeout)
	return string(s.Out.Contents()) + string(s.Err.Contents()), nil
}

func AsyncExec(cli string) (*gexec.Session, error) {
	c := strings.Fields(cli)
	commandName := path.Join(rudrPath, c[0])
	command := exec.Command(commandName, c[1:]...)
	session, err := gexec.Start(command, ginkgo.GinkgoWriter, ginkgo.GinkgoWriter)
	return session, err
}

func BeforeSuit() {
	_, err := Exec("vela install")
	gomega.Expect(err).ShouldNot(gomega.HaveOccurred())
	//Without this line, will hit issue like `<string>: Error: unknown command "scale" for "vela"`
	_, _ = Exec("vela system update")
}
