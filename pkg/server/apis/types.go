package apis

import (
	"github.com/cloud-native-application/rudrx/api/types"
	corev1alpha2 "github.com/crossplane/oam-kubernetes-runtime/apis/core/v1alpha2"
	"k8s.io/apimachinery/pkg/runtime"
)

type Environment struct {
	EnvironmentName string `json:"environmentName" binding:"required,min=1,max=32"`
	Namespace       string `json:"namespace" binding:"required,min=1,max=32"`
	Current         string `json:"current,omitempty"`
}

type EnvironmentBody struct {
	Namespace string `json:"namespace" binding:"required,min=1,max=32"`
}

type AppConfig struct {
	AppConfigName  string               `json:"appName" binding:"required,max=64"`
	Definition     runtime.RawExtension `json:"definition" binding:"required"`
	DefinitionType string               `json:"definitionType" binding:"required,max=32"`
	DefinitionName string               `json:"definitionName" binding:"required,max=64"`
}

type Response struct {
	Code int         `json:"code"`
	Data interface{} `json:"data"`
}

type CommonFlag struct {
	Name  string `json:"name"`
	Value string `json:"value"`
}

type WorkloadRunBody struct {
	EnvName      string       `json:"env_name"`
	WorkloadType string       `json:"workload_type"`
	WorkloadName string       `json:"workload_name"`
	AppGroup     string       `json:"app_group,omitempty"`
	Flags        []CommonFlag `json:"flags"`
	Staging      bool         `json:"staging,omitempty"`
	Traits       []TraitBody  `json:"traits,omitempty"`
}

type WorkloadMeta struct {
	Name       string            `json:"name"`
	Parameters []types.Parameter `json:"parameters,omitempty"`
	AppliesTo  []string          `json:"appliesTo,omitempty"`
}

type TraitMeta struct {
	Name       string   `json:"name"`
	Definition string   `json:"definition,omitempty"`
	AppliesTo  []string `json:"applies_to,omitempty"`
}

//used to present trait which is to be attached and, of which parameters are set
type TraitBody struct {
	EnvName      string       `json:"env_name"`
	Name         string       `json:"name"`
	Flags        []CommonFlag `json:"flags"`
	WorkloadName string       `json:"workload_name"`
	AppGroup     string       `json:"app_group,omitempty"`
	Staging      string       `json:"staging,omitempty"`
}

type ApplicationStatusMeta struct {
	Status   string                        `json:"Status,omitempty"`
	Workload corev1alpha2.ComponentSpec    `json:"Workload,omitempty"`
	Traits   []corev1alpha2.ComponentTrait `json:"Traits,omitempty"`
}

type CapabilityMeta struct {
	CapabilityName       string `json:"capability_name"`
	CapabilityCenterName string `json:"capability_center_name,omitempty"`
}

type CapabilityCenterMeta struct {
	Name string `json:"name"`
	Url  string `json:"url"`
}
