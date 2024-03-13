import "strconv"

gateway: {
	type: "trait"
	annotations: {}
	labels: {}
	description: "Enable public web traffic for the component, the ingress API matches K8s v1.20+."
	attributes: {
		podDisruptive: false
		appliesToWorkloads: ["deployments.apps", "statefulsets.apps"]

		status: {
			customStatus: #"""
				let nameSuffix = {
				  if parameter.name != _|_ { "-" + parameter.name }
				  if parameter.name == _|_ { "" }
				}
				let ingressMetaName = context.name + nameSuffix
				let ig  = [for i in context.outputs if (i.kind == "Ingress") && (i.metadata.name == ingressMetaName) {i}][0]
				igs: *null | string
				if ig != _|_ if ig.status != _|_ if ig.status.loadbalancer != _|_ {
				  igs: ig.status.loadbalancer.ingress[0]
				}
				igr: *null | string
				if ig != _|_ if ig.spec != _|_  {
				  igr: ig.spec.rules[0]
				}
				if igs == _|_ {
				  message: "No loadBalancer found, visiting by using 'vela port-forward " + context.appName + "'\n"
				}
				if igs != _|_ {
				  if igs.ip != _|_ {
				    if igr.host != _|_ {
				      message: "Visiting URL: " + igr.host + ", IP: " + igs.ip + "\n"
				    }
				    if igr.host == _|_ {
				      message: "Host not specified, visit the cluster or load balancer in front of the cluster, IP: " + igs.ip + "\n"
				    }
				  }
				  if igs.ip == _|_ {
				    if igr.host != _|_ {
				      message: "Visiting URL: " + igr.host + "\n"
				    }
				    if igs.host == _|_ {
				      message: "Host not specified, visit the cluster or load balancer in front of the cluster\n"
				    }
				  }
				}
				"""#
			healthPolicy: #"""
				let nameSuffix = {
				  if parameter.name != _|_ { "-" + parameter.name }
				  if parameter.name == _|_ { "" }
				}
				let ingressMetaName = context.name + nameSuffix
				let igstat  = len([for i in context.outputs if (i.kind == "Ingress") && (i.metadata.name == ingressMetaName) {i}]) > 0
				isHealth: igstat
				"""#
		}
	}
}
template: {
	let nameSuffix = {
		if parameter.name != _|_ {"-" + parameter.name}
		if parameter.name == _|_ {""}
	}

	let serviceMetaName = {
		if (parameter.existingServiceName != _|_) {parameter.existingServiceName}
		if (parameter.existingServiceName == _|_) {context.name + nameSuffix}
	}

	if (parameter.existingServiceName == _|_) {
		let serviceOutputName = "service" + nameSuffix
		outputs: (serviceOutputName): {
			apiVersion: "v1"
			kind:       "Service"
			metadata: name: "\(serviceMetaName)"
			spec: {
				selector: "app.oam.dev/component": context.name
				ports: [
					for k, v in parameter.http {
						name:       "port-" + strconv.FormatInt(v, 10)
						port:       v
						targetPort: v
					},
				]
			}
		}
	}

	let ingressOutputName = "ingress" + nameSuffix
	let ingressMetaName = context.name + nameSuffix
	legacyAPI: context.clusterVersion.minor < 19

	outputs: (ingressOutputName): {
		if legacyAPI {
			apiVersion: "networking.k8s.io/v1beta1"
		}
		if !legacyAPI {
			apiVersion: "networking.k8s.io/v1"
		}
		kind: "Ingress"
		metadata: {
			name: "\(ingressMetaName)"
			annotations: {
				if !parameter.classInSpec {
					"kubernetes.io/ingress.class": parameter.class
				}
				if parameter.gatewayHost != _|_ {
					"ingress.controller/host": parameter.gatewayHost
				}
				if parameter.annotations != _|_ {
					for key, value in parameter.annotations {
						"\(key)": "\(value)"
					}
				}
			}
			labels: {
				if parameter.labels != _|_ {
					for key, value in parameter.labels {
						"\(key)": "\(value)"
					}
				}
			}
		}
		spec: {
			if parameter.classInSpec {
				ingressClassName: parameter.class
			}
			if parameter.secretName != _|_ {
				tls: [{
					hosts: [
						parameter.domain,
					]
					secretName: parameter.secretName
				}]
			}
			rules: [{
				if parameter.domain != _|_ {
					host: parameter.domain
				}
				http: paths: [
					for k, v in parameter.http {
						path:     k
						pathType: parameter.pathType
						backend: {
							if legacyAPI {
								serviceName: serviceMetaName
								servicePort: v
							}
							if !legacyAPI {
								service: {
									name: serviceMetaName
									port: number: v
								}
							}
						}
					},
				]
			}]
		}
	}

	parameter: {
		// +usage=Specify the domain you want to expose
		domain?: string

		// +usage=Specify the mapping relationship between the http path and the workload port
		http: [string]: int

		// +usage=Specify the class of ingress to use
		class: *"nginx" | string

		// +usage=Set ingress class in '.spec.ingressClassName' instead of 'kubernetes.io/ingress.class' annotation.
		classInSpec: *false | bool

		// +usage=Specify the secret name you want to quote to use tls.
		secretName?: string

		// +usage=Specify the host of the ingress gateway, which is used to generate the endpoints when the host is empty.
		gatewayHost?: string

		// +usage=Specify a unique name for this gateway, required to support multiple gateway traits on a component
		name?: string

		// +usage=Specify a pathType for the ingress rules, defaults to "ImplementationSpecific"
		pathType: *"ImplementationSpecific" | "Prefix" | "Exact"

		// +usage=Specify the annotations to be added to the ingress
		annotations?: [string]: string

		// +usage=Specify the labels to be added to the ingress
		labels?: [string]: string

		// +usage=If specified, use an existing Service rather than creating one
		existingServiceName?: string
	}
}
