import (
	"encoding/json"
)

nocalhost: {
	type: "trait"
	annotations: {}
	labels: {
		"ui-hidden": "true"
	}
	description: "nocalhost develop configuration."
	attributes: {
		podDisruptive: true
		appliesToWorkloads: ["deployments.apps", "statefulsets.apps", "daemonsets.apps", "jobs.batch"]
	}
}

template: {
	outputs: {
		nocalhostService: {
			apiVersion: "v1"
			kind:       "Service"
			metadata: name: context.name
			spec: {
				selector: "app.oam.dev/component": context.name
				ports: [
					{
						port:       parameter.port
						targetPort: parameter.port
					},
				]
				type: "ClusterIP"
			}
		}
	}

	patch: {
		metadata: annotations: {
			"dev.nocalhost/application-name":      context.appName
			"dev.nocalhost/application-namespace": context.namespace
			"dev.nocalhost":                       json.Marshal({
				name:        context.name
				serviceType: parameter.serviceType
				"containers": [
					{
						"name": context.name
						"dev": {
							if parameter.gitUrl != _|_ {
								"gitUrl": parameter.gitUrl
							}
							if parameter.image == "go" {
								"image": "nocalhost-docker.pkg.coding.net/nocalhost/dev-images/golang:latest"
							}
							if parameter.image == "java" {
								"image": "nocalhost-docker.pkg.coding.net/nocalhost/dev-images/java:latest"
							}
							if parameter.image == "python" {
								"image": "nocalhost-docker.pkg.coding.net/nocalhost/dev-images/python:latest"
							}
							if parameter.image == "node" {
								"image": "nocalhost-docker.pkg.coding.net/nocalhost/dev-images/node:latest"
							}
							if parameter.image == "ruby" {
								"image": "nocalhost-docker.pkg.coding.net/nocalhost/dev-images/ruby:latest"
							}
							if parameter.image != "go" && parameter.image != "java" && parameter.image != "python" && parameter.image != "node" && parameter.image != "ruby" {
								"image": parameter.image
							}
							"shell":   parameter.shell
							"workDir": parameter.workDir
							if parameter.storageClass != _|_ {
								"storageClass": parameter.storageClass
							}
							"resources": {
								"limits":   parameter.resources.limits
								"requests": parameter.resources.requests
							}
							if parameter.persistentVolumeDirs != _|_ {
								persistentVolumeDirs: [
									for v in parameter.persistentVolumeDirs {
										path:     v.path
										capacity: v.capacity
									},
								]
							}
							if parameter.command != _|_ {
								"command": parameter.command
							}
							if parameter.debug != _|_ {
								"debug": parameter.debug
							}
							"hotReload": parameter.hotReload
							if parameter.sync != _|_ {
								sync: parameter.sync
							}
							if parameter.env != _|_ {
								env: [
									for v in parameter.env {
										name:  v.name
										value: v.value
									},
								]
							}
							if parameter.portForward != _|_ {
								"portForward": parameter.portForward
							}
							if parameter.portForward == _|_ {
								"portForward": ["\(parameter.port):\(parameter.port)"]
							}
						}
					},
				]
			})
		}
	}
	language: "go" | "java" | "python" | "node" | "ruby"
	parameter: {
		port:          int
		serviceType:   *"deployment" | string
		gitUrl?:       string
		image:         language | string
		shell:         *"bash" | string
		workDir:       *"/home/nocalhost-dev" | string
		storageClass?: string
		command: {
			run:   *["sh", "run.sh"] | [...string]
			debug: *["sh", "debug.sh"] | [...string]
		}
		debug?: {
			remoteDebugPort?: int
		}
		hotReload: *true | bool
		sync: {
			type:              *"send" | string
			filePattern:       *["./"] | [...string]
			ignoreFilePattern: *[".git", ".vscode", ".idea", ".gradle", "build"] | [...string]
		}
		env?: [...{
			name:  string
			value: string
		}]
		portForward?: [...string]
		persistentVolumeDirs?: [...{
			path:     string
			capacity: string
		}]
		resources: {
			limits: {
				memory: *"2Gi" | string
				cpu:    *"2" | string
			}
			requests: {
				memory: *"512Mi" | string
				cpu:    *"0.5" | string
			}
		}
	}
}
