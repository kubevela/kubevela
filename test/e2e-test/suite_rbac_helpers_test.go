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

package controllers_test

import (
	"context"
	"errors"
	"fmt"

	rbacv1 "k8s.io/api/rbac/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

const coreSuiteOwnerLabel = "e2e.kubevela.io/run"

type coreSuiteRBAC struct {
	roleName    string
	bindingName string
	runID       string
}

func installCoreSuiteRBAC(ctx context.Context, cli client.Client, runID string) (*coreSuiteRBAC, error) {
	if runID == "" {
		return nil, fmt.Errorf("core suite run ID must not be empty")
	}
	owned := &coreSuiteRBAC{
		roleName:    "oam-example-com-" + runID,
		bindingName: "oam-role-binding-" + runID,
		runID:       runID,
	}
	role := &rbacv1.ClusterRole{
		ObjectMeta: metav1.ObjectMeta{
			Name:   owned.roleName,
			Labels: map[string]string{"oam": "clusterrole", "rbac.oam.dev/aggregate-to-controller": "true", coreSuiteOwnerLabel: runID},
		},
		Rules: []rbacv1.PolicyRule{{
			APIGroups: []string{"example.com"},
			Resources: []string{rbacv1.ResourceAll},
			Verbs:     []string{rbacv1.VerbAll},
		}},
	}
	if err := cli.Create(ctx, role); err != nil {
		return nil, fmt.Errorf("create core suite ClusterRole: %w", err)
	}
	binding := &rbacv1.ClusterRoleBinding{
		ObjectMeta: metav1.ObjectMeta{
			Name:   owned.bindingName,
			Labels: map[string]string{"oam": "clusterrole", coreSuiteOwnerLabel: runID},
		},
		Subjects: []rbacv1.Subject{{Kind: "User", Name: "system:serviceaccount:oam-system:oam-kubernetes-runtime-e2e"}},
		RoleRef:  rbacv1.RoleRef{APIGroup: rbacv1.GroupName, Kind: "ClusterRole", Name: "cluster-admin"},
	}
	if err := cli.Create(ctx, binding); err != nil {
		return nil, errors.Join(fmt.Errorf("create core suite ClusterRoleBinding: %w", err), owned.deleteRole(ctx, cli))
	}
	return owned, nil
}

func (owned *coreSuiteRBAC) cleanup(ctx context.Context, cli client.Client) error {
	if owned == nil {
		return nil
	}
	return errors.Join(owned.deleteBinding(ctx, cli), owned.deleteRole(ctx, cli))
}

func (owned *coreSuiteRBAC) deleteBinding(ctx context.Context, cli client.Client) error {
	binding := &rbacv1.ClusterRoleBinding{}
	if err := cli.Get(ctx, client.ObjectKey{Name: owned.bindingName}, binding); err != nil {
		return client.IgnoreNotFound(err)
	}
	if binding.Labels[coreSuiteOwnerLabel] != owned.runID {
		return fmt.Errorf("ClusterRoleBinding %q is not owned by run %q", owned.bindingName, owned.runID)
	}
	return client.IgnoreNotFound(cli.Delete(ctx, binding))
}

func (owned *coreSuiteRBAC) deleteRole(ctx context.Context, cli client.Client) error {
	role := &rbacv1.ClusterRole{}
	if err := cli.Get(ctx, client.ObjectKey{Name: owned.roleName}, role); err != nil {
		return client.IgnoreNotFound(err)
	}
	if role.Labels[coreSuiteOwnerLabel] != owned.runID {
		return fmt.Errorf("ClusterRole %q is not owned by run %q", owned.roleName, owned.runID)
	}
	if err := cli.Delete(ctx, role); err != nil && !apierrors.IsNotFound(err) {
		return err
	}
	return nil
}
