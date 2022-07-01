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

package utils

import (
	"context"

	"github.com/emicklei/go-restful/v3"
	"k8s.io/apiserver/pkg/authentication/user"
	"k8s.io/apiserver/pkg/endpoints/request"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/oam-dev/kubevela/apis/core.oam.dev/v1beta1"
	"github.com/oam-dev/kubevela/pkg/apiserver/domain/model"
	"github.com/oam-dev/kubevela/pkg/features"
)

// KubeVelaProjectGroupPrefix the prefix kubevela project
const KubeVelaProjectGroupPrefix = "kubevela:project:"

// KubeVelaAdminGroupPrefix the prefix kubevela admin
const KubeVelaAdminGroupPrefix = "kubevela:admin:"

// ContextWithUserInfo extract user from context (parse username and project) for impersonation
func ContextWithUserInfo(ctx context.Context) context.Context {
	if !features.APIServerFeatureGate.Enabled(features.APIServerEnableImpersonation) {
		return ctx
	}
	userInfo := &user.DefaultInfo{Name: user.Anonymous}
	if username, ok := UsernameFrom(ctx); ok {
		userInfo.Name = username
	}
	if project, ok := ProjectFrom(ctx); ok {
		userInfo.Groups = []string{KubeVelaProjectGroupPrefix + project}
	}
	if userInfo.Name == model.DefaultAdminUserName && !features.APIServerFeatureGate.Enabled(features.APIServerEnableAdminImpersonation) {
		return ctx
	}
	return request.WithUser(ctx, userInfo)
}

// SetUsernameAndProjectInRequestContext .
func SetUsernameAndProjectInRequestContext(req *restful.Request, userName string, projectName string) {
	ctx := req.Request.Context()
	ctx = WithUsername(ctx, userName)
	ctx = WithProject(ctx, projectName)
	req.Request = req.Request.WithContext(ctx)
}

// NewAuthApplicationClient will carry UserInfo for mutating requests related to application automatically
func NewAuthApplicationClient(cli client.Client) client.Client {
	return &authAppClient{Client: cli}
}

type authAppClient struct {
	client.Client
}

// Status .
func (c *authAppClient) Status() client.StatusWriter {
	return &authAppStatusClient{StatusWriter: c.Client.Status()}
}

// Create .
func (c *authAppClient) Create(ctx context.Context, obj client.Object, opts ...client.CreateOption) error {
	if _, ok := obj.(*v1beta1.Application); ok {
		ctx = ContextWithUserInfo(ctx)
	}
	return c.Client.Create(ctx, obj, opts...)
}

// Delete .
func (c *authAppClient) Delete(ctx context.Context, obj client.Object, opts ...client.DeleteOption) error {
	if _, ok := obj.(*v1beta1.Application); ok {
		ctx = ContextWithUserInfo(ctx)
	}
	return c.Client.Delete(ctx, obj, opts...)
}

// Update .
func (c *authAppClient) Update(ctx context.Context, obj client.Object, opts ...client.UpdateOption) error {
	if _, ok := obj.(*v1beta1.Application); ok {
		ctx = ContextWithUserInfo(ctx)
	}
	return c.Client.Update(ctx, obj, opts...)
}

// Patch .
func (c *authAppClient) Patch(ctx context.Context, obj client.Object, patch client.Patch, opts ...client.PatchOption) error {
	if _, ok := obj.(*v1beta1.Application); ok {
		ctx = ContextWithUserInfo(ctx)
	}
	return c.Client.Patch(ctx, obj, patch, opts...)
}

type authAppStatusClient struct {
	client.StatusWriter
}

// Update .
func (c *authAppStatusClient) Update(ctx context.Context, obj client.Object, opts ...client.UpdateOption) error {
	if _, ok := obj.(*v1beta1.Application); ok {
		ctx = ContextWithUserInfo(ctx)
	}
	return c.StatusWriter.Update(ctx, obj, opts...)
}

// Patch .
func (c *authAppStatusClient) Patch(ctx context.Context, obj client.Object, patch client.Patch, opts ...client.PatchOption) error {
	if _, ok := obj.(*v1beta1.Application); ok {
		ctx = ContextWithUserInfo(ctx)
	}
	return c.StatusWriter.Patch(ctx, obj, patch, opts...)
}
