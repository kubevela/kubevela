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
	"fmt"

	authenticationv1 "k8s.io/api/authentication/v1"
	authorizationv1 "k8s.io/api/authorization/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// UserCan asks the API server whether user may do what attrs describe: a SubjectAccessReview
// as the admission webhooks run it, with the request's user, groups, UID and extra fields. The
// error means the review could not be run, not that the user may not; the caller decides
// what that means for its request (the config webhooks answer with an error response).
func UserCan(ctx context.Context, cli client.Client, user authenticationv1.UserInfo, attrs authorizationv1.ResourceAttributes) (bool, error) {
	extra := make(map[string]authorizationv1.ExtraValue, len(user.Extra))
	for k, v := range user.Extra {
		extra[k] = authorizationv1.ExtraValue(v)
	}
	review := &authorizationv1.SubjectAccessReview{
		Spec: authorizationv1.SubjectAccessReviewSpec{
			User:               user.Username,
			Groups:             user.Groups,
			UID:                user.UID,
			Extra:              extra,
			ResourceAttributes: &attrs,
		},
	}
	if err := cli.Create(ctx, review); err != nil {
		return false, fmt.Errorf("could not review whether %s may %s %s %s/%s: %w", user.Username, attrs.Verb, attrs.Resource, attrs.Namespace, attrs.Name, err)
	}
	return review.Status.Allowed, nil
}
