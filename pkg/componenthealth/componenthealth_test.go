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

package componenthealth

import (
	"context"
	"testing"

	wfprocess "github.com/kubevela/workflow/pkg/cue/process"
	"github.com/stretchr/testify/require"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	networkingv1 "k8s.io/api/networking/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	"github.com/oam-dev/kubevela/apis/core.oam.dev/common"
	"github.com/oam-dev/kubevela/pkg/cue/definition"
	"github.com/oam-dev/kubevela/pkg/cue/definition/health"
	"github.com/oam-dev/kubevela/pkg/cue/process"
	"github.com/oam-dev/kubevela/pkg/oam/util"
)

// A webservice-like workload with an auxiliary Service, and a gateway-like trait with an
// Ingress output. Health, custom status and details all read live state.
const workloadTemplate = `
output: {
	apiVersion: "apps/v1"
	kind:       "Deployment"
	metadata: {name: "backend", namespace: "default"}
	spec: replicas: parameter.replicas
}
outputs: service: {
	apiVersion: "v1"
	kind:       "Service"
	metadata: {name: "backend", namespace: "default"}
}
`

const traitTemplate = `
outputs: ingress: {
	apiVersion: "networking.k8s.io/v1"
	kind:       "Ingress"
	metadata: {name: "backend", namespace: "default"}
	spec: rules: [{host: parameter.domain}]
}
`

var workloadStatus = health.StatusRequest{
	Health: `isHealth: context.output.status.readyReplicas == parameter.replicas`,
	Custom: `message: "Ready: \(context.output.status.readyReplicas)/\(parameter.replicas) (\(context.name) in \(context.appName)@\(context.appRevision))"`,
	Details: `service: context.outputs.service.metadata.name
component: context.name`,
	Parameter: map[string]interface{}{"replicas": 2},
}

var traitStatus = health.StatusRequest{
	Health:    `isHealth: len(context.outputs.ingress.status.loadBalancer.ingress) > 0`,
	Custom:    `message: "Host: \(parameter.domain)"`,
	Parameter: map[string]interface{}{"domain": "shop.example.com"},
}

func liveObjects(readyReplicas int64, ingressReady bool) []client.Object {
	deploy := &unstructured.Unstructured{Object: map[string]interface{}{
		"apiVersion": "apps/v1", "kind": "Deployment",
		"metadata": map[string]interface{}{"name": "backend", "namespace": "default"},
		"spec":     map[string]interface{}{"replicas": int64(2)},
		"status":   map[string]interface{}{"readyReplicas": readyReplicas},
	}}
	svc := &unstructured.Unstructured{Object: map[string]interface{}{
		"apiVersion": "v1", "kind": "Service",
		"metadata": map[string]interface{}{"name": "backend", "namespace": "default"},
	}}
	lb := []interface{}{}
	if ingressReady {
		lb = append(lb, map[string]interface{}{"ip": "10.0.0.1"})
	}
	ingress := &unstructured.Unstructured{Object: map[string]interface{}{
		"apiVersion": "networking.k8s.io/v1", "kind": "Ingress",
		"metadata": map[string]interface{}{"name": "backend", "namespace": "default"},
		"status":   map[string]interface{}{"loadBalancer": map[string]interface{}{"ingress": lb}},
	}}
	return []client.Object{deploy, svc, ingress}
}

func newClient(t *testing.T, objs ...client.Object) client.Client {
	scheme := runtime.NewScheme()
	require.NoError(t, appsv1.AddToScheme(scheme))
	require.NoError(t, corev1.AddToScheme(scheme))
	require.NoError(t, networkingv1.AddToScheme(scheme))
	return fake.NewClientBuilder().WithScheme(scheme).WithObjects(objs...).Build()
}

// reference evaluates health the way the Application controller does today: from the
// render context, looking the rendered objects up in the cluster.
func reference(t *testing.T, cli client.Client) (renderCtx wfprocess.Context, workload, trait *health.StatusResult) {
	renderCtx = process.NewContext(process.ContextData{
		AppName: "shop", CompName: "backend", Namespace: "default", AppRevisionName: "shop-v3",
	})
	wd := definition.NewWorkloadAbstractEngine("webservice")
	require.NoError(t, wd.Complete(renderCtx, workloadTemplate, workloadStatus.Parameter))
	td := definition.NewTraitAbstractEngine("gateway")
	require.NoError(t, td.Complete(renderCtx, traitTemplate, traitStatus.Parameter))

	accessor := util.NewApplicationResourceNamespaceAccessor("default", "")
	eval := func(engine definition.AbstractEngine, req health.StatusRequest) *health.StatusResult {
		templateContext, err := engine.GetTemplateContext(renderCtx, cli, accessor)
		require.NoError(t, err)
		templateContext["parameter"] = req.Parameter
		// Best-effort, as collectWorkloadHealthStatus and collectTraitHealthStatus are: a CUE
		// error (e.g. a status field not populated yet) is logged, the partial result used.
		result, _ := engine.Status(templateContext, &req)
		require.NotNil(t, result)
		return result
	}
	return renderCtx, eval(wd, workloadStatus), eval(td, traitStatus)
}

