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

package envbinding

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"
	"k8s.io/apimachinery/pkg/runtime"

	"github.com/oam-dev/kubevela/apis/core.oam.dev/common"
	"github.com/oam-dev/kubevela/apis/core.oam.dev/v1alpha1"
	"github.com/oam-dev/kubevela/apis/core.oam.dev/v1beta1"
)

func TestReadPlacementDecisions(t *testing.T) {
	pld := []v1alpha1.PlacementDecision{{
		Cluster:   "example-cluster",
		Namespace: "example-namespace",
	}}
	testCases := []struct {
		Status           *v1alpha1.EnvBindingStatus
		StatusRaw        []byte
		ExpectedExists   bool
		ExpectedHasError bool
	}{{
		Status:           nil,
		StatusRaw:        []byte(`bad value`),
		ExpectedExists:   false,
		ExpectedHasError: true,
	}, {
		Status: &v1alpha1.EnvBindingStatus{
			Envs: []v1alpha1.EnvStatus{{
				Env:        "example-env",
				Placements: pld,
			}},
		},
		ExpectedExists:   true,
		ExpectedHasError: false,
	}, {
		Status: &v1alpha1.EnvBindingStatus{
			Envs: []v1alpha1.EnvStatus{{
				Env:        "bad-env",
				Placements: pld,
			}},
		},
		ExpectedExists:   false,
		ExpectedHasError: false,
	}}
	r := require.New(t)
	for _, testCase := range testCases {
		app := &v1beta1.Application{}
		_status := common.PolicyStatus{
			Name: "example-policy",
			Type: v1alpha1.EnvBindingPolicyType,
		}
		if testCase.Status == nil {
			_status.Status = &runtime.RawExtension{Raw: testCase.StatusRaw}
		} else {
			bs, err := json.Marshal(testCase.Status)
			r.NoError(err)
			_status.Status = &runtime.RawExtension{Raw: bs}
		}
		app.Status.PolicyStatus = []common.PolicyStatus{_status}
		pds, exists, err := ReadPlacementDecisions(app, "", "example-env")
		r.Equal(testCase.ExpectedExists, exists)
		if testCase.ExpectedHasError {
			r.Error(err)
			continue
		}
		r.NoError(err)
		if exists {
			r.Equal(len(pld), len(pds))
			for idx := range pld {
				r.Equal(pld[idx].Cluster, pds[idx].Cluster)
				r.Equal(pld[idx].Namespace, pds[idx].Namespace)
			}
		}
	}
}
