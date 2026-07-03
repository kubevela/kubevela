// trait-sidecar-logger.cue
//
// Demonstrates upgrade rule 2: error-field-label
//
// CUE v0.14 introduced `error` as a built-in function. Any definition that
// used `error` as an unquoted field label now fails to parse. The upgrade
// engine rewrites `error:` to `"error":` at render time.
//
// Broken line (pre-v1.11):
//   error: "unsupported log format"
//
// After upgrade:
//   "error": "unsupported log format"

"sidecar-logger": {
	type:        "trait"
	description: "Injects a logging sidecar into the component's pod. Exposes a structured status block including an error field — which conflicts with the CUE v0.14 built-in."
	appliesToWorkloads: ["deployments.apps"]
	attributes: podDisruptive: false
}

template: {
	patch: spec: template: spec: containers: context.output.spec.template.spec.containers + [{
		name:  "log-shipper"
		image: parameter.shipperImage
		env: [{
			name:  "LOG_FORMAT"
			value: parameter.format
		}, {
			name:  "LOG_LEVEL"
			value: parameter.level
		}]
		volumeMounts: [{
			name:      "varlog"
			mountPath: "/var/log"
		}]
	}]

	// ISSUE 2 (error-field-label): `error` is a CUE v0.14 built-in; using it
	// as an unquoted field label causes a parse error. The upgrade engine
	// rewrites this to `"error": ...` at render time.
	outputs: "logger-status": {
		apiVersion: "v1"
		kind:       "ConfigMap"
		metadata: {
			name:      "\(context.name)-logger-status"
			namespace: context.namespace
		}
		data: {
			format: parameter.format
			level:  parameter.level
			if parameter.format != "json" && parameter.format != "logfmt" {
				// This field name conflicts with the CUE v0.14 `error` built-in.
				error: "unsupported log format; expected json or logfmt"
			}
		}
	}

	parameter: {
		// +usage=Log shipper sidecar image
		shipperImage: *"fluent/fluent-bit:2.2" | string

		// +usage=Log output format (json or logfmt)
		format: *"json" | string

		// +usage=Log level
		level: *"info" | "debug" | "warn" | "error"
	}
}
