import (
	"vela/op"
	"encoding/json"
	"strings"
)

"build-push-image": {
	alias: ""
	annotations: {
		"definition.oam.dev/example-url": "https://raw.githubusercontent.com/kubevela/workflow/main/examples/workflow-run/built-push-image.yaml"
	}
	attributes: {}
	description: "Build and push image from git url"
	labels: {}
	type: "workflow-step"
}

template: {
	url:    strings.TrimPrefix(strings.TrimPrefix(parameter.git, "https://"), "http://")
	kaniko: op.#Apply & {
		value: {
			apiVersion: "v1"
			kind:       "Pod"
			metadata: {
				name:      "\(context.name)-\(context.stepSessionID)-kaniko"
				namespace: context.namespace
			}
			spec: {
				containers: [
					{
						args: [
							"--dockerfile=\(parameter.dockerfile)",
							"--context=git://\(url)#refs/heads/\(parameter.branch)",
							"--destination=\(parameter.image)",
							"--verbosity=\(parameter.verbosity)",
						]
						image: parameter.kanikoExecutor
						name:  "kaniko"
						if parameter.credentials != _|_ && parameter.credentials.image != _|_ {
							volumeMounts: [
								{
									mountPath: "/kaniko/.docker/"
									name:      parameter.credentials.image.name
								},
							]
						}
						if parameter.credentials != _|_ && parameter.credentials.git != _|_ {
							env: [
								{
									name: "GIT_TOKEN"
									valueFrom: {
										secretKeyRef: {
											key:  parameter.credentials.git.key
											name: parameter.credentials.git.name
										}
									}
								},
							]
						}
					},
				]
				if parameter.credentials != _|_ && parameter.credentials.image != _|_ {
					volumes: [
						{
							name: parameter.credentials.image.name
							secret: {
								defaultMode: 420
								items: [
									{
										key:  parameter.credentials.image.key
										path: "config.json"
									},
								]
								secretName: parameter.credentials.image.name
							}
						},
					]
				}
				restartPolicy: "Never"
			}
		}
	}
	log: op.#Log & {
		source: {
			resources: [{
				name:      "\(context.name)-\(context.stepSessionID)-kaniko"
				namespace: context.namespace
			}]
		}
	}
	read: op.#Read & {
		value: {
			apiVersion: "v1"
			kind:       "Pod"
			metadata: {
				name:      "\(context.name)-\(context.stepSessionID)-kaniko"
				namespace: context.namespace
			}
		}
	}
	wait: op.#ConditionalWait & {
		continue: read.value.status != _|_ && read.value.status.phase == "Succeeded"
	}
	#secret: {
		name: string
		key:  string
	}
	parameter: {
		kanikoExecutor: *"gcr.io/kaniko-project/executor:latest" | string
		git:            string
		branch:         *"master" | string
		dockerfile:     *"./Dockerfile" | string
		image:          string
		credentials?: {
			git?: {
				name: string
				key:  string
			}
			image?: {
				name: string
				key:  *".dockerconfigjson" | string
			}
		}
		verbosity: *"info" | "panic" | "fatal" | "error" | "warn" | "debug" | "trace"
	}
}
