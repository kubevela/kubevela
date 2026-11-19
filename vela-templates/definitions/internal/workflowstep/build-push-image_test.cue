import "vela/test"

// The build runs in a Pod named after the application and the step's session.
_pod: "test-app-test-step-id-kaniko"

_build: {
	context: git: "github.com/acme/shop"
	image: "ghcr.io/acme/shop:v1"
}

// _done is the build Pod as the kubelet would leave it once kaniko exits,
// with the spec the step applies for _build.
_done: {
	_phase: *"Succeeded" | string
	resources: [{
		apiVersion: "v1"
		kind:       "Pod"
		metadata: name: _pod
		spec: {
			containers: [{
				name:  "kaniko"
				image: "oamdev/kaniko-executor:v1.9.1"
				args: [
					"--dockerfile=./Dockerfile",
					"--context=git://github.com/acme/shop#refs/heads/master",
					"--destination=ghcr.io/acme/shop:v1",
					"--verbosity=info",
				]
			}]
			restartPolicy: "Never"
		}
		status: phase: _phase
	}]
}

"runs kaniko in a pod and waits for the build": test.#WorkflowStepExec & {
	definition: "build-push-image"
	parameter:  _build
	expect: {
		phase: "running"
		calls: "vela/util": "#Log": [{$params: source: resources: [{name: _pod}]}]
		resources: [{
			apiVersion: "v1"
			kind:       "Pod"
			metadata: name: _pod
			spec: {
				restartPolicy: "Never"
				containers: [{
					name:  "kaniko"
					image: "oamdev/kaniko-executor:v1.9.1"
					args: [
						"--dockerfile=./Dockerfile",
						"--context=git://github.com/acme/shop#refs/heads/master",
						"--destination=ghcr.io/acme/shop:v1",
						"--verbosity=info",
					]
					env?: _|_
					volumeMounts?: [...{mountPath: !="/kaniko/.docker/"}]
				}]
				volumes?: [...{name: !="registry"}]
			}
		}]
	}
}

"succeeds once the build pod has succeeded": test.#WorkflowStepExec & _done & {
	definition: "build-push-image"
	parameter:  _build
	expect: phase: "succeeded"
}

"keeps waiting while the build pod has not succeeded": test.#WorkflowStepExec & _done & {
	_phase:     "Running"
	definition: "build-push-image"
	parameter:  _build
	expect: phase: "running"
}

"builds the given branch of a git context, trimming a git:// scheme": test.#WorkflowStepExec & {
	definition: "build-push-image"
	parameter: {context: {git: "git://github.com/acme/shop", branch: "release"}, image: "ghcr.io/acme/shop:v1"}
	expect: resources: [{
		apiVersion: "v1"
		kind:       "Pod"
		metadata: name: _pod
		spec: containers: [{args: [_, "--context=git://github.com/acme/shop#refs/heads/release", _, _]}]
	}]
}

"passes any other context to kaniko as it is": test.#WorkflowStepExec & {
	definition: "build-push-image"
	parameter: {context: "s3://builds/shop.tar.gz", image: "ghcr.io/acme/shop:v1"}
	expect: resources: [{
		apiVersion: "v1"
		kind:       "Pod"
		metadata: name: _pod
		spec: containers: [{args: [_, "--context=s3://builds/shop.tar.gz", _, _]}]
	}]
}

"builds with the given executor, dockerfile, platform, build args and verbosity": test.#WorkflowStepExec & {
	definition: "build-push-image"
	parameter: _build & {
		kanikoExecutor: "gcr.io/kaniko-project/executor:v1.23.0"
		dockerfile:     "./build/Dockerfile"
		platform:       "linux/arm64"
		buildArgs: ["VERSION=1.0", "DEBUG=false"]
		verbosity: "debug"
	}
	expect: resources: [{
		apiVersion: "v1"
		kind:       "Pod"
		metadata: name: _pod
		spec: containers: [{
			image: "gcr.io/kaniko-project/executor:v1.23.0"
			args: [
				"--dockerfile=./build/Dockerfile",
				"--context=git://github.com/acme/shop#refs/heads/master",
				"--destination=ghcr.io/acme/shop:v1",
				"--verbosity=debug",
				"--customPlatform=linux/arm64",
				"--build-arg=VERSION=1.0",
				"--build-arg=DEBUG=false",
			]
		}]
	}]
}

"mounts the registry credentials and exposes the git token": test.#WorkflowStepExec & {
	definition: "build-push-image"
	parameter: _build & {credentials: {
		image: name: "registry"
		git: {name: "git-token", key: "token"}
	}}
	expect: resources: [{
		apiVersion: "v1"
		kind:       "Pod"
		metadata: name: _pod
		spec: {
			containers: [{
				volumeMounts: [{mountPath: "/kaniko/.docker/", name: "registry"}, ...]
				env: [{name: "GIT_TOKEN", valueFrom: secretKeyRef: {name: "git-token", key: "token"}}]
			}]
			volumes: [{
				name: "registry"
				secret: {
					secretName:  "registry"
					defaultMode: 420
					items: [{key: ".dockerconfigjson", path: "config.json"}]
				}
			}, ...]
		}
	}]
}

"an image is required": test.#WorkflowStepExec & {
	definition: "build-push-image"
	parameter: context: git: "github.com/acme/shop"
	expect: {
		phase:   "failed"
		message: =~"destination"
		calls: "vela/kube"?: _|_
	}
}
