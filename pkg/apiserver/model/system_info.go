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

func init() {
	RegisterModel(&SystemInfo{})
}

// LoginType is the type of login
type LoginType string

const (
	// LoginTypeDex is the dex login type
	LoginTypeDex LoginType = "dex"
	// LoginTypeLocal is the local login type
	LoginTypeLocal LoginType = "local"
)

// SystemInfo systemInfo model
type SystemInfo struct {
	BaseModel
	InstallID        string    `json:"installID"`
	EnableCollection bool      `json:"enableCollection"`
	LoginType        LoginType `json:"loginType"`
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
