import (
	"vela/op"
	"encoding/json"
	"strings"
)

"build-push-image": {
	alias: ""
	attributes: {}
	description: "Build and push image from git url"
	annotations: {
		"category": "CI Integration"
	}
	labels: {}
	type: "workflow-step"
}

template: {
	url: {
		if parameter.context.git != _|_ {
			address: strings.TrimPrefix(parameter.context.git, "git://")
			value:   "git://\(address)#refs/heads/\(parameter.context.branch)"
		}
		if parameter.context.git == _|_ {
			value: parameter.context
		}
	}
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
							"--context=\(url.value)",
							"--destination=\(parameter.image)",
							"--verbosity=\(parameter.verbosity)",
							if parameter.platform != _|_ {
								"--customPlatform=\(parameter.platform)"
							},
							if parameter.buildArgs != _|_ for arg in parameter.buildArgs {
								"--build-arg=\(arg)"
							},
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
	#git: {
		git:    string
		branch: *"master" | string
	}
	parameter: {
		// +usage=Specify the kaniko executor image, default to oamdev/kaniko-executor:v1.9.1
		kanikoExecutor: *"oamdev/kaniko-executor:v1.9.1" | string
		// +usage=Specify the context to build image, you can use context with git and branch or directly specify the context, please refer to https://github.com/GoogleContainerTools/kaniko#kaniko-build-contexts
		context: #git | string
		// +usage=Specify the dockerfile
		dockerfile: *"./Dockerfile" | string
		// +usage=Specify the image
		image: string
		// +usage=Specify the platform to build
		platform?: string
		// +usage=Specify the build args
		buildArgs?: [...string]
		// +usage=Specify the credentials to access git and image registry
		credentials?: {
			// +usage=Specify the credentials to access git
			git?: {
				// +usage=Specify the secret name
				name: string
				// +usage=Specify the secret key
				key: string
			}
			// +usage=Specify the credentials to access image registry
			image?: {
				// +usage=Specify the secret name
				name: string
				// +usage=Specify the secret key
				key: *".dockerconfigjson" | string
			}
		}
		// +usage=Specify the verbosity level
		verbosity: *"info" | "panic" | "fatal" | "error" | "warn" | "debug" | "trace"
	}
}
