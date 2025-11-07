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
					if parameter.healthStatus == _|_ {
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
	output: _
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
		
		// Inline values (highest priority)
		values?: {...}
		
		// Value sources (merged in order) - TODO: Not yet implemented
		// valuesFrom?: [...{
		// 	kind: "Secret" | "ConfigMap" | "OCIRepository"
		// 	name: string
		// 	namespace?: string
		// 	key?: string        // Specific key in ConfigMap/Secret
		// 	url?: string        // For OCIRepository
		// 	tag?: string        // For OCIRepository
		// 	optional?: bool | *false // Don't fail if source doesn't exist
		// }]
		
		// Health status criteria - defines when the Helm deployment is considered healthy
		healthStatus?: [...{
			// Resource to check
			resource: {
				// Resource kind (e.g., "Deployment", "StatefulSet", "Job", "Service")
				kind: string
				// Optional: specific resource name (if not specified, checks first of kind)
				name?: string
			}
			// Health condition to verify
			condition: {
				// Condition type to check:
				// - "Ready": Resource has Ready=True condition
				// - "Available": Resource has Available=True condition  
				// - "Progressing": Check if still progressing (use status: "False" to wait for completion)
				// - "ReplicasReady": All replicas are ready (for Deployment/StatefulSet)
				// - "PodReady": Pod is Running or Succeeded (for Job/Pod)
				// - "JobComplete": Job has Complete=True condition
				type: "Ready" | "Available" | "Progressing" | "ReplicasReady" | "PodReady" | "JobComplete"
				// Expected status (default: "True", use "False" for conditions like Progressing)
				status?: "True" | "False"
			}
		}]
		
		// Rendering options
		options?: {
			includeCRDs?: bool | *true      // Install CRDs from chart
			skipTests?: bool | *true         // Skip test resources
			skipHooks?: bool | *false        // Skip hook resources
			createNamespace?: bool | *true   // Create namespace if it doesn't exist
			timeout?: string | *"5m"         // Rendering timeout
			maxHistory?: int | *10           // Revisions to keep
			atomic?: bool | *false           // Rollback on failure
			wait?: bool | *false             // Wait for resources
			waitTimeout?: string | *"10m"    // Wait timeout
			force?: bool | *false            // Force resource updates
			recreatePods?: bool | *false     // Recreate pods on upgrade
			cleanupOnFail?: bool | *false    // Cleanup on failure
			
			// Cache configuration
			cache?: {
				// Cache key prefix (defaults to "{context.appName}-{context.name}")
				// Examples: "shared", "dev-cluster", "prod-env"
				key?: string
				// TTL for this specific chart (overrides automatic detection)
				// Examples: "24h", "5m", "30s", "0" (disable cache)
				ttl?: string
				// Or specify different TTLs for immutable vs mutable versions
				immutableTTL?: string | *"24h"  // TTL for semantic versions (1.2.3, v2.0.0)
				mutableTTL?: string | *"5m"     // TTL for mutable tags (latest, dev, main)
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
			name: context.name
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

	// Render the Helm chart using the provider
	_rendered: helm.#Render & {
		$params: {
			chart: parameter.chart
			release: _release
			if parameter.values != _|_ {
				values: parameter.values
			}
			// TODO: valuesFrom not yet implemented
			// if parameter.valuesFrom != _|_ {
			// 	valuesFrom: parameter.valuesFrom
			// }
			options: _options
		}
	}
	
	// Set outputs from rendered resources
	if _rendered.$returns.resources != _|_ {
		if len(_rendered.$returns.resources) > 0 {
			// Take first resource as primary output
			output: _rendered.$returns.resources[0]
		}
		
		// Put remaining resources in outputs
		if len(_rendered.$returns.resources) > 1 {
			outputs: {
				for i, res in _rendered.$returns.resources if i > 0 {
					"helm-resource-\(i)": res
				}
			}
		}
	}
}