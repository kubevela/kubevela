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
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/require"
	authenticationv1 "k8s.io/api/authentication/v1"
	authorizationv1 "k8s.io/api/authorization/v1"
	"k8s.io/client-go/kubernetes/scheme"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/client/interceptor"
)

func TestUserCan(t *testing.T) {
	r := require.New(t)
	ctx := context.Background()
	var seen *authorizationv1.SubjectAccessReview
	cli := interceptor.NewClient(fake.NewClientBuilder().WithScheme(scheme.Scheme).Build(), interceptor.Funcs{
		Create: func(_ context.Context, _ client.WithWatch, obj client.Object, _ ...client.CreateOption) error {
			review, ok := obj.(*authorizationv1.SubjectAccessReview)
			if !ok {
				return errors.New("not a review")
			}
			seen = review
			switch review.Spec.ResourceAttributes.Name {
			case "granted":
				review.Status.Allowed = true
			case "broken":
				return errors.New("the API server is away")
			}
			return nil
		},
	})
	user := authenticationv1.UserInfo{Username: "alice", Groups: []string{"scope-users"}, UID: "u1", Extra: map[string]authenticationv1.ExtraValue{"k": {"v"}}}
	attrs := authorizationv1.ResourceAttributes{Verb: "get", Group: "config.oam.dev", Version: "v1alpha1", Resource: "configtemplates", Namespace: "wf-b"}

	attrs.Name = "granted"
	allowed, err := UserCan(ctx, cli, user, attrs)
	r.NoError(err)
	r.True(allowed)
	r.Equal("alice", seen.Spec.User)
	r.Equal([]string{"scope-users"}, seen.Spec.Groups)
	r.Equal("u1", seen.Spec.UID)
	r.Equal(authorizationv1.ExtraValue{"v"}, seen.Spec.Extra["k"], "the whole identity travels, as the API server authorizes it")
	r.Equal("wf-b", seen.Spec.ResourceAttributes.Namespace)

	attrs.Name = "denied"
	allowed, err = UserCan(ctx, cli, user, attrs)
	r.NoError(err)
	r.False(allowed)

	attrs.Name = "broken"
	_, err = UserCan(ctx, cli, user, attrs)
	r.Error(err, "a review that could not run is an error, not a denial")
	r.Contains(err.Error(), "could not review whether alice may get configtemplates wf-b/broken")
}