// specFor is what the hub would put in Component.spec.health: context values from the
// render context, and each subject's CUE and parameters, with resource refs from the index.
func specFor(renderCtx wfprocess.Context) Spec {
	return Spec{
		Context: definition.GetBaseContextLabels(renderCtx),
		Workload: &Subject{
			Status:  workloadStatus,
			Output:  &ResourceRef{APIVersion: "apps/v1", Kind: "Deployment", Namespace: "default", Name: "backend"},
			Outputs: map[string]ResourceRef{"service": {APIVersion: "v1", Kind: "Service", Namespace: "default", Name: "backend"}},
		},
		Traits: []Trait{{Type: "gateway", Subject: Subject{
			Status:  traitStatus,
			Outputs: map[string]ResourceRef{"ingress": {APIVersion: "networking.k8s.io/v1", Kind: "Ingress", Namespace: "default", Name: "backend"}},
		}}},
	}
}

func TestEvaluateMatchesTheApplicationControllerPath(t *testing.T) {
	cases := map[string]struct {
		readyReplicas int64
		ingressReady  bool
	}{
		"all healthy":          {readyReplicas: 2, ingressReady: true},
		"workload unhealthy":   {readyReplicas: 1, ingressReady: true},
		"trait unhealthy":      {readyReplicas: 2, ingressReady: false},
		"everything unhealthy": {readyReplicas: 0, ingressReady: false},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			r := require.New(t)
			cli := newClient(t, liveObjects(tc.readyReplicas, tc.ingressReady)...)
			renderCtx, wantWorkload, wantTrait := reference(t, cli)

			got, err := Evaluate(context.Background(), cli, specFor(renderCtx))
			r.NoError(err)

			r.Equal(wantWorkload.Healthy, got.WorkloadHealthy, "workload health")
			wantMessage := wantWorkload.Message
			if wantMessage == "" {
				wantMessage = wantTrait.Message // collectHealthStatus: first trait message fills an empty one
			}
			r.Equal(wantMessage, got.Message, "workload message, else the first trait's")
			r.Equal(wantWorkload.Details, got.Details, "workload details")
			r.Len(got.Traits, 1)
			r.Equal(common.ApplicationTraitStatus{
				Type: "gateway", Healthy: wantTrait.Healthy, Message: wantTrait.Message, Details: wantTrait.Details,
			}, got.Traits[0])
			r.Equal(wantWorkload.Healthy && wantTrait.Healthy, got.Healthy, "component health is workload AND traits")
		})
	}
}

// The workload's resources may not exist yet on a fresh apply. Today that is an error from
// the lookup; the evaluator keeps that behaviour and says which object is missing.
func TestEvaluateReportsAMissingResource(t *testing.T) {
	cli := newClient(t)
	_, err := Evaluate(context.Background(), cli, specFor(process.NewContext(process.ContextData{
		AppName: "shop", CompName: "backend", Namespace: "default", AppRevisionName: "shop-v3",
	})))
	require.ErrorContains(t, err, "Deployment default/backend")
}

// A component whose workload a trait manages has no workload output of its own. That is what
// the agent evaluates for a trait-only Component, so the traits alone decide its health.
func TestEvaluateWithNoWorkload(t *testing.T) {
	cli := newClient(t, liveObjects(3, true)...)
	spec := specFor(process.NewContext(process.ContextData{
		AppName: "shop", CompName: "backend", Namespace: "default", AppRevisionName: "shop-v3",
	}))
	spec.Workload = nil

	status, err := Evaluate(context.Background(), cli, spec)
	require.NoError(t, err)
	require.True(t, status.Healthy, "the traits are healthy, so the component is")
	require.False(t, status.WorkloadHealthy, "there is no workload to be healthy")
	require.Len(t, status.Traits, 1)
	require.True(t, status.Traits[0].Healthy)

	cli = newClient(t, liveObjects(3, false)...)
	status, err = Evaluate(context.Background(), cli, spec)
	require.NoError(t, err)
	require.False(t, status.Healthy, "an unhealthy trait is enough on its own")
	require.False(t, status.Traits[0].Healthy)
	require.Equal(t, "Host: shop.example.com", status.Message, "the trait's own message stands; the default is for traits that give none")
}

func TestRollup(t *testing.T) {
	cases := map[string]struct {
		workloadHealthy bool
		skipWorkload    bool
		message         string
		traits          []common.ApplicationTraitStatus
		wantHealthy     bool
		wantMessage     string
	}{
		"healthy workload, healthy traits": {
			workloadHealthy: true, traits: []common.ApplicationTraitStatus{{Healthy: true}},
			wantHealthy: true,
		},
		"healthy workload, unhealthy trait": {
			workloadHealthy: true, traits: []common.ApplicationTraitStatus{{Healthy: false}},
			wantHealthy: false,
		},
		"pending traits do not count": {
			workloadHealthy: true, traits: []common.ApplicationTraitStatus{{Healthy: false, Pending: true}},
			wantHealthy: true,
		},
		"workload skipped, traits healthy": {
			skipWorkload: true, traits: []common.ApplicationTraitStatus{{Healthy: true}},
			wantHealthy: true,
		},
		"workload skipped, trait unhealthy, no message": {
			skipWorkload: true, traits: []common.ApplicationTraitStatus{{Healthy: false}},
			wantHealthy: false, wantMessage: "traits are not healthy",
		},
		"workload skipped, trait unhealthy, keeps message": {
			skipWorkload: true, message: "ingress pending", traits: []common.ApplicationTraitStatus{{Healthy: false}},
			wantHealthy: false, wantMessage: "ingress pending",
		},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			status := &common.ApplicationComponentStatus{Healthy: true, WorkloadHealthy: tc.workloadHealthy, Message: tc.message, Traits: tc.traits}
			Rollup(status, !tc.skipWorkload)
			require.Equal(t, tc.wantHealthy, status.Healthy)
			require.Equal(t, tc.wantMessage, status.Message)
		})
	}
}
