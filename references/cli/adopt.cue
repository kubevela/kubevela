import "list"

#Resource: {
	apiVersion: string
	kind:       string
	metadata: {
		name:       string
		namespace?: string
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
		workflow?: {...}
	}
}

#AdoptOptions: {
	mode:         *"read-only" | "take-over"
	type:         *"helm" | string
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
	}]
	resourceMap: {
		for key, val in resourceCategoryMap {
			"\(key)": [ for r in _resources if r._category == key {r}]
		}
		unknown: [ for r in _resources if r._category == "unknown" {r}]
	}

	unknownKinds: {for r in resourceMap.unknown {"\(r.kind)": true}}
	unknownByKinds: {for kind, val in unknownKinds {
		"\(kind)": [ for r in resourceMap.unknown if r.kind == kind {r}]
	}}

	appName: $args.appName
	comps: [
		if len(resourceMap.crd) > 0 {
			type: "k8s-objects"
			name: "\(appName).crds"
			properties: objects: [ for r in resourceMap.crd {
				apiVersion: r.apiVersion
				kind:       r.kind
				metadata: name: r.metadata.name
			}]
		},
		for r in resourceMap.ns {
			type: "k8s-objects"
			name: "\(appName).ns.\(r.metadata.name)"
			properties: objects: [{
				apiVersion: r.apiVersion
				kind:       r.kind
				metadata: name: r.metadata.name
			}]
		},
		for r in resourceMap.workload + resourceMap.service {
			type: "k8s-objects"
			name: "\(appName).\(r.kind).\(r.metadata.name)"
			properties: objects: [{
				apiVersion: r.apiVersion
				kind:       r.kind
				metadata: name:      r.metadata.name
				metadata: namespace: r.metadata.namespace
				spec: r.spec
			}]
		},
		for key in ["config", "sa", "operator", "storage"] if len(resourceMap[key]) > 0 {
			type: "k8s-objects"
			name: "\(appName).\(key)"
			properties: objects: [ for r in resourceMap.config {
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
			name: "\(appName).\(kind)"
			properties: objects: [ for r in rs {
				apiVersion: r.apiVersion
				kind:       r.kind
				metadata: name: r.metadata.name
			}]
		},
	]

	$returns: #Application & {
		metadata: {
			name:      $args.appName
			namespace: $args.appNamespace
			labels: "app.oam.dev/adopt": $args.type
		}
		spec: components: comps
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
	}
}
