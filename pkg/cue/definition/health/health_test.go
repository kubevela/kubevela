/*
Copyright 2025 The KubeVela Authors.

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

package health

import (
	"strings"
	"testing"

	"cuelang.org/go/cue/token"
	"github.com/stretchr/testify/assert"
)

func TestCheckHealth(t *testing.T) {
	cases := map[string]struct {
		tpContext  map[string]interface{}
		healthTemp string
		parameter  interface{}
		exp        bool
	}{
		"normal-equal": {
			tpContext: map[string]interface{}{
				"output": map[string]interface{}{
					"status": map[string]interface{}{
						"readyReplicas": 4,
						"replicas":      4,
					},
				},
			},
			healthTemp: "isHealth:  context.output.status.readyReplicas == context.output.status.replicas",
			parameter:  nil,
			exp:        true,
		},
		"normal-false": {
			tpContext: map[string]interface{}{
				"output": map[string]interface{}{
					"status": map[string]interface{}{
						"readyReplicas": 4,
						"replicas":      5,
					},
				},
			},
			healthTemp: "isHealth: context.output.status.readyReplicas == context.output.status.replicas",
			parameter:  nil,
			exp:        false,
		},
		"array-case-equal": {
			tpContext: map[string]interface{}{
				"output": map[string]interface{}{
					"status": map[string]interface{}{
						"conditions": []interface{}{
							map[string]interface{}{"status": "True"},
						},
					},
				},
			},
			healthTemp: `isHealth: context.output.status.conditions[0].status == "True"`,
			parameter:  nil,
			exp:        true,
		},
		"parameter-false": {
			tpContext: map[string]interface{}{
				"output": map[string]interface{}{
					"status": map[string]interface{}{
						"replicas": 4,
					},
				},
				"outputs": map[string]interface{}{
					"my": map[string]interface{}{
						"status": map[string]interface{}{
							"readyReplicas": 4,
						},
					},
				},
			},
			healthTemp: "isHealth: context.outputs[parameter.res].status.readyReplicas == context.output.status.replicas",
			parameter: map[string]string{
				"res": "my",
			},
			exp: true,
		},
	}
	for message, ca := range cases {
		healthy, err := CheckHealth(ca.tpContext, ca.healthTemp, ca.parameter)
		assert.NoError(t, err, message)
		assert.Equal(t, ca.exp, healthy, message)
	}
}

func TestGetStatusMessage(t *testing.T) {
	cases := map[string]struct {
		tpContext  map[string]interface{}
		parameter  interface{}
		statusTemp string
		expMessage string
	}{
		"field-with-array-and-outputs": {
			tpContext: map[string]interface{}{
				"outputs": map[string]interface{}{
					"service": map[string]interface{}{
						"spec": map[string]interface{}{
							"type":      "NodePort",
							"clusterIP": "10.0.0.1",
							"ports": []interface{}{
								map[string]interface{}{
									"port": 80,
								},
							},
						},
					},
					"ingress": map[string]interface{}{
						"rules": []interface{}{
							map[string]interface{}{
								"host": "example.com",
							},
						},
					},
				},
			},
			statusTemp: `message: "type: " + context.outputs.service.spec.type + " clusterIP:" + context.outputs.service.spec.clusterIP + " ports:" + "\(context.outputs.service.spec.ports[0].port)" + " domain:" + context.outputs.ingress.rules[0].host`,
			expMessage: "type: NodePort clusterIP:10.0.0.1 ports:80 domain:example.com",
		},
		"complex status": {
			tpContext: map[string]interface{}{
				"outputs": map[string]interface{}{
					"ingress": map[string]interface{}{
						"spec": map[string]interface{}{
							"rules": []interface{}{
								map[string]interface{}{
									"host": "example.com",
								},
							},
						},
						"status": map[string]interface{}{
							"loadBalancer": map[string]interface{}{
								"ingress": []interface{}{
									map[string]interface{}{
										"ip": "10.0.0.1",
									},
								},
							},
						},
					},
				},
			},
			statusTemp: `if len(context.outputs.ingress.status.loadBalancer.ingress) > 0 {
	message: "Visiting URL: " + context.outputs.ingress.spec.rules[0].host + ", IP: " + context.outputs.ingress.status.loadBalancer.ingress[0].ip
}
if len(context.outputs.ingress.status.loadBalancer.ingress) == 0 {
	message: "No loadBalancer found, visiting by using 'vela port-forward " + context.appName + " --route'\n"
}`,
			expMessage: "Visiting URL: example.com, IP: 10.0.0.1",
		},
		"status use parameter field": {
			tpContext: map[string]interface{}{
				"outputs": map[string]interface{}{
					"test-name": map[string]interface{}{
						"spec": map[string]interface{}{
							"type":      "NodePort",
							"clusterIP": "10.0.0.1",
							"ports": []interface{}{
								map[string]interface{}{
									"port": 80,
								},
							},
						},
					},
				},
			},
			parameter: map[string]interface{}{
				"configInfo": map[string]string{
					"name": "test-name",
				},
			},
			statusTemp: `message: parameter.configInfo.name + ".type: " + context.outputs["\(parameter.configInfo.name)"].spec.type`,
			expMessage: "test-name.type: NodePort",
		},
		"import package in template": {
			tpContext: map[string]interface{}{
				"outputs": map[string]interface{}{
					"service": map[string]interface{}{
						"spec": map[string]interface{}{
							"type":      "NodePort",
							"clusterIP": "10.0.0.1",
							"ports": []interface{}{
								map[string]interface{}{
									"port": 80,
								},
							},
						},
					},
					"ingress": map[string]interface{}{
						"rules": []interface{}{
							map[string]interface{}{
								"host": "example.com",
							},
						},
					},
				},
			},
			statusTemp: `import "strconv"
      message: "ports: " + strconv.FormatInt(context.outputs.service.spec.ports[0].port,10)`,
			expMessage: "ports: 80",
		},
	}
	for message, ca := range cases {
		gotMessage, err := getStatusMessage(ca.tpContext, ca.statusTemp, ca.parameter)
		assert.NoError(t, err, message)
		assert.Equal(t, ca.expMessage, gotMessage, message)
	}
}

func TestGetStatus(t *testing.T) {
	cases := map[string]struct {
		tpContext map[string]interface{}
		parameter interface{}
		statusCue string
		expStatus map[string]string
		expErr    bool
	}{
		"test-simple-output-status": {
			tpContext: map[string]interface{}{
				"output": map[string]interface{}{
					"replicas":      3,
					"readyReplicas": 3,
				},
			},
			parameter: make(map[string]interface{}),
			statusCue: strings.TrimSpace(`
				deploymentReady: context.output.readyReplicas == context.output.replicas
				deploymentHealthy: context.output.readyReplicas > 2
			`),
			expStatus: map[string]string{
				"deploymentReady":   "true",
				"deploymentHealthy": "true",
			},
		},
		"test-simple-outputs-status": {
			tpContext: map[string]interface{}{
				"outputs": map[string]interface{}{
					"deployment": map[string]interface{}{
						"replicas":      3,
						"readyReplicas": 3,
					},
				},
			},
			parameter: make(map[string]interface{}),
			statusCue: strings.TrimSpace(`
				deploymentReady: context.outputs.deployment.readyReplicas == context.outputs.deployment.replicas
				deploymentHealthy: context.outputs.deployment.readyReplicas > 2
			`),
			expStatus: map[string]string{
				"deploymentReady":   "true",
				"deploymentHealthy": "true",
			},
		},
		"test-field-is-already-a-string": {
			tpContext: map[string]interface{}{
				"output": map[string]interface{}{
					"stringVal": "abc",
				},
			},
			parameter: make(map[string]interface{}),
			statusCue: strings.TrimSpace(`
				stringVal: context.output.stringVal
			`),
			expStatus: map[string]string{
				"stringVal": "abc",
			},
		},
		"test-field-not-exists": {
			tpContext: map[string]interface{}{
				"outputs": map[string]interface{}{
					"deployment": map[string]interface{}{},
				},
			},
			parameter: make(map[string]interface{}),
			statusCue: strings.TrimSpace(`
				notExists: context.output.notExists
			`),
			expStatus: map[string]string{
				"notExists": token.BOTTOM.String(),
			},
		},
		"test-field-not-exists-has-no-impact-on-others": {
			tpContext: map[string]interface{}{
				"output": map[string]interface{}{
					"deployment": map[string]interface{}{
						"exists": "abc",
					},
				},
			},
			parameter: make(map[string]interface{}),
			statusCue: strings.TrimSpace(`
				exists: context.output.deployment.exists
				notExists: context.output.notExists
			`),
			expStatus: map[string]string{
				"notExists": token.BOTTOM.String(),
				"exists":    "abc",
			},
		},
		"test-parameter-in-status": {
			tpContext: map[string]interface{}{
				"output": map[string]interface{}{
					"anOutput": 2,
				},
			},
			parameter: map[string]interface{}{
				"aParam": 2,
			},
			statusCue: strings.TrimSpace(`
				sum: context.output.anOutput + parameter.aParam
			`),
			expStatus: map[string]string{
				"sum": "4",
			},
		},
		"test-key-input-too-large-skipped": {
			tpContext: map[string]interface{}{
				"output": map[string]interface{}{
					"anOutput": 2,
				},
			},
			parameter: make(map[string]interface{}),
			statusCue: strings.TrimSpace(`
				thisIsAKeyThatIsWayTooLongToBeUsedAsAKeyInTheStatusMapAndShouldBeSkipped: context.output.anOutput
			`),
			expStatus: map[string]string{},
		},
		"test-not-exists-with-fallback": {
			tpContext: map[string]interface{}{
				"output": map[string]interface{}{},
			},
			parameter: make(map[string]interface{}),
			statusCue: strings.TrimSpace(`
				healthy: *context.output.isHealthy | false
			`),
			expStatus: map[string]string{
				"healthy": "false",
			},
		},
		"test-invalid-context-fails-gracefully": {
			tpContext: map[string]interface{}{
				"createFailure": make(chan int), // This simulates json parsing failures
			},
			parameter: make(map[string]interface{}),
			statusCue: strings.TrimSpace(`
				healthy: *context.output.isHealthy | false
			`),
			expStatus: map[string]string{},
			expErr:    true,
		},
		"test-annotations-ignored-but-referencable": {
			tpContext: map[string]interface{}{
				"output": map[string]interface{}{
					"replicas":      3,
					"readyReplicas": 3,
				},
			},
			parameter: make(map[string]interface{}),
			statusCue: strings.TrimSpace(`
		        a: 1 @local()
				b: 2 @exclude()
				c: a + b
			`),
			expStatus: map[string]string{
				"c": "3",
			},
		},
		"test-$-fields-ignored-but-referencable": {
			tpContext: map[string]interface{}{
				"output": map[string]interface{}{
					"replicas":      3,
					"readyReplicas": 3,
				},
			},
			parameter: make(map[string]interface{}),
			statusCue: strings.TrimSpace(`
		        $a: 1
				$b: 2
				c: $a + $b
			`),
			expStatus: map[string]string{
				"c": "3",
			},
		},
	}

	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			status, err := getStatusMap(tc.tpContext, tc.statusCue, tc.parameter)
			if !tc.expErr {
				assert.NoError(t, err)
			}
			assert.Equal(t, tc.expStatus, status, "status fields should match")
		})
	}
}
