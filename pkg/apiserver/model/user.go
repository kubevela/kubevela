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

import (
	"fmt"
	"strings"
	"time"

	"github.com/form3tech-oss/jwt-go"
)

func init() {
	RegisterModel(&User{})
	RegisterModel(&ProjectUser{})
	RegisterModel(&Role{})
	RegisterModel(&PermPolicy{})
	RegisterModel(&PermPolicyTemplate{})
}

// DefaultAdminUserName default admin user name
var DefaultAdminUserName = "admin"

// User is the model of user
type User struct {
	BaseModel
	Name          string    `json:"name"`
	Email         string    `json:"email"`
	Alias         string    `json:"alias,omitempty"`
	Password      string    `json:"password,omitempty"`
	Disabled      bool      `json:"disabled"`
	LastLoginTime time.Time `json:"lastLoginTime,omitempty"`
	// UserRoles binding the platform level roles
	UserRoles []string `json:"userRoles"`
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

// ProjectUser is the model of user in project
type ProjectUser struct {
	BaseModel
	Username    string `json:"username"`
	ProjectName string `json:"projectName"`
	// UserRoles binding the project level roles
	UserRoles []string `json:"userRoles"`
}

// TableName return custom table name
func (u *ProjectUser) TableName() string {
	return tableNamePrefix + "project_user"
}

// ShortTableName return custom table name
func (u *ProjectUser) ShortTableName() string {
	return "pusr"
}

// PrimaryKey return custom primary key
func (u *ProjectUser) PrimaryKey() string {
	return fmt.Sprintf("%s-%s", u.ProjectName, verifyUserValue(u.Username))
}

// Index return custom index
func (u *ProjectUser) Index() map[string]string {
	index := make(map[string]string)
	if u.Username != "" {
		index["username"] = verifyUserValue(u.Username)
	}
	if u.ProjectName != "" {
		index["projectName"] = u.ProjectName
	}
	return index
}

func verifyUserValue(v string) string {
	s := strings.ReplaceAll(v, "@", "-")
	s = strings.ReplaceAll(s, " ", "-")
	return strings.ToLower(s)
}

// CustomClaims is the custom claims
type CustomClaims struct {
	Username  string `json:"username"`
	GrantType string `json:"grantType"`
	jwt.StandardClaims
}

// Role is a model for a new RBAC mode.
type Role struct {
	BaseModel
	Name         string   `json:"name"`
	Alias        string   `json:"alias"`
	Project      string   `json:"project,omitempty"`
	PermPolicies []string `json:"permPolicies"`
}

// PermPolicy is a model for a new RBAC mode.
type PermPolicy struct {
	BaseModel
	Name      string   `json:"name"`
	Alias     string   `json:"alias"`
	Project   string   `json:"project,omitempty"`
	Resources []string `json:"resources"`
	Actions   []string `json:"actions"`
	// Effect option values: Allow,Deny
	Effect    string     `json:"effect"`
	Principal *Principal `json:"principal,omitempty"`
	Condition *Condition `json:"condition,omitempty"`
}

// Principal is a model for a new RBAC mode.
type Principal struct {
	// Type options: User or Role
	Type  string   `json:"type"`
	Names []string `json:"names"`
}

// Condition is a model for a new RBAC mode.
type Condition struct {
}

// TableName return custom table name
func (r *Role) TableName() string {
	return tableNamePrefix + "role"
}

// ShortTableName return custom table name
func (r *Role) ShortTableName() string {
	return "role"
}

// PrimaryKey return custom primary key
func (r *Role) PrimaryKey() string {
	if r.Project == "" {
		return r.Name
	}
	return fmt.Sprintf("%s-%s", r.Project, r.Name)
}

// Index return custom index
func (r *Role) Index() map[string]string {
	index := make(map[string]string)
	if r.Name != "" {
		index["name"] = r.Name
	}
	if r.Project != "" {
		index["project"] = r.Project
	}
	return index
}

// TableName return custom table name
func (p *PermPolicy) TableName() string {
	return tableNamePrefix + "perm"
}

// ShortTableName return custom table name
func (p *PermPolicy) ShortTableName() string {
	return "perm"
}

// PrimaryKey return custom primary key
func (p *PermPolicy) PrimaryKey() string {
	if p.Project == "" {
		return p.Name
	}
	return fmt.Sprintf("%s-%s", p.Project, p.Name)
}

// Index return custom index
func (p *PermPolicy) Index() map[string]string {
	index := make(map[string]string)
	if p.Name != "" {
		index["name"] = p.Name
	}
	if p.Project != "" {
		index["project"] = p.Project
	}
	if p.Principal != nil && p.Principal.Type != "" {
		index["principal.type"] = p.Principal.Type
	}
	return index
}

// PermPolicyTemplate is a model for a new RBAC mode.
type PermPolicyTemplate struct {
	BaseModel
	Name  string `json:"name"`
	Alias string `json:"alias"`
	// Level options: project or platform
	Level     string     `json:"level"`
	Resources []string   `json:"resources"`
	Actions   []string   `json:"actions"`
	Effect    string     `json:"effect"`
	Condition *Condition `json:"condition,omitempty"`
}

// TableName return custom table name
func (p *PermPolicyTemplate) TableName() string {
	return tableNamePrefix + "perm_temp"
}

// ShortTableName return custom table name
func (p *PermPolicyTemplate) ShortTableName() string {
	return "perm_temp"
}

// PrimaryKey return custom primary key
func (p *PermPolicyTemplate) PrimaryKey() string {
	return p.Name
}

// Index return custom index
func (p *PermPolicyTemplate) Index() map[string]string {
	index := make(map[string]string)
	if p.Name != "" {
		index["name"] = p.Name
	}
	if p.Level != "" {
		index["level"] = p.Level
	}
	return index
}
