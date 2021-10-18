/*
 Copyright 2021. The KubeVela Authors.

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

package usecase

import (
	"context"

	"github.com/oam-dev/kubevela/pkg/apiserver/clients"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	v1 "k8s.io/api/core/v1"
	kerrors "k8s.io/apimachinery/pkg/api/errors"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/oam-dev/kubevela/apis/core.oam.dev/v1beta1"
	apisv1 "github.com/oam-dev/kubevela/pkg/apiserver/rest/apis/v1"
)

// OAMApplicationUsecase oam_application usecase
type OAMApplicationUsecase interface {
	CreateOrUpdateOAMApplication(context.Context, apisv1.ApplicationRequest, string, string) error
	GetOAMApplication(context.Context, string, string) (*apisv1.ApplicationResponse, error)
	DeleteOAMApplication(context.Context, string, string) error
}

// NewOAMApplicationUsecase new oam_application usecase
func NewOAMApplicationUsecase() OAMApplicationUsecase {
	kubeClient, _ := clients.GetKubeClient()
	return &oamApplicationUsecaseImpl{kubeClient: kubeClient}
}

type oamApplicationUsecaseImpl struct {
	kubeClient client.Client
}

// CreateOrUpdateOAMApplication create or update application
func (o oamApplicationUsecaseImpl) CreateOrUpdateOAMApplication(ctx context.Context, request apisv1.ApplicationRequest, name, namespace string) error {
	ns := new(v1.Namespace)
	err := o.kubeClient.Get(ctx, client.ObjectKey{Name: namespace}, ns)
	if kerrors.IsNotFound(err) {
		ns.Name = namespace
		if err = o.kubeClient.Create(ctx, ns); err != nil {
			return err
		}
	}

	app := &v1beta1.Application{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
		},
		Spec: v1beta1.ApplicationSpec{
			Components: request.Components,
			Policies:   request.Policies,
			Workflow:   request.Workflow,
		},
	}

	existApp := new(v1beta1.Application)
	err = o.kubeClient.Get(ctx, client.ObjectKey{Name: name, Namespace: namespace}, existApp)
	if err != nil {
		if kerrors.IsNotFound(err) {
			return o.kubeClient.Create(ctx, app)
		}
		return err
	}

	existApp.Spec = app.Spec
	return o.kubeClient.Update(ctx, existApp)
}

// GetOAMApplication get application
func (o oamApplicationUsecaseImpl) GetOAMApplication(ctx context.Context, name, namespace string) (*apisv1.ApplicationResponse, error) {
	app := new(v1beta1.Application)
	if err := o.kubeClient.Get(ctx, client.ObjectKey{Name: name, Namespace: namespace}, app); err != nil {
		return nil, err
	}
	return &apisv1.ApplicationResponse{
		APIVersion: app.APIVersion,
		Kind:       app.Kind,
		Spec:       app.Spec,
		Status:     app.Status,
	}, nil
}

// DeleteOAMApplication delete application
func (o oamApplicationUsecaseImpl) DeleteOAMApplication(ctx context.Context, name, namespace string) error {
	return client.IgnoreNotFound(o.kubeClient.Delete(ctx, &v1beta1.Application{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
		},
	}))
}
