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

package sets

import (
	"fmt"
	"testing"

	"cuelang.org/go/cue"
	"github.com/bmizerany/assert"
)

func TestPatch(t *testing.T) {

	testCase := []struct {
		base   string
		patch  string
		result string
	}{
		{
			base:  `containers: [{name: "x1"},{name: "x2"},...]`,
			patch: `containers: [{name: "x1"},{name: "x2",image: "pause:0.1"}]`,
			result: `containers: [{
	name: "x1"
}, {
	name:  "x2"
	image: "pause:0.1"
}]
`,
		},

		{
			base:   `containers: [{name: "x1"},{name: "x2"},...]`,
			patch:  `containers: [{name: "x2"},{name: "x1"}]`,
			result: "_|_\n",
		},

		{
			base:   `containers: [{name: _|_},{name: "x2"},...]`,
			patch:  `containers: [{name: _|_},{name: "x2"}]`,
			result: "_|_\n",
		},

		{
			base: `containers: [{name: "x1"},{name: "x2"},...]`,
			patch: `
// +patchKey=name
containers: [{name: "x2"},{name: "x1"}]`,
			result: `// +patchKey=name
containers: [{
	name: "x1"
}, {
	name: "x2"
}, ...]
`,
		},

		{
			base: `containers: [close({namex: "x1"}),...]`,
			patch: `
// +patchKey=name
containers: [{name: "x2"},{name: "x1"}]`,
			result: `			// +patchKey=name
containers: [_|_, // field "name" not allowed in closed struct{
	name: "x1"
}, ...]
`,
		},

		{
			base: `containers: [{name: "x1"},{name: "x2"},...]`,
			patch: `
// +patchKey=name
containers: [{name: "x4"},{name: "x3"},{name: "x1"}]`,
			result: `// +patchKey=name
containers: [{
	name: "x1"
}, {
	name: "x2"
}, {
	name: "x4"
}, {
	name: "x3"
}, ...]
`,
		},

		{
			base: `containers: [{name: "x1"},{name: "x2"},...]`,
			patch: `
// +patchKey=name
containers: [{noname: "x3"}]`,
			result: "_|_\n",
		},
		{
			base: `containers: [{name: "x1"},{name: "x2"},...]`,
			patch: `
// +patchKey=name
containers: []`,
			result: `// +patchKey=name
containers: [{
	name: "x1"
}, {
	name: "x2"
}, ...]
`,
		},
		{
			base: `containers: [{name: "x1"},{name: "x2"},...]`,
			patch: `
// +patchKey=name
containers: [{noname: "x3"},{name: "x1"}]`,
			result: `// +patchKey=name
containers: [{
	name: "x1"
}, {
	name: "x2"
}, ...]
`,
		},
		{
			base: `containers: [{name: "x1"},{name: "x2", envs:[ {name: "OPS",value: string},...]},...]`,
			patch: `
// +patchKey=name
containers: [{name: "x2", envs: [{name: "OPS", value: "OAM"}]}]`,
			result: `// +patchKey=name
containers: [{
	name: "x1"
}, {
	name: "x2"
	envs: [{
		name:  "OPS"
		value: "OAM"
	}, ...]
}, ...]
`,
		},
		{
			base: `containers: [close({name: "x1"}),close({name: "x2", envs:[{name: "OPS",value: string},...]}),...]`,
			patch: `
// +patchKey=name
containers: [{name: "x2", envs: [close({name: "OPS", value: "OAM"})]}]`,
			result: `// +patchKey=name
containers: [close({
	name: "x1"
}), close({
	name: "x2"
	envs: [close({
		name:  "OPS"
		value: "OAM"
	}), ...]
}), ...]
`,
		},

		{
			base: `containers: [{name: "x1"},{name: "x2", envs:[ {name: "OPS",value: string},...]},...]`,
			patch: `
// +patchKey=name
containers: [{name: "x2", envs: [{name: "USER", value: "DEV"},{name: "OPS", value: "OAM"}]}]`,
			result: `// +patchKey=name
containers: [{
	name: "x1"
}, {
	name: "x2"
	envs: [{
		name:  "OPS"
		value: "OAM"
	}, {
		name:  "USER"
		value: "DEV"
	}, ...]
}, ...]
`,
		},

		{
			base: `containers: [{name: "x1"},{name: "x2", envs:[ {key: "OPS",value: string},...]},...]`,
			patch: `
// +patchKey=name
containers: [{name: "x2", 
// +patchKey=key
envs: [{key: "USER", value: "DEV"},{key: "OPS", value: "OAM"}]}]`,
			result: `// +patchKey=name
containers: [{
	name: "x1"
}, {
	name: "x2"
	// +patchKey=key
	envs: [{
		key:   "OPS"
		value: "OAM"
	}, {
		key:   "USER"
		value: "DEV"
	}, ...]
}, ...]
`,
		},
		{
			base: `envFrom: [{
					secretRef: {
						name:  "nginx-rds"
					}},...]`,
			patch: `
// +patchKey=secretRef.name
envFrom: [{
					secretRef: {
						name:  "nginx-redis"
					}},...]
`,
			result: `// +patchKey=secretRef.name
envFrom: [{
	secretRef: {
		name: "nginx-rds"
	}
}, {
	secretRef: {
		name: "nginx-redis"
	}
}, ...]
`},
		{
			base: `
             containers: [{
                 name: "c1"
             },{
                 name: "c2"
                 envFrom: [{
					secretRef: {
						name:  "nginx-rds"
                 }},...]
             },...]`,
			patch: `
             // +patchKey=name
             containers: [{
                 name: "c2"
                 // +patchKey=secretRef.name
                 envFrom: [{
					secretRef: {
						name:  "nginx-redis"
                 }},...]
             }]`,
			result: `// +patchKey=name
containers: [{
	name: "c1"
}, {
	name: "c2"
	// +patchKey=secretRef.name
	envFrom: [{
		secretRef: {
			name: "nginx-rds"
		}
	}, {
		secretRef: {
			name: "nginx-redis"
		}
	}, ...]
}, ...]
`},

		{
			base: `
             containers: [{
               volumeMounts: [{name: "k1", path: "p1"},{name: "k1", path: "p2"},...]
             },...]
			volumes: [{name: "x1",value: "v1"},{name: "x2",value: "v2"},...]
`,

			patch: `
			 // +patchKey=name
             volumes: [{name: "x1",value: "v1"},{name: "x3",value: "x2"}]
             
             containers: [{
               volumeMounts: [{name: "k1", path: "p1"},{name: "k1", path: "p2"},{ name:"k2", path: "p3"}]
             },...]`,
			result: `containers: [{
	volumeMounts: [{
		path: "p1"
		name: "k1"
	}, {
		path: "p2"
		name: "k1"
	}, {
		path: "p3"
		name: "k2"
	}]
}, ...]
// +patchKey=name
volumes: [{
	name:  "x1"
	value: "v1"
}, {
	name:  "x2"
	value: "v2"
}, {
	name:  "x3"
	value: "x2"
}, ...]
`},

		{
			base: `
containers: [{
	name: "c1"
},{
	name: "c2"
	envFrom: [{
		secretRef: {
			name:  "nginx-rds"
		},
	}, {
		configMapRef: {
			name:  "nginx-rds"
		},
	},...]
},...]`,
			patch: `
// +patchKey=name
containers: [{
	name: "c2"
	// +patchKey=secretRef.name,configMapRef.name
	envFrom: [{
		secretRef: {
			name:  "nginx-redis"
		},
	}, {
		configMapRef: {
			name:  "nginx-redis"
		},
	},...]
}]`,
			result: `// +patchKey=name
containers: [{
	name: "c1"
}, {
	name: "c2"
	// +patchKey=secretRef.name,configMapRef.name
	envFrom: [{
		secretRef: {
			name: "nginx-rds"
		}
	}, {
		configMapRef: {
			name: "nginx-rds"
		}
	}, {
		secretRef: {
			name: "nginx-redis"
		}
	}, {
		configMapRef: {
			name: "nginx-redis"
		}
	}, ...]
}, ...]
`},
	}

	for i, tcase := range testCase {
		t.Run(fmt.Sprintf("case-%d", i), func(t *testing.T) {
			v, _ := StrategyUnify(tcase.base, tcase.patch)
			assert.Equal(t, v, tcase.result, fmt.Sprintf("testPatch for case(no:%d) %s", i, v))
		})
	}
}

