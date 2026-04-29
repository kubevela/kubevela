// helmchart.cue - Component definition for Helm charts

import (
	"vela/helm"
)

"helmchart": {
	type: "component"
	annotations: {
		"definition.oam.dev/description": "Deploy Helm charts natively in KubeVela without FluxCD"
	}
	labels: {
		"custom.definition.oam.dev/category": "helm"
	}
	attributes: {
		workload: type: "autodetects.core.oam.dev"
		status: {
			healthPolicy: #"""
				_healthCheck: {
					_criteria: [] | *parameter.healthStatus
					if len(_criteria) > 0 {
						_criteriaResults: [ for criterion in _criteria {
							_targetKind: criterion.resource.kind
							_targetName: criterion.resource.name
							_conditionType: criterion.condition.type
							_conditionStatus: *"True" | criterion.condition.status
							// Search through all outputs for matching resource
							_matchingResources: [ for outputKey, resource in context.outputs 
								if resource.kind == _targetKind && 
									(_targetName == _|_ || resource.metadata.name == _targetName) { 
									resource 
								} 
							]
							if len(_matchingResources) > 0 {
								_resource: _matchingResources[0]
								if _resource.status.conditions != _|_ {
									// Look for matching condition
									_matchingConditions: [ for cond in _resource.status.conditions 
										if cond.type == _conditionType && cond.status == _conditionStatus { 
											cond 
										} 
									]
									result: len(_matchingConditions) > 0
								}
								if _resource.status.conditions == _|_ {
									result: false
								}
							}
							if len(_matchingResources) == 0 {
								result: false
							}
						} ]
						_failedCriteria: [ for r in _criteriaResults if r.result == false { r } ]
						result: len(_failedCriteria) == 0
					}
					if parameter.healthStatus == _|_ || len(_criteria) == 0 {
						result: true
					}
				}
				isHealth: _healthCheck.result
				"""#
			customStatus: #"""
				if context.status.healthy {
					message: "Deployed"
				}
				if !context.status.healthy {
					message: "Deploying"
				}
				"""#
		}
	}
}

template: {
	output:  _
	outputs: _

	parameter: {
		// Chart source configuration
		chart: {
			// Chart location - automatically detected based on format:
			// - OCI: "oci://ghcr.io/org/charts/app"
			// - Direct URL: "https://example.com/charts/app-1.0.0.tgz"
			// - Repo chart: "postgresql" (requires repoURL to be set)
			source: string

			// Repository URL for repository-based charts
			repoURL?: string

			// Version/tag for repository and OCI charts (ignored for direct URLs)
			version?: string | *"latest"

			// Authentication (optional) - TODO: Not yet implemented
			// auth?: {
			// 	// Reference to Secret containing credentials
			// 	secretRef?: {
			// 		name: string
			// 		namespace?: string | *context.namespace
			// 	}
			// }
		}

		// Release configuration (optional - uses context defaults)
		release?: {
			// Release name (defaults to component name)
			name?: string | *context.name
			// Target namespace (defaults to Application namespace)
			namespace?: string | *context.namespace
		}

		// +usage=Inline values merged with the highest priority; override everything in valuesFrom.
		values?: {...}

		// +usage=Additional values sources merged in array order. Later entries override earlier ones on conflict, and inline `values` override everything in valuesFrom. Deep-merges map keys; arrays are replaced (not concatenated); null is preserved. On every reconcile the controller computes a content fingerprint over all referenced sources and folds it into the workflow revision; an external edit to a referenced ConfigMap/Secret triggers a `helm upgrade` on the next reconcile (default resync ~5 min). Sources are read from the control-plane cluster regardless of where the chart is deployed. Suppressed when the `app.oam.dev/publishVersion` annotation is set so explicit pins remain hard.
		valuesFrom?: [...{
			// +usage=Source kind. Only Secret and ConfigMap are supported; OCIRepository is reserved for a future release.
			kind: "Secret" | "ConfigMap"
			// +usage=Name of the Secret or ConfigMap.
			name: string
			// +usage=Namespace of the Secret or ConfigMap. Defaults to the chart release namespace, which itself defaults to the Application's own namespace when release.namespace is unset. Cross-namespace references are rejected to prevent cross-tenant reads via the controller's cluster-wide RBAC.
			namespace?: string
			// +usage=Key inside .data whose value is parsed as YAML. Defaults to "values.yaml" (FluxCD / Helm convention). Only ConfigMap.data and Secret.data are read; ConfigMap.binaryData is rejected.
			key?: string
			// +usage=If true, a missing Secret/ConfigMap or missing key is skipped silently. Parse errors and permission errors still fail the render.
			optional?: bool | *false
		}]

		// Health status criteria - defines when the Helm deployment is considered healthy
		healthStatus?: [...{
			// Resource to check
			resource: {
				// Resource kind (e.g., "Deployment", "StatefulSet", "Job", "Service")
				kind: string
				// Optional: specific resource name (if not specified, checks first of kind)
				name?: string
			}
			// Health condition to verify.
			// The type must match an actual Kubernetes .status.conditions[].type value.
			// Common condition types by resource kind:
			//   Deployment:  "Available", "Progressing"
			//   StatefulSet: "Available"
			//   Pod:         "Ready", "ContainersReady", "Initialized", "PodScheduled"
			//   Job:         "Complete", "Failed"
			//   Node:        "Ready"
			// Note: resources without .status.conditions (e.g., Service, ConfigMap)
			// will always evaluate to unhealthy — do not use them as health criteria.
			condition: {
				type: string
				// Expected status (default: "True", use "False" for conditions like Progressing)
				status?: "True" | "False"
			}
		}]

		// Rendering options
		options?: {
			includeCRDs?:     bool | *true    // Install CRDs from chart
			skipTests?:       bool | *true    // Skip test resources
			skipHooks?:       bool | *false   // Skip hook resources
			createNamespace?: bool | *true    // Create namespace if it doesn't exist
			timeout?:         string | *"5m"  // Rendering timeout
			maxHistory?:      int | *10       // Revisions to keep
			atomic?:          bool | *false   // Rollback on failure
			wait?:            bool | *false   // Wait for resources
			waitTimeout?:     string | *"10m" // Wait timeout
			force?:           bool | *false   // Force resource updates
			recreatePods?:    bool | *false   // Recreate pods on upgrade
			cleanupOnFail?:   bool | *false   // Cleanup on failure

			// Cache configuration
			cache?: {
				// Cache key prefix (defaults to "{context.appName}-{context.name}")
				// Examples: "shared", "dev-cluster", "prod-env"
				key?: string
				// TTL for this specific chart (overrides automatic detection)
				// Examples: "24h", "5m", "30s", "0" (disable cache)
				ttl?: string
				// Or specify different TTLs for immutable vs mutable versions
				immutableTTL?: string | *"24h" // TTL for semantic versions (1.2.3, v2.0.0)
				mutableTTL?:   string | *"5m"  // TTL for mutable tags (latest, dev, main)
			}

			// Post-rendering - Future enhancement
			// Planned: CUE-based post-rendering for resource transformation
			// Would allow users to write CUE templates to modify rendered resources
			// with full access to KubeVela context (appName, namespace, etc.)
			// Requires CUE-in-CUE runtime execution capability
			// postRender?: {
			// 	template: string  // CUE template for transforming resources
			// }
		}
	}

	// Set default release configuration
	_release: {
		if parameter.release != _|_ {
			parameter.release
		}
		if parameter.release == _|_ {
			name:      context.name
			namespace: context.namespace
		}
	}

	// Set default options with cache key
	_options: {
		if parameter.options != _|_ {
			parameter.options
		}
		if parameter.options == _|_ {
			cache: {
				key: "\(context.appName)-\(context.name)"
			}
		}
		if parameter.options != _|_ && parameter.options.cache != _|_ && parameter.options.cache.key == _|_ {
			cache: {
				parameter.options.cache
				key: "\(context.appName)-\(context.name)"
			}
		}
	}

	// Capture KubeVela runtime context BEFORE the $params block to avoid
	// CUE scoping collision: naming a field "context" inside $params would
	// cause inner "context.xxx" references to resolve to the field itself
	// (self-reference) instead of KubeVela's runtime context object.
	_velaContext: {
		appName:      context.appName
		appNamespace: context.namespace
		name:         context.name
		namespace:    context.namespace
	}

	// Render the Helm chart using the provider
	_rendered: helm.#Render & {
		$params: {
			chart:   parameter.chart
			release: _release
			if parameter.values != _|_ {
				values: parameter.values
			}

			if parameter.valuesFrom != _|_ {
				valuesFrom: parameter.valuesFrom
			}
			options: _options

			// Pass KubeVela ownership context so the provider can inject labels
			"context": _velaContext
		}
	}

	// Primary output: an audit ConfigMap that records release metadata.
	// This gives KubeVela a stable, predictable primary resource regardless
	// of what the Helm chart contains (chart rendering order is not guaranteed).
	output: {
		apiVersion: "v1"
		kind:       "ConfigMap"
		metadata: {
			name:      "\(_release.name)-helm-release"
			namespace: _release.namespace
			labels: {
				"app.oam.dev/name":      context.appName
				"app.oam.dev/namespace": context.namespace
				"app.oam.dev/component": context.name
				"helm.oam.dev/chart":    parameter.chart.source
			}
		}
		data: {
			chartSource:      parameter.chart.source
			releaseName:      _release.name
			releaseNamespace: _release.namespace
			if parameter.chart.repoURL != _|_ {
				repoURL: parameter.chart.repoURL
			}
			if parameter.chart.version != _|_ {
				chartVersion: parameter.chart.version
			}
			resourceCount: "\(len(_rendered.$returns.resources))"
		}
	}

	// All rendered Helm resources go into outputs with stable keys
	if _rendered.$returns.resources != _|_ {
		if len(_rendered.$returns.resources) > 0 {
			outputs: {
				for i, res in _rendered.$returns.resources {
					"helm-resource-\(i)": res
				}
			}
		}
	}
}
