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

package auth

import (
	"context"
	"fmt"
	"regexp"
	"strings"

	authv1 "k8s.io/api/authentication/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apiserver/pkg/authentication/user"
	"k8s.io/apiserver/pkg/endpoints/request"
	utilfeature "k8s.io/apiserver/pkg/util/feature"
	"k8s.io/utils/strings/slices"

	monitorContext "github.com/kubevela/pkg/monitor/context"

	"github.com/oam-dev/kubevela/apis/core.oam.dev/v1beta1"
	"github.com/oam-dev/kubevela/pkg/features"
	"github.com/oam-dev/kubevela/pkg/oam"
)

const (
	groupSeparator = ","
)

// ContextWithUserInfo inject username & group from app annotations into context
// If serviceAccount is set and username is empty, identity will user the serviceAccount
func ContextWithUserInfo(ctx context.Context, app *v1beta1.Application) context.Context {
	if app == nil {
		return ctx
	}
	return request.WithUser(ctx, GetUserInfoInAnnotation(&app.ObjectMeta))
}

// MonitorContextWithUserInfo inject username & group from app annotations into monitor context
func MonitorContextWithUserInfo(ctx monitorContext.Context, app *v1beta1.Application) monitorContext.Context {
	_ctx := ctx.GetContext()
	authCtx := ContextWithUserInfo(_ctx, app)
	ctx.SetContext(authCtx)
	return ctx
}

// ContextClearUserInfo clear user info in context
func ContextClearUserInfo(ctx context.Context) context.Context {
	return request.WithUser(ctx, nil)
}

// SetUserInfoInAnnotation set username and group from userInfo into annotations
// it will clear the existing service account annotation in avoid of permission leak
func SetUserInfoInAnnotation(obj *metav1.ObjectMeta, userInfo authv1.UserInfo) {
	if AuthenticationWithUser {
		metav1.SetMetaDataAnnotation(obj, oam.AnnotationApplicationUsername, userInfo.Username)
	}
	re := regexp.MustCompile(strings.ReplaceAll(AuthenticationGroupPattern, "*", ".*"))
	var groups []string
	for _, group := range userInfo.Groups {
		if re.MatchString(group) {
			groups = append(groups, group)
		}
	}
	metav1.SetMetaDataAnnotation(obj, oam.AnnotationApplicationGroup, strings.Join(groups, groupSeparator))
}

// GetUserInfoInAnnotation extract user info from annotations
// support compatibility for serviceAccount when name is empty
func GetUserInfoInAnnotation(obj *metav1.ObjectMeta) user.Info {
	annotations := obj.GetAnnotations()
	if annotations == nil {
		annotations = map[string]string{}
	}

	name := annotations[oam.AnnotationApplicationUsername]
	if serviceAccountName := annotations[oam.AnnotationApplicationServiceAccountName]; serviceAccountName != "" && name == "" {
		name = fmt.Sprintf("system:serviceaccount:%s:%s", obj.GetNamespace(), serviceAccountName)
	}

	if name == "" && utilfeature.DefaultMutableFeatureGate.Enabled(features.AuthenticateApplication) {
		name = AuthenticationDefaultUser
	}

	return &user.DefaultInfo{
		Name: name,
		Groups: slices.Filter(
			[]string{},
			strings.Split(annotations[oam.AnnotationApplicationGroup], groupSeparator),
			func(s string) bool {
				return len(strings.TrimSpace(s)) > 0
			}),
	}
}
