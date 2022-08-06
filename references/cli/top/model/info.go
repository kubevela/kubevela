package model

import (
	"fmt"
	"runtime"
	"strconv"
	"strings"

	"k8s.io/cli-runtime/pkg/genericclioptions"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd/api"

	"github.com/oam-dev/kubevela/apis/types"
	"github.com/oam-dev/kubevela/pkg/utils/helm"
	"github.com/oam-dev/kubevela/version"
)

// Info is system info struct
type Info struct {
	config *api.Config
}

const (
	// UnknownVersion unknown info
	Unknown = "UNKNOWN"
)

// NewInfo return a new info struct
func NewInfo() *Info {
	info := &Info{}
	k := genericclioptions.NewConfigFlags(true)
	rawConfig, err := k.ToRawKubeConfigLoader().RawConfig()
	if err == nil {
		info.config = &rawConfig
	}
	return info
}

// CurrentContext return current context info
func (i *Info) CurrentContext() string {
	if i.config != nil {
		return i.config.CurrentContext
	}
	return Unknown
}

func (i *Info) ClusterNum() string {
	if i.config != nil {
		return strconv.Itoa(len(i.config.Clusters))
	}
	return "0"
}

// K8SVersion return k8s version info
func K8SVersion(config *rest.Config) string {
	client, err := kubernetes.NewForConfig(config)
	if err != nil {
		return Unknown
	}
	serverVersion, err := client.ServerVersion()
	if err != nil {
		return Unknown
	}
	vStr := fmt.Sprintf("%s.%s", serverVersion.Major, strings.Replace(serverVersion.Minor, "+", "", 1))
	return vStr
}

// VelaCLIVersion return vela cli version info
func VelaCLIVersion() string {
	return version.VelaVersion
}

// VelaCoreVersion return vela core version info
func VelaCoreVersion() string {
	results, err := helm.GetHelmRelease(types.DefaultKubeVelaNS)
	if err != nil {
		return Unknown
	}

	for _, result := range results {
		if result.Chart.ChartFullPath() == types.DefaultKubeVelaChartName {
			return result.Chart.AppVersion()
		}
	}
	return Unknown
}

// GOLangVersion return golang version info
func GOLangVersion() string {
	return runtime.Version()
}
