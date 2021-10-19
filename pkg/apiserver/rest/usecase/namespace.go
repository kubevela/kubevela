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

package usecase

import (
	"context"
	"time"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/oam-dev/kubevela/pkg/apiserver/clients"
	"github.com/oam-dev/kubevela/pkg/apiserver/log"
	apisv1 "github.com/oam-dev/kubevela/pkg/apiserver/rest/apis/v1"
	"github.com/oam-dev/kubevela/pkg/apiserver/rest/utils/bcode"
)

// NamespaceUsecase namespace manage usecase.
// Namespace acts as the tenant isolation model on the control side.
type NamespaceUsecase interface {
	ListNamespaces(ctx context.Context) ([]apisv1.NamespaceBase, error)
	CreateNamespace(ctx context.Context, req apisv1.CreateNamespaceRequest) (*apisv1.NamespaceBase, error)
}

// AnnotationDescription set namespace description in annotation
const AnnotationDescription string = "description"

// LabelCreator set namesapce creator in labels
const LabelCreator string = "creator"

type namespaceUsecaseImpl struct {
	kubeClient client.Client
}

// NewNamespaceUsecase new namespace usecase
func NewNamespaceUsecase() NamespaceUsecase {
	kubecli, err := clients.GetKubeClient()
	if err != nil {
		log.Logger.Fatalf("get kubeclient failure %s", err.Error())
	}
	return &namespaceUsecaseImpl{kubeClient: kubecli}
}

// ListNamespaces list controller cluster namespaces
func (n *namespaceUsecaseImpl) ListNamespaces(ctx context.Context) ([]apisv1.NamespaceBase, error) {

	// TODO: Consider whether to query only namespaces created by Vela
	var kubeNamespaces corev1.NamespaceList
	if err := n.kubeClient.List(ctx, &kubeNamespaces, &client.ListOptions{}); err != nil {
		log.Logger.Errorf("query namespace list from cluster failure %s", err.Error())
		return nil, bcode.ErrNamespaceQuery
	}
	var namespaces []apisv1.NamespaceBase
	for _, namesapce := range kubeNamespaces.Items {
		namespaces = append(namespaces, apisv1.NamespaceBase{
			Name:        namesapce.Name,
			Description: namesapce.Annotations[AnnotationDescription],
			CreateTime:  namesapce.CreationTimestamp.Time,
			UpdateTime:  namesapce.CreationTimestamp.Time,
		})
	}
	return namespaces, nil
}

// CreateNamespace create namespace to controller cluster
func (n *namespaceUsecaseImpl) CreateNamespace(ctx context.Context, req apisv1.CreateNamespaceRequest) (*apisv1.NamespaceBase, error) {
	if err := n.kubeClient.Create(ctx, &corev1.Namespace{
		ObjectMeta: metav1.ObjectMeta{
			Name: req.Name,
			Labels: map[string]string{
				LabelCreator: "kubevela",
			},
			Annotations: map[string]string{
				AnnotationDescription: req.Description,
			},
		},
		Spec: corev1.NamespaceSpec{},
	}); err != nil {
		if apierrors.IsAlreadyExists(err) {
			return nil, bcode.ErrNamespaceIsExist
		}
		return nil, err
	}
	return &apisv1.NamespaceBase{
		Name:        req.Name,
		Description: req.Description,
		CreateTime:  time.Now(),
		UpdateTime:  time.Now(),
	}, nil
}
