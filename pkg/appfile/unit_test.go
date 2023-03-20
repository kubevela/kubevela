/*
Copyright 2023 The KubeVela Authors.

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

package appfile

import (
	"context"
	"fmt"
	"testing"

	"github.com/stretchr/testify/require"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	"github.com/oam-dev/kubevela/apis/core.oam.dev/common"
	"github.com/oam-dev/kubevela/apis/core.oam.dev/v1beta1"
)

func TestIsNotFoundInAppFile(t *testing.T) {
	require.True(t, IsNotFoundInAppFile(fmt.Errorf("ComponentDefinition XXX not found in appfile")))
}

func TestIsNotFoundInAppRevision(t *testing.T) {
	require.True(t, IsNotFoundInAppRevision(fmt.Errorf("ComponentDefinition XXX not found in app revision")))
}

func TestParseWorkloadFromRevisionAndClient(t *testing.T) {
	ctx := context.Background()
	cli := fake.NewClientBuilder().WithScheme(scheme).Build()
	p := &Parser{
		client:     cli,
		tmplLoader: LoadTemplate,
	}
	comp := common.ApplicationComponent{
		Name:       "test",
		Type:       "test",
		Properties: &runtime.RawExtension{Raw: []byte(`{}`)},
		Traits: []common.ApplicationTrait{{
			Type:       "tr",
			Properties: &runtime.RawExtension{Raw: []byte(`{}`)},
		}, {
			Type:       "internal",
			Properties: &runtime.RawExtension{Raw: []byte(`{}`)},
		}},
	}
	appRev := &v1beta1.ApplicationRevision{}
	cd := &v1beta1.ComponentDefinition{ObjectMeta: metav1.ObjectMeta{Name: "test"}}
	td := &v1beta1.TraitDefinition{ObjectMeta: metav1.ObjectMeta{Name: "tr"}}
	require.NoError(t, cli.Create(ctx, cd))
	require.NoError(t, cli.Create(ctx, td))
	appRev.Spec.TraitDefinitions = map[string]*v1beta1.TraitDefinition{"internal": {}}
	_, err := p.ParseWorkloadFromRevisionAndClient(ctx, comp, appRev)
	require.NoError(t, err)

	_comp1 := comp.DeepCopy()
	_comp1.Type = "bad"
	_, err = p.ParseWorkloadFromRevisionAndClient(ctx, *_comp1, appRev)
	require.Error(t, err)

	_comp2 := comp.DeepCopy()
	_comp2.Traits[0].Type = "bad"
	_, err = p.ParseWorkloadFromRevisionAndClient(ctx, *_comp2, appRev)
	require.Error(t, err)

	_comp3 := comp.DeepCopy()
	_comp3.Traits[0].Properties = &runtime.RawExtension{Raw: []byte(`bad`)}
	_, err = p.ParseWorkloadFromRevisionAndClient(ctx, *_comp3, appRev)
	require.Error(t, err)
}
