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
	"io"
	"reflect"
	"strings"
	"sync"

	"github.com/gosuri/uitable/util/wordwrap"
	"github.com/xlab/treeprint"
	rbacv1 "k8s.io/api/rbac/v1"
	"k8s.io/apiextensions-apiserver/pkg/apis/apiextensions"
	kerrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/utils/strings/slices"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"

	"github.com/oam-dev/kubevela/pkg/multicluster"
	"github.com/oam-dev/kubevela/pkg/utils"
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

// Scope the scope of the object
func (ref authObjRef) Scope() apiextensions.ResourceScope {
	if ref.Namespace == "" {
		return apiextensions.ClusterScoped
	}
	return apiextensions.NamespaceScoped
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
			bindingsBranch := branch.AddMetaBranch("Scope", "")
			for _, ref := range info.RoleBindingRefs {
				var prefix string
				if ref.Namespace != "" {
					prefix = ref.Namespace + " "
				}
				bindingsBranch.AddMetaNode(authObjRef(ref).Scope(), fmt.Sprintf("%s(%s %s)", prefix, ref.Kind, ref.Name))
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

// PrivilegeDescription describe the privilege to grant
type PrivilegeDescription interface {
	GetCluster() string
	GetRoles() []client.Object
	GetRoleBinding([]rbacv1.Subject) client.Object
}

const (
	// KubeVelaReaderRoleName a role that can read any resources
	KubeVelaReaderRoleName = "kubevela:reader"
	// KubeVelaWriterRoleName a role that can read/write any resources
	KubeVelaWriterRoleName = "kubevela:writer"
	// KubeVelaWriterAppRoleName a role that can read/write any application
	KubeVelaWriterAppRoleName = "kubevela:writer:application"
	// KubeVelaReaderAppRoleName a role that can read any application
	KubeVelaReaderAppRoleName = "kubevela:reader:application"
)

// ScopedPrivilege includes all resource privileges in the destination
type ScopedPrivilege struct {
	Prefix    string
	Cluster   string
	Namespace string
	ReadOnly  bool
}

// GetCluster the cluster of the privilege
func (p *ScopedPrivilege) GetCluster() string {
	return p.Cluster
}

// GetRoles the underlying Roles/ClusterRoles for the privilege
func (p *ScopedPrivilege) GetRoles() []client.Object {
	if p.ReadOnly {
		return []client.Object{&rbacv1.ClusterRole{
			ObjectMeta: metav1.ObjectMeta{Name: p.Prefix + KubeVelaReaderRoleName},
			Rules: []rbacv1.PolicyRule{
				{APIGroups: []string{rbacv1.APIGroupAll}, Resources: []string{rbacv1.ResourceAll}, Verbs: []string{"get", "list", "watch"}},
				{NonResourceURLs: []string{rbacv1.NonResourceAll}, Verbs: []string{"get", "list", "watch"}},
			},
		}}
	}
	return []client.Object{&rbacv1.ClusterRole{
		ObjectMeta: metav1.ObjectMeta{Name: p.Prefix + KubeVelaWriterRoleName},
		Rules: []rbacv1.PolicyRule{
			{APIGroups: []string{rbacv1.APIGroupAll}, Resources: []string{rbacv1.ResourceAll}, Verbs: []string{"get", "list", "watch", "create", "update", "patch", "delete"}},
			{NonResourceURLs: []string{rbacv1.NonResourceAll}, Verbs: []string{"get", "list", "watch", "create", "update", "patch", "delete"}},
		},
	}}
}

// GetRoleBinding the underlying RoleBinding/ClusterRoleBinding for the privilege
func (p *ScopedPrivilege) GetRoleBinding(subs []rbacv1.Subject) client.Object {
	var binding client.Object
	var roleName = KubeVelaWriterRoleName
	if p.ReadOnly {
		roleName = KubeVelaReaderRoleName
	}
	if p.Namespace == "" {
		binding = &rbacv1.ClusterRoleBinding{
			RoleRef:  rbacv1.RoleRef{Kind: "ClusterRole", APIGroup: rbacv1.GroupName, Name: roleName},
			Subjects: subs,
		}
	} else {
		binding = &rbacv1.RoleBinding{
			RoleRef:  rbacv1.RoleRef{Kind: "ClusterRole", APIGroup: rbacv1.GroupName, Name: roleName},
			Subjects: subs,
		}
		binding.SetNamespace(p.Namespace)
	}
	binding.SetName(p.Prefix + roleName + ":binding")
	return binding
}

// ApplicationPrivilege includes the application privileges in the destination
type ApplicationPrivilege struct {
	Prefix    string
	Cluster   string
	Namespace string
	ReadOnly  bool
}

// GetCluster the cluster of the privilege
func (a *ApplicationPrivilege) GetCluster() string {
	return a.Cluster
}

// GetRoles the underlying Roles/ClusterRoles for the privilege
func (a *ApplicationPrivilege) GetRoles() []client.Object {
	verbs := []string{"get", "list", "watch", "create", "update", "patch", "delete"}
	name := a.Prefix + KubeVelaWriterAppRoleName
	if a.ReadOnly {
		verbs = []string{"get", "list", "watch"}
		name = a.Prefix + KubeVelaReaderAppRoleName
	}
	return []client.Object{
		&rbacv1.ClusterRole{
			ObjectMeta: metav1.ObjectMeta{Name: name},
			Rules: []rbacv1.PolicyRule{
				{
					APIGroups: []string{"core.oam.dev"},
					Resources: []string{"applications", "applications/status", "policies", "workflows", "workflowruns", "workflowruns/status"},
					Verbs:     verbs,
				},
				{
					APIGroups: []string{""},
					Resources: []string{"secrets", "configmaps"},
					Verbs:     verbs,
				},
			},
		},
	}
}

// GetRoleBinding the underlying RoleBinding/ClusterRoleBinding for the privilege
func (a *ApplicationPrivilege) GetRoleBinding(subs []rbacv1.Subject) client.Object {
	var binding client.Object
	var roleName = KubeVelaWriterAppRoleName
	if a.ReadOnly {
		roleName = KubeVelaReaderAppRoleName
	}
	if a.Namespace == "" {
		binding = &rbacv1.ClusterRoleBinding{
			RoleRef:  rbacv1.RoleRef{Kind: "ClusterRole", APIGroup: rbacv1.GroupName, Name: roleName},
			Subjects: subs,
		}
	} else {
		binding = &rbacv1.RoleBinding{
			RoleRef:  rbacv1.RoleRef{Kind: "ClusterRole", APIGroup: rbacv1.GroupName, Name: roleName},
			Subjects: subs,
		}
		binding.SetNamespace(a.Namespace)
	}
	binding.SetName(a.Prefix + roleName + ":binding")
	return binding
}

func mergeSubjects(src []rbacv1.Subject, merge []rbacv1.Subject) []rbacv1.Subject {
	subs := append([]rbacv1.Subject{}, src...)
	for _, sub := range merge {
		contains := false
		for _, s := range subs {
			if reflect.DeepEqual(sub, s) {
				contains = true
				break
			}
		}
		if !contains {
			subs = append(subs, sub)
		}
	}
	return subs
}

func removeSubjects(src []rbacv1.Subject, toRemove []rbacv1.Subject) []rbacv1.Subject {
	var subs []rbacv1.Subject
	for _, sub := range src {
		add := true
		for _, t := range toRemove {
			if reflect.DeepEqual(t, sub) {
				add = false
				break
			}
		}
		if add {
			subs = append(subs, sub)
		}
	}
	return subs
}

type opts struct {
	replace bool
}

// WithReplace means to replace all subjects, this is only useful in Grant Privileges
func WithReplace(o *opts) {
	o.replace = true
}

// GrantPrivileges grant privileges to identity
func GrantPrivileges(ctx context.Context, cli client.Client, privileges []PrivilegeDescription, identity *Identity, writer io.Writer, optionFuncs ...func(*opts)) error {
	var options = &opts{}
	for _, fc := range optionFuncs {
		fc(options)
	}
	subs := identity.Subjects()
	if len(subs) == 0 {
		return fmt.Errorf("failed to find RBAC subjects in identity")
	}
	for _, p := range privileges {
		cluster := p.GetCluster()
		_ctx := multicluster.ContextWithClusterName(ctx, cluster)
		for _, role := range p.GetRoles() {
			kind, key := "ClusterRole", role.GetName()
			if role.GetNamespace() != "" {
				kind, key = "Role", role.GetNamespace()+"/"+role.GetName()
			}
			res, err := utils.CreateOrUpdate(_ctx, cli, role)
			if err != nil {
				return fmt.Errorf("failed to create/update %s %s in %s: %w", kind, key, cluster, err)
			}
			if res != controllerutil.OperationResultNone {
				_, _ = fmt.Fprintf(writer, "%s %s %s in %s.\n", kind, key, res, cluster)
			}
		}
		binding := p.GetRoleBinding(subs)
		kind, key := "ClusterRoleBinding", binding.GetName()
		if binding.GetNamespace() != "" {
			kind, key = "RoleBinding", binding.GetNamespace()+"/"+binding.GetName()
		}
		switch bindingObj := binding.(type) {
		case *rbacv1.RoleBinding:
			obj := &rbacv1.RoleBinding{}
			if err := cli.Get(_ctx, client.ObjectKeyFromObject(bindingObj), obj); err == nil {
				if options.replace {
					bindingObj.Subjects = obj.Subjects
				} else {
					bindingObj.Subjects = mergeSubjects(bindingObj.Subjects, obj.Subjects)
				}
			}
		case *rbacv1.ClusterRoleBinding:
			obj := &rbacv1.ClusterRoleBinding{}
			if err := cli.Get(_ctx, client.ObjectKeyFromObject(bindingObj), obj); err == nil {
				if options.replace {
					bindingObj.Subjects = obj.Subjects
				} else {
					bindingObj.Subjects = mergeSubjects(bindingObj.Subjects, obj.Subjects)
				}
			}
		}
		res, err := utils.CreateOrUpdate(_ctx, cli, binding)
		if err != nil {
			return fmt.Errorf("failed to create/update %s %s in %s: %w", kind, key, cluster, err)
		}
		_, _ = fmt.Fprintf(writer, "%s %s %s in %s.\n", kind, key, res, cluster)
	}
	return nil
}

// RevokePrivileges revoke privileges (notice that the revoking process only deletes bond subject in the
// RoleBinding/ClusterRoleBinding, it does not ensure the identity's other related privileges are removed to
// prevent identity from accessing)
func RevokePrivileges(ctx context.Context, cli client.Client, privileges []PrivilegeDescription, identity *Identity, writer io.Writer, optionFuncs ...func(*opts)) error {
	var options = &opts{}
	for _, fc := range optionFuncs {
		fc(options)
	}
	subs := identity.Subjects()
	if len(subs) == 0 {
		return fmt.Errorf("failed to find RBAC subjects in identity")
	}
	for _, p := range privileges {
		cluster := p.GetCluster()
		_ctx := multicluster.ContextWithClusterName(ctx, cluster)
		binding := p.GetRoleBinding(subs)
		kind, key := "ClusterRoleBinding", binding.GetName()
		if binding.GetNamespace() != "" {
			kind, key = "RoleBinding", binding.GetNamespace()+"/"+binding.GetName()
		}
		var err error
		remove := false
		var toDel client.Object
		switch bindingObj := binding.(type) {
		case *rbacv1.RoleBinding:
			obj := &rbacv1.RoleBinding{}
			if err = cli.Get(_ctx, client.ObjectKeyFromObject(bindingObj), obj); err == nil {
				bindingObj.Subjects = removeSubjects(obj.Subjects, bindingObj.Subjects)
				remove = len(bindingObj.Subjects) == 0
				toDel = obj
			}
		case *rbacv1.ClusterRoleBinding:
			obj := &rbacv1.ClusterRoleBinding{}
			if err = cli.Get(_ctx, client.ObjectKeyFromObject(bindingObj), obj); err == nil {
				bindingObj.Subjects = removeSubjects(obj.Subjects, bindingObj.Subjects)
				remove = len(bindingObj.Subjects) == 0
				toDel = obj
			}
		}
		if err != nil {
			if !kerrors.IsNotFound(err) {
				return fmt.Errorf("failed to fetch %s %s in cluster %s: %w", kind, key, cluster, err)
			}
			return nil
		}
		if remove {
			if err = cli.Delete(_ctx, toDel); err != nil {
				return fmt.Errorf("failed to delete %s %s in cluster %s: %w", kind, key, cluster, err)
			}
		} else {
			res, err := utils.CreateOrUpdate(_ctx, cli, binding)
			if err != nil {
				return fmt.Errorf("failed to update %s %s in cluster %s: %w", kind, key, cluster, err)
			}
			_, _ = fmt.Fprintf(writer, "%s %s %s in cluster %s.\n", kind, key, res, cluster)
		}
	}
	return nil
}
