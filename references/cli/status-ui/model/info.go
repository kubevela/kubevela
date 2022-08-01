package model

type Info struct {
	name        string
	k8sVersion  string
	velaVersion string
}

func NewInfo() *Info {
	return &Info{
		name:        "local",
		k8sVersion:  "v20.0.1",
		velaVersion: "v1.4.8",
	}
}

func (c Info) Name() string {
	return c.name
}

func (c Info) K8SVersion() string {
	return c.k8sVersion
}

func (c Info) VelaVersion() string {
	return c.velaVersion
}
