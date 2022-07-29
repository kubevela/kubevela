"service-account": {
	type: "trait"
	annotations: {}
	labels: {}
	description: "Specify serviceAccount for your workload which follows the pod spec in path 'spec.template'."
	attributes: {
		podDisruptive: false
		appliesToWorkloads: ["deployments.apps", "statefulsets.apps", "daemonsets.apps", "jobs.batch"]
	}
}
template: {
	#Privileges: {
		// +usage=Specify the verbs to be allowed for the resource
		verbs: [...string]
		// +usage=Specify the apiGroups of the resource
		apiGroups?: [...string]
		// +usage=Specify the resources to be allowed
		resources?: [...string]
		// +usage=Specify the resourceNames to be allowed
		resourceNames?: [...string]
		// +usage=Specify the resource url to be allowed
		nonResourceURLs?: [...string]
		// +usage=Specify the scope of the privileges, default to be namespace scope
		scope: *"namespace" | "cluster"
	}
	parameter: {
		// +usage=Specify the name of ServiceAccount
		name: string
		// +usage=Specify whether to create new ServiceAccount or not
		create: *false | bool
		// +usage=Specify the privileges of the ServiceAccount, if not empty, RoleBindings(ClusterRoleBindings) will be created
		privileges?: [...#Privileges]
	}
	// +patchStrategy=retainKeys
	patch: spec: template: spec: serviceAccountName: parameter.name

	_clusterPrivileges: [ for p in parameter.privileges if p.scope == "cluster" {p}]
	_namespacePrivileges: [ for p in parameter.privileges if p.scope == "namespace" {p}]
	outputs: {
		if parameter.create {
			"service-account": {
				apiVersion: "v1"
				kind:       "ServiceAccount"
				metadata: name: parameter.name
			}
		}
		if parameter.privileges != _|_ {
			if len(_clusterPrivileges) > 0 {
				"cluster-role": {
					apiVersion: "rbac.authorization.k8s.io/v1"
					kind:       "ClusterRole"
					metadata: name: "\(context.namespace):\(parameter.name)"
					rules: [ for p in _clusterPrivileges {
						verbs: p.verbs
						if p.apiGroups != _|_ {
							apiGroups: p.apiGroups
						}
						if p.resources != _|_ {
							resources: p.resources
						}
						if p.resourceNames != _|_ {
							resourceNames: p.resourceNames
						}
						if p.nonResourceURLs != _|_ {
							nonResourceURLs: p.nonResourceURLs
						}
					}]
				}
				"cluster-role-binding": {
					apiVersion: "rbac.authorization.k8s.io/v1"
					kind:       "ClusterRoleBinding"
					metadata: name: "\(context.namespace):\(parameter.name)"
					roleRef: {
						apiGroup: "rbac.authorization.k8s.io"
						kind:     "ClusterRole"
						name:     "\(context.namespace):\(parameter.name)"
					}
					subjects: [{
						kind:      "ServiceAccount"
						name:      parameter.name
						namespace: "\(context.namespace)"
					}]
				}
			}
			if len(_namespacePrivileges) > 0 {
				"role": {
					apiVersion: "rbac.authorization.k8s.io/v1"
					kind:       "Role"
					metadata: name: parameter.name
					rules: [ for p in _namespacePrivileges {
						verbs: p.verbs
						if p.apiGroups != _|_ {
							apiGroups: p.apiGroups
						}
						if p.resources != _|_ {
							resources: p.resources
						}
						if p.resourceNames != _|_ {
							resourceNames: p.resourceNames
						}
						if p.nonResourceURLs != _|_ {
							nonResourceURLs: p.nonResourceURLs
						}
					}]
				}
				"role-binding": {
					apiVersion: "rbac.authorization.k8s.io/v1"
					kind:       "RoleBinding"
					metadata: name: parameter.name
					roleRef: {
						apiGroup: "rbac.authorization.k8s.io"
						kind:     "Role"
						name:     parameter.name
					}
					subjects: [{
						kind: "ServiceAccount"
						name: parameter.name
					}]
				}
			}
		}
	}
}
