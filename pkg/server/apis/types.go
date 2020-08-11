package apis

import "k8s.io/apimachinery/pkg/runtime"

type Environment struct {
	EnvironmentName string `json:"environmentName" binding:"exists,min=1,max=128"`
	Namespace       string `json:"namespace" binding:"exists,min=1,max=128"`
}

type AppConfig struct {
	AppConfigName  string               `json:"appName" binding:"exists,max=256"`
	Definition     runtime.RawExtension `json:"definition" binding:"exists"`
	DefinitionType string               `json:"definitionType" binding:"exists,max=256"`
	DefinitionName string               `json:"definitionName" binding:"exists,max=256"`
}
