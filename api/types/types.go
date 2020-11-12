package types

const (
	DefaultOAMNS               = "vela-system"
	DefaultOAMReleaseName      = "kubevela"
	DefaultOAMRuntimeChartName = "vela-core"
	DefaultOAMVersion          = ">0.0.0-0"

	DefaultEnvName      = "default"
	DefaultAppNamespace = "default"
)

const (
	AnnDescription = "definition.oam.dev/description"

	LabelPodSpecable = "workload.oam.dev/podspecable"
)

const (
	StatusDeployed = "Deployed"
	StatusStaging  = "Staging"
)

type EnvMeta struct {
	Name      string `json:"name"`
	Namespace string `json:"namespace"`
	Email     string `json:"email,omitempty"`
	Domain    string `json:"domain,omitempty"`

	// Below are not arguments, should be auto-generated
	Issuer  string `json:"issuer"`
	Current string `json:"current,omitempty"`
}

const (
	TagCommandType = "commandType"

	TypeStart   = "Getting Started"
	TypeApp     = "Applications"
	TypeTraits  = "Traits"
	TypeRelease = "Release"
	TypeOthers  = "Others"
	TypeSystem  = "System"
)
