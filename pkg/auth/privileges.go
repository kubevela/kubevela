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
	"strings"
	"sync"

	"github.com/gosuri/uitable/util/wordwrap"
	"github.com/xlab/treeprint"
	rbacv1 "k8s.io/api/rbac/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/utils/strings/slices"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/oam-dev/kubevela/pkg/multicluster"
	velaerrors "github.com/oam-dev/kubevela/pkg/utils/errors"
	"github.com/oam-dev/kubevela/pkg/utils/parallel"
)

// PrivilegeInfo describes one privilege in Kubernetes. Either one ClusterRole or
// one Role is referenced. Related PolicyRules that describes the resource level
// admissions are included. The RoleBindingRefs records where this RoleRef comes
// from (from which ClusterRoleBinding or RoleBinding).
type PrivilegeInfo struct {
	Rules           []rbacv1.PolicyRule `json:"rules,omitempty"`
	RoleRef         `json:"roleRef,omitempty"`
	RoleBindingRefs []RoleBindingRef `json:"roleBindingRefs,omitempty"`
}

type authObjRef struct {
	Kind      string `json:"kind,omitempty"`
	Name      string `json:"name,omitempty"`
	Namespace string `json:"namespace,omitempty"`
}

// FullName the namespaced name string
func (ref authObjRef) FullName() string {
	if ref.Namespace == "" {
		return ref.Name
	}
	return ref.Namespace + "/" + ref.Name
}

// RoleRef the references to ClusterRole or Role
type RoleRef authObjRef

// RoleBindingRef the reference to ClusterRoleBinding or RoleBinding
type RoleBindingRef authObjRef

// ListPrivileges retrieve privilege information in specified clusters
func ListPrivileges(ctx context.Context, cli client.Client, clusters []string, identity *Identity) (map[string][]PrivilegeInfo, error) {
	var m sync.Map
	errs := parallel.Run(func(cluster string) error {
		info, err := listPrivilegesInCluster(ctx, cli, cluster, identity)
		if err != nil {
			return err
		}
		m.Store(cluster, info)
		return nil
	}, clusters, parallel.DefaultParallelism)
	if err := velaerrors.AggregateErrors(errs.([]error)); err != nil {
		return nil, err
	}
	privilegesMap := make(map[string][]PrivilegeInfo)
	m.Range(func(key, value interface{}) bool {
		privilegesMap[key.(string)] = value.([]PrivilegeInfo)
		return true
	})
	return privilegesMap, nil
}

