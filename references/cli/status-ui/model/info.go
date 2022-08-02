package model

import (
	"fmt"
	"github.com/oam-dev/kubevela/apis/types"
	"github.com/oam-dev/kubevela/pkg/utils/helm"
	"github.com/oam-dev/kubevela/version"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"runtime"
	"strings"
)

type Info struct {
	cluster string
}

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
	serverVersion, err := client.ServerVersion()
	if err != nil {
		return "UNKNOWN"
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
