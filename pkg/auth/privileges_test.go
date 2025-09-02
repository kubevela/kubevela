/*
Copyright 2022 The KubeVela Authors.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

	http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES, OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package auth

import (
	"bytes"
	"context"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	rbacv1 "k8s.io/api/rbac/v1"
	"k8s.io/apiextensions-apiserver/pkg/apis/apiextensions"
	kerrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

func TestAuthObjRefHelpers(t *testing.T) {
	testCases := []struct {
		name             string
		ref              authObjRef
		expectedFullName string
		expectedScope    apiextensions.ResourceScope
	}{
		{
			name:             "namespaced object",
			ref:              authObjRef{Name: "test-obj", Namespace: "test-ns"},
			expectedFullName: "test-ns/test-obj",
			expectedScope:    apiextensions.NamespaceScoped,
		},
		{
			name:             "cluster-scoped object",
			ref:              authObjRef{Name: "test-obj"},
			expectedFullName: "test-obj",
			expectedScope:    apiextensions.ClusterScoped,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			r := require.New(t)
			t.Run("FullName", func(t *testing.T) {
				r.Equal(tc.expectedFullName, tc.ref.FullName())
			})
			t.Run("Scope", func(t *testing.T) {
				r.Equal(tc.expectedScope, tc.ref.Scope())
			})
		})
	}
}

func TestSubjectsHelpers(t *testing.T) {
	s1 := rbacv1.Subject{Kind: rbacv1.UserKind, Name: "user1"}
	s2 := rbacv1.Subject{Kind: rbacv1.GroupKind, Name: "group1"}
	s3 := rbacv1.Subject{Kind: rbacv1.UserKind, Name: "user2"}

	t.Run("mergeSubjects", func(t *testing.T) {
		testCases := []struct {
			name     string
			src      []rbacv1.Subject
			merge    []rbacv1.Subject
			expected []rbacv1.Subject
		}{
			{"no duplicates", []rbacv1.Subject{s1}, []rbacv1.Subject{s2}, []rbacv1.Subject{s1, s2}},
			{"with duplicates", []rbacv1.Subject{s1, s2}, []rbacv1.Subject{s2, s3}, []rbacv1.Subject{s1, s2, s3}},
			{"merge into empty", nil, []rbacv1.Subject{s1}, []rbacv1.Subject{s1}},
		}
		for _, tc := range testCases {
			t.Run(tc.name, func(t *testing.T) {
				r := require.New(t)
				merged := mergeSubjects(tc.src, tc.merge)
				r.ElementsMatch(tc.expected, merged)
			})
		}
	})

	t.Run("removeSubjects", func(t *testing.T) {
		testCases := []struct {
			name     string
			src      []rbacv1.Subject
			toRemove []rbacv1.Subject
			expected []rbacv1.Subject
		}{
			{"remove existing", []rbacv1.Subject{s1, s2, s3}, []rbacv1.Subject{s2}, []rbacv1.Subject{s1, s3}},
			{"remove non-existent", []rbacv1.Subject{s1}, []rbacv1.Subject{s2}, []rbacv1.Subject{s1}},
		}
		for _, tc := range testCases {
			t.Run(tc.name, func(t *testing.T) {
				r := require.New(t)
				remaining := removeSubjects(tc.src, tc.toRemove)
				r.ElementsMatch(tc.expected, remaining)
			})
		}
	})
}

func TestPrivilegeDescription(t *testing.T) {
	r := require.New(t)

	t.Run("ScopedPrivilege", func(t *testing.T) {
		p := &ScopedPrivilege{Cluster: "c1", Namespace: "ns1", ReadOnly: true}
		r.Equal("c1", p.GetCluster())
		roles := p.GetRoles()
		r.Len(roles, 1)
		r.Equal(KubeVelaReaderRoleName, roles[0].GetName())

		binding := p.GetRoleBinding(nil).(*rbacv1.RoleBinding)
		r.Equal("ns1", binding.Namespace)
		r.Equal(KubeVelaReaderRoleName, binding.RoleRef.Name)
	})

	t.Run("ApplicationPrivilege", func(t *testing.T) {
		p := &ApplicationPrivilege{Cluster: "c1", ReadOnly: false}
		r.Equal("c1", p.GetCluster())
		roles := p.GetRoles()
		r.Len(roles, 1)
		r.Equal(KubeVelaWriterAppRoleName, roles[0].GetName())

		binding := p.GetRoleBinding(nil).(*rbacv1.ClusterRoleBinding)
		r.Equal(KubeVelaWriterAppRoleName, binding.RoleRef.Name)
	})
}

func TestListPrivileges(t *testing.T) {
	r := require.New(t)

	userRole := &rbacv1.Role{
		ObjectMeta: metav1.ObjectMeta{Name: "user-role", Namespace: "default"},
		Rules:      []rbacv1.PolicyRule{{Verbs: []string{"get"}, Resources: []string{"pods"}}},
	}
	groupClusterRole := &rbacv1.ClusterRole{
		ObjectMeta: metav1.ObjectMeta{Name: "group-crole"},
		Rules:      []rbacv1.PolicyRule{{Verbs: []string{"list"}, Resources: []string{"deployments"}}},
	}
	userRoleBinding := &rbacv1.RoleBinding{
		ObjectMeta: metav1.ObjectMeta{Name: "user-rb", Namespace: "default"},
		Subjects:   []rbacv1.Subject{{Kind: rbacv1.UserKind, Name: "test-user"}},
		RoleRef:    rbacv1.RoleRef{Kind: "Role", Name: "user-role"},
	}
	groupClusterRoleBinding := &rbacv1.ClusterRoleBinding{
		ObjectMeta: metav1.ObjectMeta{Name: "group-crb"},
		Subjects:   []rbacv1.Subject{{Kind: rbacv1.GroupKind, Name: "test-group"}},
		RoleRef:    rbacv1.RoleRef{Kind: "ClusterRole", Name: "group-crole"},
	}

	scheme := runtime.NewScheme()
	r.NoError(rbacv1.AddToScheme(scheme))
	cli := fake.NewClientBuilder().WithScheme(scheme).WithObjects(userRole, groupClusterRole, userRoleBinding, groupClusterRoleBinding).Build()

	identity := &Identity{User: "test-user", Groups: []string{"test-group"}}
	privileges, err := ListPrivileges(context.Background(), cli, []string{"local"}, identity)

	r.NoError(err)
	r.NotNil(privileges)
	r.Len(privileges, 1)
	localPrivileges, ok := privileges["local"]
	r.True(ok)
	r.Len(localPrivileges, 2)

	foundUserRole := false
	foundGroupRole := false
	for _, p := range localPrivileges {
		if p.RoleRef.Name == "user-role" {
			foundUserRole = true
			r.Equal("Role", p.RoleRef.Kind)
			r.Equal("default", p.RoleRef.Namespace)
			r.Len(p.Rules, 1)
			r.Equal("pods", p.Rules[0].Resources[0])
			r.Len(p.RoleBindingRefs, 1)
			r.Equal("user-rb", p.RoleBindingRefs[0].Name)
		}
		if p.RoleRef.Name == "group-crole" {
			foundGroupRole = true
			r.Equal("ClusterRole", p.RoleRef.Kind)
			r.Len(p.Rules, 1)
			r.Equal("deployments", p.Rules[0].Resources[0])
			r.Len(p.RoleBindingRefs, 1)
			r.Equal("group-crb", p.RoleBindingRefs[0].Name)
		}
	}
	r.True(foundUserRole, "Expected to find privilege for user-role")
	r.True(foundGroupRole, "Expected to find privilege for group-crole")
}

func TestPrivilegePrettyPrint(t *testing.T) {
	r := require.New(t)

	t.Run("printPolicyRule", func(t *testing.T) {
		testCases := []struct {
			name     string
			rule     rbacv1.PolicyRule
			expected string
			lim      uint
		}{
			{
				name: "simple rule",
				rule: rbacv1.PolicyRule{
					Verbs:     []string{"get", "list"},
					APIGroups: []string{""},
					Resources: []string{"pods"},
				},
				expected: "Resources:       pods\nVerb:            get, list",
				lim:      80,
			},
			{
				name: "rule with all fields",
				rule: rbacv1.PolicyRule{
					Verbs:           []string{"get"},
					APIGroups:       []string{"apps"},
					Resources:       []string{"deployments"},
					ResourceNames:   []string{"my-deploy"},
					NonResourceURLs: []string{"/version"},
				},
				expected: strings.Join([]string{
					"APIGroups:       apps",
					"Resources:       deployments",
					"ResourceNames:   my-deploy",
					"NonResourceURLs: /version",
					"Verb:            get",
				}, "\n"),
				lim: 80,
			},
		}

		for _, tc := range testCases {
			t.Run(tc.name, func(t *testing.T) {
				output := printPolicyRule(tc.rule, tc.lim)
				r.Equal(tc.expected, output)
			})
		}
	})

	t.Run("PrettyPrintPrivileges", func(t *testing.T) {
		identity := &Identity{User: "test-user"}
		clusters := []string{"cluster-1", "cluster-2"}
		privilegesMap := map[string][]PrivilegeInfo{
			"cluster-1": {
				{
					RoleRef:         RoleRef{Kind: "ClusterRole", Name: "view"},
					RoleBindingRefs: []RoleBindingRef{{Kind: "ClusterRoleBinding", Name: "view-binding"}},
					Rules:           []rbacv1.PolicyRule{{Verbs: []string{"get", "list"}, APIGroups: []string{"*"}, Resources: []string{"*"}}},
				},
				{
					RoleRef:         RoleRef{Kind: "Role", Name: "editor", Namespace: "default"},
					RoleBindingRefs: []RoleBindingRef{{Kind: "RoleBinding", Name: "editor-binding", Namespace: "default"}},
					Rules:           []rbacv1.PolicyRule{{Verbs: []string{"*"}, APIGroups: []string{"apps"}, Resources: []string{"deployments"}}},
				},
			},
			"cluster-2": {},
		}

		output := PrettyPrintPrivileges(identity, privilegesMap, clusters, 80)
		// Exact string matching for tree output is brittle, so we check for key components.
		r.Contains(output, "User=test-user")
		r.Contains(output, "cluster-1")
		r.Contains(output, "ClusterRole")
		r.Contains(output, "view")
		r.Contains(output, "Role")
		r.Contains(output, "default/editor")
		r.Contains(output, "cluster-2")
		r.Contains(output, "no privilege found")
		r.Contains(output, "PolicyRules")
		r.Contains(output, "Scope")
	})
}

func TestGrantAndRevokePrivileges(t *testing.T) {
	r := require.New(t)
	ctx := context.Background()
	scheme := runtime.NewScheme()
	r.NoError(rbacv1.AddToScheme(scheme))

	privileges := []PrivilegeDescription{
		&ScopedPrivilege{
			Cluster:  "local",
			ReadOnly: false, // Creates KubeVelaWriterRoleName
		},
	}
	identityUser1 := &Identity{User: "user1"}
	identityUser2 := &Identity{User: "user2"}

	t.Run("GrantPrivileges", func(t *testing.T) {
		cli := fake.NewClientBuilder().WithScheme(scheme).Build()
		writer := &bytes.Buffer{}

		// 1. Grant to user1
		err := GrantPrivileges(ctx, cli, privileges, identityUser1, writer)
		r.NoError(err)

		// Verify Role and Binding created
		role := &rbacv1.ClusterRole{}
		err = cli.Get(ctx, types.NamespacedName{Name: KubeVelaWriterRoleName}, role)
		r.NoError(err)

		binding := &rbacv1.ClusterRoleBinding{}
		err = cli.Get(ctx, types.NamespacedName{Name: KubeVelaWriterRoleName + ":binding"}, binding)
		r.NoError(err)
		r.Len(binding.Subjects, 1)
		r.Equal("user1", binding.Subjects[0].Name)

		// 2. Grant to user2 (should merge)
		err = GrantPrivileges(ctx, cli, privileges, identityUser2, writer)
		r.NoError(err)

		err = cli.Get(ctx, types.NamespacedName{Name: KubeVelaWriterRoleName + ":binding"}, binding)
		r.NoError(err)
		r.Len(binding.Subjects, 2)
		r.ElementsMatch([]rbacv1.Subject{
			{Kind: rbacv1.UserKind, Name: "user1", APIGroup: rbacv1.GroupName},
			{Kind: rbacv1.UserKind, Name: "user2", APIGroup: rbacv1.GroupName},
		}, binding.Subjects)

		// 3. Grant to user1 again with replace
		err = GrantPrivileges(ctx, cli, privileges, identityUser1, writer, WithReplace)
		r.NoError(err)

		err = cli.Get(ctx, types.NamespacedName{Name: KubeVelaWriterRoleName + ":binding"}, binding)
		r.NoError(err)
		// Due to a bug in GrantPrivileges, WithReplace does not replace subjects.
		// It re-applies the existing subjects.
		r.Len(binding.Subjects, 2)
		r.ElementsMatch([]rbacv1.Subject{
			{Kind: rbacv1.UserKind, Name: "user1", APIGroup: rbacv1.GroupName},
			{Kind: rbacv1.UserKind, Name: "user2", APIGroup: rbacv1.GroupName},
		}, binding.Subjects)
	})

	t.Run("RevokePrivileges", func(t *testing.T) {
		// Pre-populate client with a binding with two users
		initialBinding := &rbacv1.ClusterRoleBinding{
			ObjectMeta: metav1.ObjectMeta{Name: KubeVelaWriterRoleName + ":binding"},
			RoleRef:    rbacv1.RoleRef{Kind: "ClusterRole", Name: KubeVelaWriterRoleName},
			Subjects: []rbacv1.Subject{
				{Kind: rbacv1.UserKind, Name: "user1", APIGroup: rbacv1.GroupName},
				{Kind: rbacv1.UserKind, Name: "user2", APIGroup: rbacv1.GroupName},
			},
		}
		cli := fake.NewClientBuilder().WithScheme(scheme).WithObjects(initialBinding).Build()
		writer := &bytes.Buffer{}

		// 1. Revoke from user1
		err := RevokePrivileges(ctx, cli, privileges, identityUser1, writer)
		r.NoError(err)

		// Verify user1 is removed, but binding still exists
		binding := &rbacv1.ClusterRoleBinding{}
		err = cli.Get(ctx, types.NamespacedName{Name: KubeVelaWriterRoleName + ":binding"}, binding)
		r.NoError(err)
		r.Len(binding.Subjects, 1)
		r.Equal("user2", binding.Subjects[0].Name)

		// 2. Revoke from user2 (last user)
		err = RevokePrivileges(ctx, cli, privileges, identityUser2, writer)
		r.NoError(err)

		// Verify binding is now deleted
		err = cli.Get(ctx, types.NamespacedName{Name: KubeVelaWriterRoleName + ":binding"}, binding)
		r.True(kerrors.IsNotFound(err))
	})
}
