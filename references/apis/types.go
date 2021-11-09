/*
Copyright 2021 The KubeVela Authors.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package apis

import (
	"k8s.io/apimachinery/pkg/runtime"

	corev1alpha2 "github.com/oam-dev/kubevela/apis/core.oam.dev/v1alpha2"
	"github.com/oam-dev/kubevela/apis/types"
)

// Environment contains all info needed in `vela env` command
type Environment struct {
	EnvName   string `json:"envName" binding:"required,min=1,max=32"`
	Namespace string `json:"namespace" binding:"required,min=1,max=32"`
	Email     string `json:"email"`
	Domain    string `json:"domain"`
	Current   string `json:"current,omitempty"`
}

// EnvironmentBody used for restful API in dashboard server
type EnvironmentBody struct {
	Namespace string `json:"namespace" binding:"required,min=1,max=32"`
}

// Response used for restful API response in dashboard server
type Response struct {
	Code int         `json:"code"`
	Data interface{} `json:"data" swaggerignore:"true"`
}

// CommonFlag used for restful API flags in dashboard server
type CommonFlag struct {
	Name  string `json:"name"`
	Value string `json:"value"`
}

// WorkloadMeta store workload metadata for dashboard restful API server
type WorkloadMeta struct {
	Name        string            `json:"name"`
	Parameters  []types.Parameter `json:"parameters,omitempty"`
	Description string            `json:"description,omitempty"`
}

// TraitBody used to present trait which is to be attached and, of which parameters are set
type TraitBody struct {
	EnvName       string       `json:"envName"`
	Name          string       `json:"name"`
	Flags         []CommonFlag `json:"flags"`
	ComponentName string       `json:"componentName"`
	AppName       string       `json:"appName,omitempty"`
	Staging       string       `json:"staging,omitempty"`
}

// ComponentMeta store component info for dashboard restful API server
type ComponentMeta struct {
	Name     string               `json:"name"`
	Status   string               `json:"status,omitempty"`
	Workload runtime.RawExtension `json:"workload,omitempty"`
	// WorkloadName for `vela comp ls`
	WorkloadName string                        `json:"workloadName,omitempty"`
	Traits       []corev1alpha2.ComponentTrait `json:"traits,omitempty"`
	// TraitNames for `vela comp ls`
	TraitNames  []string                              `json:"traitsNames,omitempty"`
	App         string                                `json:"app"`
	CreatedTime string                                `json:"createdTime,omitempty"`
	AppConfig   corev1alpha2.ApplicationConfiguration `json:"-"`
	Component   corev1alpha2.Component                `json:"-"`
}

// ApplicationMeta used for dashboard restful API server
type ApplicationMeta struct {
	Name        string          `json:"name"`
	Status      string          `json:"status,omitempty"`
	Components  []ComponentMeta `json:"components,omitempty"`
	CreatedTime string          `json:"createdTime,omitempty"`
}

// CapabilityMeta used for dashboard restful API server
type CapabilityMeta struct {
	CapabilityName       string `json:"capabilityName"`
	CapabilityCenterName string `json:"capabilityCenterName,omitempty"`
}

// RegistryConfig is used to store registry config in file
type RegistryConfig struct {
	Name  string `json:"name"`
	URL   string `json:"url"`
	Token string `json:"token"`
}
