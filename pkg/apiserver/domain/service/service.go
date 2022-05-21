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

package service

import (
	"context"
	"fmt"

	"github.com/oam-dev/kubevela/pkg/apiserver/config"
)

// needInitData register the service that need to init data
var needInitData []DataInit

// InitServiceBean init all service instance
func InitServiceBean(c config.Config) []interface{} {
	clusterService := NewClusterService()
	rbacService := NewRBACService()
	projectService := NewProjectService()
	envService := NewEnvService()
	targetService := NewTargetService()
	workflowService := NewWorkflowService()
	oamApplicationService := NewOAMApplicationService()
	velaQLService := NewVelaQLService()
	definitionService := NewDefinitionService()
	addonService := NewAddonService(c.AddonCacheTime)
	envBindingService := NewEnvBindingService()
	systemInfoService := NewSystemInfoService()
	helmService := NewHelmService()
	userService := NewUserService()
	authenticationService := NewAuthenticationService()
	configService := NewConfigService()
	applicationService := NewApplicationService()
	webhookService := NewWebhookService()
	needInitData = []DataInit{clusterService, userService, rbacService, projectService, targetService, systemInfoService}
	return []interface{}{
		clusterService, rbacService, projectService, envService, targetService, workflowService, oamApplicationService,
		velaQLService, definitionService, addonService, envBindingService, systemInfoService, helmService, userService,
		authenticationService, configService, applicationService, webhookService,
	}
}

// DataInit the service set that needs init data
type DataInit interface {
	Init(ctx context.Context) error
}

// InitData init data
func InitData(ctx context.Context) error {
	for _, init := range needInitData {
		if err := init.Init(ctx); err != nil {
			return fmt.Errorf("database init failure %w", err)
		}
	}
	return nil
}
