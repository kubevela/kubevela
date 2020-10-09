package appfile

import (
	"os/exec"
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

func (b *Build) BuildImage(ctx *Context) error {
	cmd := exec.Command("docker", "build", "-t", b.Image, "-f", b.Docker.File, b.Docker.Context)
	out, err := cmd.CombinedOutput()
	ctx.IO.Infof("%s\n", out)
	if err != nil {
		return err
	}
	return b.pushImage(ctx)
}

func (b *Build) pushImage(ctx *Context) error {
	ctx.IO.Infof("pushing image (%s)...\n", b.Image)

	switch {
	case b.Push.Local == "kind":
		cmd := exec.Command("kind", "load", "docker-image", b.Image)
		out, err := cmd.CombinedOutput()
		ctx.IO.Infof("%s\n", out)
		return err
	}
	cmd := exec.Command("docker", "push", b.Image)
	out, err := cmd.CombinedOutput()
	ctx.IO.Infof("%s\n", out)
	return err
}
