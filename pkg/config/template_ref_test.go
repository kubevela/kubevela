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
	"os"
	"testing"

	"github.com/stretchr/testify/require"
	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	"sigs.k8s.io/yaml"
)

// The Config CRD carries the rule as generated from the marker on ConfigSpec; the chart is what
// users apply, so a change to the marker that is not regenerated, or a hand edit of the chart,
// shows here.
func TestTemplateRefRuleIsInTheConfigCRD(t *testing.T) {
	data, err := os.ReadFile("../../charts/vela-core/crds/config.oam.dev_configs.yaml")
	require.NoError(t, err)

	var crd apiextensionsv1.CustomResourceDefinition
	require.NoError(t, yaml.Unmarshal(data, &crd))
	require.Len(t, crd.Spec.Versions, 1)
	require.NotNil(t, crd.Spec.Versions[0].Schema)

	spec := crd.Spec.Versions[0].Schema.OpenAPIV3Schema.Properties["spec"]
	require.Len(t, spec.XValidations, 1)
	require.Equal(t,
		"has(self.templateRef) == has(oldSelf.templateRef) && (!has(self.templateRef) || self.templateRef == oldSelf.templateRef)",
		spec.XValidations[0].Rule)
	require.Equal(t, TemplateRefImmutableMessage, spec.XValidations[0].Message)
}
