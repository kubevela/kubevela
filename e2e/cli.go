/*
Copyright 2021 The KubeVela Authors.

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

package e2e

import (
	"os"
	"os/exec"
	"path"
	"strings"
	"time"

	"github.com/Netflix/go-expect"
	"github.com/hinshun/vt10x"
	"github.com/onsi/ginkgo"
	"github.com/onsi/gomega"
	"github.com/onsi/gomega/gexec"
)

var rudrPath = GetCliBinary()

// GetCliBinary is to build kubevela binary.
func GetCliBinary() string {
	cwd, _ := os.Getwd()
	return path.Join(cwd, "../..", "./bin")
}

// Exec executes a command
func Exec(cli string) (string, error) {
	var output []byte
	session, err := asyncExec(cli)
	if err != nil {
		return string(output), err
	}
	s := session.Wait(60 * time.Second)
	return string(s.Out.Contents()) + string(s.Err.Contents()), nil
}

// ExecAndTerminate executes a long-running command and terminate it after 3s
func ExecAndTerminate(cli string) (string, error) {
	var output []byte
	session, err := asyncExec(cli)
	if err != nil {
		return string(output), err
	}
	time.Sleep(3 * time.Second)
	s := session.Terminate()
	return string(s.Out.Contents()) + string(s.Err.Contents()), nil
}

// LongTimeExec executes a long-running command and wait it exits by itself
func LongTimeExec(cli string, timeout time.Duration) (string, error) {
	var output []byte
	session, err := asyncExec(cli)
	if err != nil {
		return string(output), err
	}
	s := session.Wait(timeout)
	return string(s.Out.Contents()) + string(s.Err.Contents()), nil
}

func asyncExec(cli string) (*gexec.Session, error) {
	c := strings.Fields(cli)
	commandName := path.Join(rudrPath, c[0])
	command := exec.Command(commandName, c[1:]...)
	session, err := gexec.Start(command, ginkgo.GinkgoWriter, ginkgo.GinkgoWriter)
	return session, err
}

func LongTimeExecWithEnv(cli string, timeout time.Duration, env []string) (string, error) {
	var output []byte
	c := strings.Fields(cli)
	commandName := path.Join(rudrPath, c[0])
	command := exec.Command(commandName, c[1:]...)
	command.Env = os.Environ()
	command.Env = append(command.Env, env...)

	session, err := gexec.Start(command, ginkgo.GinkgoWriter, ginkgo.GinkgoWriter)
	if err != nil {
		return string(output), err
	}
	s := session.Wait(timeout)
	return string(s.Out.Contents()) + string(s.Err.Contents()), nil
}

// InteractiveExec executes a command with interactive input
func InteractiveExec(cli string, consoleFn func(*expect.Console)) (string, error) {
	var output []byte
	console, _, err := vt10x.NewVT10XConsole(expect.WithStdout(ginkgo.GinkgoWriter))
	gomega.Expect(err).NotTo(gomega.HaveOccurred())
	defer console.Close()
	doneC := make(chan struct{})

	go func() {
		defer ginkgo.GinkgoRecover()
		defer close(doneC)
		consoleFn(console)
	}()

	c := strings.Fields(cli)
	commandName := path.Join(rudrPath, c[0])
	command := exec.Command(commandName, c[1:]...)
	command.Stdin = console.Tty()

	session, err := gexec.Start(command, console.Tty(), console.Tty())
	if err != nil {
		return string(output), err
	}
	s := session.Wait(300 * time.Second)
	err = console.Tty().Close()
	if err != nil {
		return string(output), err
	}
	<-doneC
	if err != nil {
		return string(output), err
	}
	return string(s.Out.Contents()) + string(s.Err.Contents()), nil
}

func BeforeSuit() {
	// sync capabilities from cluster into local
	_, _ = Exec("vela workloads")
}
