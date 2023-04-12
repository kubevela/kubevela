/*
Copyright 2023 The KubeVela Authors.

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

package config

import (
	"context"
	_ "embed"

	"github.com/oam-dev/kubevela/pkg/utils/helm"
	"github.com/oam-dev/kubevela/pkg/utils/registries"

	"github.com/kubevela/pkg/cue/cuex/providers"
	cuexruntime "github.com/kubevela/pkg/cue/cuex/runtime"
	"github.com/kubevela/pkg/util/runtime"
	"k8s.io/klog/v2"
)

// ImageRegistryVars is the vars for image registry validation
type ImageRegistryVars struct {
	Registry string `json:"registry"`
	Auth     struct {
		Username string `json:"username"`
		Password string `json:"password"`
		Email    string `json:"email"`
	} `json:"auth"`
	Insecure bool `json:"insecure"`
	UseHTTP  bool `json:"useHTTP"`
}

// HelmRepositoryVars is the vars for helm repository validation
type HelmRepositoryVars struct {
	URL      string `json:"url"`
	Username string `json:"username"`
	Password string `json:"password"`
	CAFile   string `json:"caFile"`
}

// ResponseVars is the returns for resource
type ResponseVars struct {
	Result  bool   `json:"result"`
	Message string `json:"message"`
}

// ImageRegistryParams is the params for image registry
type ImageRegistryParams providers.Params[ImageRegistryVars]

// HelmRepositoryParams is the params for helm repository
type HelmRepositoryParams providers.Params[HelmRepositoryVars]

// ValidationReturns returned struct for http response
type ValidationReturns providers.Returns[ResponseVars]

// ImageRegistry .
func ImageRegistry(ctx context.Context, validationParams *ImageRegistryParams) (*ValidationReturns, error) {
	params := validationParams.Params
	imageRegistry := &registries.ImageRegistry{
		Registry: params.Registry,
		Auth:     params.Auth,
		Insecure: params.Insecure,
		UseHTTP:  params.Insecure,
	}
	registryHelper := registries.NewRegistryHelper()
	var message string
	ok, err := registryHelper.Auth(ctx, imageRegistry)
	if err != nil {
		message = err.Error()
		klog.Errorf("validate image-registry %s failed, err: %v", imageRegistry, err)
	}
	return &ValidationReturns{
		Returns: ResponseVars{
			Result:  ok,
			Message: message,
		},
	}, nil
}

// HelmRepository .
func HelmRepository(ctx context.Context, validationParams *HelmRepositoryParams) (*ValidationReturns, error) {
	params := validationParams.Params
	helmRepository := &helm.Repository{
		URL:      params.URL,
		Username: params.Username,
		Password: params.Password,
		CAFile:   params.CAFile,
	}
	helmHelper := helm.NewHelper()
	var message string
	ok, err := helmHelper.ValidateRepo(ctx, helmRepository)
	if err != nil {
		message = err.Error()
		klog.Errorf("validate helm-repository %s failed, err: %v", helmRepository, err)
	}
	return &ValidationReturns{
		Returns: ResponseVars{
			Result:  ok,
			Message: message,
		},
	}, nil
}

// ProviderName .
const ProviderName = "config"

//go:embed config.cue
var template string

// Package .
var Package = runtime.Must(cuexruntime.NewInternalPackage(ProviderName, template, map[string]cuexruntime.ProviderFn{
	"image-registry":  cuexruntime.GenericProviderFn[ImageRegistryParams, ValidationReturns](ImageRegistry),
	"helm-repository": cuexruntime.GenericProviderFn[HelmRepositoryParams, ValidationReturns](HelmRepository),
}))
