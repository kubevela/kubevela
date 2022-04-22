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
	"testing"

	"github.com/stretchr/testify/require"
	authv1 "k8s.io/api/authentication/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apiserver/pkg/authentication/user"
	utilfeature "k8s.io/apiserver/pkg/util/feature"
	featuregatetesting "k8s.io/component-base/featuregate/testing"

	"github.com/oam-dev/kubevela/apis/core.oam.dev/v1beta1"
	"github.com/oam-dev/kubevela/pkg/features"
	"github.com/oam-dev/kubevela/pkg/oam"
)

func TestContextWithUserInfo(t *testing.T) {
	defer featuregatetesting.SetFeatureGateDuringTest(t, utilfeature.DefaultFeatureGate, features.AuthenticateApplication, true)()
	AuthenticationWithUser = true
	defer func() {
		AuthenticationWithUser = false
	}()
	testCases := map[string]struct {
		UserInfo       *authv1.UserInfo
		ServiceAccount string
		ExpectUserInfo user.Info
	}{
		"empty": {
			ExpectUserInfo: &user.DefaultInfo{
				Name:   user.Anonymous,
				Groups: []string{},
			},
		},
		"service-account": {
			ServiceAccount: "sa",
			ExpectUserInfo: &user.DefaultInfo{
				Name:   "system:serviceaccount:default:sa",
				Groups: []string{},
			},
		},
		"user-with-groups": {
			UserInfo: &authv1.UserInfo{
				Username: "user",
				Groups:   []string{"group0", "kubevela:group1", "kubevela:group2"},
			},
			ServiceAccount: "override",
			ExpectUserInfo: &user.DefaultInfo{
				Name:   "user",
				Groups: []string{"kubevela:group1", "kubevela:group2"},
			},
		},
	}
	for name, tt := range testCases {
		t.Run(name, func(t *testing.T) {
			r := require.New(t)
			app := &v1beta1.Application{}
			app.SetNamespace("default")
			if tt.UserInfo != nil {
				SetUserInfoInAnnotation(&app.ObjectMeta, *tt.UserInfo)
			}
			if tt.ServiceAccount != "" {
				metav1.SetMetaDataAnnotation(&app.ObjectMeta, oam.AnnotationApplicationServiceAccountName, tt.ServiceAccount)
			}
			r.Equal(tt.ExpectUserInfo, GetUserInfoInAnnotation(&app.ObjectMeta))
		})
	}
}
