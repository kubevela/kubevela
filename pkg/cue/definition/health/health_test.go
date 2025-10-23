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
				b: 2 @private()
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
			_, status, err := getStatusMap(tc.tpContext, tc.statusCue, tc.parameter)
			if !tc.expErr {
				assert.NoError(t, err)
			}
			assert.Equal(t, tc.expStatus, status, "status fields should match")
		})
	}
}

func TestContextPassing(t *testing.T) {
	cases := map[string]struct {
		initialCtx  map[string]interface{}
		request     StatusRequest
		expMessage  string
		expDetails  map[string]string
		validateCtx func(t *testing.T, ctx map[string]interface{})
	}{
		"basic-context-passing": {
			initialCtx: map[string]interface{}{},
			request: StatusRequest{
				Parameter: map[string]interface{}{},
				Details: strings.TrimSpace(`
					stringValue: "example"
					intValue: 1 + 2
				`),
				Custom: strings.TrimSpace(`
					message: "\(context.status.details.stringValue) \(context.status.details.intValue)"
				`),
			},
			expMessage: "example 3",
			expDetails: map[string]string{
				"stringValue": "example",
				"intValue":    "3",
			},
		},
		"complex-types-in-context": {
			initialCtx: map[string]interface{}{
				"outputs": map[string]interface{}{
					"service": map[string]interface{}{
						"port": 8080,
					},
				},
			},
			request: StatusRequest{
				Parameter: map[string]interface{}{
					"replicas": 3,
				},
				Details: strings.TrimSpace(`
					replicas: parameter.replicas
					port: context.outputs.service.port
					isReady: parameter.replicas > 0 && context.outputs.service.port > 0
					config: {
						enabled: true
						timeout: 30
					} @private()
					configEnabled: config.enabled
					configTimeout: config.timeout
				`),
				Custom: strings.TrimSpace(`
					message: "Service on port \(context.status.details.port) with \(context.status.details.replicas) replicas is ready: \(context.status.details.isReady)"
				`),
			},
			expMessage: "Service on port 8080 with 3 replicas is ready: true",
			expDetails: map[string]string{
				"replicas":      "3",
				"port":          "8080",
				"isReady":       "true",
				"configEnabled": "true",
				"configTimeout": "30",
			},
			validateCtx: func(t *testing.T, ctx map[string]interface{}) {
				statusCtx := ctx["status"].(map[string]interface{})
				details := statusCtx["details"].(map[string]interface{})

				assert.Equal(t, int64(3), details["replicas"])
				assert.Equal(t, int64(8080), details["port"])
				assert.Equal(t, true, details["isReady"])
				assert.Equal(t, true, details["configEnabled"])
				assert.Equal(t, int64(30), details["configTimeout"])

				assert.Nil(t, details["config"])
			},
		},
		"array-handling-in-context": {
			initialCtx: map[string]interface{}{},
			request: StatusRequest{
				Parameter: map[string]interface{}{},
				Details: strings.TrimSpace(`
					$ports: [80, 443, 8080]
					$protocols: ["http", "https", "http"]
					$mappings: [
						{port: 80, protocol: "http"},
						{port: 443, protocol: "https"}
					]
					portCount: len($ports)
					firstPort: $ports[0]
					mainProtocol: $protocols[0]
					portsString: "80,443,8080"
				`),
				Custom: strings.TrimSpace(`
					message: "Serving on \(len(context.status.details.$ports)) ports"
				`),
			},
			expMessage: "Serving on 3 ports",
			expDetails: map[string]string{
				"portCount":    "3",
				"firstPort":    "80",
				"mainProtocol": "http",
				"portsString":  "80,443,8080",
			},
			validateCtx: func(t *testing.T, ctx map[string]interface{}) {
				statusCtx := ctx["status"].(map[string]interface{})
				details := statusCtx["details"].(map[string]interface{})

				ports := details["$ports"].([]interface{})
				assert.Len(t, ports, 3)
				assert.Equal(t, int64(80), ports[0])
				assert.Equal(t, int64(443), ports[1])
				assert.Equal(t, int64(8080), ports[2])

				protocols := details["$protocols"].([]interface{})
				assert.Len(t, protocols, 3)
				assert.Equal(t, "http", protocols[0])
				assert.Equal(t, "https", protocols[1])

				mappings := details["$mappings"].([]interface{})
				assert.Len(t, mappings, 2)

				assert.Equal(t, int64(3), details["portCount"])
				assert.Equal(t, int64(80), details["firstPort"])
				assert.Equal(t, "http", details["mainProtocol"])
				assert.Equal(t, "80,443,8080", details["portsString"])
			},
		},
		"nested-references": {
			initialCtx: map[string]interface{}{
				"appName": "my-app",
			},
			request: StatusRequest{
				Parameter: map[string]interface{}{
					"env": "production",
				},
				Details: strings.TrimSpace(`
					environment: parameter.env
					$appInfo: {
						name: context.appName
						env: parameter.env
						fullName: "\(context.appName)-\(parameter.env)"
					}
					appName: $appInfo.name
					appEnv: $appInfo.env
					appFullName: $appInfo.fullName
				`),
				Custom: strings.TrimSpace(`
					message: "Deployed \(context.status.details.$appInfo.fullName) to \(context.status.details.environment)"
				`),
			},
			expMessage: "Deployed my-app-production to production",
			expDetails: map[string]string{
				"environment": "production",
				"appName":     "my-app",
				"appEnv":      "production",
				"appFullName": "my-app-production",
			},
			validateCtx: func(t *testing.T, ctx map[string]interface{}) {
				statusCtx := ctx["status"].(map[string]interface{})
				details := statusCtx["details"].(map[string]interface{})

				appInfo := details["$appInfo"].(map[string]interface{})
				assert.Equal(t, "my-app", appInfo["name"])
				assert.Equal(t, "production", appInfo["env"])
				assert.Equal(t, "my-app-production", appInfo["fullName"])

				assert.Equal(t, "production", details["environment"])
				assert.Equal(t, "my-app", details["appName"])
				assert.Equal(t, "production", details["appEnv"])
				assert.Equal(t, "my-app-production", details["appFullName"])
			},
		},
		"existing-status-preserved": {
			initialCtx: map[string]interface{}{
				"status": map[string]interface{}{
					"existingField": "should-be-preserved",
				},
			},
			request: StatusRequest{
				Parameter: map[string]interface{}{},
				Details: strings.TrimSpace(`
					newField: "added-value"
				`),
				Custom: strings.TrimSpace(`
					message: "Status has existing: \(context.status.existingField)"
				`),
			},
			expMessage: "Status has existing: should-be-preserved",
			expDetails: map[string]string{
				"newField": "added-value",
			},
			validateCtx: func(t *testing.T, ctx map[string]interface{}) {
				statusCtx := ctx["status"].(map[string]interface{})
				assert.Equal(t, "should-be-preserved", statusCtx["existingField"])
				assert.NotNil(t, statusCtx["details"])
			},
		},
		"dollar-fields-in-context-only": {
			initialCtx: map[string]interface{}{},
			request: StatusRequest{
				Parameter: map[string]interface{}{
					"baseValue": 10,
				},
				Details: strings.TrimSpace(`
					$multiplier: 2
					$offset: 5
					result: parameter.baseValue * $multiplier + $offset
					displayText: "Result is \(result)"
				`),
				Custom: strings.TrimSpace(`
					message: "Computed using multiplier \(context.status.details.$multiplier) and offset \(context.status.details.$offset)"
				`),
			},
			expMessage: "Computed using multiplier 2 and offset 5",
			expDetails: map[string]string{
				"result":      "25",
				"displayText": "Result is 25",
			},
			validateCtx: func(t *testing.T, ctx map[string]interface{}) {
				statusCtx := ctx["status"].(map[string]interface{})
				details := statusCtx["details"].(map[string]interface{})

				assert.Equal(t, int64(2), details["$multiplier"])
				assert.Equal(t, int64(5), details["$offset"])
				assert.Equal(t, int64(25), details["result"])
				assert.Equal(t, "Result is 25", details["displayText"])
			},
		},
		"health-check-references-status-details": {
			initialCtx: map[string]interface{}{
				"output": map[string]interface{}{
					"status": map[string]interface{}{
						"replicas":      5,
						"readyReplicas": 3,
					},
				},
			},
			request: StatusRequest{
				Parameter: map[string]interface{}{},
				Details: strings.TrimSpace(`
					replicas: context.output.status.replicas
					readyReplicas: context.output.status.readyReplicas
					percentReady: "\(readyReplicas * 100 / replicas)%"
				`),
				Health: strings.TrimSpace(`
					isHealth: context.status.details.replicas == context.status.details.readyReplicas
				`),
				Custom: strings.TrimSpace(`
					message: "Deployment status: \(context.status.details.percentReady) ready"
				`),
			},
			expMessage: "Deployment status: 60% ready",
			expDetails: map[string]string{
				"replicas":      "5",
				"readyReplicas": "3",
				"percentReady":  "60%",
			},
			validateCtx: func(t *testing.T, ctx map[string]interface{}) {
				statusCtx := ctx["status"].(map[string]interface{})
				assert.Equal(t, false, statusCtx["healthy"])
				details := statusCtx["details"].(map[string]interface{})
				assert.Equal(t, int64(5), details["replicas"])
				assert.Equal(t, int64(3), details["readyReplicas"])
			},
		},
		"message-references-health-and-details": {
			initialCtx: map[string]interface{}{
				"output": map[string]interface{}{
					"status": map[string]interface{}{
						"phase":         "Running",
						"replicas":      3,
						"readyReplicas": 3,
					},
				},
			},
			request: StatusRequest{
				Parameter: map[string]interface{}{},
				Details: strings.TrimSpace(`
					phase: context.output.status.phase
					replicas: context.output.status.replicas
					readyReplicas: context.output.status.readyReplicas
				`),
				Health: strings.TrimSpace(`
					isHealth: context.status.details.phase == "Running" && context.status.details.readyReplicas == context.status.details.replicas
				`),
				Custom: strings.TrimSpace(`
					if context.status.healthy {
						message: "Deployment is healthy: \(context.status.details.readyReplicas)/\(context.status.details.replicas) replicas ready"
					}
					if !context.status.healthy {
						message: "Deployment unhealthy: \(context.status.details.readyReplicas)/\(context.status.details.replicas) replicas ready"
					}
				`),
			},
			expMessage: "Deployment is healthy: 3/3 replicas ready",
			expDetails: map[string]string{
				"phase":         "Running",
				"replicas":      "3",
				"readyReplicas": "3",
			},
			validateCtx: func(t *testing.T, ctx map[string]interface{}) {
				statusCtx := ctx["status"].(map[string]interface{})
				assert.Equal(t, true, statusCtx["healthy"])
			},
		},
		"complex-health-with-computed-details": {
			initialCtx: map[string]interface{}{
				"output": map[string]interface{}{
					"status": map[string]interface{}{
						"capacity":  100,
						"used":      85,
						"threshold": 80,
					},
				},
			},
			request: StatusRequest{
				Parameter: map[string]interface{}{},
				Details: strings.TrimSpace(`
					capacity: context.output.status.capacity
					used: context.output.status.used
					threshold: context.output.status.threshold
					$utilization: used * 100.0 / capacity
					utilizationPercent: "\($utilization)%"
					$overThreshold: $utilization > threshold
				`),
				Health: strings.TrimSpace(`
					isHealth: !context.status.details.$overThreshold
				`),
				Custom: strings.TrimSpace(`
					if context.status.healthy {
						message: "Resource usage OK at \(context.status.details.utilizationPercent)"
					}
					if !context.status.healthy {
						message: "Resource usage HIGH at \(context.status.details.utilizationPercent) (threshold: \(context.status.details.threshold)%)"
					}
				`),
			},
			expMessage: "Resource usage HIGH at 85.0% (threshold: 80%)",
			expDetails: map[string]string{
				"capacity":           "100",
				"used":               "85",
				"threshold":          "80",
				"utilizationPercent": "85.0%",
			},
			validateCtx: func(t *testing.T, ctx map[string]interface{}) {
				statusCtx := ctx["status"].(map[string]interface{})
				assert.Equal(t, false, statusCtx["healthy"])
				details := statusCtx["details"].(map[string]interface{})
				assert.Equal(t, float64(85), details["$utilization"])
				assert.Equal(t, true, details["$overThreshold"])
			},
		},
	}

	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			ctx := make(map[string]interface{})
			for k, v := range tc.initialCtx {
				ctx[k] = v
			}

			result, err := GetStatus(ctx, &tc.request)
			assert.NoError(t, err)
			assert.Equal(t, tc.expMessage, result.Message)
			assert.Equal(t, tc.expDetails, result.Details)

			if tc.validateCtx != nil {
				tc.validateCtx(t, ctx)
			}
		})
	}
}

