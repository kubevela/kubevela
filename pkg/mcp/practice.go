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

package mcp

import (
	"context"
	"fmt"

	appsv1 "k8s.io/api/apps/v1"
	authorizationv1 "k8s.io/api/authorization/v1"
	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/oam-dev/kubevela/apis/core.oam.dev/v1beta1"
	"github.com/oam-dev/kubevela/pkg/oam"
)

// Delivery styles reported for a namespace.
const (
	StyleKubeVela   = "kubevela"
	StyleKubernetes = "kubernetes"
	StyleMixed      = "mixed"
	StyleEmpty      = "empty"

	// deliveryDefaultLabel is the namespace label a platform team sets to
	// declare how new work there is delivered: kubevela or kubernetes.
	deliveryDefaultLabel = "app.oam.dev/delivery-default"
)

// AccessReviewFunc reports whether the caller may perform verb on a
// resource in a namespace. Options.AccessReview lets tests supply one.
type AccessReviewFunc func(ctx context.Context, cli client.Client, namespace, verb string, gr schema.GroupResource) (bool, error)

// DeliveryPractice describes how a namespace delivers workloads today, so an
// agent follows that practice instead of steering toward one approach.
type DeliveryPractice struct {
	Namespace string `json:"namespace"`
	// Applications counts KubeVela Applications in the namespace.
	Applications int `json:"applications" jsonschema:"KubeVela Applications in the namespace."`
	// Workloads counts Deployments, StatefulSets, DaemonSets and CronJobs.
	Workloads int `json:"workloads" jsonschema:"Deployments, StatefulSets, DaemonSets and CronJobs in the namespace."`
	// WorkloadsManagedByApplications counts the workloads KubeVela labels as
	// rendered by an Application.
	WorkloadsManagedByApplications int `json:"workloadsManagedByApplications" jsonschema:"Workloads that an Application rendered and manages."`
	// Style summarises the counts: kubevela, kubernetes, mixed or empty.
	Style string `json:"style" jsonschema:"kubevela when every workload comes from an Application, kubernetes when none does, mixed when both exist, empty when there are no workloads or Applications. Follow it for new work."`
	// DeclaredDefault is the platform's declared default for new work.
	DeclaredDefault string `json:"declaredDefault,omitempty" jsonschema:"The platform team's declared default for new work in this namespace, kubevela or kubernetes. Use it instead of asking on a blank start."`
	// CanCreatePlainWorkloads is whether the caller may create Deployments.
	// Nil when the access review could not be made.
	CanCreatePlainWorkloads *bool `json:"canCreatePlainWorkloads,omitempty" jsonschema:"Whether the caller may create plain Deployments here. When false, do not offer plain Kubernetes manifests."`
	// Truncated is set when a list hit the count bound, so the counts are
	// lower bounds.
	Truncated bool `json:"truncated,omitempty" jsonschema:"True when a count reached its bound and is a lower bound."`
}

// deliveryPractice gathers the practice report for one namespace. The
// namespace allowlist applies like every other namespaced read.
func (d *deps) deliveryPractice(ctx context.Context, namespace string) (*DeliveryPractice, []string, error) {
	if err := d.namespaceAllowed(namespace); err != nil {
		return nil, nil, err
	}
	cli := d.f.Client()
	if cli == nil {
		return nil, nil, errNoConfig
	}
	p := &DeliveryPractice{Namespace: namespace}
	var warnings []string
	opts := []client.ListOption{client.InNamespace(namespace), client.Limit(int64(maxListItems))}

	var apps v1beta1.ApplicationList
	if err := cli.List(ctx, &apps, opts...); err != nil {
		return nil, nil, fmt.Errorf("list applications in %s: %w", namespace, err)
	}
	p.Applications = len(apps.Items)
	p.Truncated = apps.Continue != ""

	workloads := []struct {
		kind string
		list client.ObjectList
	}{
		{"Deployment", &appsv1.DeploymentList{}},
		{"StatefulSet", &appsv1.StatefulSetList{}},
		{"DaemonSet", &appsv1.DaemonSetList{}},
		{"CronJob", &batchv1.CronJobList{}},
	}
	for _, w := range workloads {
		if err := cli.List(ctx, w.list, opts...); err != nil {
			warnings = append(warnings, fmt.Sprintf("%s count unavailable: %s", w.kind, redactLine(err.Error())))
			continue
		}
		total, managed, more := countManaged(w.list)
		p.Workloads += total
		p.WorkloadsManagedByApplications += managed
		p.Truncated = p.Truncated || more
	}
	p.Style = deliveryStyle(p)

	var ns corev1.Namespace
	switch err := cli.Get(ctx, client.ObjectKey{Name: namespace}, &ns); {
	case err == nil:
		if v := ns.Labels[deliveryDefaultLabel]; v == StyleKubeVela || v == StyleKubernetes {
			p.DeclaredDefault = v
		}
	default:
		warnings = append(warnings, "declared default unavailable: "+redactLine(err.Error()))
	}

	allowed, err := d.opts.AccessReview(ctx, cli, namespace, "create", schema.GroupResource{Group: "apps", Resource: "deployments"})
	if err != nil {
		warnings = append(warnings, "access review unavailable: "+redactLine(err.Error()))
	} else {
		p.CanCreatePlainWorkloads = &allowed
	}
	return p, warnings, nil
}

// countManaged counts items and those carrying the label KubeVela stamps on
// every object an Application renders.
func countManaged(list client.ObjectList) (total, managed int, more bool) {
	visit := func(m metav1.Object) {
		total++
		if m.GetLabels()[oam.LabelAppName] != "" {
			managed++
		}
	}
	switch l := list.(type) {
	case *appsv1.DeploymentList:
		for i := range l.Items {
			visit(&l.Items[i])
		}
		more = l.Continue != ""
	case *appsv1.StatefulSetList:
		for i := range l.Items {
			visit(&l.Items[i])
		}
		more = l.Continue != ""
	case *appsv1.DaemonSetList:
		for i := range l.Items {
			visit(&l.Items[i])
		}
		more = l.Continue != ""
	case *batchv1.CronJobList:
		for i := range l.Items {
			visit(&l.Items[i])
		}
		more = l.Continue != ""
	}
	return total, managed, more
}

// deliveryStyle reduces the counts to one word. Applications with no
// workloads yet still count as KubeVela practice.
func deliveryStyle(p *DeliveryPractice) string {
	unmanaged := p.Workloads - p.WorkloadsManagedByApplications
	switch {
	case p.Applications == 0 && p.Workloads == 0:
		return StyleEmpty
	case unmanaged == 0:
		return StyleKubeVela
	case p.Applications == 0 && p.WorkloadsManagedByApplications == 0:
		return StyleKubernetes
	default:
		return StyleMixed
	}
}

// selfAccessReview is the default AccessReviewFunc: a SelfSubjectAccessReview
// as the caller, the same check kubectl auth can-i makes.
func selfAccessReview(ctx context.Context, cli client.Client, namespace, verb string, gr schema.GroupResource) (bool, error) {
	review := &authorizationv1.SelfSubjectAccessReview{
		Spec: authorizationv1.SelfSubjectAccessReviewSpec{
			ResourceAttributes: &authorizationv1.ResourceAttributes{
				Namespace: namespace,
				Verb:      verb,
				Group:     gr.Group,
				Resource:  gr.Resource,
			},
		},
	}
	if err := cli.Create(ctx, review); err != nil {
		return false, err
	}
	return review.Status.Allowed, nil
}
