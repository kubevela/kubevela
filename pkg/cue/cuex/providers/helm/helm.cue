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
			// +usage=Authentication for private chart repositories.
			// The referenced Secret MUST be one of: kubernetes.io/basic-auth,
			// kubernetes.io/dockerconfigjson, kubernetes.io/tls, or Opaque.
			// User-supplied bearer tokens MUST NOT be used with OCI sources;
			// the registry performs its own Basic->Bearer exchange per the OCI
			// Distribution Spec. Tokens are treated as opaque per RFC 7519.
			auth?: {
				// +usage=Reference to a Kubernetes Secret containing credentials.
				secretRef?: {
					// +usage=Secret name.
					name: string
					// +usage=Secret namespace. MAY be omitted (defaults to the
					// release namespace). When set, it MUST equal either the
					// release namespace or the Application namespace.
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
			// +usage=Source kind. Only Secret and ConfigMap are supported.
			kind: "Secret" | "ConfigMap"
			// +usage=Resource name
			name: string
			// +usage=Resource namespace
			namespace?: string
			// +usage=Specific key in ConfigMap/Secret
			key?: string
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
			// +usage=Install CRDs from the chart's crds/ directory
			includeCRDs?: bool
			// +usage=Skip test resources
			skipTests?: bool
			// +usage=Skip hook resources
			skipHooks?: bool
			// +usage=Create namespace if it doesn't exist
			createNamespace?: bool
			// +usage=Rendering and wait timeout (Helm SDK uses one value for both)
			timeout?: string
			// +usage=Revisions to keep (upgrade-only; ignored on first install)
			maxHistory?: int
			// +usage=Rollback on failure
			atomic?: bool
			// +usage=Wait for resources to become ready
			wait?: bool
			// +usage=Force resource updates
			force?: bool
			// +usage=Recreate pods (upgrade-only; ignored on first install)
			recreatePods?: bool
			// +usage=Cleanup on failure (upgrade-only; ignored on first install)
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
