/*
Copyright 2021 The KubeVela Authors.

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

package multicluster

import (
	"context"

	"github.com/pkg/errors"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/oam-dev/kubevela/apis/core.oam.dev/v1alpha1"
	"github.com/oam-dev/kubevela/apis/core.oam.dev/v1beta1"
	"github.com/oam-dev/kubevela/pkg/appfile"
	"github.com/oam-dev/kubevela/pkg/cue/model/value"
	"github.com/oam-dev/kubevela/pkg/multicluster"
	"github.com/oam-dev/kubevela/pkg/policy/envbinding"
	wfContext "github.com/oam-dev/kubevela/pkg/workflow/context"
	"github.com/oam-dev/kubevela/pkg/workflow/providers"
	oamProvider "github.com/oam-dev/kubevela/pkg/workflow/providers/oam"
	wfTypes "github.com/oam-dev/kubevela/pkg/workflow/types"
)

const (
	// ProviderName is provider name for install.
	ProviderName = "multicluster"
)

type provider struct {
	client.Client
	app         *v1beta1.Application
	af          *appfile.Appfile
	apply       oamProvider.ComponentApply
	healthCheck oamProvider.ComponentHealthCheck
	renderer    oamProvider.WorkloadRenderer
}

func (p *provider) ReadPlacementDecisions(ctx wfContext.Context, v *value.Value, act wfTypes.Action) error {
	policy, err := v.GetString("inputs", "policyName")
	if err != nil {
		return err
	}
	env, err := v.GetString("inputs", "envName")
	if err != nil {
		return err
	}
	decisions, exists, err := envbinding.ReadPlacementDecisions(p.app, policy, env)
	if err != nil {
		return err
	}
	if exists {
		return v.FillObject(map[string]interface{}{"decisions": decisions}, "outputs")
	}
	return v.FillObject(map[string]interface{}{}, "outputs")
}

func (p *provider) MakePlacementDecisions(ctx wfContext.Context, v *value.Value, act wfTypes.Action) error {
	policy, err := v.GetString("inputs", "policyName")
	if err != nil {
		return err
	}
	env, err := v.GetString("inputs", "envName")
	if err != nil {
		return err
	}
	val, err := v.LookupValue("inputs", "placement")
	if err != nil {
		return err
	}

	// TODO detect env change
	placement := &v1alpha1.EnvPlacement{}
	if err = val.UnmarshalTo(placement); err != nil {
		return errors.Wrapf(err, "failed to parse placement while making placement decision")
	}

	var namespace, clusterName string
	// check if namespace selector is valid
	if placement.NamespaceSelector != nil {
		if len(placement.NamespaceSelector.Labels) != 0 {
			return errors.Errorf("invalid env %s: namespace selector in cluster-gateway does not support label selector for now", env)
		}
		namespace = placement.NamespaceSelector.Name
	}
	// check if cluster selector is valid
	if placement.ClusterSelector != nil {
		if len(placement.ClusterSelector.Labels) != 0 {
			return errors.Errorf("invalid env %s: cluster selector does not support label selector for now", env)
		}
		clusterName = placement.ClusterSelector.Name
	}
	// set fallback cluster
	if clusterName == "" {
		clusterName = multicluster.ClusterLocalName
	}
	// check if target cluster exists
	if clusterName != multicluster.ClusterLocalName {
		if _, err := multicluster.GetVirtualCluster(context.Background(), p.Client, clusterName); err != nil {
			return errors.Wrapf(err, "failed to get cluster %s for env %s", clusterName, env)
		}
	}
	// write result back
	decisions := []v1alpha1.PlacementDecision{{
		Cluster:   clusterName,
		Namespace: namespace,
	}}
	if err = envbinding.WritePlacementDecisions(p.app, policy, env, decisions); err != nil {
		return err
	}
	return v.FillObject(map[string]interface{}{"decisions": decisions}, "outputs")
}

func (p *provider) PatchApplication(ctx wfContext.Context, v *value.Value, act wfTypes.Action) error {
	env, err := v.GetString("inputs", "envName")
	if err != nil {
		return err
	}
	patch := v1alpha1.EnvPatch{}
	selector := &v1alpha1.EnvSelector{}

	obj, err := v.LookupValue("inputs", "patch")
	if err == nil {
		if err = obj.UnmarshalTo(&patch); err != nil {
			return errors.Wrapf(err, "failed to unmarshal patch for env %s", env)
		}
	}
	obj, err = v.LookupValue("inputs", "selector")
	if err == nil {
		if err = obj.UnmarshalTo(selector); err != nil {
			return errors.Wrapf(err, "failed to unmarshal selector for env %s", env)
		}
	} else {
		selector = nil
	}

	newApp, err := envbinding.PatchApplication(p.app, &patch, selector)
	if err != nil {
		return errors.Wrapf(err, "failed to patch app for env %s", env)
	}
	return v.FillObject(newApp, "outputs")
}

func (p *provider) ListClusters(ctx wfContext.Context, v *value.Value, act wfTypes.Action) error {
	secrets, err := multicluster.ListExistingClusterSecrets(context.Background(), p.Client)
	if err != nil {
		return err
	}
	var clusters []string
	for _, secret := range secrets {
		clusters = append(clusters, secret.Name)
	}
	return v.FillObject(clusters, "outputs", "clusters")
}

func (p *provider) Deploy(ctx wfContext.Context, v *value.Value, act wfTypes.Action) error {
	policyNames, err := v.GetStringSlice("policies")
	if err != nil {
		return err
	}
	parallelism, err := v.GetInt64("parallelism")
	if err != nil {
		return err
	}
	if parallelism <= 0 {
		return errors.Errorf("parallelism cannot be smaller than 1")
	}
	ignoreTerraformComponent, err := v.GetBool("ignoreTerraformComponent")
	if err != nil {
		return err
	}
	executor := NewDeployWorkflowStepExecutor(p.Client, p.af, p.apply, p.healthCheck, p.renderer, ignoreTerraformComponent)
	healthy, reason, err := executor.Deploy(context.Background(), policyNames, int(parallelism))
	if err != nil {
		return err
	}
	if !healthy {
		act.Wait(reason)
	}
	return nil
}

// Install register handlers to provider discover.
func Install(p providers.Providers, c client.Client, app *v1beta1.Application, af *appfile.Appfile, apply oamProvider.ComponentApply, healthCheck oamProvider.ComponentHealthCheck, renderer oamProvider.WorkloadRenderer) {
	prd := &provider{Client: c, app: app, af: af, apply: apply, healthCheck: healthCheck, renderer: renderer}
	p.Register(ProviderName, map[string]providers.Handler{
		"read-placement-decisions": prd.ReadPlacementDecisions,
		"make-placement-decisions": prd.MakePlacementDecisions,
		"patch-application":        prd.PatchApplication,
		"list-clusters":            prd.ListClusters,
		"deploy":                   prd.Deploy,
	})
}
