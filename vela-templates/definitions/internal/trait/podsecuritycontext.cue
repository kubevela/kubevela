podsecuritycontext: {
	type: "trait"
	annotations: {}
	description: "Adds security context to the pod spec in path 'spec.template.spec.securityContext'."
	attributes: {
		podDisruptive: true
		appliesToWorkloads: ["deployments.apps", "statefulsets.apps", "daemonsets.apps", "jobs.batch"]
	}
}

template: {
	patch: spec: template: spec: {
		securityContext: {
			if parameter.appArmorProfile != _|_ {
				appArmorProfile: parameter.appArmorProfile
			}
			if parameter.fsGroup != _|_ {
				fsGroup: parameter.fsGroup
			}
			if parameter.runAsGroup != _|_ {
				runAsGroup: parameter.runAsGroup
			}
			if parameter.runAsUser != _|_ {
				runAsUser: parameter.runAsUser
			}
			if parameter.seccompProfile != _|_ {
				seccompProfile: parameter.seccompProfile
			}
			runAsNonRoot: parameter.runAsNonRoot
		}
	}

	parameter: {
		// +usage=Specify the AppArmor profile for the pod
		appArmorProfile?: {
			type:             "RuntimeDefault" | "Unconfined" | "Localhost"
			localhostProfile: string
		}
		fsGroup?:    int
		runAsGroup?: int
		// +usage=Specify the UID to run the entrypoint of the container process
		runAsUser?: int
		// +usage=Specify if the container runs as a non-root user
		runAsNonRoot: *true | bool
		// +usage=Specify the seccomp profile for the pod
		seccompProfile?: {
			type:             "RuntimeDefault" | "Unconfined" | "Localhost"
			localhostProfile: string
		}
	}
}
