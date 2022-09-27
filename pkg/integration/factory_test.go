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

package integration

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestParseIntegrationTemplate(t *testing.T) {
	r := require.New(t)
	content, err := ioutil.ReadFile("testdata/helm-repo.cue")
	r.Equal(err, nil)
	var inf = &kubeIntegrationFactory{}
	template, err := inf.ParseTemplate("default", content)
	r.Equal(err, nil)
	r.NotEqual(template, nil)
	r.Equal(template.Name, "helm-repository")
	r.NotEqual(template.Schema, nil)
	r.Equal(len(template.Schema.Properties), 4)
	out, _ := json.Marshal(template.Schema)
	fmt.Println(string(out))
}
