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

package model

import "time"

func init() {
	RegisterModel(&SystemInfo{})
}

const (
	// LoginTypeDex is the dex login type
	LoginTypeDex string = "dex"
	// LoginTypeLocal is the local login type
	LoginTypeLocal string = "local"
)

// SystemInfo systemInfo model
type SystemInfo struct {
	BaseModel
	InstallID                   string        `json:"installID"`
	EnableCollection            bool          `json:"enableCollection"`
	StatisticInfo               StatisticInfo `json:"statisticInfo,omitempty"`
	LoginType                   string        `json:"loginType"`
	DexUserDefaultProjects      []ProjectRef  `json:"projects"`
	DexUserDefaultPlatformRoles []string      `json:"dexUserDefaultPlatformRoles"`
}

// ProjectRef set the project name and roles
type ProjectRef struct {
	Name  string   `json:"name"`
	Roles []string `json:"roles"`
}

// UpdateDexConfig update dex config
type UpdateDexConfig struct {
	Connectors      []map[string]interface{}
	StaticPasswords []StaticPassword
	VelaAddress     string
}

// DexConfig dex config
type DexConfig struct {
	Issuer           string                   `json:"issuer"`
	Web              DexWeb                   `json:"web"`
	Storage          DexStorage               `json:"storage"`
	Telemetry        Telemetry                `json:"telemetry"`
	Frontend         WebConfig                `json:"frontend"`
	StaticClients    []DexStaticClient        `json:"staticClients"`
	Connectors       []map[string]interface{} `json:"connectors,omitempty"`
	EnablePasswordDB bool                     `json:"enablePasswordDB"`
	StaticPasswords  []StaticPassword         `json:"staticPasswords,omitempty"`
}

// StaticPassword is the static password for dex
type StaticPassword struct {
	Email    string `json:"email"`
	Hash     string `json:"hash"`
	Username string `json:"username"`
}

// StatisticInfo the system statistic info
type StatisticInfo struct {
	ClusterCount        string            `json:"clusterCount,omitempty"`
	AppCount            string            `json:"appCount,omitempty"`
	EnabledAddon        map[string]string `json:"enabledAddon,omitempty"`
	TopKCompDef         []string          `json:"topKCompDef,omitempty"`
	TopKTraitDef        []string          `json:"topKTraitDef,omitempty"`
	TopKWorkflowStepDef []string          `json:"topKWorkflowStepDef,omitempty"`
	TopKPolicyDef       []string          `json:"topKPolicyDef,omitempty"`
	UpdateTime          time.Time         `json:"updateTime,omitempty"`
}

// DexStorage dex storage
type DexStorage struct {
	Type   string           `json:"type"`
	Config DexStorageConfig `json:"config,omitempty"`
}

// DexStorageConfig is the storage config of dex
type DexStorageConfig struct {
	InCluster bool `json:"inCluster"`
}

// DexWeb dex web
type DexWeb struct {
	HTTP           string   `json:"http"`
	HTTPS          string   `json:"https"`
	TLSCert        string   `json:"tlsCert"`
	TLSKey         string   `json:"tlsKey"`
	AllowedOrigins []string `json:"allowedOrigins"`
}

// WebConfig holds the server's frontend templates and asset configuration.
type WebConfig struct {
	LogoURL string

	// Defaults to "dex"
	Issuer string

	// Defaults to "light"
	Theme string
}

// Telemetry is the config format for telemetry including the HTTP server config.
type Telemetry struct {
	HTTP string `json:"http"`
}

// DexStaticClient dex static client
type DexStaticClient struct {
	ID           string   `json:"id"`
	Name         string   `json:"name"`
	Secret       string   `json:"secret"`
	RedirectURIs []string `json:"redirectURIs"`
}

// TableName return custom table name
func (u *SystemInfo) TableName() string {
	return tableNamePrefix + "system_info"
}

// ShortTableName is the compressed version of table name for kubeapi storage and others
func (u *SystemInfo) ShortTableName() string {
	return "sysi"
}

// PrimaryKey return custom primary key
func (u *SystemInfo) PrimaryKey() string {
	return u.InstallID
}

// Index return custom index
func (u *SystemInfo) Index() map[string]string {
	index := make(map[string]string)
	if u.InstallID != "" {
		index["installID"] = u.InstallID
	}
	return index
}
