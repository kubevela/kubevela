import "vela/test"

_cron: {
	definition: "cron-task"
	context: {name: "report", appName: "shop"}
}

"renders a CronJob with the default policies": test.#ComponentRender & _cron & {
	parameter: {image: "shop-report:1.0", schedule: "0 2 * * *"}
	expect: {
		output: {
			apiVersion: "batch/v1"
			kind:       "CronJob"
			spec: {
				schedule:                   "0 2 * * *"
				concurrencyPolicy:          "Allow"
				suspend:                    false
				successfulJobsHistoryLimit: 3
				failedJobsHistoryLimit:     1
				startingDeadlineSeconds?:   _|_
				jobTemplate: {
					metadata: {
						labels: {"app.oam.dev/name": "shop", "app.oam.dev/component": "report"} @exact()
						annotations?: _|_
					}
					spec: {
						parallelism:              1
						completions:              1
						backoffLimit:             6
						ttlSecondsAfterFinished?: _|_
						activeDeadlineSeconds?:   _|_
						template: {
							metadata: {
								labels: {"app.oam.dev/name": "shop", "app.oam.dev/component": "report"} @exact()
							}
							spec: {
								restartPolicy: "Never"
								containers: [{name: "report", image: "shop-report:1.0", resources?: _|_, volumeMounts?: _|_}]
								volumes?:          _|_
								imagePullSecrets?: _|_
								hostAliases?:      _|_
							}
						}
					}
				}
			}
		}
		outputs: {} @exact()
	}
}

"a cluster older than 1.25 gets the v1beta1 CronJob": test.#ComponentRender & _cron & {
	context: clusterVersion: {major: "1", minor: 24, gitVersion: "v1.24.17", platform: "linux/amd64"}
	parameter: {image: "shop-report:1.0", schedule: "0 2 * * *"}
	expect: output: {apiVersion: "batch/v1beta1", kind: "CronJob"}
}

"a schedule is required": test.#ComponentRender & _cron & {
	parameter: image: "shop-report:1.0"
	expect: error: parameter: [=~"schedule"]
}

"an image is required": test.#ComponentRender & _cron & {
	parameter: schedule: "0 2 * * *"
	expect: error: parameter: [=~"image"]
}

"scheduling and history policies pass through": test.#ComponentRender & _cron & {
	parameter: {
		image:                      "shop-report:1.0"
		schedule:                   "*/15 * * * *"
		concurrencyPolicy:          "Forbid"
		suspend:                    true
		startingDeadlineSeconds:    120
		successfulJobsHistoryLimit: 5
		failedJobsHistoryLimit:     2
	}
	expect: output: spec: {
		schedule:                   "*/15 * * * *"
		concurrencyPolicy:          "Forbid"
		suspend:                    true
		startingDeadlineSeconds:    120
		successfulJobsHistoryLimit: 5
		failedJobsHistoryLimit:     2
	}
}

"job count, deadlines, retries and restart policy": test.#ComponentRender & _cron & {
	parameter: {
		image:                   "shop-report:1.0"
		schedule:                "0 2 * * *"
		count:                   2
		ttlSecondsAfterFinished: 3600
		activeDeadlineSeconds:   600
		backoffLimit:            1
		restart:                 "OnFailure"
	}
	expect: output: spec: jobTemplate: spec: {
		parallelism:             2
		completions:             2
		ttlSecondsAfterFinished: 3600
		activeDeadlineSeconds:   600
		backoffLimit:            1
		template: spec: restartPolicy: "OnFailure"
	}
}

"labels and annotations go on both the job and its pods": test.#ComponentRender & _cron & {
	parameter: {
		image:    "shop-report:1.0"
		schedule: "0 2 * * *"
		labels: team:       "finance"
		annotations: owner: "finance"
	}
	expect: output: spec: jobTemplate: {
		metadata: {
			labels: {team: "finance", "app.oam.dev/component": "report"}
			annotations: owner: "finance"
		}
		spec: template: metadata: {
			labels: {team: "finance", "app.oam.dev/component": "report"}
			annotations: owner: "finance"
		}
	}
}

"command, environment, pull policy and host aliases": test.#ComponentRender & _cron & {
	parameter: {
		image:           "shop-report:1.0"
		schedule:        "0 2 * * *"
		imagePullPolicy: "Always"
		imagePullSecrets: ["registry"]
		cmd: ["/report", "--daily"]
		env: [{name: "TZ", value: "Europe/Dublin"}]
		hostAliases: [{ip: "10.0.0.1", hostnames: ["smtp.local"]}]
	}
	expect: output: spec: jobTemplate: spec: template: spec: {
		imagePullSecrets: [{name: "registry"}]
		hostAliases: [{ip: "10.0.0.1", hostnames: ["smtp.local"]}]
		containers: [{
			imagePullPolicy: "Always"
			command: ["/report", "--daily"]
			env: [{name: "TZ", value: "Europe/Dublin"}]
		}]
	}
}

"cpu and memory are both request and limit": test.#ComponentRender & _cron & {
	parameter: {image: "shop-report:1.0", schedule: "0 2 * * *", cpu: "500m", memory: "256Mi"}
	expect: output: spec: jobTemplate: spec: template: spec: containers: [{resources: {
		requests: {cpu: "500m", memory: "256Mi"}
		limits: {cpu: "500m", memory: "256Mi"}
	}}]
}

