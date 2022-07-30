package model

type Cluster struct {
	name        string
	k8sVersion  string
	velaVersion string
}

func NewCluster() *Cluster {
	return &Cluster{
		name:        "local",
		k8sVersion:  "v20.0.1",
		velaVersion: "v1.4.8",
	}
}

func (c Cluster) Name() string {
	return c.name
}

func (c Cluster) K8SVersion() string {
	return c.k8sVersion
}

func (c Cluster) VelaVersion() string {
	return c.velaVersion
}
