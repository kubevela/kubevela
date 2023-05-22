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

package env

import (
	"path/filepath"
	"testing"
	"time"

	"github.com/google/go-cmp/cmp"
	"github.com/stretchr/testify/assert"
	"k8s.io/client-go/rest"
	"sigs.k8s.io/controller-runtime/pkg/envtest"

	"github.com/oam-dev/kubevela/apis/types"
	"github.com/oam-dev/kubevela/pkg/utils/common"
)

var testEnv *envtest.Environment
var cfg *rest.Config

func TestCreateEnv(t *testing.T) {

	testEnv = &envtest.Environment{
		ControlPlaneStartTimeout: time.Minute,
		ControlPlaneStopTimeout:  time.Minute,
		CRDDirectoryPaths: []string{
			filepath.Join("../../..", "charts/vela-core/crds"), // this has all the required CRDs,
		},
	}
	var err error
	cfg, err = testEnv.Start()
	assert.NoError(t, err)
	defer func() {
		assert.NoError(t, testEnv.Stop())
	}()
	common.SetConfig(cfg)

	type want struct {
		data string
	}
	testcases := []struct {
		name    string
		envMeta *types.EnvMeta
		want    want
	}{
		{
			name: "env-application",
			envMeta: &types.EnvMeta{
				Name:      "env-application",
				Namespace: "default",
			},
			want: want{
				data: "",
			},
		},
		{
			name: "default",
			envMeta: &types.EnvMeta{
				Name:      "default",
				Namespace: "default",
			},
			want: want{
				data: "the namespace default was already assigned to env env-application",
			},
		},
	}
	for _, tc := range testcases {
		t.Run(tc.name, func(t *testing.T) {
			if err != nil && cmp.Diff(tc.want.data, err.Error()) != "" {
				t.Errorf("CreateEnv(...): \n -want: \n%s,\n +got:\n%s", tc.want.data, err.Error())
			}
		})
	}

}
