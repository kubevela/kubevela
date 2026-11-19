import (
	"vela/test"
	"vela/kube"
)

platform: kube.#Apply & {$params: value: {apiVersion: "v1", kind: "Namespace", metadata: name: "platform"}} @before()

shared: test.#Seed & {$params: objects: [
	{apiVersion: "v1", kind: "ConfigMap", metadata: {name: "shared", namespace: "platform"}, data: tier: "gold"},
	{apiVersion: "v1", kind: "ConfigMap", metadata: name: "unscoped"},
]} @before()

// Created, not applied: the second case's copy succeeds because what a
// hook creates is deleted when its scope ends, here after each case.
seed: {
	read: kube.#Read & {$params: value: {apiVersion: "v1", kind: "ConfigMap", metadata: {name: "shared", namespace: "platform"}}}
	copy: test.#Seed & {$params: objects: [{
		apiVersion: "v1"
		kind:       "ConfigMap"
		metadata: {name: "per-case", namespace: "platform"}
		data: read.$returns.value.data
	}]}
	local: test.#Seed & {$params: objects: [{apiVersion: "v1", kind: "ConfigMap", metadata: name: "local"}]}
} @beforeEach()

// Runs after each case, before cleanup deletes what the case created; the
// step's ConfigMap would go either way.
tidy: kube.#Delete & {$params: value: {apiVersion: "v1", kind: "ConfigMap", metadata: {name: "mine", namespace: "platform"}}} @afterEach()

report: kube.#Apply & {$params: value: {apiVersion: "v1", kind: "ConfigMap", metadata: {name: "report", namespace: "default"}}} @after()

_seen: [
	{apiVersion: "v1", kind: "ConfigMap", metadata: {name: "shared", namespace: "platform"}, data: tier: "gold"},
	{apiVersion: "v1", kind: "ConfigMap", metadata: {name: "unscoped", namespace: "default"}},
	{apiVersion: "v1", kind: "ConfigMap", metadata: {name: "per-case", namespace: "platform"}, data: tier: "gold"},
	{apiVersion: "v1", kind: "ConfigMap", metadata: name: "local"},
	{apiVersion: "v1", kind: "ConfigMap", metadata: {name: "mine", namespace: "platform"}},
]

"sees what the hooks made": test.#WorkflowStepExec & {
	definition: "../steps/publish"
	context: namespace: "platform"
	parameter: {name: "mine", data: {}}
	expect: {
		phase: "succeeded"
		// Hooks' calls are setup, not the step's.
		calls: "vela/kube": "#Apply": [_]
		resources: _seen
	}
}

"runs each hook again": test.#WorkflowStepExec & {
	definition: "../steps/publish"
	context: namespace: "platform"
	parameter: {name: "mine", data: {}}
	expect: resources: _seen
}
