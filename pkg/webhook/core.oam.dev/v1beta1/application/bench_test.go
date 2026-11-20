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

package application

import "testing"

// Admission derives a definition's parameter contract once per distinct
// definition per request. The reduction is cached; the compile cannot be,
// because cue values from one Context are not safe for concurrent use and
// admission requests are concurrent.
//
//	before   104,117 ns/op
//	after     ~57,000 ns/op   (the compile, which remains)
const benchDefinitionTemplate = `
import "strconv"

#Port: {
	port: int
	name?: string
}

parameter: {
	image: string
	imagePullPolicy?: *"IfNotPresent" | "Always" | "Never"
	ports?: [...#Port]
	env?: [...{name: string, value?: string}]
	replicas: *1 | int
	labels?: [string]: string
	resources?: {
		requests?: {cpu?: string, memory?: string}
		limits?: {cpu?: string, memory?: string}
	}
}

output: {
	apiVersion: "apps/v1"
	kind:       "Deployment"
	spec: replicas: parameter.replicas
	_x: strconv.Itoa(parameter.replicas)
}
`

func BenchmarkParameterBlockOnly(b *testing.B) {
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		if _, ok := parameterBlockOnly(benchDefinitionTemplate); !ok {
			b.Fatal("did not extract")
		}
	}
}

// The reduction alone, which is what the cache removes from the path.
func BenchmarkParameterBlockSource(b *testing.B) {
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		if _, ok := parameterBlockSource(benchDefinitionTemplate); !ok {
			b.Fatal("did not extract")
		}
	}
}
