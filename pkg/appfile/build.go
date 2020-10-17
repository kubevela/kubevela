package appfile

import (
	"os/exec"

	cmdutil "github.com/oam-dev/kubevela/pkg/commands/util"
)

type Build struct {
	Image  string `json:"image,omitempty"`
	Push   Push   `json:"push,omitempty"`
	Docker Docker `json:"docker,omitempty"`
}

type Docker struct {
	File    string `json:"file"`
	Context string `json:"context"`
}

type Push struct {
	Local    string `json:"local,omitempty"`
	Registry string `json:"registry,omitempty"`
}

func (b *Build) BuildImage(io cmdutil.IOStreams) error {
	cmd := exec.Command("docker", "build", "-t", b.Image, "-f", b.Docker.File, b.Docker.Context)
	out, err := cmd.CombinedOutput()
	io.Infof("%s\n", out)
	if err != nil {
		return err
	}
	return b.pushImage(io)
}

func (b *Build) pushImage(io cmdutil.IOStreams) error {
	io.Infof("pushing image (%s)...\n", b.Image)

	switch {
	case b.Push.Local == "kind":
		cmd := exec.Command("kind", "load", "docker-image", b.Image)
		out, err := cmd.CombinedOutput()
		io.Infof("%s\n", out)
		return err
	}
	cmd := exec.Command("docker", "push", b.Image)
	out, err := cmd.CombinedOutput()
	io.Infof("%s\n", out)
	return err
}
