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
	"strings"
	"testing"

	"github.com/pkg/errors"
	"github.com/stretchr/testify/require"

	"github.com/kubevela/workflow/pkg/cue/model/value"
	"github.com/kubevela/workflow/pkg/mock"

	apicommon "github.com/oam-dev/kubevela/apis/core.oam.dev/common"
	"github.com/oam-dev/kubevela/apis/core.oam.dev/v1beta1"
	"github.com/oam-dev/kubevela/apis/types"
	"github.com/oam-dev/kubevela/pkg/appfile"
)

func fakeWorkloadRenderer(_ context.Context, comp apicommon.ApplicationComponent) (*appfile.Workload, error) {
	if strings.HasPrefix(comp.Name, "error") {
		return nil, errors.New(comp.Name)
	}
	if strings.HasPrefix(comp.Name, "terraform") {
		return &appfile.Workload{CapabilityCategory: types.TerraformCategory}, nil
	}
	return &appfile.Workload{CapabilityCategory: types.CUECategory}, nil
}

func TestLoadTerraformComponents(t *testing.T) {
	r := require.New(t)
	testCases := []struct {
		Inputs   []apicommon.ApplicationComponent
		HasError bool
		Outputs  []apicommon.ApplicationComponent
	}{{
		Inputs:   []apicommon.ApplicationComponent{{Name: "error"}},
		HasError: true,
	}, {
		Inputs:  []apicommon.ApplicationComponent{{Name: "terraform-1"}, {Name: "cue"}, {Name: "terraform-2"}},
		Outputs: []apicommon.ApplicationComponent{{Name: "terraform-1"}, {Name: "terraform-2"}},
	}, {
		Inputs:  []apicommon.ApplicationComponent{{Name: "cue"}},
		Outputs: []apicommon.ApplicationComponent{},
	}}
	for _, testCase := range testCases {
		app := &v1beta1.Application{}
		app.Spec.Components = testCase.Inputs
		p := &provider{
			app:      app,
			renderer: fakeWorkloadRenderer,
		}
		act := &mock.Action{}
		v, err := value.NewValue("", nil, "")
		r.NoError(err)
		err = p.LoadTerraformComponents(nil, nil, v, act)
		if testCase.HasError {
			r.Error(err)
			continue
		}
		r.NoError(err)
		outputs, err := v.LookupValue("outputs", "components")
		r.NoError(err)
		var comps []apicommon.ApplicationComponent
		r.NoError(outputs.UnmarshalTo(&comps))
		r.Equal(testCase.Outputs, comps)
	}
}

func TestGetConnectionStatus(t *testing.T) {
	r := require.New(t)
	testCases := []struct {
		ComponentName string
		Services      []apicommon.ApplicationComponentStatus
		Healthy       bool
		Error         string
	}{{
		ComponentName: "",
		Error:         "failed to get component name",
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
		p := &provider{
			app:      app,
			renderer: fakeWorkloadRenderer,
		}
		act := &mock.Action{}
		v, err := value.NewValue("", nil, "")
		r.NoError(err)
		if testCase.ComponentName != "" {
			r.NoError(v.FillObject(map[string]string{"componentName": testCase.ComponentName}, "inputs"))
		}
		err = p.GetConnectionStatus(nil, nil, v, act)
		if testCase.Error != "" {
			r.Error(err)
			r.Contains(err.Error(), testCase.Error)
			continue
		}
		r.NoError(err)
		healthy, err := v.GetBool("outputs", "healthy")
		r.NoError(err)
		r.Equal(testCase.Healthy, healthy)
	}
}
