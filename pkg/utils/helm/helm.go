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

package helm

import (
	"context"
	"fmt"
	"log"
	"os"

	v1 "k8s.io/api/core/v1"
	types2 "k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"helm.sh/helm/v3/pkg/action"
	"helm.sh/helm/v3/pkg/cli"
	"helm.sh/helm/v3/pkg/release"

	"github.com/oam-dev/kubevela/pkg/utils/common"
)

const (
	userNameSecKey     = "username"
	userPasswordSecKey = "password"
	caFileSecKey       = "caFile"
	keyFileKey         = "keyFile"
	certFileKey        = "certFile"
)

var (
	settings = cli.New()
)

func debug(format string, v ...interface{}) {
	if settings.Debug {
		format = fmt.Sprintf("[debug] %s\n", format)
		_ = log.Output(2, fmt.Sprintf(format, v...))
	}
}

// GetHelmRelease will get helm release
func GetHelmRelease(ns string) ([]*release.Release, error) {
	actionConfig := new(action.Configuration)
	client := action.NewList(actionConfig)

	if err := actionConfig.Init(settings.RESTClientGetter(), ns, os.Getenv("HELM_DRIVER"), debug); err != nil {
		return nil, err
	}
	results, err := client.Run()
	if err != nil {
		return nil, err
	}

	return results, nil
}

// SetHTTPOption will read username and password from secret return a httpOption that contain these info.
func SetHTTPOption(ctx context.Context, k8sClient client.Client, secretRef types2.NamespacedName) (*common.HTTPOption, error) {
	sec := v1.Secret{}
	err := k8sClient.Get(ctx, secretRef, &sec)
	if err != nil {
		return nil, err
	}
	opts := &common.HTTPOption{}
	if len(sec.Data[userNameSecKey]) != 0 && len(sec.Data[userPasswordSecKey]) != 0 {
		opts.Username = string(sec.Data[userNameSecKey])
		opts.Password = string(sec.Data[userPasswordSecKey])
	}
	if len(sec.Data[caFileSecKey]) != 0 {
		opts.CaFile = string(sec.Data[caFileSecKey])
	}
	if len(sec.Data[certFileKey]) != 0 {
		opts.CertFile = string(sec.Data[certFileKey])
	}
	if len(sec.Data[keyFileKey]) != 0 {
		opts.KeyFile = string(sec.Data[keyFileKey])
	}
	return opts, nil
}
