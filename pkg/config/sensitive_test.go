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
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/oam-dev/kubevela/apis/types"
)

func TestSensitiveTurnedOffError(t *testing.T) {
	require.EqualError(t, SensitiveTurnedOffError("wf-a", "t"), "config template wf-a/t: sensitive cannot be changed from true to false")
}

// The webhook and the CLI repeat the CRD rule's message; the CRD is generated from the marker on
// ConfigTemplateSpec, so a change to either side shows here.
func TestSensitiveTurnedOffMessageIsTheCRDRules(t *testing.T) {
	crd, err := os.ReadFile("../../charts/vela-core/crds/config.oam.dev_configtemplates.yaml")
	require.NoError(t, err)
	require.Contains(t, string(crd), "- message: "+SensitiveTurnedOffMessage)
}

// ReadConfigSecret is what users read through and refuses a sensitive Secret;
// SecretInputProperties is a controller's read of a config it keeps for its own use.
func TestSecretInputPropertiesReadsWhatReadConfigSecretRefuses(t *testing.T) {
	secret := &v1.Secret{
		ObjectMeta: metav1.ObjectMeta{Name: "c", Namespace: "ns", Annotations: map[string]string{types.AnnotationConfigSensitive: "true"}},
		Data:       map[string][]byte{SaveInputPropertiesKey: []byte(`{"host":"example.com"}`)},
	}
	_, err := ReadConfigSecret(secret)
	require.ErrorIs(t, err, ErrSensitiveConfig)

	got, err := SecretInputProperties(secret)
	require.NoError(t, err)
	require.Equal(t, map[string]interface{}{"host": "example.com"}, got)

	delete(secret.Data, SaveInputPropertiesKey)
	_, err = SecretInputProperties(secret)
	require.EqualError(t, err, "secret ns/c has no input-properties")
}