func TestStrategyPatch(t *testing.T) {
	testCase := []struct {
		base   string
		patch  string
		result string
	}{
		{
			base: `
spec: {
  strategy: {
    type: "rollingUpdate"
    rollingUpdate: maxSurge: "30%"
	}
}
`,
			patch: `
spec: {
  // +patchStrategy=retainKeys
  strategy: type: "recreate"
}
`,
			result: `spec: {
	strategy: {
		// +patchStrategy=retainKeys
		type: "recreate"
	}
}
`},

		{
			base: `
spec: {
  strategy: close({
    type: "rollingUpdate"
    rollingUpdate: maxSurge: "30%"
	})
}
`,
			patch: `
spec: {
  // +patchStrategy=retainKeys
  strategy: type: "recreate"
}
`,
			result: `spec: {
	strategy: {
		// +patchStrategy=retainKeys
		type: "recreate"
	}
}
`},

		{
			base: `
volumes: [{
	name: "test-volume"
	cinder: {
		volumeID: "<volume id>"
		fsType: "ext4"
	}
}]
`,
			patch: `
// +patchStrategy=retainKeys
// +patchKey=name
volumes: [
{
	name: "test-volume"
	configMap: name: "conf-name"
}]
`,
			result: `// +patchStrategy=retainKeys
// +patchKey=name
volumes: [{
	name: "test-volume"
	configMap: {
		name: "conf-name"
	}
}]
`},

		{
			base: `
volumes: [{
	name: "empty-volume"
	emptyDir: {}
},
{
	name: "test-volume"
	cinder: {
		volumeID: "<volume id>"
		fsType: "ext4"
	}
}]
`,
			patch: `
// +patchStrategy=retainKeys
// +patchKey=name
volumes: [
{
	name: "test-volume"
	configMap: name: "conf-name"
}]
`,
			result: `// +patchStrategy=retainKeys
// +patchKey=name
volumes: [{
	name: "empty-volume"
	emptyDir: {}
}, {
	name: "test-volume"
	configMap: {
		name: "conf-name"
	}
}]
`},

		{
			base: `
containers: [{
	name: "c1"
	image: "image1"
},
{
	name: "c2"
	envs:[{name: "e1",value: "v1"}]
}]
`,
			patch: `
// +patchKey=name
containers: [{
	name: "c2"
	// +patchStrategy=retainKeys
	envs:[{name: "e1",value: "v2"},...]
}]
`,
			result: `// +patchKey=name
containers: [{
	name:  "c1"
	image: "image1"
}, {
	name: "c2"
	// +patchStrategy=retainKeys
	envs: [{
		name:  "e1"
		value: "v2"
	}]
}]
`},

		{
			base: `
spec: containers: [{
	name: "c1"
	image: "image1"
},
{
	name: "c2"
	envs:[{name: "e1",value: "v1"}]
}]
`,
			patch: `
// +patchKey=name
// +patchStrategy=retainKeys
spec: {
	containers: [{
		name: "c2"
		envs:[{name: "e1",value: "v2"}]
}]}
`,
			result: `// +patchKey=name
// +patchStrategy=retainKeys
spec: {
	containers: [{
		name: "c2"
		envs: [{
			name:  "e1"
			value: "v2"
		}, ...]
	}, ...]
}
`}, {
			base: `
kind: "Old"
metadata: {
	name: "Old"
	labels: keep: "true"
}
`,
			patch: `// +patchStrategy=retainKeys
kind: "New"
metadata: {
	// +patchStrategy=retainKeys
	name: "New"
}
`,
			result: `	// +patchStrategy=retainKeys
kind: "New"
metadata: {
	// +patchStrategy=retainKeys
	name: "New"
	labels: {
		keep: "true"
	}
}
`},
	}

	for i, tcase := range testCase {
		v, _ := StrategyUnify(tcase.base, tcase.patch)
		assert.Equal(t, v, tcase.result, fmt.Sprintf("testPatch for case(no:%d) %s", i, v))
	}
}

func TestParseCommentTags(t *testing.T) {
	temp := `
// +patchKey=name
// +testKey1=testValue1
	// +testKey2=testValue2
// +testKey3 =testValue3
//    +testKey4 = testValue4
// invalid=x
// +invalid=x y
// +invalid
x: null
`

	var r cue.Runtime
	inst, err := r.Compile("-", temp)
	if err != nil {
		t.Error(err)
		return
	}
	ms := findCommentTag(inst.Lookup("x").Doc())
	assert.Equal(t, ms, map[string]string{
		"patchKey": "name",
		"testKey1": "testValue1",
		"testKey2": "testValue2",
		"testKey3": "testValue3",
		"testKey4": "testValue4",
	})
}
