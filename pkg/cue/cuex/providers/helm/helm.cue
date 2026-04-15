package helm

#Render: {
	#do:       "render"
	#provider: "helm"

	// +usage=The params for rendering Helm chart
	$params: {
		// +usage=Chart source configuration
		chart: {
			// +usage=Chart location (OCI: "oci://...", URL: "https://.../*.tgz", Repo: "chartname")
			source: string
			// +usage=Repository URL for repository-based charts
			repoURL?: string
			// +usage=Version/tag for repository and OCI charts
			version?: string
			// +usage=Authentication configuration
			auth?: {
				// +usage=Reference to Secret containing credentials
				secretRef?: {
					// +usage=Secret name
					name: string
					// +usage=Secret namespace
					namespace?: string
				}
			}
		}
		
		// +usage=Release configuration
		release?: {
			// +usage=Release name
			name?: string
			// +usage=Target namespace
			namespace?: string
		}
		
		// +usage=Inline values for the chart
		values?: {...}
		
		// +usage=Value sources to merge
		valuesFrom?: [...{
			// +usage=Source kind (Secret, ConfigMap, OCIRepository)
			kind: "Secret" | "ConfigMap" | "OCIRepository"
			// +usage=Resource name
			name: string
			// +usage=Resource namespace
			namespace?: string
			// +usage=Specific key in ConfigMap/Secret
			key?: string
			// +usage=URL for OCIRepository
			url?: string
			// +usage=Tag for OCIRepository
			tag?: string
			// +usage=Don't fail if source doesn't exist
			optional?: bool
		}]
		
		// +usage=KubeVela ownership context — injected as labels on every deployed resource
		context?: {
			// +usage=Application name
			appName: string
			// +usage=Application namespace
			appNamespace: string
			// +usage=Component name
			name: string
			// +usage=Component namespace
			namespace: string
		}

		// +usage=Rendering options
		options?: {
			// +usage=Install CRDs from chart
			includeCRDs?: bool
			// +usage=Skip test resources
			skipTests?: bool
			// +usage=Skip hook resources
			skipHooks?: bool
			// +usage=Create namespace if it doesn't exist
			createNamespace?: bool
			// +usage=Rendering timeout
			timeout?: string
			// +usage=Revisions to keep
			maxHistory?: int
			// +usage=Rollback on failure
			atomic?: bool
			// +usage=Wait for resources
			wait?: bool
			// +usage=Wait timeout
			waitTimeout?: string
			// +usage=Force resource updates
			force?: bool
			// +usage=Recreate pods on upgrade
			recreatePods?: bool
			// +usage=Cleanup on failure
			cleanupOnFail?: bool
			
			// +usage=Cache configuration
			cache?: {
				// +usage=Cache key prefix
				key?: string
				// +usage=TTL for all versions
				ttl?: string
				// +usage=TTL for immutable versions
				immutableTTL?: string
				// +usage=TTL for mutable versions
				mutableTTL?: string
			}
			
			// +usage=Post-rendering configuration
			postRender?: {
				// +usage=Kustomize patches
				kustomize?: {
					patches?: [...]
					patchesJson6902?: [...]
					patchesStrategicMerge?: [...]
					images?: [...]
					replicas?: [...]
				}
				// +usage=External binary
				exec?: {
					command: string
					args?: [...]
					env?: [...]
				}
			}
		}
	}
	
	// +usage=The returns of rendering Helm chart
	$returns?: {
		// +usage=Rendered Kubernetes resources
		resources: [...{
			apiVersion?: string
			kind?: string
			metadata?: {...}
			...
		}]
		// +usage=Chart notes
		notes?: string
	}
	...
}

#Uninstall: {
	#do:       "uninstall"
	#provider: "helm"

	// +usage=The params for uninstalling a Helm release
	$params: {
		// +usage=Release to uninstall
		release: {
			// +usage=Release name
			name: string
			// +usage=Release namespace
			namespace: string
		}
		// +usage=Retain release history after uninstall
		keepHistory?: bool
	}

	// +usage=The returns of uninstalling a Helm release
	$returns?: {
		// +usage=Whether the uninstall succeeded
		success: bool
		// +usage=Additional message (e.g. error detail)
		message?: string
	}
	...
}