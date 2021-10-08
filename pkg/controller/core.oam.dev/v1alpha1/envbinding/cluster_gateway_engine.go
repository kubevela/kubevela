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

// TODO only support cluster name now, should support selector and namespace later
func (engine *ClusterGatewayEngine) prepare(ctx context.Context, configs []v1alpha1.EnvConfig) error {
	engine.clusterDecisions = make(map[string]v1alpha1.ClusterDecision)
	clusterNameToConfig := make(map[string]string)
	for _, config := range configs {
		if config.Placement.NamespaceSelector != nil {
			return errors.Errorf("invalid env %s: namespace selector in cluster-gateway is not supported now", config.Name)
		}
		if config.Placement.ClusterSelector == nil {
			return errors.Errorf("invalid env %s: cluster selector must be set for now", config.Name)
		}
		if len(config.Placement.ClusterSelector.Labels) != 0 {
			return errors.Errorf("invalid env %s: cluster selector does not support label selector for now", config.Name)
		}
		if len(config.Placement.ClusterSelector.Name) == 0 {
			return errors.Errorf("invalid env %s: cluster selector must set cluster name for now", config.Name)
		}
		clusterName := config.Placement.ClusterSelector.Name
		if dupConfigName, ok := clusterNameToConfig[clusterName]; ok {
			return errors.Errorf("invalid env %s: cluster name %s is conflict with env %s", config.Name, clusterName, dupConfigName)
		}
		clusterNameToConfig[clusterName] = config.Name
		if clusterName != multicluster.ClusterLocalName {
			if err := engine.Get(ctx, types.NamespacedName{Namespace: multicluster.ClusterGatewaySecretNamespace, Name: clusterName}, &v1.Secret{}); err != nil {
				return errors.Wrapf(err, "failed to get cluster %s for env %s", clusterName, config.Name)
			}
		}
		engine.clusterDecisions[config.Name] = v1alpha1.ClusterDecision{Env: config.Name, Cluster: clusterName}
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
		raw, err := runtime.DefaultUnstructuredConverter.ToUnstructured(app.PatchedApp)
		if err != nil {
			return nil, errors.Wrapf(err, "failed to convert app [Env: %s](%s/%s) into unstructured", app.envConfig.Name, app.PatchedApp.Namespace, app.PatchedApp.Name)
		}
		patchedApp := &unstructured.Unstructured{Object: raw}
		multicluster.SetClusterName(patchedApp, clusterName)
		app.ScheduledManifests[patchedApp.GetName()] = patchedApp
	}
	var decisions []v1alpha1.ClusterDecision
	for _, decision := range engine.clusterDecisions {
		decisions = append(decisions, decision)
	}
	return decisions, nil
}
