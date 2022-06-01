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

package config

import (
	"context"
	"fmt"
	"strings"

	tcv1beta1 "github.com/oam-dev/terraform-controller/api/v1beta1"
	"github.com/pkg/errors"
	kerrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/oam-dev/kubevela/apis/core.oam.dev/common"
	"github.com/oam-dev/kubevela/apis/core.oam.dev/v1beta1"
	"github.com/oam-dev/kubevela/apis/types"
	"github.com/oam-dev/kubevela/pkg/apiserver/domain/model"
)

const (
	errAuthenticateProvider   = "failed to authenticate Terraform cloud provider %s for %s"
	errProviderExists         = "terraform provider %s for %s already exists"
	errDeleteProvider         = "failed to delete Terraform Provider %s err: %w"
	errCouldNotDeleteProvider = "the Terraform Provider %s could not be disabled because it was created by enabling a Terraform provider or was manually created"
	errCheckProviderExistence = "failed to check if Terraform Provider %s exists"
)

// UIParam is the UI parameters from VelaUX for the application
type UIParam struct {
	Alias       string `json:"alias"`
	Description string `json:"description"`
	Project     string `json:"project"`
}

// CreateApplication creates a new application for the config
func CreateApplication(ctx context.Context, k8sClient client.Client, name, componentType, properties string, ui UIParam) error {
	if strings.HasPrefix(componentType, types.TerraformComponentPrefix) {
		existed, err := IsTerraformProviderExisted(ctx, k8sClient, name)
		if err != nil {
			return errors.Wrapf(err, errAuthenticateProvider, name, componentType)
		}
		if existed {
			return fmt.Errorf(errProviderExists, name, componentType)
		}
	}
	app := v1beta1.Application{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: types.DefaultKubeVelaNS,
			Annotations: map[string]string{
				types.AnnotationConfigAlias:       ui.Alias,
				types.AnnotationConfigDescription: ui.Description,
			},
			Labels: map[string]string{
				model.LabelSourceOfTruth: model.FromInner,
				types.LabelConfigCatalog: types.VelaCoreConfig,
				types.LabelConfigType:    componentType,
				types.LabelConfigProject: ui.Project,
			},
		},
		Spec: v1beta1.ApplicationSpec{
			Components: []common.ApplicationComponent{
				{
					Name:       name,
					Type:       componentType,
					Properties: &runtime.RawExtension{Raw: []byte(properties)},
				},
			},
		},
	}
	return k8sClient.Create(ctx, &app)
}

// DeleteApplication deletes a config application, including a Terraform provider.
// For a Terraform Provider, it can come from
// 1). manually created a Terraform Provider object, like https://github.com/oam-dev/terraform-controller/blob/master/getting-started.md#aws
// 2). by enabling a Terraform provider addon in version older than v1.3.0
// 3). by create a Terraform provider via `vela provider add`
// 4). by VelaUX
// We will only target on deleting a provider which comes from 3) or 4) as for 1), it can be easily delete by hand, and
// for 2), it will be recreated by the addon.
func DeleteApplication(ctx context.Context, k8sClient client.Client, name string, isTerraformProvider bool) error {
	if isTerraformProvider {
		existed, err := IsTerraformProviderExisted(ctx, k8sClient, name)
		if err != nil {
			return errors.Wrapf(err, errCheckProviderExistence, name)
		}
		if existed {
			// In version 1.3.0, we used `providerAppName` as the name of the application to create a provider, but
			// in version 1.3.1, a config name is the config name, ie, the provider name. To keep backward compatibility,
			// we need to check the legacy name and the current name of the application.
			legacyName := fmt.Sprintf("%s-%s", types.ProviderAppPrefix, name)
			err1 := k8sClient.Get(ctx, client.ObjectKey{Namespace: types.DefaultKubeVelaNS, Name: legacyName}, &v1beta1.Application{})
			if err1 == nil {
				name = legacyName
			}
			if err1 != nil {
				if !kerrors.IsNotFound(err1) {
					return fmt.Errorf(errDeleteProvider, name, err1)
				}
				err2 := k8sClient.Get(ctx, client.ObjectKey{Namespace: types.DefaultKubeVelaNS, Name: name}, &v1beta1.Application{})
				if err2 != nil {
					if kerrors.IsNotFound(err2) {
						return fmt.Errorf(errCouldNotDeleteProvider, name)
					}
					return fmt.Errorf(errDeleteProvider, name, err2)
				}
			}
		}
	}

	a := &v1beta1.Application{}
	if err := k8sClient.Get(ctx, client.ObjectKey{Namespace: types.DefaultKubeVelaNS, Name: name}, a); err != nil {
		return err
	}
	if err := k8sClient.Delete(ctx, a); err != nil {
		return err
	}
	return nil
}

// ListTerraformProviders returns a list of Terraform providers.
func ListTerraformProviders(ctx context.Context, k8sClient client.Client) ([]tcv1beta1.Provider, error) {
	l := &tcv1beta1.ProviderList{}
	if err := k8sClient.List(ctx, l, client.InNamespace(types.ProviderNamespace)); err != nil {
		return nil, err
	}
	return l.Items, nil
}

// IsTerraformProviderExisted returns whether a Terraform provider exists.
func IsTerraformProviderExisted(ctx context.Context, k8sClient client.Client, name string) (bool, error) {
	l, err := ListTerraformProviders(ctx, k8sClient)
	if err != nil {
		return false, err
	}
	for _, p := range l {
		if p.Name == name {
			return true, nil
		}
	}
	return false, nil
}
