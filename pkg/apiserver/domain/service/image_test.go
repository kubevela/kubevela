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

package service

import (
	"testing"

	"gotest.tools/assert"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	velatypes "github.com/oam-dev/kubevela/apis/types"
	v1 "github.com/oam-dev/kubevela/pkg/apiserver/interfaces/api/dto/v1"
)

func TestGetImageInfo(t *testing.T) {

	s2 := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "s2",
			Namespace: velatypes.DefaultKubeVelaNS,
			Labels: map[string]string{
				velatypes.LabelConfigCatalog:    velatypes.VelaCoreConfig,
				velatypes.LabelConfigType:       velatypes.ImageRegistry,
				velatypes.LabelConfigProject:    "",
				velatypes.LabelConfigIdentifier: "index.docker.io",
			},
		},
		Data: map[string][]byte{
			"insecure-skip-verify": []byte("true"),
			".dockerconfigjson":    []byte(`{"auths":{"index.docker.io":{"auth":"aHlicmlkY2xvdWRAcHJvZC5YTEyMw==","username":"xxx","password":"yyy"}}}`),
		},
	}

	insecure, useHTTP, user, pass := getAccountFromSecret(*s2, "index.docker.io")
	assert.DeepEqual(t, user, "xxx")
	assert.DeepEqual(t, pass, "yyy")
	assert.DeepEqual(t, insecure, true)
	assert.DeepEqual(t, useHTTP, false)

	var cf v1.ImageInfo
	// Test the public image
	err := getImageInfo("nginx", false, false, "", "", &cf)
	assert.DeepEqual(t, err, nil)
	assert.DeepEqual(t, cf.Info.Config.Entrypoint, []string{"/docker-entrypoint.sh"})

	// Test the private image
	err = getImageInfo("nginx424ru823-should-not-existed", false, false, "abc", "efg", &cf)
	assert.DeepEqual(t, err.Error(), "incorrect username or password")

	err = getImageInfo("text.registry/test-image", false, false, "", "", &cf)
	assert.DeepEqual(t, err != nil, true)
}
