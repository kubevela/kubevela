import (
	"vela/kube"
	"vela/builtin"
	"vela/util"
	"strings"
)

"vela-cli": {
	type: "workflow-step"
	annotations: {
		"category": "Scripts & Commands"
	}
	labels: {}
	description: "Run a vela command"
}
template: {
	mountsArray: [
		if parameter.storage != _|_ && parameter.storage.secret != _|_ for v in parameter.storage.secret {
			{
				name:      "secret-" + v.name
				mountPath: v.mountPath
				if v.subPath != _|_ {
					subPath: v.subPath
				}
			}
		},
		if parameter.storage != _|_ && parameter.storage.hostPath != _|_ for v in parameter.storage.hostPath {
			{
				name:      "hostpath-" + v.name
				mountPath: v.mountPath
			}
		},
	]

	volumesList: [
		if parameter.storage != _|_ && parameter.storage.secret != _|_ for v in parameter.storage.secret {
			{
				name: "secret-" + v.name
				secret: {
					defaultMode: v.defaultMode
					secretName:  v.secretName
					if v.items != _|_ {
						items: v.items
					}
				}
			}
		},
		if parameter.storage != _|_ && parameter.storage.hostPath != _|_ for v in parameter.storage.hostPath {
			{
				name: "hostpath-" + v.name
				hostPath: {
					path: v.path
					type: v.type
				}
			}
		},
	]

	deDupVolumesArray: [
		for val in [
			for i, vi in volumesList {
				for j, vj in volumesList if j < i && vi.name == vj.name {
					_ignore: true
				}
				vi
			},
		] if val._ignore == _|_ {
			val
		},
	]

	// The Job runs as this account in the Application's namespace. Without it the Job creates no
	// pod and never finishes, so the step fails at once instead. Any other read error (a requester
	// who may not get service accounts) leaves the check to the Job itself.
	account: kube.#Read & {
		$params: value: {
			apiVersion: "v1"
			kind:       "ServiceAccount"
			metadata: {
				name:      parameter.serviceAccountName
				namespace: context.namespace
			}
		}
	}
	noAccount: {
		if account.$returns.err != _|_ if strings.Contains(account.$returns.err, "not found") {
			breakWorkflow: builtin.#Fail & {
				$params: message: "service account \(parameter.serviceAccountName) not found in namespace \(context.namespace): the vela-cli Job runs as it"
			}
		}
	}

	job: kube.#Apply & {
		$params: {
			value: {
				apiVersion: "batch/v1"
				kind:       "Job"
				metadata: {
					name: "\(context.name)-\(context.stepName)-\(context.stepSessionID)"
					// The Application's namespace, whatever service account is named: the Job
					// runs with what an account of that name there may do.
					namespace: context.namespace
				}
				spec: {
					backoffLimit: 3
					template: {
						metadata: {
							labels: {
								"workflow.oam.dev/step-name": "\(context.name)-\(context.stepName)"
							}
						}
						spec: {
							containers: [
								{
									name:         "\(context.name)-\(context.stepName)-\(context.stepSessionID)-job"
									image:        parameter.image
									command:      parameter.command
									volumeMounts: mountsArray
								},
							]
							restartPolicy:      "Never"
							serviceAccountName: parameter.serviceAccountName
							volumes:            deDupVolumesArray
						}
					}
				}
			}
		}
	}

	log: util.#Log & {
		$params: {
			source: {
				resources: [{labelSelector: {
					"workflow.oam.dev/step-name": "\(context.name)-\(context.stepName)"
				}}]
			}
		}
	}

	fail: {
		if job.$returns.value.status != _|_ if job.$returns.value.status.failed != _|_ {
			if job.$returns.value.status.failed > 2 {
				breakWorkflow: builtin.#Fail & {
					$params: message: "failed to execute vela command"
				}
			}
		}
	}

	wait: builtin.#ConditionalWait & {
		if job.$returns.value.status != _|_ if job.$returns.value.status.succeeded != _|_ {
			$params: continue: job.$returns.value.status.succeeded > 0
		}
	}

	parameter: {
		// +usage=Specify the vela command
		command: [...string]
		// +usage=Specify the image
		image: *"oamdev/vela-cli:v1.12.0-alpha.2" | string
		// +usage=The service account the Job runs as, in the Application's namespace
		serviceAccountName: *"default" | string
		storage?: {
			// +usage=Mount Secret type storage
			secret?: [...{
				name:        string
				mountPath:   string
				subPath?:    string
				defaultMode: *420 | int
				secretName:  string
				items?: [...{
					key:  string
					path: string
					mode: *511 | int
				}]
			}]
			// +usage=Declare host path type storage
			hostPath?: [...{
				name:      string
				path:      string
				mountPath: string
				type:      *"Directory" | "DirectoryOrCreate" | "FileOrCreate" | "File" | "Socket" | "CharDevice" | "BlockDevice"
			}]
		}
	}
}
