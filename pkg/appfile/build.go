package appfile

import (
	"os/exec"

	cmdutil "github.com/oam-dev/kubevela/pkg/commands/util"
)

type Build struct {
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

func (b *Build) BuildImage(io cmdutil.IOStreams, image string) error {
	cmd := exec.Command("docker", "build", "-t", image, "-f", b.Docker.File, b.Docker.Context)
	out, err := cmd.CombinedOutput()
	io.Infof("%s\n", out)
	if err != nil {
		return err
	}
	return b.pushImage(io, image)
}

func (b *Build) pushImage(io cmdutil.IOStreams, image string) error {
	io.Infof("pushing image (%s)...\n", image)

	switch {
	case b.Push.Local == "kind":
		cmd := exec.Command("kind", "load", "docker-image", image)
		out, err := cmd.CombinedOutput()
		io.Infof("%s\n", out)
		return err
	}

	cmd := exec.Command("docker", "push", image)
	out, err := cmd.CombinedOutput()
	io.Infof("%s\n", out)
	return err
}
