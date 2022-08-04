package model

import (
	"fmt"
	"runtime"
	"strings"

	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"

	"github.com/oam-dev/kubevela/apis/types"
	"github.com/oam-dev/kubevela/pkg/utils/helm"
	"github.com/oam-dev/kubevela/version"
)

type Info struct {
	cluster string
}

const (
	UNKOWN_VERSION = "UNKNOWN"
)

func NewInfo() *Info {
	return &Info{
		cluster: "local",
	}
}

func (i Info) Cluster() string {
	return i.cluster
}

func (i Info) K8SVersion(config *rest.Config) string {
	client, err := kubernetes.NewForConfig(config)
	if err != nil {
		return UNKOWN_VERSION
	}
	serverVersion, err := client.ServerVersion()
	if err != nil {
		return UNKOWN_VERSION
	}
	vStr := fmt.Sprintf("%s.%s", serverVersion.Major, strings.Replace(serverVersion.Minor, "+", "", 1))
	return vStr
}

func (i Info) VelaCLIVersion() string {
	return version.VelaVersion
}

func (i Info) VelaCoreVersion() string {
	results, err := helm.GetHelmRelease(types.DefaultKubeVelaNS)
	if err != nil {
		return "UNKNOWN"
	}

	for _, result := range results {
		if result.Chart.ChartFullPath() == types.DefaultKubeVelaChartName {
			return result.Chart.AppVersion()
		}
	}
	return "UNKNOWN"
}

func (i Info) GOLangVersion() string {
	return runtime.Version()
}
