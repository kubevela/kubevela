package apis

import "k8s.io/apimachinery/pkg/runtime"

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
