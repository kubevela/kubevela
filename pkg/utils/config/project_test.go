/*
Copyright 2022 The KubeVela Authors.

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
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"testing"

	"github.com/stretchr/testify/assert"
	corev1 "k8s.io/api/core/v1"

	"github.com/oam-dev/kubevela/apis/types"
)

var ResponseString = "Hello HTTP Get."

func TestMatchProject(t *testing.T) {
	s := runtime.NewScheme()
	corev1.AddToScheme(s)
	secret1 := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "s1",
			Namespace: types.DefaultKubeVelaNS,
			Labels: map[string]string{
				types.LabelConfigProject: "p1",
			},
		},
	}

	secret2 := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "s2",
			Namespace: types.DefaultKubeVelaNS,
			Labels: map[string]string{
				types.LabelConfigProject: "",
			},
		},
	}

	type args struct {
		secret  *corev1.Secret
		project string
	}

	type want struct {
		matched bool
	}

	testcases := []struct {
		name string
		args args
		want want
	}{
		{
			name: "matched",
			args: args{
				project: "p99",
				secret:  secret1,
			},
			want: want{
				matched: false,
			},
		},
		{
			name: "not matched",
			args: args{
				project: "p99",
				secret:  secret2,
			},
			want: want{
				matched: true,
			},
		},
	}

	for _, tc := range testcases {
		t.Run(tc.name, func(t *testing.T) {
			got := ProjectMatched(tc.args.secret, tc.args.project)
			assert.Equal(t, tc.want.matched, got)
		})
	}
}
