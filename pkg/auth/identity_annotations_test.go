/*
Copyright 2026 The KubeVela Authors.

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
	utilfeature "k8s.io/apiserver/pkg/util/feature"
	featuregatetesting "k8s.io/component-base/featuregate/testing"

	"github.com/oam-dev/kubevela/pkg/features"
	"github.com/oam-dev/kubevela/pkg/oam"
)

func withUser(t *testing.T, on bool) {
	t.Helper()
	before := AuthenticationWithUser
	AuthenticationWithUser = on
	t.Cleanup(func() { AuthenticationWithUser = before })
}

// With authentication.withUser off, Applications run as the default user with the requester's
// groups, so a username in the annotations is never the requester's and is never acted as.
func TestWithoutWithUserNoUsernameIsKeptOrActedAs(t *testing.T) {
	featuregatetesting.SetFeatureGateDuringTest(t, utilfeature.DefaultMutableFeatureGate, features.AuthenticateApplication, true)
	withUser(t, false)
	requester := authv1.UserInfo{Username: "tenant", Groups: []string{"kubevela:team-a"}}

	meta := &metav1.ObjectMeta{Namespace: "team-a", Annotations: map[string]string{oam.AnnotationApplicationUsername: "cluster-admin-user"}}
	SetUserInfoInAnnotation(meta, requester)
	require.NotContains(t, meta.Annotations, oam.AnnotationApplicationUsername, "a username the requester wrote is removed")
	require.Equal(t, "kubevela:team-a", meta.Annotations[oam.AnnotationApplicationGroup])

	stored := &metav1.ObjectMeta{Namespace: "team-a", Annotations: map[string]string{
		oam.AnnotationApplicationUsername: "cluster-admin-user", oam.AnnotationApplicationGroup: "kubevela:team-a"}}
	got := GetUserInfoInAnnotation(stored)
	require.Equal(t, AuthenticationDefaultUser, got.GetName(), "one stored before is not acted as either")
	require.Equal(t, []string{"kubevela:team-a"}, got.GetGroups())
}

func TestWithUserTheRequesterIsActedAs(t *testing.T) {
	featuregatetesting.SetFeatureGateDuringTest(t, utilfeature.DefaultMutableFeatureGate, features.AuthenticateApplication, true)
	withUser(t, true)
	meta := &metav1.ObjectMeta{Namespace: "team-a", Annotations: map[string]string{oam.AnnotationApplicationUsername: "cluster-admin-user"}}
	SetUserInfoInAnnotation(meta, authv1.UserInfo{Username: "tenant"})
	require.Equal(t, "tenant", meta.Annotations[oam.AnnotationApplicationUsername], "the requester replaces what was written")
	require.Equal(t, "tenant", GetUserInfoInAnnotation(meta).GetName())
}

// With the gate off there is no webhook stamping and nothing to impersonate against: the legacy
// annotations are read as written.
func TestWithoutAuthenticationTheAnnotationsAreReadAsWritten(t *testing.T) {
	featuregatetesting.SetFeatureGateDuringTest(t, utilfeature.DefaultMutableFeatureGate, features.AuthenticateApplication, false)
	withUser(t, false)
	meta := &metav1.ObjectMeta{Namespace: "team-a", Annotations: map[string]string{oam.AnnotationApplicationServiceAccountName: "deployer"}}
	require.Equal(t, "system:serviceaccount:team-a:deployer", GetUserInfoInAnnotation(meta).GetName())
}
