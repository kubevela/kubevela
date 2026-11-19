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
	rbacv1 "k8s.io/api/rbac/v1"
)

func TestIdentity(t *testing.T) {
	t.Run("String", func(t *testing.T) {
		testCases := map[string]struct {
			identity Identity
			expected string
		}{
			"user only": {
				identity: Identity{User: "test-user"},
				expected: "User=test-user",
			},
			"user and groups": {
				identity: Identity{User: "test-user", Groups: []string{"group1", "group2"}},
				expected: "User=test-user Groups=group1,group2",
			},
			"service account only": {
				identity: Identity{ServiceAccount: "sa-name", ServiceAccountNamespace: "sa-ns"},
				expected: "SA=system:serviceaccount:sa-ns:sa-name",
			},
			"all fields": {
				identity: Identity{User: "test-user", Groups: []string{"group1"}, ServiceAccount: "sa-name", ServiceAccountNamespace: "sa-ns"},
				expected: "User=test-user Groups=group1 SA=system:serviceaccount:sa-ns:sa-name",
			},
			"empty": {
				identity: Identity{},
				expected: "",
			},
		}
		for name, tc := range testCases {
			t.Run(name, func(t *testing.T) {
				r := require.New(t)
				r.Equal(tc.expected, tc.identity.String())
			})
		}
	})

	t.Run("Match", func(t *testing.T) {
		identity := &Identity{
			User:                    "test-user",
			Groups:                  []string{"group1", "group2"},
			ServiceAccount:          "sa-name",
			ServiceAccountNamespace: "sa-ns",
		}
		testCases := map[string]struct {
			subject  rbacv1.Subject
			expected bool
		}{
			"match user": {
				subject:  rbacv1.Subject{Kind: rbacv1.UserKind, Name: "test-user"},
				expected: true,
			},
			"not match user": {
				subject:  rbacv1.Subject{Kind: rbacv1.UserKind, Name: "another-user"},
				expected: false,
			},
			"match group": {
				subject:  rbacv1.Subject{Kind: rbacv1.GroupKind, Name: "group1"},
				expected: true,
			},
			"not match group": {
				subject:  rbacv1.Subject{Kind: rbacv1.GroupKind, Name: "group3"},
				expected: false,
			},
			"match service account": {
				subject:  rbacv1.Subject{Kind: rbacv1.ServiceAccountKind, Name: "sa-name", Namespace: "sa-ns"},
				expected: true,
			},
			"not match service account name": {
				subject:  rbacv1.Subject{Kind: rbacv1.ServiceAccountKind, Name: "another-sa", Namespace: "sa-ns"},
				expected: false,
			},
			"not match service account namespace": {
				subject:  rbacv1.Subject{Kind: rbacv1.ServiceAccountKind, Name: "sa-name", Namespace: "another-ns"},
				expected: false,
			},
			"unknown kind": {
				subject:  rbacv1.Subject{Kind: "Unknown", Name: "test-user"},
				expected: false,
			},
		}
		for name, tc := range testCases {
			t.Run(name, func(t *testing.T) {
				r := require.New(t)
				r.Equal(tc.expected, identity.Match(tc.subject))
			})
		}
	})

	t.Run("MatchAny", func(t *testing.T) {
		identity := &Identity{
			User:   "test-user",
			Groups: []string{"group1"},
		}
		testCases := map[string]struct {
			subjects []rbacv1.Subject
			expected bool
		}{
			"match one": {
				subjects: []rbacv1.Subject{
					{Kind: rbacv1.GroupKind, Name: "group-other"},
					{Kind: rbacv1.UserKind, Name: "test-user"},
				},
				expected: true,
			},
			"match none": {
				subjects: []rbacv1.Subject{
					{Kind: rbacv1.GroupKind, Name: "group-other"},
					{Kind: rbacv1.UserKind, Name: "user-other"},
				},
				expected: false,
			},
			"empty subjects": {
				subjects: []rbacv1.Subject{},
				expected: false,
			},
		}
		for name, tc := range testCases {
			t.Run(name, func(t *testing.T) {
				r := require.New(t)
				r.Equal(tc.expected, identity.MatchAny(tc.subjects))
			})
		}
	})

	t.Run("Regularize", func(t *testing.T) {
		testCases := map[string]struct {
			identity *Identity
			expected *Identity
		}{
			"trim spaces": {
				identity: &Identity{User: "  user  ", ServiceAccount: " sa  "},
				expected: &Identity{User: "user", ServiceAccount: "sa", ServiceAccountNamespace: "default"},
			},
			"remove duplicate groups": {
				identity: &Identity{User: "user", Groups: []string{" g1 ", "g2", " g1", ""}},
				expected: &Identity{User: "user", Groups: []string{"g1", "g2", ""}},
			},
			"default sa namespace": {
				identity: &Identity{ServiceAccount: "sa"},
				expected: &Identity{ServiceAccount: "sa", ServiceAccountNamespace: "default"},
			},
			"no change": {
				identity: &Identity{User: "user", Groups: []string{"g1"}},
				expected: &Identity{User: "user", Groups: []string{"g1"}},
			},
		}
		for name, tc := range testCases {
			t.Run(name, func(t *testing.T) {
				r := require.New(t)
				tc.identity.Regularize()
				r.Equal(tc.expected, tc.identity)
			})
		}
	})

	t.Run("Validate", func(t *testing.T) {
		testCases := map[string]struct {
			identity  Identity
			expectErr bool
		}{
			"valid user": {
				identity:  Identity{User: "user"},
				expectErr: false,
			},
			"valid service account": {
				identity:  Identity{ServiceAccount: "sa", ServiceAccountNamespace: "ns"},
				expectErr: false,
			},
			"invalid empty": {
				identity:  Identity{},
				expectErr: true,
			},
			"invalid user and sa": {
				identity:  Identity{User: "user", ServiceAccount: "sa"},
				expectErr: true,
			},
			"invalid group and sa": {
				identity:  Identity{Groups: []string{"g1"}, ServiceAccount: "sa"},
				expectErr: true,
			},
			"invalid sa namespace without sa": {
				identity:  Identity{User: "user", ServiceAccountNamespace: "ns"},
				expectErr: true,
			},
		}
		for name, tc := range testCases {
			t.Run(name, func(t *testing.T) {
				r := require.New(t)
				err := tc.identity.Validate()
				if tc.expectErr {
					r.Error(err)
				} else {
					r.NoError(err)
				}
			})
		}
	})

	t.Run("Subjects", func(t *testing.T) {
		testCases := map[string]struct {
			identity Identity
			expected []rbacv1.Subject
		}{
			"user and groups": {
				identity: Identity{User: "user", Groups: []string{"g1", "g2"}},
				expected: []rbacv1.Subject{
					{Kind: rbacv1.UserKind, APIGroup: rbacv1.GroupName, Name: "user"},
					{Kind: rbacv1.GroupKind, APIGroup: rbacv1.GroupName, Name: "g1"},
					{Kind: rbacv1.GroupKind, APIGroup: rbacv1.GroupName, Name: "g2"},
				},
			},
			"service account": {
				identity: Identity{ServiceAccount: "sa", ServiceAccountNamespace: "ns"},
				expected: []rbacv1.Subject{
					{Kind: rbacv1.ServiceAccountKind, Name: "sa", Namespace: "ns"},
				},
			},
			"empty": {
				identity: Identity{},
				expected: nil,
			},
		}
		for name, tc := range testCases {
			t.Run(name, func(t *testing.T) {
				r := require.New(t)
				r.ElementsMatch(tc.expected, tc.identity.Subjects())
			})
		}
	})
}