"volumeMounts mount each kind with a subPath, a volume shared by name once": test.#ComponentRender & _cron & {
	parameter: {
		image:    "shop-report:1.0"
		schedule: "0 2 * * *"
		volumeMounts: {
			pvc: [{name: "data", mountPath: "/data", subPath: "reports", claimName: "shop-data"}]
			configMap: [{name: "conf", mountPath: "/etc/report", cmName: "report-conf"}]
			secret: [{name: "creds", mountPath: "/etc/creds", secretName: "report-creds"}]
			emptyDir: [
				{name: "tmp", mountPath: "/tmp"},
				{name: "tmp", mountPath: "/var/tmp", subPath: "var"},
			]
			hostPath: [{name: "logs", mountPath: "/logs", path: "/var/log/report"}]
		}
	}
	expect: output: spec: jobTemplate: spec: template: spec: {
		containers: [{volumeMounts: [
			{name: "data", mountPath: "/data", subPath: "reports"},
			{name: "conf", mountPath: "/etc/report", subPath?: _|_},
			{name: "creds", mountPath: "/etc/creds"},
			{name: "tmp", mountPath: "/tmp"},
			{name: "tmp", mountPath: "/var/tmp", subPath: "var"},
			{name: "logs", mountPath: "/logs"},
		]}]
		volumes: [
			{name: "data", persistentVolumeClaim: claimName: "shop-data"},
			{name: "conf", configMap: {name: "report-conf", defaultMode: 420}},
			{name: "creds", secret: {secretName: "report-creds", defaultMode: 420}},
			{name: "tmp", emptyDir: medium: ""},
			{name: "logs", hostPath: path: "/var/log/report"},
		]
	}
}

"the deprecated volumes still mount": test.#ComponentRender & _cron & {
	parameter: {
		image:    "shop-report:1.0"
		schedule: "0 2 * * *"
		volumes: [{name: "creds", mountPath: "/etc/creds", type: "secret", secretName: "report-creds"}]
	}
	expect: output: spec: jobTemplate: spec: template: spec: {
		containers: [{volumeMounts: [{name: "creds", mountPath: "/etc/creds"}]}]
		volumes: [{name: "creds", secret: secretName: "report-creds"}]
	}
}

"probes pass through": test.#ComponentRender & _cron & {
	parameter: {
		image:    "shop-report:1.0"
		schedule: "0 2 * * *"
		livenessProbe: exec: command: ["cat", "/alive"]
		readinessProbe: tcpSocket: port: 8080
	}
	expect: output: spec: jobTemplate: spec: template: spec: containers: [{
		livenessProbe: exec: command: ["cat", "/alive"]
		readinessProbe: tcpSocket: port: 8080
	}]
}

"every probe field passes through": test.#ComponentRender & _cron & {
	parameter: {
		image:    "shop-report:1.0"
		schedule: "0 2 * * *"
		livenessProbe: {
			httpGet: {path: "/healthz", port: 8080}
			initialDelaySeconds: 5
			periodSeconds:       20
			timeoutSeconds:      2
			successThreshold:    1
			failureThreshold:    6
		}
		readinessProbe: {
			exec: command: ["cat", "/alive"]
			initialDelaySeconds: 3
			periodSeconds:       15
			timeoutSeconds:      4
			successThreshold:    2
			failureThreshold:    5
		}
	}
	expect: output: spec: jobTemplate: spec: template: spec: containers: [{
		livenessProbe: {
			httpGet: {path: "/healthz", port: 8080}
			initialDelaySeconds: 5
			periodSeconds:       20
			timeoutSeconds:      2
			successThreshold:    1
			failureThreshold:    6
		}
		readinessProbe: {
			exec: command: ["cat", "/alive"]
			initialDelaySeconds: 3
			periodSeconds:       15
			timeoutSeconds:      4
			successThreshold:    2
			failureThreshold:    5
		}
	}]
}

"a tcpSocket liveness and an httpGet readiness probe pass through": test.#ComponentRender & _cron & {
	parameter: {
		image:    "shop-report:1.0"
		schedule: "0 2 * * *"
		livenessProbe: tcpSocket: port: 8080
		readinessProbe: httpGet: {path: "/ready", port: 8080}
	}
	expect: output: spec: jobTemplate: spec: template: spec: containers: [{
		livenessProbe: tcpSocket: port: 8080
		readinessProbe: httpGet: {path: "/ready", port: 8080}
	}]
}

"environment can come from a Secret or a ConfigMap": test.#ComponentRender & _cron & {
	parameter: {
		image:    "shop-report:1.0"
		schedule: "0 2 * * *"
		env: [
			{name: "PASSWORD", valueFrom: secretKeyRef: {name: "shop-db", key: "password"}},
			{name: "MODE", valueFrom: configMapKeyRef: {name: "shop-conf", key: "mode"}},
		]
	}
	expect: output: spec: jobTemplate: spec: template: spec: containers: [{env: [
		{name: "PASSWORD", valueFrom: secretKeyRef: {name: "shop-db", key: "password"}},
		{name: "MODE", valueFrom: configMapKeyRef: {name: "shop-conf", key: "mode"}},
	]}]
}

"a deprecated emptyDir volume keeps its medium": test.#ComponentRender & _cron & {
	parameter: {
		image:    "shop-report:1.0"
		schedule: "0 2 * * *"
		volumes: [{name: "cache", mountPath: "/cache", type: "emptyDir", medium: "Memory"}]
	}
	expect: output: spec: jobTemplate: spec: template: spec: {
		containers: [{volumeMounts: [{name: "cache", mountPath: "/cache"}]}]
		volumes: [{name: "cache", emptyDir: medium: "Memory"}]
	}
}
