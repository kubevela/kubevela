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

package config

import (
	"testing"

	"github.com/stretchr/testify/require"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestGitOpsManager(t *testing.T) {
	cases := map[string]struct {
		labels, annotations map[string]string
		tool, marker        string
	}{
		"nothing":                       {},
		"Argo CD annotation tracking":   {annotations: map[string]string{argoCDTrackingIDAnnotation: "app:/Secret:ns/n"}, tool: "Argo CD", marker: "annotation argocd.argoproj.io/tracking-id"},
		"Flux kustomization":            {labels: map[string]string{fluxKustomizationLabel: "configs"}, tool: "Flux", marker: "label kustomize.toolkit.fluxcd.io/name"},
		"Flux helm release":             {labels: map[string]string{fluxHelmReleaseLabel: "configs"}, tool: "Flux", marker: "label helm.toolkit.fluxcd.io/name"},
		"Helm":                          {labels: map[string]string{managedByLabel: "Helm"}, tool: "Helm", marker: "label app.kubernetes.io/managed-by=Helm"},
		"Helm wins over instance":       {labels: map[string]string{instanceLabel: "rel", managedByLabel: "Helm"}, tool: "Helm", marker: "label app.kubernetes.io/managed-by=Helm"},
		"instance alone":                {labels: map[string]string{instanceLabel: "app"}, tool: "Argo CD (label tracking) or another tool", marker: "label app.kubernetes.io/instance"},
		"instance with another manager": {labels: map[string]string{instanceLabel: "app", managedByLabel: "kustomize"}, tool: "Argo CD (label tracking) or another tool", marker: "label app.kubernetes.io/instance"},
		"the annotation wins over Flux": {annotations: map[string]string{argoCDTrackingIDAnnotation: "x"}, labels: map[string]string{fluxKustomizationLabel: "y"}, tool: "Argo CD", marker: "annotation argocd.argoproj.io/tracking-id"},
		"Flux wins over instance":       {labels: map[string]string{fluxKustomizationLabel: "y", instanceLabel: "z"}, tool: "Flux", marker: "label kustomize.toolkit.fluxcd.io/name"},
		"an empty value is no marker":   {labels: map[string]string{fluxKustomizationLabel: "", instanceLabel: ""}},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			tool, marker := GitOpsManager(&metav1.ObjectMeta{Labels: tc.labels, Annotations: tc.annotations})
			require.Equal(t, tc.tool, tool)
			require.Equal(t, tc.marker, marker)
		})
	}
}

func TestGitOpsNote(t *testing.T) {
	require.Empty(t, gitOpsNote(&metav1.ObjectMeta{}, "Secret", "Config"))
	require.Equal(t,
		"the legacy Secret is managed by Flux (label kustomize.toolkit.fluxcd.io/name): keep this Config where it is kept instead, so the next sync does not reapply the legacy object",
		gitOpsNote(&metav1.ObjectMeta{Labels: map[string]string{fluxKustomizationLabel: "c"}}, "Secret", "Config"))
	require.Equal(t,
		"the legacy ConfigMap may be managed by Argo CD (label tracking) or another tool (label app.kubernetes.io/instance): if so, keep this ConfigTemplate where it is kept instead, so the next sync does not reapply the legacy object",
		gitOpsNote(&metav1.ObjectMeta{Labels: map[string]string{instanceLabel: "c"}}, "ConfigMap", "ConfigTemplate"))
}
