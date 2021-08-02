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

package stdlib

import (
	"testing"

	"github.com/bmizerany/assert"
)

func TestGetPackages(t *testing.T) {
	d := &discover{}
	f1 := file{
		name: "network.cue",
		path: "kube/core",
		content: `
service: {
    apiVersion: "v1"
    kind: "Service"
}
`,
	}

	f2 := file{
		name: "security.cue",
		path: "kube/core",
		content: `
secret: {
    apiVersion: "v1"
    kind: "Secret"
}
`,
	}

	f3 := file{
		name: "apps.cue",
		path: "kube/apps",
		content: `
deployment: {
    apiVersion: "apps/v1"
    kind: "Deployment"
}
`,
	}
	d.addFile(f1)
	d.addFile(f2)
	d.addFile(f3)
	for path, pkg := range d.packages() {
		switch path {
		case "kube/core":
			assert.Equal(t, pkg, `
service: {
    apiVersion: "v1"
    kind: "Service"
}


secret: {
    apiVersion: "v1"
    kind: "Secret"
}

`)
		case "kube/apps":
			assert.Equal(t, pkg, `
deployment: {
    apiVersion: "apps/v1"
    kind: "Deployment"
}

`)
		default:
			t.Error("package path invalid")
		}

	}
}
