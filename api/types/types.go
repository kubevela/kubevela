package types

const (
	DefaultOAMNS               = "oam-system"
	DefaultOAMReleaseName      = "core-runtime"
	DefaultOAMRuntimeChartName = "oam-kubernetes-runtime"
	DefaultOAMRepoName         = "crossplane-master"
	DefaultOAMRepoUrl          = "https://charts.crossplane.io/master"
	DefaultOAMVersion          = ">0.0.0-0"

	DefaultEnvName      = "default"
	DefaultAppNamespace = "default"
)

const (
	AnnApiVersion = "oam.appengine.info/apiVersion"
	AnnKind       = "oam.appengine.info/kind"

	// ComponentWorkloadDefLabel indicate which workloaddefinition generate from
	ComponentWorkloadDefLabel = "vela.oam.dev/workloadDef"
	TraitDefLabel             = "vela.oam.dev/traitDef"
)

type EnvMeta struct {
	Namespace string `json:"namespace"`
	Name      string `json:"name"`
}

const (
	TagCommandType = "commandType"

	TypeStart     = "Getting Started"
	TypeApp       = "Applications"
	TypeWorkloads = "Workloads"
	TypeTraits    = "Traits"
	TypeRelease   = "Release"
	TypeOthers    = "Others"
	TypeSystem    = "System"
)