func TestGetStatusWithDefinitionAndHiddenLabels(t *testing.T) {
	testCases := []struct {
		name            string
		templateContext map[string]interface{}
		statusFields    string
		wantNoErr       bool
		description     string
	}{
		{
			name: "handles definition labels without panic",
			templateContext: map[string]interface{}{
				"output": map[string]interface{}{
					"metadata": map[string]interface{}{
						"name": "test",
					},
				},
			},
			statusFields: `
#SomeDefinition: {
	name: string
	type: string
}

status: #SomeDefinition & {
	name: "test"
	type: "healthy"
}
`,
			wantNoErr:   true,
			description: "Should handle definition labels (#SomeDefinition) without panicking",
		},
		{
			name: "handles hidden labels without panic",
			templateContext: map[string]interface{}{
				"output": map[string]interface{}{
					"metadata": map[string]interface{}{
						"name": "test",
					},
				},
			},
			statusFields: `
_hiddenField: "internal"

status: {
	name: "test"
	internal: _hiddenField
}
`,
			wantNoErr:   true,
			description: "Should handle hidden labels (_hiddenField) without panicking",
		},
		{
			name: "handles pattern labels without panic",
			templateContext: map[string]interface{}{
				"output": map[string]interface{}{
					"metadata": map[string]interface{}{
						"name": "test",
					},
				},
			},
			statusFields: `
[string]: _

status: {
	name: "test"
	healthy: true
}
`,
			wantNoErr:   true,
			description: "Should handle pattern labels ([string]: _) without panicking",
		},
		{
			name: "handles mixed label types without panic",
			templateContext: map[string]interface{}{
				"output": map[string]interface{}{
					"metadata": map[string]interface{}{
						"name": "test",
					},
				},
			},
			statusFields: `
#Definition: {
	field: string
}

_hidden: "value"

normalField: "visible"

status: {
	name: normalField
	type: _hidden
	def: #Definition & {field: "test"}
}
`,
			wantNoErr:   true,
			description: "Should handle mixed label types without panicking",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			request := &StatusRequest{
				Details:   tc.statusFields,
				Parameter: map[string]interface{}{},
			}

			// This should not panic even with definition or hidden labels
			result, err := GetStatus(tc.templateContext, request)

			if tc.wantNoErr {
				// We expect no panic and a valid result
				assert.NotNil(t, result, tc.description)
				// The function may return an error for invalid CUE, but it shouldn't panic
				if err != nil {
					t.Logf("Got expected error (non-panic): %v", err)
				}
			} else {
				assert.Error(t, err, tc.description)
			}
		})
	}
}

func TestGetStatusMapWithComplexSelectors(t *testing.T) {
	// Test that getStatusMap doesn't panic with various selector types
	testCases := []struct {
		name            string
		statusFields    string
		templateContext map[string]interface{}
		shouldNotPanic  bool
	}{
		{
			name: "definition selector in context",
			statusFields: `
#Config: {
	enabled: bool
}

config: #Config & {
	enabled: true
}
`,
			templateContext: map[string]interface{}{},
			shouldNotPanic:  true,
		},
		{
			name: "hidden field selector",
			statusFields: `
_internal: {
	secret: "hidden"
}

public: _internal.secret
`,
			templateContext: map[string]interface{}{},
			shouldNotPanic:  true,
		},
		{
			name: "optional field selector",
			statusFields: `
optional?: string

required: string | *"default"
`,
			templateContext: map[string]interface{}{},
			shouldNotPanic:  true,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			if tc.shouldNotPanic {
				// The function should not panic
				assert.NotPanics(t, func() {
					_, _, _ = getStatusMap(tc.templateContext, tc.statusFields, nil)
				}, "getStatusMap should not panic with %s", tc.name)
			}
		})
	}
}