func listPrivilegesInCluster(ctx context.Context, cli client.Client, cluster string, identity *Identity) ([]PrivilegeInfo, error) {
	ctx = multicluster.ContextWithClusterName(ctx, cluster)
	clusterRoleBindings := &rbacv1.ClusterRoleBindingList{}
	roleBindings := &rbacv1.RoleBindingList{}
	if err := cli.List(ctx, clusterRoleBindings); err != nil {
		return nil, err
	}
	roleRefMap := make(map[RoleRef][]RoleBindingRef)
	for _, clusterRoleBinding := range clusterRoleBindings.Items {
		if identity.MatchAny(clusterRoleBinding.Subjects) {
			roleRef := RoleRef{
				Kind: clusterRoleBinding.RoleRef.Kind,
				Name: clusterRoleBinding.RoleRef.Name,
			}
			roleRefMap[roleRef] = append(roleRefMap[roleRef], RoleBindingRef{
				Kind: "ClusterRoleBinding",
				Name: clusterRoleBinding.Name})
		}
	}
	if err := cli.List(ctx, roleBindings); err != nil {
		return nil, err
	}
	for _, roleBinding := range roleBindings.Items {
		for i := range roleBinding.Subjects {
			roleBinding.Subjects[i].Namespace = roleBinding.Namespace
		}
		if identity.MatchAny(roleBinding.Subjects) {
			roleRef := RoleRef{
				Kind: roleBinding.RoleRef.Kind,
				Name: roleBinding.RoleRef.Name,
			}
			if roleRef.Kind == "Role" {
				roleRef.Namespace = roleBinding.Namespace
			}
			roleRefMap[roleRef] = append(roleRefMap[roleRef], RoleBindingRef{
				Kind:      "RoleBinding",
				Name:      roleBinding.Name,
				Namespace: roleBinding.Namespace})
		}
	}

	var infos []PrivilegeInfo
	for roleRef, roleBindingRefs := range roleRefMap {
		infos = append(infos, PrivilegeInfo{RoleRef: roleRef, RoleBindingRefs: roleBindingRefs})
	}
	var m sync.Map
	errs := parallel.Run(func(info PrivilegeInfo) error {
		key := types.NamespacedName{Namespace: info.RoleRef.Namespace, Name: info.RoleRef.Name}
		var rules []rbacv1.PolicyRule
		if info.RoleRef.Kind == "Role" {
			role := &rbacv1.Role{}
			if err := cli.Get(ctx, key, role); err != nil {
				return err
			}
			rules = role.Rules
		} else {
			clusterRole := &rbacv1.ClusterRole{}
			if err := cli.Get(ctx, key, clusterRole); err != nil {
				return err
			}
			rules = clusterRole.Rules
		}
		m.Store(authObjRef(info.RoleRef).FullName(), rules)
		return nil
	}, infos, parallel.DefaultParallelism)
	if err := velaerrors.AggregateErrors(errs.([]error)); err != nil {
		return nil, err
	}
	for i, info := range infos {
		obj, ok := m.Load(authObjRef(info.RoleRef).FullName())
		if ok {
			infos[i].Rules = obj.([]rbacv1.PolicyRule)
		}
	}
	return infos, nil
}

func printPolicyRule(rule rbacv1.PolicyRule, lim uint) string {
	var rows []string
	addRow := func(name string, values []string) {
		values = slices.Filter(nil, values, func(s string) bool {
			return len(s) > 0
		})
		if len(values) > 0 {
			s := wordwrap.WrapString(strings.Join(values, ", "), lim)
			for i, line := range strings.Split(s, "\n") {
				prefix := []byte(name + " ")
				if i > 0 {
					for j := range prefix {
						prefix[j] = ' '
					}
				}
				rows = append(rows, string(prefix)+line)
			}
		}
	}
	addRow("APIGroups:      ", rule.APIGroups)
	addRow("Resources:      ", rule.Resources)
	addRow("ResourceNames:  ", rule.ResourceNames)
	addRow("NonResourceURLs:", rule.NonResourceURLs)
	addRow("Verb:           ", rule.Verbs)
	return strings.Join(rows, "\n")
}

// PrettyPrintPrivileges print cluster privileges map in tree format
func PrettyPrintPrivileges(identity *Identity, privilegesMap map[string][]PrivilegeInfo, clusters []string, lim uint) string {
	tree := treeprint.New()
	tree.SetValue(identity.String())
	for _, cluster := range clusters {
		privileges, exists := privilegesMap[cluster]
		if !exists {
			continue
		}
		root := tree.AddMetaBranch("Cluster", cluster)
		for _, info := range privileges {
			branch := root.AddMetaBranch(info.RoleRef.Kind, authObjRef(info.RoleRef).FullName())
			bindingsBranch := branch.AddMetaBranch("Bindings", "")
			for _, ref := range info.RoleBindingRefs {
				bindingsBranch.AddMetaNode(ref.Kind, authObjRef(ref).FullName())
			}
			rulesBranch := branch.AddMetaBranch("PolicyRules", "")
			for _, rule := range info.Rules {
				rulesBranch.AddNode(printPolicyRule(rule, lim))
			}
		}
		if len(privileges) == 0 {
			root.AddNode("no privilege found")
		}
	}
	return tree.String()
}
