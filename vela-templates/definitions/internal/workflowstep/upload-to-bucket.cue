import (
	"vela/op"
)

"upload-to-bucket": {
	alias: ""
	annotations: {}
	attributes: {}
	description: "Get folder from pvc, upload it to object storage bucket"
	labels: {}
	type: "workflow-step"
}

template: {
	job: op.#Apply & {
		value: {
			apiVersion: "batch/v1"
			kind:       "Job"
			metadata: {
				name:      "\(context.name)-\(context.stepName)-\(context.stepSessionID)-job"
				namespace: context.namespace
			}
			spec: {
				template: {
					metadata: {
						labels: {
							"workflow.oam.dev/step-name": "\(context.name)-\(context.stepName)"
						}
					}
					spec: {
						containers: [
							{
								name:  "\(context.name)-\(context.stepName)-\(context.stepSessionID)-container"
								image: "alpine:latest"
								volumeMounts: [
									{
										name:      "\(context.name)-\(context.stepName)-\(context.stepSessionID)-volume"
										mountPath: parameter.pvc.mountPath
									},
								]
								command: [
									"sh",
									"-c",
									"""
									apk update && apk add sudo curl unzip bash
									sudo -v ; curl https://gosspublic.alicdn.com/ossutil/install.sh | sudo bash
									ossutil config -e \(parameter.oss.endpoint) -i \(parameter.oss.accessKey.id) -k \(parameter.oss.accessKey.secret)
									ossutil rm oss://\(parameter.oss.bucket) -rf
									ossutil cp -r \(parameter.pvc.mountPath) oss://\(parameter.oss.bucket) -f
									""",
								]
							},
						]
						volumes: [
							{
								name: "\(context.name)-\(context.stepName)-\(context.stepSessionID)-volume"
								persistentVolumeClaim: claimName: parameter.pvc.name
							},
						]
						restartPolicy: "OnFailure"
					}
				}
			}
		}
	}

	log: op.#Log & {
		source: {
			resources: [{labelSelector: {
				"workflow.oam.dev/step-name": "\(context.name)-\(context.stepName)"
			}}]
		}
	}

	wait: op.#ConditionalWait & {
		continue: job.value.status != _|_ && job.value.status.phase == "Succeeded"
	}

	parameter: {
		// +usage=Specify the pvc to get generated files
		pvc: {
			// +usage=Specify the pvc name
			name: string
			// +usage=Specify the pvc mountPath
			mountPath: string
		}

		// +usage=Specify the alibaba oss bucket as backend
		oss: {
			// +usage=Specify the credentials to access alibaba oss
			accessKey: {
				id:     string
				secret: string
			}
			// +usage=Specify the target oss bucket
			bucket: string
			// +usage=Specify the target oss endpoint
			endpoint: string
		}
	}
}
