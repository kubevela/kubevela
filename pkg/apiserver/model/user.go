/*
Copyright 2022 The KubeVela Authors.

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

import "strings"

func init() {
	RegisterModel(&User{})
}

// User is the model of user
type User struct {
	BaseModel
	Name     string `json:"name"`
	Email    string `json:"email"`
	Alias    string `json:"alias,omitempty"`
	Password string `json:"password,omitempty"`
	Disabled bool   `json:"disabled"`
}

// TableName return custom table name
func (u *User) TableName() string {
	return tableNamePrefix + "user"
}

// ShortTableName return custom table name
func (u *User) ShortTableName() string {
	return "usr"
}

// PrimaryKey return custom primary key
func (u *User) PrimaryKey() string {
	return verifyUserValue(u.Name)
}

// Index return custom index
func (u *User) Index() map[string]string {
	index := make(map[string]string)
	if u.Name != "" {
		index["name"] = verifyUserValue(u.Name)
	}
	if u.Email != "" {
		index["email"] = verifyUserValue(u.Email)
	}
	return index
}

func verifyUserValue(v string) string {
	s := strings.ReplaceAll(v, "@", "-")
	s = strings.ReplaceAll(s, " ", "-")
	return s
}
