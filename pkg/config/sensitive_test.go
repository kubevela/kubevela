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
