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

package utils

import (
	"context"
	"encoding/json"
	"reflect"
	"testing"

	"github.com/stretchr/testify/assert"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

func TestGetDexConnectors(t *testing.T) {
	ctx := context.Background()
	type args struct {
		k8sClient client.Client
	}
	type want struct {
		connectors []map[string]interface{}
		err        error
	}

	ldap := map[string]interface{}{
		"clientID":     "clientID",
		"clientSecret": "clientSecret",
		"callbackURL":  "redirectURL",
		"xxx":          map[string]interface{}{"aaa": "bbb", "ccc": "ddd"},
	}
	data, err := json.Marshal(ldap)
	assert.NoError(t, err)

	secret := &v1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "a",
			Namespace: "vela-system",
			Labels: map[string]string{
				"app.oam.dev/source-of-truth": "from-inner-system",
				"config.oam.dev/catalog":      "velacore-config",
				"config.oam.dev/type":         "config-dex-connector",
				"config.oam.dev/sub-type":     "ldap",
				"project":                     "abc",
			},
		},
		Data: map[string][]byte{
			"ldap": data,
		},
		Type: v1.SecretTypeOpaque,
	}

	k8sClient := fake.NewClientBuilder().WithObjects(secret).Build()

	testcaes := map[string]struct {
		args args
		want want
	}{

		"test": {args: args{
			k8sClient: k8sClient,
		},
			want: want{
				connectors: []map[string]interface{}{{
					"id":   "a",
					"name": "a",
					"type": "ldap",
					"config": map[string]interface{}{
						"clientID":     "clientID",
						"clientSecret": "clientSecret",
						"callbackURL":  "redirectURL",
						"xxx":          map[string]interface{}{"aaa": "bbb", "ccc": "ddd"},
					},
				}},
			},
		},
	}

	for name, tc := range testcaes {
		t.Run(name, func(t *testing.T) {
			got, err := GetDexConnectors(ctx, tc.args.k8sClient)
			if err != tc.want.err {
				t.Errorf("%s: GetDexConnectors() error = %v, wantErr %v", name, err, tc.want.err)
				return
			}
			if !reflect.DeepEqual(got, tc.want.connectors) {
				t.Errorf("%s: GetDexConnectors() = %v, want %v", name, got, tc.want.connectors)
			}
		})
	}
}
