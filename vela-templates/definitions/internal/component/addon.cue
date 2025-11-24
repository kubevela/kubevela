import "vela/addon"

addon: {
	type: "component"
	annotations: {
		"category": "System Extensions"
	}
	labels: {}
	description: "Install and render an addon as a component, enabling addons to be used as first-class KubeVela components"
	attributes: {
		workload: {
			definition: {
				apiVersion: "core.oam.dev/v1beta1"
				kind:       "Application"
			}
			type: "applications.core.oam.dev"
		}
		status: {
			details: {
				$name: "\(context.output.metadata.labels["addons.oam.dev/name"])"
				$version: "\(context.output.metadata.annotations["addons.oam.dev/version"])"
				$requestedVersion: *parameter.version | "latest"
				$registry: "\(context.output.metadata.annotations["addons.oam.dev/registry"])"
				$registryType: "\(context.output.metadata.annotations["addons.oam.dev/registry-type"])"
				$registryUrl: "\(context.output.metadata.annotations["addons.oam.dev/registry-url"])"
				name: $name
				requestedVersion: $requestedVersion
				resolvedVersion: $version
				requestedRegistry: *parameter.registry | "<not specified>"
				registry: $registry
				registryType: $registryType
				registryUrl: $registryUrl
			}
			customStatus: #"""
				if context.status.healthy {
					addonAppStatus: context.output.status.status
					message: "Addon \(parameter.addon) v\(context.outputs.addons.resolvedVersion) from \(context.outputs.addons.registry) rendered successfully"
				}
				if !context.status.healthy {
					message: "Rendering addon \(parameter.addon)..."
				}
			"""#
			healthPolicy: #"""
				isHealth: context.output.status.status == "running"
			"""#
		}
	}
}

template: {
	// Render the addon using the CueX provider
	_addonRender: addon.#Render & {
		$params: {
			addon: parameter.addon
			properties: parameter.properties
			if parameter.version != _|_ {
				version: parameter.version
			}
			if parameter.registry != _|_ {
				registry: parameter.registry
			}
			if parameter.include != _|_ {
				include: parameter.include
			}
		}
	}

	// The addon application becomes the main output
	output: _addonRender.$returns.application

	// Additional resources become auxiliary outputs
	outputs: {
		// Create auxiliary outputs for each rendered resource
		for i, resource in _addonRender.$returns.resources {
			"addon-resource-\(i)": resource
		}
	}

	parameter: {
		// +usage=The name of the addon to install. Can be in format 'registry/addon' to specify registry
		addon: string | *context.name

		// +usage=Version of the addon to install. Can be exact version ("1.2.3") or constraint (">=1.0.0")
		version?: string

		// +usage=Registry to search for the addon (optional)
		registry?: string

		// +usage=Configuration properties for the addon
		properties: {...} | *{}
		
		// +usage=Selectively include addon components (default: all)
		include?: {
			// +usage=Include ComponentDefinitions, TraitDefinitions, WorkflowStepDefinitions
			definitions: bool | *true
			// +usage=Include config templates
			configTemplates: bool | *true
			// +usage=Include VelaQL views
			views: bool | *true
			// +usage=Include auxiliary resources from the addon
			resources: bool | *true
		}
	}
}