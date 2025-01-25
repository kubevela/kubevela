import (
	"vela/op"
)

"build-source-code": {
	alias: ""
	annotations: {}
	attributes: {}
	description: "Init storage, clone git repo, build source code and persist generated folder in pvc"
	labels: {}
	type: "workflow-step"
}

template: {
	pvc: op.#Apply & {
		value: {
			apiVersion: "v1"
			kind:       "PersistentVolumeClaim"
			metadata: {
				name:      parameter.pvc.name
				namespace: context.namespace
			}
			spec: {
				storageClassName: parameter.pvc.storageClassName
				accessModes: ["ReadWriteOnce"]
				resources: requests: storage: "1Gi"
			}
		}
	}

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
								image: parameter.image
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
									git clone -b \(parameter.repo.branch) \(parameter.repo.url) repo/
									cd repo/
									\(parameter.cmd)
									generated_dir=`ls -t .|head -n1|awk '{print $0}'`
									cp -r $generated_dir/* \(parameter.pvc.mountPath)
									""",
								]
							},
						]
						volumes: [
							{
								name: "\(context.name)-\(context.stepName)-\(context.stepSessionID)-volume"
								persistentVolumeClaim: claimName: pvc.value.metadata.name
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
		// +usage=Specify the base image where source code to be built
		image: string

		// +usage=Specify the commands to build source code
		cmd: string

		// +usage=Specify the git repo for your source code
		repo: {
			// +usage=Specify the url
			url: string
			// +usage=Specify the branch, default to master
			branch: *"master" | string
		}

		// +usage=Specify the pvc to create for generated files storage
		pvc: {
			// +usage=Specify the pvc name
			name: string
			// +usage=Specify which storageclass that pvc will use
			storageClassName: string
			// +usage=Specify the pvc mountPath
			mountPath: string
		}
	}
}
