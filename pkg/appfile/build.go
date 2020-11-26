package appfile

import (
	"errors"
	"io"
	"os/exec"
	"strings"

	cmdutil "github.com/oam-dev/kubevela/pkg/commands/util"
)

// Build defines the build section of AppFile
type Build struct {
	Push   Push   `json:"push,omitempty"`
	Docker Docker `json:"docker,omitempty"`
}

// Docker defines the docker build section
type Docker struct {
	File    string `json:"file"`
	Context string `json:"context"`
}

// Push defines where to push your image
type Push struct {
	Local    string `json:"local,omitempty"`
	Registry string `json:"registry,omitempty"`
}

func asyncLog(reader io.Reader, stream cmdutil.IOStreams) {
	cache := ""
	buf := make([]byte, 1024)
	for {
		num, err := reader.Read(buf)
		if err != nil && errors.Is(err, io.EOF) {
			return
		}
		if num > 0 {
			b := buf[:num]
			s := strings.Split(string(b), "\n")
			line := strings.Join(s[:len(s)-1], "\n")
			stream.Infof("%s%s\n", cache, line)
			cache = s[len(s)-1]
		}
		if errors.Is(err, io.EOF) {
			break
		}
	}
}

// BuildImage will build a image with name and context.
func (b *Build) BuildImage(io cmdutil.IOStreams, image string) error {
	//nolint:gosec
	// TODO(hongchaodeng): remove this dependency by using go lib
	cmd := exec.Command("docker", "build", "-t", image, "-f", b.Docker.File, b.Docker.Context)
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		io.Errorf("BuildImage exec command error, message:%s\n", err.Error())
		return err
	}
	stderr, err := cmd.StderrPipe()
	if err != nil {
		io.Errorf("BuildImage exec command error, message:%s\n", err.Error())
		return err
	}
	if err := cmd.Start(); err != nil {
		io.Errorf("BuildImage exec command error, message:%s\n", err.Error())
		return err
	}
	go asyncLog(stdout, io)
	go asyncLog(stderr, io)
	if err := cmd.Wait(); err != nil {
		io.Errorf("BuildImage wait for command execution error:%s", err.Error())
		return err
	}
	return b.pushImage(io, image)
}

func (b *Build) pushImage(io cmdutil.IOStreams, image string) error {
	io.Infof("pushing image (%s)...\n", image)
	switch {
	case b.Push.Local == "kind":
		//nolint:gosec
		cmd := exec.Command("kind", "load", "docker-image", image)
		stdout, err := cmd.StdoutPipe()
		if err != nil {
			io.Errorf("pushImage(kind) exec command error, message:%s\n", err.Error())
			return err
		}
		stderr, err := cmd.StderrPipe()
		if err != nil {
			io.Errorf("pushImage(kind) exec command error, message:%s\n", err.Error())
			return err
		}
		if err := cmd.Start(); err != nil {
			io.Errorf("pushImage(kind) exec command error, message:%s\n", err.Error())
			return err
		}
		go asyncLog(stdout, io)
		go asyncLog(stderr, io)
		if err := cmd.Wait(); err != nil {
			io.Errorf("pushImage(kind) wait for command execution error:%s", err.Error())
			return err
		}
		return nil
	default:
	}
	//nolint:gosec
	cmd := exec.Command("docker", "push", image)
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		io.Errorf("pushImage(docker push) exec command error, message:%s\n", err.Error())
		return err
	}
	stderr, err := cmd.StderrPipe()
	if err != nil {
		io.Errorf("pushImage(docker push) exec command error, message:%s\n", err.Error())
		return err
	}
	if err := cmd.Start(); err != nil {
		io.Errorf("pushImage(docker push) exec command error, message:%s\n", err.Error())
		return err
	}
	go asyncLog(stdout, io)
	go asyncLog(stderr, io)
	if err := cmd.Wait(); err != nil {
		io.Errorf("pushImage(docker push) wait for command execution error:%s", err.Error())
		return err
	}
	return nil
}
