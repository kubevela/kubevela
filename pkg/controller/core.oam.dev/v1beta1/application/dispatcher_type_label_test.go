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

package application

import (
	"testing"

	"github.com/stretchr/testify/require"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"

	"github.com/oam-dev/kubevela/apis/core.oam.dev/v1beta1"
	"github.com/oam-dev/kubevela/pkg/appfile"
	"github.com/oam-dev/kubevela/pkg/oam"
)

func outputLabelled(typeLabel string) *unstructured.Unstructured {
	u := &unstructured.Unstructured{}
	u.SetLabels(map[string]string{oam.TraitTypeLabel: typeLabel})
	return u
}

// A module trait referenced as "note" labels its outputs with the installed
// name. The revision stores the definition under that name too, so the stage
// lookup finds it without going to the cluster (a nil client would panic).
func TestGetTraitDispatchStageFindsModuleTraitByInstalledName(t *testing.T) {
	appRev := &v1beta1.ApplicationRevision{}
	appRev.Spec.TraitDefinitions = map[string]*v1beta1.TraitDefinition{
		"widget-kit-v1-note": {
			ObjectMeta: metav1.ObjectMeta{Name: "widget-kit-v1-note"},
			Spec:       v1beta1.TraitDefinitionSpec{Stage: v1beta1.PostDispatch},
		},
	}
	stage, err := getTraitDispatchStage(nil, "widget-kit-v1-note", appRev, nil)
	require.NoError(t, err)
	require.Equal(t, PostDispatch, stage)
}

func TestByTraitTypeMatchesOnTypeLabel(t *testing.T) {
	ready := []*unstructured.Unstructured{outputLabelled("widget-kit-v1-note"), outputLabelled("scaler")}

	moduleTrait := appfile.Trait{Name: "note", TypeLabel: "widget-kit-v1-note"}
	legacyTrait := appfile.Trait{Name: "scaler"}

	filter := ByTraitType(ready, nil)
	require.True(t, filter(moduleTrait), "a module trait is matched by its installed name, not the name it was written as")
	require.True(t, filter(legacyTrait), "a trait without a TypeLabel is matched by its name")

	filter = ByTraitType(ready, []*unstructured.Unstructured{outputLabelled("widget-kit-v1-note")})
	require.False(t, filter(moduleTrait), "a trait in the stage being checked is excluded")
	require.True(t, filter(legacyTrait))
}
