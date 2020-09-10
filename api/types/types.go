package types

const (
	DefaultOAMNS               = "oam-system"
	DefaultOAMReleaseName      = "core-runtime"
	DefaultOAMRuntimeChartName = "oam-kubernetes-runtime"
	DefaultOAMRepoName         = "crossplane-master"
	DefaultOAMRepoURL          = "https://charts.crossplane.io/master"
	DefaultOAMVersion          = ">0.0.0-0"

	DefaultEnvName      = "default"
	DefaultAppNamespace = "default"
)

const (
	AnnAPIVersion = "definition.oam.dev/apiVersion"
	AnnKind       = "definition.oam.dev/kind"

	// Indicate which workloadDefinition generate from
	AnnWorkloadDef = "workload.oam.dev/name"
	// Indicate which traitDefinition generate from
	AnnTraitDef = "trait.oam.dev/name"
)

const (
	StatusDeployed = "Deployed"
	StatusStaging  = "Staging"
)

type EnvMeta struct {
	Name      string `json:"name"`
	Current   string `json:"current,omitempty"`
	Namespace string `json:"namespace"`
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
