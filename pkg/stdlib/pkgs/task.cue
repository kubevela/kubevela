
let defaultToolImage = "gcr.io/tekton-releases/github.com/tektoncd/pipeline/cmd/entrypoint:v0.27.2"
let defaultBaseImage = "gcr.io/distroless/base@sha256:aa4fd987555ea10e1a4ec8765da8158b5ffdfef1e72da512c7ede509bc9966c4"

#Task: {
	#do:       "steps"
	name:      string
	namespace: string
	workspaces: [_name_=string]: #workspace & {name: "\(_name_)"}
	secrets: [_name_=string]:    #secret & {name:    "\(_name_)"}
	steps: [...#Script]

	toolImage: *defaultToolImage | string
	baseImage: *defaultBaseImage | string

	generate_scripts_: [ for i, x in steps {
		"""
        scriptfile="/vela/scripts/script-\(i)"
        touch ${scriptfile} && chmod +x ${scriptfile}
        cat > ${scriptfile} << _EOF_
        \(base64.Encode(null, x.script))
        _EOF_
        /vela/tools/entrypoint decode-script ${scriptfile}

        """
	}]

	name_:      name
	namespace_: namespace

	apply: #Apply & {
		value: #PodTask & {
			_settings: {
				scripts_:    strings.Join(generate_scripts_, "")
				workspaces_: workspaces
				secrets_:    secrets
				toolImage_:  toolImage
				baseImage_:  baseImage
			}

			metadata: {
				name:      name_
				namespace: namespace_
			}
			spec: containers: [ for i, step in steps {
				#StepContainer
				name:  step.name
				image: step.image
				env:   step.envs

				_settings: {
					workspaceMounts_: step.workspaceMounts
					secretMounts_:    step.secretMounts
					index_:           i
				}
			}]
		}
	}
}

#Script: {
	name:   string
	image:  string
	script: string
	envs: [...{name: string, value: string}]
	workspaceMounts: [...{workspace: #workspace, mountPath: string}]
	secretMounts: [...{secret: #secret, mountPath: string}]
}

#workspace: {
	name: string
}

#secret: {
	name: string
	items: [...{key: string, path: string}]
}

#PodTask: {
	_settings: {
		scripts_: string
		workspaces_: {...}
		secrets_: {...}
		volumes_: [ for x in workspaces_ {name: x.name, emptyDir: {}}]
		secretVolumes_: [ for x in secrets_ {name: x.name, secret: {secretName: x.name, items: x.items}}]
		toolImage_: string
		baseImage_: string
	}

	apiVersion: "v1"
	kind:       "Pod"
	metadata: {
		annotations: "vela.dev/ready": "READY"
		namespace: *"default" | string
		name:      string
	}
	spec: {
		containers: [...#StepContainer]
		initContainers: [
			{
				name: "place-tools"
				command: ["/ko-app/entrypoint", "cp", "/ko-app/entrypoint", "/vela/tools/entrypoint"]
				image:           _settings.toolImage_
				imagePullPolicy: "IfNotPresent"
				volumeMounts: [{name: "vela-internal-tools", mountPath: "/vela/tools"}]
			}, {
				name:            "place-scripts"
				imagePullPolicy: "IfNotPresent"
				image:           _settings.baseImage_
				command: ["sh"]
				args: ["-c", _settings.scripts_]
				volumeMounts: [{name: "vela-internal-scripts", mountPath: "/vela/scripts"}, {name: "vela-internal-tools", mountPath: "/vela/tools"}]
			}]
		volumes: [
				{emptyDir: {}
				name: "vela-internal-workspace"
			},
			{emptyDir: {}
				name: "vela-internal-home"
			},
			{emptyDir: {}
				name: "vela-internal-results"
			},
			{emptyDir: {}
				name: "vela-internal-steps"
			},
			{emptyDir: {}
				name: "vela-internal-scripts"
			},
			{emptyDir: {}
				name: "vela-internal-tools"
			},
			{downwardAPI: {
				defaultMode: 420
				items: [{
					fieldRef: {
						apiVersion: "v1"
						fieldPath:  "metadata.annotations['vela.dev/ready']"
					}
					path: "ready"
				}]
			}
					name: "vela-internal-downward"
				}] + _settings.volumes_ + _settings.secretVolumes_
		restartPolicy: "Never"
	}
}

#StepContainer: {
	_settings: {
		index_: int
		workspaceMounts_: [...{workspace: #workspace, mountPath: string}]
		volumeMounts_: [ for v in workspaceMounts_ {name: v.workspace.name, mountPath: v.mountPath}]

		secretMounts_: [...{secret: #secret, mountPath: string}]
		secretVolumeMounts_: [ for v in secretMounts_ {name: v.secret.name, mountPath: v.mountPath}]
	}

	name: string
	command: ["/vela/tools/entrypoint"]

	args: *["-wait_file", "/vela/downward/ready", "-wait_file_content", "-post_file", "/vela/tools/\(_settings.index_)", "-termination_path", "/vela/termination", "-step_metadata_dir", "/vela/steps/step-\(name)", "-step_metadata_dir_link", "/vela/steps/\(_settings.index_)", "-entrypoint", "/vela/scripts/script-\(_settings.index_)", "--"] | [...string]
	if _settings.index_ > 0 {
		args: ["-wait_file", "/vela/tools/\(_settings.index_-1)", "-post_file", "/vela/tools/\(_settings.index_)", "-termination_path", "/vela/termination", "-step_metadata_dir", "/vela/steps/step-\(name)", "-step_metadata_dir_link", "/vela/steps/\(_settings.index_)", "-entrypoint", "/vela/scripts/script-\(_settings.index_)", "--"]
	}

	env?:                     _
	image:                    string
	imagePullPolicy:          "Always"
	terminationMessagePath:   "/vela/termination"
	terminationMessagePolicy: "File"
	volumeMounts:             [{
		name:      "vela-internal-scripts"
		mountPath: "/vela/scripts"
	}, {
		name:      "vela-internal-tools"
		mountPath: "/vela/tools"
	}, {
		name:      "vela-internal-downward"
		mountPath: "/vela/downward"
	}, {
		name:      "vela-internal-workspace"
		mountPath: "/workspace"
	}, {
		name:      "vela-internal-home"
		mountPath: "/vela/home"
	}, {
		name:      "vela-internal-results"
		mountPath: "/vela/results"
	}, {
		name:      "vela-internal-steps"
		mountPath: "/vela/steps"
	}] + _settings.volumeMounts_ + _settings.secretVolumeMounts_
}
