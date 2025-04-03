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

package terraform

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	apicommon "github.com/oam-dev/kubevela/apis/core.oam.dev/common"
	"github.com/oam-dev/kubevela/apis/core.oam.dev/v1beta1"
	"github.com/oam-dev/kubevela/apis/types"
	"github.com/oam-dev/kubevela/pkg/appfile"
	oamprovidertypes "github.com/oam-dev/kubevela/pkg/workflow/providers/types"
)

func fakeWorkloadRenderer(_ context.Context, comp apicommon.ApplicationComponent) (*appfile.Component, error) {
	if strings.HasPrefix(comp.Name, "error") {
		return nil, errors.New(comp.Name)
	}
	if strings.HasPrefix(comp.Name, "terraform") {
		return &appfile.Component{CapabilityCategory: types.TerraformCategory}, nil
	}
	return &appfile.Component{CapabilityCategory: types.CUECategory}, nil
}

func TestLoadTerraformComponents(t *testing.T) {
	r := require.New(t)
	ctx := context.Background()
	scheme := runtime.NewScheme()
	r.NoError(v1beta1.AddToScheme(scheme))
	cli := fake.NewClientBuilder().WithScheme(scheme).Build()
	terraformCD := &v1beta1.ComponentDefinition{
		ObjectMeta: metav1.ObjectMeta{Name: "terraform"},
		Spec: v1beta1.ComponentDefinitionSpec{
			Schematic: &apicommon.Schematic{
				Terraform: &apicommon.Terraform{},
			},
		},
	}
	require.NoError(t, cli.Create(ctx, terraformCD))
	cueCD := &v1beta1.ComponentDefinition{
		ObjectMeta: metav1.ObjectMeta{Name: "cue"},
		Spec: v1beta1.ComponentDefinitionSpec{
			Schematic: &apicommon.Schematic{
				CUE: &apicommon.CUE{},
			},
		},
	}
	require.NoError(t, cli.Create(ctx, cueCD))

	testCases := []struct {
		Inputs   []apicommon.ApplicationComponent
		HasError bool
		Outputs  []apicommon.ApplicationComponent
	}{
		{
			Inputs:   []apicommon.ApplicationComponent{{Name: "error"}},
			HasError: true,
		},
		{
			Inputs: []apicommon.ApplicationComponent{
				{Name: "terraform-1", Type: "terraform"},
				{Name: "cue", Type: "cue"},
				{Name: "terraform-2", Type: "terraform"},
			},
			Outputs: []apicommon.ApplicationComponent{
				{Name: "terraform-1", Type: "terraform"},
				{Name: "terraform-2", Type: "terraform"},
			},
		},
		{
			Inputs:  []apicommon.ApplicationComponent{{Name: "cue", Type: "cue"}},
			Outputs: []apicommon.ApplicationComponent{},
		},
	}
	for _, testCase := range testCases {
		app := &v1beta1.Application{}
		app.Spec.Components = testCase.Inputs
		res, err := LoadTerraformComponents(ctx, &oamprovidertypes.OAMParams[any]{
			RuntimeParams: oamprovidertypes.RuntimeParams{
				WorkloadRender: fakeWorkloadRenderer,
				App:            app,
			},
		})
		if testCase.HasError {
			r.Error(err)
			continue
		}
		r.NoError(err)
		r.Equal(testCase.Outputs, res.Outputs.Components)
	}
}

func TestGetConnectionStatus(t *testing.T) {
	ctx := context.Background()
	r := require.New(t)
	testCases := []struct {
		ComponentName string
		Services      []apicommon.ApplicationComponentStatus
		Healthy       bool
		Error         string
	}{{
		ComponentName: "",
		Error:         "componentName is required",
	}, {
		ComponentName: "comp",
		Services: []apicommon.ApplicationComponentStatus{{
			Name:    "not-comp",
			Healthy: true,
		}},
		Healthy: false,
	}, {
		ComponentName: "comp",
		Services: []apicommon.ApplicationComponentStatus{{
			Name:    "not-comp",
			Healthy: true,
		}, {
			Name:    "comp",
			Healthy: true,
		}},
		Healthy: true,
	}, {
		ComponentName: "comp",
		Services: []apicommon.ApplicationComponentStatus{{
			Name:    "not-comp",
			Healthy: true,
		}, {
			Name:    "comp",
			Healthy: false,
		}},
		Healthy: false,
	}}
	for _, testCase := range testCases {
		app := &v1beta1.Application{}
		app.Status.Services = testCase.Services
		res, err := GetConnectionStatus(ctx, &ConnectionParams{
			Params: Inputs[ComponentNameVars]{
				Inputs: ComponentNameVars{
					ComponentName: testCase.ComponentName,
				},
			},
			RuntimeParams: oamprovidertypes.RuntimeParams{
				App: app,
			},
		})
		if testCase.Error != "" {
			r.Error(err)
			r.Contains(err.Error(), testCase.Error)
			continue
		}
		r.NoError(err)
		r.Equal(testCase.Healthy, res.Outputs.Healthy)
	}
}
