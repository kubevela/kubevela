
let toolImage = "gcr.io/tekton-releases/github.com/tektoncd/pipeline/cmd/entrypoint:v0.27.2"
let baseImage = "gcr.io/distroless/base@sha256:aa4fd987555ea10e1a4ec8765da8158b5ffdfef1e72da512c7ede509bc9966c4"

#Task: {
	#do:       "steps"
	name:      string
	namespace: string
	workspace: [_name_=string]: #workspace & {name: "\(_name_)"}
	steps: [...#Script]

	_name:      string
	_namespace: string
	_generate_scripts: [ for i, x in steps {"""
scriptfile="/vela/scripts/script-\(i)"
touch ${scriptfile} && chmod +x ${scriptfile}
cat > ${scriptfile} << '_EOF_'
\(base64.Encode(null, x.script))
_EOF_
"""}]
	do: kube.#Apply & {
		value: #PodTask & {
			_scripts:   strings.Join(_generate_scripts, "")
			_workspace: workspace
			metadata: {
				name:      _name
				namespace: _namespace
			}
			spec: containers: [ for i, step in steps {
				#StepContainer
				name:             step.name
				image:            step.image
				_workspaceMounts: workspace
				_index:           i
			}]
		}
	}
}

#Script: {
	name:   string
	image:  string
	script: string
	envs?: [...{name: string, value: string}]
	workspaceMounts: [...{workspace: #workspace, mountPath: string}]
}

#workspace: {
	name: string
}

#PodTask: {
	_scripts: string
	_workspace: [_name=string]: #workspace
	_volumes: [ for x in _workspace {name: x.name, emptyDir: {}}]
	apiVersion: "v1"
	kind:       "Pod"
	metadata: {
		annotations: "vela.dev/ready": "READY"
		namespace: *"default" | string
		name:      string
	}
	spec: {
		initContainers: [
			{
				name: "place-tools"
				command: ["/ko-app/entrypoint", "cp", "/ko-app/entrypoint", "/vela/tools/entrypoint"]
				image:           toolImage
				imagePullPolicy: "IfNotPresent"
				volumeMounts: [{name: "vela-internal-tools", mountPath: "/vela/tools"}]
			}, {
				name:            "place-scripts"
				imagePullPolicy: "IfNotPresent"
				command: ["sh"]
				args: ["-c", _scripts + "\n/vela/tools/entrypoint decode-script \"${scriptfile}\""]
				volumeMounts: [{name: "vela-internal-scripts", mountPath: "/vela/scripts"}, {name: "vela-internal-tools", mountPath: "/vela/tools"}]
			}]
		volumes: [
				{emptyDir: {}
				name: "vela-internal-workspace"
			},
			{emptyDir: {}
				name: vela - internal - home
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
			}] + _volumes
	}
}

#StepContainer: {
	_index:    int
	_waitFile: *"/vela/downward/ready" | string
	if _index != 0 {
		_waitFile: "/vela/tools/\(_index)"
	}
	_workspaceMounts: [...{workspace: #workspace, mountPath: string}]
	_volumeMounts: [ for v in _workspaceMounts {name: v.name, mountPath: v.mountPath}]

	name: string
	args: ["-wait_file", _waitFile, "-wait_file_content", "-post_file", "/vela/tools/\(_index)", "-termination_path", "/vela/termination", "-step_metadata_dir", "/vela/steps/step-\(name)", "-step_metadata_dir_link", "/vela/steps/\(_index)", "-entrypoint", "/vela/scripts/script-\(_index)", "--"]
	command: ["/vela/tools/entrypoint"]
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
	}] + _volumeMounts
}
