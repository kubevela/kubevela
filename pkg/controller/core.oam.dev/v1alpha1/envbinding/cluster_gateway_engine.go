/*
 Copyright 2021. The KubeVela Authors.

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

package envbinding

import (
	"context"

	"github.com/pkg/errors"
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/oam-dev/kubevela/apis/core.oam.dev/v1alpha1"
	"github.com/oam-dev/kubevela/apis/core.oam.dev/v1beta1"
	"github.com/oam-dev/kubevela/pkg/appfile"
	"github.com/oam-dev/kubevela/pkg/multicluster"
)

const (
	// OverrideNamespaceLabelKey identifies the override namespace for patched Application
	OverrideNamespaceLabelKey = "envbinding.oam.dev/override-namespace"
)

// ClusterGatewayEngine construct the multicluster engine of using cluster-gateway
type ClusterGatewayEngine struct {
	client.Client
	envBindingName   string
	clusterDecisions map[string]v1alpha1.ClusterDecision
}

// NewClusterGatewayEngine create multicluster engine to use cluster-gateway
func NewClusterGatewayEngine(cli client.Client, envBindingName string) ClusterManagerEngine {
	return &ClusterGatewayEngine{
		Client:         cli,
		envBindingName: envBindingName,
	}
}

// TODO only support single cluster name and namespace name now, should support label selector
func (engine *ClusterGatewayEngine) prepare(ctx context.Context, configs []v1alpha1.EnvConfig) error {
	engine.clusterDecisions = make(map[string]v1alpha1.ClusterDecision)
	locationToConfig := make(map[string]string)
	for _, config := range configs {
		var namespace, clusterName string
		// check if namespace selector is valid
		if config.Placement.NamespaceSelector != nil {
			if len(config.Placement.NamespaceSelector.Labels) != 0 {
				return errors.Errorf("invalid env %s: namespace selector in cluster-gateway does not support label selector for now", config.Name)
			}
			namespace = config.Placement.NamespaceSelector.Name
		}
		// check if cluster selector is valid
		if config.Placement.ClusterSelector != nil {
			if len(config.Placement.ClusterSelector.Labels) != 0 {
				return errors.Errorf("invalid env %s: cluster selector does not support label selector for now", config.Name)
			}
			clusterName = config.Placement.ClusterSelector.Name
		}
		// set fallback cluster
		if clusterName == "" {
			clusterName = multicluster.ClusterLocalName
		}
		// check if current environment uses the same cluster and namespace as resource destination with other environment, if yes, a conflict occurs
		location := clusterName + "/" + namespace
		if dupConfigName, ok := locationToConfig[location]; ok {
			return errors.Errorf("invalid env %s: location %s conflict with env %s", config.Name, location, dupConfigName)
		}
		locationToConfig[clusterName] = config.Name
		// check if target cluster exists
		if clusterName != multicluster.ClusterLocalName {
			if err := engine.Get(ctx, types.NamespacedName{Namespace: multicluster.ClusterGatewaySecretNamespace, Name: clusterName}, &v1.Secret{}); err != nil {
				return errors.Wrapf(err, "failed to get cluster %s for env %s", clusterName, config.Name)
			}
		}
		engine.clusterDecisions[config.Name] = v1alpha1.ClusterDecision{Env: config.Name, Cluster: clusterName, Namespace: namespace}
	}
	return nil
}

func (engine *ClusterGatewayEngine) initEnvBindApps(ctx context.Context, envBinding *v1alpha1.EnvBinding, baseApp *v1beta1.Application, appParser *appfile.Parser) ([]*EnvBindApp, error) {
	return CreateEnvBindApps(envBinding, baseApp)
}

func (engine *ClusterGatewayEngine) schedule(ctx context.Context, apps []*EnvBindApp) ([]v1alpha1.ClusterDecision, error) {
	for _, app := range apps {
		app.ScheduledManifests = make(map[string]*unstructured.Unstructured)
		clusterName := engine.clusterDecisions[app.envConfig.Name].Cluster
		namespace := engine.clusterDecisions[app.envConfig.Name].Namespace
		raw, err := runtime.DefaultUnstructuredConverter.ToUnstructured(app.PatchedApp)
		if err != nil {
			return nil, errors.Wrapf(err, "failed to convert app [Env: %s](%s/%s) into unstructured", app.envConfig.Name, app.PatchedApp.Namespace, app.PatchedApp.Name)
		}
		patchedApp := &unstructured.Unstructured{Object: raw}
		multicluster.SetClusterName(patchedApp, clusterName)
		SetOverrideNamespace(patchedApp, namespace)
		app.ScheduledManifests[patchedApp.GetName()] = patchedApp
	}
	var decisions []v1alpha1.ClusterDecision
	for _, decision := range engine.clusterDecisions {
		decisions = append(decisions, decision)
	}
	return decisions, nil
}

// SetOverrideNamespace set the override namespace for object in its label
func SetOverrideNamespace(obj *unstructured.Unstructured, overrideNamespace string) {
	if overrideNamespace != "" {
		labels := obj.GetLabels()
		if labels == nil {
			labels = map[string]string{}
		}
		labels[OverrideNamespaceLabelKey] = overrideNamespace
		obj.SetLabels(labels)
	}
}
