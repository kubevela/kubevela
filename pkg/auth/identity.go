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
	"fmt"
	"strings"

	corev1 "k8s.io/api/core/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	"k8s.io/apiserver/pkg/authentication/serviceaccount"
	"k8s.io/utils/strings/slices"
)

// Identity the kubernetes identity
type Identity struct {
	User                    string
	Groups                  []string
	ServiceAccount          string
	ServiceAccountNamespace string
}

// String .
func (identity *Identity) String() string {
	var tokens []string
	if identity.User != "" {
		tokens = append(tokens, "User="+identity.User)
	}
	if len(identity.Groups) > 0 {
		tokens = append(tokens, "Groups="+strings.Join(identity.Groups, ","))
	}
	if identity.ServiceAccount != "" {
		tokens = append(tokens, "SA="+serviceaccount.MakeUsername(identity.ServiceAccountNamespace, identity.ServiceAccount))
	}
	return strings.Join(tokens, " ")
}

// Match validate if identity matches rbac subject
func (identity *Identity) Match(subject rbacv1.Subject) bool {
	switch subject.Kind {
	case rbacv1.UserKind:
		return subject.Name == identity.User
	case rbacv1.GroupKind:
		return slices.Contains(identity.Groups, subject.Name)
	case rbacv1.ServiceAccountKind:
		return serviceaccount.MatchesUsername(subject.Namespace, subject.Name,
			serviceaccount.MakeUsername(identity.ServiceAccountNamespace, identity.ServiceAccount))
	default:
		return false
	}
}

// MatchAny validate if identity matches any one of the rbac subjects
func (identity *Identity) MatchAny(subjects []rbacv1.Subject) bool {
	for _, subject := range subjects {
		if identity.Match(subject) {
			return true
		}
	}
	return false
}

// Regularize clean up input info
func (identity *Identity) Regularize() {
	identity.User = strings.TrimSpace(identity.User)
	groupMap := map[string]struct{}{}
	var groups []string
	for _, group := range identity.Groups {
		group = strings.TrimSpace(group)
		if _, found := groupMap[group]; !found {
			groupMap[group] = struct{}{}
			groups = append(groups, group)
		}
	}
	identity.Groups = groups
	identity.ServiceAccount = strings.TrimSpace(identity.ServiceAccount)
	if identity.ServiceAccount != "" {
		if identity.ServiceAccountNamespace == "" {
			identity.ServiceAccountNamespace = corev1.NamespaceDefault
		}
	}
}

// Validate check if identity is valid
func (identity *Identity) Validate() error {
	if identity.User == "" && identity.ServiceAccount == "" {
		return fmt.Errorf("either `user` or `serviceaccount` should be set")
	}
	if identity.User != "" && identity.ServiceAccount != "" {
		return fmt.Errorf("cannot set `user` and `serviceaccount` at the same time")
	}
	if len(identity.Groups) > 0 && identity.ServiceAccount != "" {
		return fmt.Errorf("cannot set `group` and `serviceaccount` at the same time")
	}
	if identity.ServiceAccount == "" && identity.ServiceAccountNamespace != "" {
		return fmt.Errorf("cannot set serviceaccount namespace when serviceaccount is not set")
	}
	return nil
}

// Subjects return rbac subjects
func (identity *Identity) Subjects() []rbacv1.Subject {
	var subs []rbacv1.Subject
	if identity.User != "" {
		subs = append(subs, rbacv1.Subject{Kind: rbacv1.UserKind, APIGroup: rbacv1.GroupName, Name: identity.User})
	}
	for _, group := range identity.Groups {
		subs = append(subs, rbacv1.Subject{Kind: rbacv1.GroupKind, APIGroup: rbacv1.GroupName, Name: group})
	}
	if identity.ServiceAccount != "" {
		subs = append(subs, rbacv1.Subject{Kind: rbacv1.ServiceAccountKind, Name: identity.ServiceAccount, Namespace: identity.ServiceAccountNamespace})
	}
	return subs
}
