package apis

import (
	"github.com/cloud-native-application/rudrx/api/types"
	"k8s.io/apimachinery/pkg/runtime"
)

type Environment struct {
	EnvironmentName string `json:"environmentName" binding:"required,min=1,max=32"`
	Namespace       string `json:"namespace" binding:"required,min=1,max=32"`
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

type WorkloadFlag struct {
	Name  string `json:"name"`
	Value string `json:"value"`
}
type WorkloadRunBody struct {
	EnvName      string         `json:"env_name"`
	WorkloadType string         `json:"workload_type"`
	WorkloadName string         `json:"workload_name"`
	AppGroup     string         `json:"app_group"`
	Flags        []WorkloadFlag `json:"flags"`
	Staging      bool           `json:"staging"`
}

type Capability struct {
	Name       string            `json:"name"`
	Parameters []types.Parameter `json:"parameters,omitempty"`
	AppliesTo  []string          `json:"appliesTo,omitempty"`
}
