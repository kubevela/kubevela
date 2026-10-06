import "vela/test"

_web: {
	definition: "podsecuritycontext"
	workload: {
		apiVersion: "apps/v1"
		kind:       "Deployment"
		metadata: name: "web"
		spec: template: spec: containers: [{name: "web", image: "shop:1.0"}]
	}
}

"pods run as non-root by default": test.#TraitRender & _web & {
	expect: output: spec: template: spec: {
		securityContext: {runAsNonRoot: true} @exact()
		containers: [{name: "web", image: "shop:1.0", securityContext?: _|_}]
	}
}

"sets the user, group and filesystem group": test.#TraitRender & _web & {
	parameter: {runAsUser: 1000, runAsGroup: 3000, fsGroup: 2000}
	expect: output: spec: template: spec: {
		securityContext: {
			runAsNonRoot: true
			runAsUser:    1000
			runAsGroup:   3000
			fsGroup:      2000
		} @exact()
	}
}

"root can be allowed": test.#TraitRender & _web & {
	parameter: runAsNonRoot: false
	expect: output: spec: template: spec: securityContext: runAsNonRoot: false
}

"sets the seccomp and AppArmor profiles": test.#TraitRender & _web & {
	parameter: {
		seccompProfile: {type: "Localhost", localhostProfile: "profiles/shop.json"}
		appArmorProfile: type: "RuntimeDefault"
	}
	expect: output: spec: template: spec: securityContext: {
		seccompProfile: {type: "Localhost", localhostProfile: "profiles/shop.json"}
		appArmorProfile: {type: "RuntimeDefault"} @exact()
	}
}

"an unknown profile type is rejected": test.#TraitRender & _web & {
	parameter: seccompProfile: type: "Strict"
	expect: error: {
		parameter: [=~"seccompProfile.type"] @contains()
	}
}
