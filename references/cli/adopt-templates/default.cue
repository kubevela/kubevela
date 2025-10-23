import (
	"list"
	"strings"
)

#Resource: {
	apiVersion: string
	kind:       string
	metadata: {
		name:       string
		namespace?: string
		annotations?: [string]: string
		...
	}
	...
}

#Component: {
	type: string
	name: string
	properties: {...}
	dependsOn?: [...string]
	traits?: [...#Trait]
}

#Trait: {
	type: string
	properties: {...}
}

#Policy: {
	type: string
	name: string
	properties?: {...}
}

#WorkflowStep: {
	type: string
	name: string
	properties?: {...}
	subSteps?: [...{...}]
}

#Application: {
	apiVersion: "core.oam.dev/v1beta1"
	kind:       "Application"
	metadata: {
		name:       string
		namespace?: string
		labels?: [string]:      string
		annotations?: [string]: string
	}
	spec: {
		components: [...#Component]
		policies?: [...#Policy]
		workflow?: {
			steps: [...#WorkflowStep]
		}
	}
}

#AdoptOptions: {
	mode:         *"read-only" | "take-over"
	type:         *"native" | "helm" | string
	appName:      string
	appNamespace: string
	resources: [...#Resource]
	...
}

#Adopt: {
	$args:    #AdoptOptions
	$returns: #Application

	// adopt logics

	resourceCategoryMap: {
		crd: ["CustomResourceDefinition"]
		ns: ["Namespace"]
		workload: ["Deployment", "StatefulSet", "DaemonSet", "CloneSet"]
		service: ["Service", "Ingress", "HTTPRoute"]
		config: ["ConfigMap", "Secret"]
		sa: ["ServiceAccount", "Role", "RoleBinding", "ClusterRole", "ClusterRoleBinding"]
		operator: ["MutatingWebhookConfiguration", "ValidatingWebhookConfiguration", "APIService"]
		storage: ["PersistentVolume", "PersistentVolumeClaim"]
	}

	_resources: [ for r in $args.resources {
		r
		_category: *"unknown" | string
		for key, kinds in resourceCategoryMap if list.Contains(kinds, r.kind) {
			_category: key
		}
		_cluster: *"local" | string
		if r.metadata.labels != _|_ if r.metadata.labels["app.oam.dev/cluster"] != _|_ {
			_cluster: r.metadata.labels["app.oam.dev/cluster"]
		}
	}]

	_clusters: [ for _cluster, _ in {for r in _resources {"\(r._cluster)": true}} {_cluster}]

	appName: $args.appName
	clusterEntries: {
		for cluster in _clusters {
			"\(cluster)": {
				_prefix: *"" | string
				if cluster != "local" {
					_prefix: cluster + ":"
				}
				resourceMap: {
					for key, val in resourceCategoryMap {
						"\(key)": [ for r in _resources if r._category == key && r._cluster == cluster {r}]
					}
					unknown: [ for r in _resources if r._category == "unknown" && r._cluster == cluster {r}]
				}

				unknownKinds: {for r in resourceMap.unknown {"\(r.kind)": true}}
				unknownByKinds: {for kind, val in unknownKinds {
					"\(kind)": [ for r in resourceMap.unknown if r.kind == kind {r}]
				}}

				comps: [
					if len(resourceMap.crd) > 0 {
						type: "k8s-objects"
						name: "\(_prefix)crds"
						properties: objects: [ for r in resourceMap.crd {
							apiVersion: r.apiVersion
							kind:       r.kind
							metadata: name: r.metadata.name
						}]
					},
					if len(resourceMap.ns) > 0 {
						type: "k8s-objects"
						name: "\(_prefix)ns"
						properties: objects: [ for r in resourceMap.ns {
							apiVersion: r.apiVersion
							kind:       r.kind
							metadata: name: r.metadata.name
						}]
					},
					for r in list.Concat([resourceMap.workload, resourceMap.service]) {
						type: "k8s-objects"
						name: _prefix + strings.ToLower("\(r.kind)-\(r.metadata.name)")
						properties: objects: [{
							apiVersion: r.apiVersion
							kind:       r.kind
							metadata: name: r.metadata.name
							if r.metadata.namespace != _|_ {
								metadata: namespace: r.metadata.namespace
							}
							spec: r.spec
						}]
					},
					for key in ["config", "sa", "operator", "storage"] if len(resourceMap[key]) > 0 {
						type: "k8s-objects"
						name: "\(_prefix)\(key)"
						properties: objects: [ for r in resourceMap[key] {
							apiVersion: r.apiVersion
							kind:       r.kind
							metadata: name: r.metadata.name
							if r.metadata.namespace != _|_ {
								metadata: namespace: r.metadata.namespace
							}
						}]
					},
					for kind, rs in unknownByKinds {
						type: "k8s-objects"
						name: "\(_prefix)\(kind)"
						properties: objects: [ for r in rs {
							apiVersion: r.apiVersion
							kind:       r.kind
							metadata: name: r.metadata.name
							if r.metadata.namespace != _|_ {
								metadata: namespace: r.metadata.namespace
							}
						}]
					},
				]
			}
		}
	}

	$returns: #Application & {
		metadata: {
			name:      $args.appName
			namespace: $args.appNamespace
			labels: "app.oam.dev/adopt": $args.type
		}
		spec: components: [ for cluster, entry in clusterEntries for comp in entry.comps {comp}]
		spec: policies: [
			{
				type: $args.mode
				name: $args.mode
				properties: rules: [{
					selector: componentNames: [ for comp in spec.components {comp.name}]
				}]
			},
			if $args.mode == "take-over" {
				type: "garbage-collect"
				name: "garbage-collect"
				properties: rules: [{
					strategy: "never"
					selector: resourceTypes: ["CustomResourceDefinition"]
				}]
			},
			if $args.mode == "take-over" {
				type: "apply-once"
				name: "apply-once"
				properties: rules: [{
					selector: resourceTypes: ["CustomResourceDefinition"]
				}]
			}]
		spec: workflow: steps: [{
			type: "step-group"
			name: "apply-component"
			subSteps: [ for c, entry in clusterEntries for comp in entry.comps {
				type: "apply-component"
				name: "apply-component:" + comp.name
				properties: component: comp.name
				if c != "local" {
					properties: cluster: c
				}
			}]
		}]
	}
}
