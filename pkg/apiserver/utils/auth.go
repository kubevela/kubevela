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

	"github.com/oam-dev/kubevela/pkg/apiserver/domain/model"
	"github.com/oam-dev/kubevela/pkg/auth"
	"github.com/oam-dev/kubevela/pkg/features"
)

// KubeVelaProjectGroupPrefix the prefix kubevela project
const KubeVelaProjectGroupPrefix = "kubevela:project:"

// KubeVelaProjectReadGroupPrefix the prefix kubevela project group that only has the read permissions
const KubeVelaProjectReadGroupPrefix = "kubevela:project-ro:"

// KubeVelaAdminGroupPrefix the prefix kubevela admin
const KubeVelaAdminGroupPrefix = "kubevela:admin:"

// TemplateReaderGroup This group includes the permission that read the ConfigMap in the vela-system namespace.
const TemplateReaderGroup = "template-reader"

// UXDefaultGroup This group means directly using the original identity registered by the cluster.
const UXDefaultGroup = "kubevela:ux"

// ContextWithUserInfo extract user from context (parse username and project) for impersonation
func ContextWithUserInfo(ctx context.Context) context.Context {
	if !features.APIServerFeatureGate.Enabled(features.APIServerEnableImpersonation) {
		return ctx
	}
	userInfo := &user.DefaultInfo{Name: user.Anonymous}
	if username, ok := UsernameFrom(ctx); ok {
		userInfo.Name = username
	}
	if project, ok := ProjectFrom(ctx); ok && project != "" {
		userInfo.Groups = []string{KubeVelaProjectGroupPrefix + project, auth.KubeVelaClientGroup}
	} else {
		userInfo.Groups = []string{UXDefaultGroup}
	}
	if userInfo.Name == model.DefaultAdminUserName && features.APIServerFeatureGate.Enabled(features.APIServerEnableAdminImpersonation) {
		userInfo.Groups = []string{UXDefaultGroup}
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

// NewAuthClient will carry UserInfo for mutating requests automatically
func NewAuthClient(cli client.Client) client.Client {
	return &authClient{Client: cli}
}

type authClient struct {
	client.Client
}

// Status .
func (c *authClient) Status() client.StatusWriter {
	return &authAppStatusClient{StatusWriter: c.Client.Status()}
}

// Get .
func (c *authClient) Get(ctx context.Context, key client.ObjectKey, obj client.Object) error {
	ctx = ContextWithUserInfo(ctx)
	return c.Client.Get(ctx, key, obj)
}

// List .
func (c *authClient) List(ctx context.Context, obj client.ObjectList, opts ...client.ListOption) error {
	ctx = ContextWithUserInfo(ctx)
	return c.Client.List(ctx, obj, opts...)
}

// Create .
func (c *authClient) Create(ctx context.Context, obj client.Object, opts ...client.CreateOption) error {
	ctx = ContextWithUserInfo(ctx)
	return c.Client.Create(ctx, obj, opts...)
}

// Delete .
func (c *authClient) Delete(ctx context.Context, obj client.Object, opts ...client.DeleteOption) error {
	ctx = ContextWithUserInfo(ctx)
	return c.Client.Delete(ctx, obj, opts...)
}

// Update .
func (c *authClient) Update(ctx context.Context, obj client.Object, opts ...client.UpdateOption) error {
	ctx = ContextWithUserInfo(ctx)

	return c.Client.Update(ctx, obj, opts...)
}

// Patch .
func (c *authClient) Patch(ctx context.Context, obj client.Object, patch client.Patch, opts ...client.PatchOption) error {
	ctx = ContextWithUserInfo(ctx)
	return c.Client.Patch(ctx, obj, patch, opts...)
}

// DeleteAllOf .
func (c *authClient) DeleteAllOf(ctx context.Context, obj client.Object, opts ...client.DeleteAllOfOption) error {
	ctx = ContextWithUserInfo(ctx)
	return c.Client.DeleteAllOf(ctx, obj, opts...)
}

type authAppStatusClient struct {
	client.StatusWriter
}

// Update .
func (c *authAppStatusClient) Update(ctx context.Context, obj client.Object, opts ...client.UpdateOption) error {
	ctx = ContextWithUserInfo(ctx)
	return c.StatusWriter.Update(ctx, obj, opts...)
}

// Patch .
func (c *authAppStatusClient) Patch(ctx context.Context, obj client.Object, patch client.Patch, opts ...client.PatchOption) error {
	ctx = ContextWithUserInfo(ctx)
	return c.StatusWriter.Patch(ctx, obj, patch, opts...)
}
