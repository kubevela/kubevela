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
package features

import (
	"context"
	"fmt"
	"sort"
	"strconv"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/util/retry"
	"k8s.io/component-base/featuregate"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/oam-dev/kubevela/pkg/utils/util"
)

const (
	// FeatureGatesConfigMapName is the ConfigMap, in the system namespace, where
	// the controller publishes KubeVela's feature gates when it starts: a key per
	// gate, "true" or "false". Other processes that run KubeVela code, such as
	// VelaUX, read it to behave as the controller does.
	FeatureGatesConfigMapName = "kubevela-feature-gates"
	// FeatureGatesVersionAnnotation is the KubeVela version that published them.
	FeatureGatesVersionAnnotation = "core.oam.dev/kubevela-version"
)

// KubeVelaFeatures are KubeVela's own gates, not the Kubernetes API server's
// that share the process's feature gate.
func KubeVelaFeatures() []featuregate.Feature {
	out := make([]featuregate.Feature, 0, len(defaultFeatureGates))
	for f := range defaultFeatureGates {
		out = append(out, f)
	}
	sort.Slice(out, func(i, j int) bool { return out[i] < out[j] })
	return out
}

// GateStates are whether each of the named features is on in gate.
func GateStates(gate featuregate.FeatureGate, names []featuregate.Feature) map[string]string {
	out := make(map[string]string, len(names))
	for _, name := range names {
		out[string(name)] = strconv.FormatBool(gate.Enabled(name))
	}
	return out
}

// Publish writes the named features' states in gate to
// FeatureGatesConfigMapName in namespace, replacing what an earlier start wrote.
// It creates the ConfigMap with util.OAMLabel and refuses to write one without it.
// A write that races another replica's is retried on a fresh read; every replica
// of one build and flags writes the same content.
func Publish(ctx context.Context, cli client.Client, namespace, version string, gate featuregate.FeatureGate, names []featuregate.Feature) error {
	racedAnotherWriter := func(err error) bool { return apierrors.IsConflict(err) || apierrors.IsAlreadyExists(err) }
	return retry.OnError(retry.DefaultRetry, racedAnotherWriter, func() error {
		return publish(ctx, cli, namespace, version, gate, names)
	})
}

func publish(ctx context.Context, cli client.Client, namespace, version string, gate featuregate.FeatureGate, names []featuregate.Feature) error {
	cm := &corev1.ConfigMap{}
	err := cli.Get(ctx, types.NamespacedName{Namespace: namespace, Name: FeatureGatesConfigMapName}, cm)
	if err != nil && !apierrors.IsNotFound(err) {
		return err
	}
	exists := err == nil
	if !exists {
		cm = &corev1.ConfigMap{ObjectMeta: metav1.ObjectMeta{Name: FeatureGatesConfigMapName, Namespace: namespace, Labels: map[string]string{}}}
		for k, v := range util.OAMLabel {
			cm.Labels[k] = v
		}
	}
	for k, v := range util.OAMLabel {
		if cm.Labels[k] != v {
			return fmt.Errorf("ConfigMap %s/%s is not KubeVela's: it lacks the label %s=%s", namespace, FeatureGatesConfigMapName, k, v)
		}
	}
	if cm.Annotations == nil {
		cm.Annotations = map[string]string{}
	}
	cm.Annotations[FeatureGatesVersionAnnotation] = version
	cm.Data = GateStates(gate, names)
	if exists {
		return cli.Update(ctx, cm)
	}
	return cli.Create(ctx, cm)
}
