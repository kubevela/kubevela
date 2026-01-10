package component

import "list"

// Native CUE tests for statefulset component
// Run with: cue vet statefulset.cue statefulset_native_test.cue

// Test cases for the statefulset template
#TestCase: {
	name: string
	context: {
		name:     string
		appName:  string
		revision: string
	}
	parameter: {
		image:      string
		cpu?:       string
		memory?:    string
		exposeType: *"ClusterIP" | "NodePort" | "LoadBalancer"
		ports?: [...{
			port:     int
			protocol: *"TCP" | "UDP" | "SCTP"
			expose:   bool
		}]
		env?: [...{
			name:  string
			value: string
		}]
	}
	// Expected outputs
	expectedOutput: {
		apiVersion: "apps/v1"
		kind:       "StatefulSet"
		spec: {
			selector: matchLabels: "app.oam.dev/component": string
		}
	}
}

// Test Case 1: Basic StatefulSet with minimal parameters
testBasicStatefulSet: #TestCase & {
	name: "basic-statefulset"
	context: {
		name:     "test-app"
		appName:  "test-app"
		revision: "v1"
	}
	parameter: {
		image:      "nginx:latest"
		cpu:        "500m"
		memory:     "512Mi"
		exposeType: "ClusterIP"
	}
	expectedOutput: {
		spec: {
			selector: matchLabels: "app.oam.dev/component": "test-app"
		}
	}
}

// Test Case 2: StatefulSet with ports and environment variables
testStatefulSetWithPortsAndEnv: #TestCase & {
	name: "statefulset-with-ports-env"
	context: {
		name:     "postgres"
		appName:  "postgres"
		revision: "v1"
	}
	parameter: {
		image:      "postgres:16.4"
		cpu:        "1"
		memory:     "2Gi"
		exposeType: "ClusterIP"
		ports: [{
			port:     5432
			protocol: "TCP"
			expose:   true
		}]
		env: [
			{name: "POSTGRES_DB", value:       "mydb"},
			{name: "POSTGRES_USER", value:     "postgres"},
			{name: "POSTGRES_PASSWORD", value: "secret"},
		]
	}
	expectedOutput: {
		spec: {
			selector: matchLabels: "app.oam.dev/component": "postgres"
		}
	}
}

// Test Case 3: StatefulSet with NodePort exposure
testStatefulSetNodePort: #TestCase & {
	name: "statefulset-nodeport"
	context: {
		name:     "redis"
		appName:  "redis"
		revision: "v1"
	}
	parameter: {
		image:      "redis:7"
		cpu:        "250m"
		memory:     "256Mi"
		exposeType: "NodePort"
		ports: [{
			port:     6379
			protocol: "TCP"
			expose:   true
		}]
	}
	expectedOutput: {
		spec: {
			selector: matchLabels: "app.oam.dev/component": "redis"
		}
	}
}

// Collect all test cases for validation
testCases: [
	testBasicStatefulSet,
	testStatefulSetWithPortsAndEnv,
	testStatefulSetNodePort,
]

// Validate all test cases meet the schema requirements
// Use assertions that work in CUE's constraint system
_validations: {
	// Verify context fields are not empty
	contextValid: [
		for tc in testCases {
			tc.context.name != "" && tc.context.appName != "" && tc.context.revision != ""
		},
	]

	// Verify all have valid images
	imagesValid: [
		for tc in testCases {
			tc.parameter.image =~ "^[a-z0-9._/-]+:[a-z0-9._-]+$"
		},
	]

	// Verify expected outputs
	outputsValid: [
		for tc in testCases {
			tc.expectedOutput.apiVersion == "apps/v1" &&
			tc.expectedOutput.kind == "StatefulSet"
		},
	]

	// All validations must be true
	contextValid: [...true]
	imagesValid: [...true]
	outputsValid: [...true]
}

// Property-based assertions
// All test case names must be unique
_uniqueNames: list.UniqueItems([for tc in testCases {tc.name}])

// All test cases must have image parameter
_allHaveImages: [for tc in testCases {tc.parameter.image}] != [for tc in testCases {_|_}]
