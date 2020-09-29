package apis

import (
	corev1alpha2 "github.com/crossplane/oam-kubernetes-runtime/apis/core/v1alpha2"
	"k8s.io/apimachinery/pkg/runtime"

	"github.com/oam-dev/kubevela/api/types"
)

type Environment struct {
	EnvName   string `json:"envName" binding:"required,min=1,max=32"`
	Namespace string `json:"namespace" binding:"required,min=1,max=32"`
	Email     string `json:"email"`
	Domain    string `json:"domain"`
	Current   string `json:"current,omitempty"`
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
	EnvName      string       `json:"envName"`
	WorkloadType string       `json:"workloadType"`
	WorkloadName string       `json:"workloadName"`
	AppName      string       `json:"appName,omitempty"`
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
	AppliesTo  []string `json:"appliesTo,omitempty"`
}

//used to present trait which is to be attached and, of which parameters are set
type TraitBody struct {
	EnvName       string       `json:"envName"`
	Name          string       `json:"name"`
	Flags         []CommonFlag `json:"flags"`
	ComponentName string       `json:"componentName"`
	AppName       string       `json:"appName,omitempty"`
	Staging       string       `json:"staging,omitempty"`
}

//type ComponentMeta struct {
//	Name     string                        `json:"name"`
//	Status   string                        `json:"status,omitempty"`
//	Workload runtime.RawExtension          `json:"workload,omitempty"`
//	Traits   []corev1alpha2.ComponentTrait `json:"traits,omitempty"`
//}

type ComponentMeta struct {
	Name     string               `json:"name"`
	Status   string               `json:"status,omitempty"`
	Workload runtime.RawExtension `json:"workload,omitempty"`
	//WorkloadName for `vela comp ls`
	WorkloadName string                        `json:"workloadName,omitempty"`
	Traits       []corev1alpha2.ComponentTrait `json:"traits,omitempty"`
	//TraitNames for `vela comp ls`
	TraitNames  []string                              `json:"traitsNames,omitempty"`
	App         string                                `json:"app"`
	CreatedTime string                                `json:"createdTime,omitempty"`
	AppConfig   corev1alpha2.ApplicationConfiguration `json:"-"`
	Component   corev1alpha2.Component                `json:"-"`
}

type ApplicationMeta struct {
	Name        string          `json:"name"`
	Status      string          `json:"status,omitempty"`
	Components  []ComponentMeta `json:"components,omitempty"`
	CreatedTime string          `json:"createdTime,omitempty"`
}

type CapabilityMeta struct {
	CapabilityName       string `json:"capabilityName"`
	CapabilityCenterName string `json:"capabilityCenterName,omitempty"`
}

type CapabilityCenterMeta struct {
	Name string `json:"name"`
	URL  string `json:"url"`
}
