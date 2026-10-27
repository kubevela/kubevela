import "vela/test"

_web: {
	definition: "affinity"
	workload: {
		apiVersion: "apps/v1"
		kind:       "Deployment"
		metadata: name: "web"
		spec: template: spec: containers: [{name: "web", image: "shop:1.0"}]
	}
}

_term: {labelSelector: matchLabels: app: "cache", topologyKey: "kubernetes.io/hostname"}

"required pod affinity lands on the pod spec": test.#TraitRender & _web & {
	parameter: podAffinity: required: [_term]
	expect: output: spec: template: spec: {
		containers: [{name: "web", image: "shop:1.0"}]
		affinity: {
			podAffinity: {
				requiredDuringSchedulingIgnoredDuringExecution: [{
					labelSelector: matchLabels: app: "cache"
					topologyKey: "kubernetes.io/hostname"
				}]
				preferredDuringSchedulingIgnoredDuringExecution?: _|_
			}
		} @exact()
		tolerations?: _|_
	}
}

"preferred pod anti-affinity carries its weight and term": test.#TraitRender & _web & {
	parameter: podAntiAffinity: preferred: [{weight: 50, podAffinityTerm: _term}]
	expect: output: spec: template: spec: {
		affinity: {
			podAntiAffinity: {
				preferredDuringSchedulingIgnoredDuringExecution: [{
					weight: 50
					podAffinityTerm: {labelSelector: matchLabels: app: "cache", topologyKey: "kubernetes.io/hostname"}
				}]
				requiredDuringSchedulingIgnoredDuringExecution?: _|_
			}
		} @exact()
	}
}

"preferred pod affinity carries its weight and term": test.#TraitRender & _web & {
	parameter: podAffinity: preferred: [{weight: 50, podAffinityTerm: _term}]
	expect: output: spec: template: spec: {
		affinity: {
			podAffinity: {
				preferredDuringSchedulingIgnoredDuringExecution: [{
					weight: 50
					podAffinityTerm: {labelSelector: matchLabels: app: "cache", topologyKey: "kubernetes.io/hostname"}
				}]
				requiredDuringSchedulingIgnoredDuringExecution?: _|_
			}
		} @exact()
	}
}

"required pod anti-affinity lands on the pod spec": test.#TraitRender & _web & {
	parameter: podAntiAffinity: required: [_term]
	expect: output: spec: template: spec: {
		affinity: {
			podAntiAffinity: {
				requiredDuringSchedulingIgnoredDuringExecution: [{
					labelSelector: matchLabels: app: "cache"
					topologyKey: "kubernetes.io/hostname"
				}]
				preferredDuringSchedulingIgnoredDuringExecution?: _|_
			}
		} @exact()
	}
}

"namespaces and a namespace selector are passed through": test.#TraitRender & _web & {
	parameter: podAffinity: required: [{
		topologyKey: "zone"
		namespaces: ["shop", "cache"]
		namespaceSelector: matchLabels: team: "shop"
	}]
	expect: output: spec: template: spec: affinity: podAffinity: requiredDuringSchedulingIgnoredDuringExecution: [{
		topologyKey: "zone"
		namespaces: ["shop", "cache"]
		namespaceSelector: matchLabels: team: "shop"
		labelSelector?: _|_
	}]
}

"a label selector expression defaults to In": test.#TraitRender & _web & {
	parameter: podAffinity: required: [{
		topologyKey: "zone"
		labelSelector: matchExpressions: [{key: "app", values: ["cache"]}]
	}]
	expect: output: spec: template: spec: affinity: podAffinity: requiredDuringSchedulingIgnoredDuringExecution: [{
		labelSelector: matchExpressions: [{key: "app", operator: "In", values: ["cache"]}]
	}]
}

"node affinity takes selector terms and preferences": test.#TraitRender & _web & {
	parameter: nodeAffinity: {
		required: nodeSelectorTerms: [{matchExpressions: [{key: "disktype", values: ["ssd"]}]}]
		preferred: [{weight: 10, preference: matchFields: [{key: "metadata.name", operator: "NotIn", values: ["node-1"]}]}]
	}
	expect: output: spec: template: spec: {
		affinity: {
			nodeAffinity: {
				requiredDuringSchedulingIgnoredDuringExecution: nodeSelectorTerms: [{
					matchExpressions: [{key: "disktype", operator: "In", values: ["ssd"]}]
					matchFields?: _|_
				}]
				preferredDuringSchedulingIgnoredDuringExecution: [{
					weight: 10
					preference: matchFields: [{key: "metadata.name", operator: "NotIn", values: ["node-1"]}]
				}]
			}
		} @exact()
	}
}

"tolerations default to Equal and keep only what is given": test.#TraitRender & _web & {
	parameter: tolerations: [
		{key: "dedicated", value: "shop", effect: "NoSchedule"},
		{operator: "Exists", effect: "NoExecute", tolerationSeconds: 300},
	]
	expect: output: spec: template: spec: {
		tolerations: [
			{key: "dedicated", operator: "equal", value: "shop", effect: "NoSchedule", tolerationSeconds?: _|_},
			{operator: "Exists", effect: "NoExecute", tolerationSeconds: 300, key?: _|_, value?: _|_},
		]
		affinity?: _|_
	}
}

"no parameters leave the pod spec alone": test.#TraitRender & _web & {
	expect: {
		output: spec: template: spec: {
			containers: [{name: "web", image: "shop:1.0"}]
			affinity?:    _|_
			tolerations?: _|_
		}
		outputs: {} @exact()
	}
}

"a weight above 100 is rejected": test.#TraitRender & _web & {
	parameter: podAffinity: preferred: [{weight: 101, podAffinityTerm: _term}]
	expect: error: {
		parameter: [=~"weight"] @contains()
	}
}

"nodeSelector overlays onto the pod spec": test.#TraitRender & _web & {
	parameter: nodeSelector: disktype: "ssd"
	expect: output: spec: template: spec: {
		nodeSelector: disktype: "ssd"
		containers: [{name: "web", image: "shop:1.0"}]
	}
}

"raw affinity takes precedence over wrapped nodeAffinity": test.#TraitRender & _web & {
	parameter: {
		affinity: nodeAffinity: requiredDuringSchedulingIgnoredDuringExecution: nodeSelectorTerms: [{
			matchExpressions: [{key: "node-type", operator: "In", values: ["infra"]}]
		}]
		nodeAffinity: required: nodeSelectorTerms: [{matchExpressions: [{key: "disktype", values: ["ssd"]}]}]
	}
	expect: output: spec: template: spec: {
		affinity: nodeAffinity: requiredDuringSchedulingIgnoredDuringExecution: nodeSelectorTerms: [{
			matchExpressions: [{key: "node-type", operator: "In", values: ["infra"]}]
		}]
	}
}

"a pod affinity term needs a topology key": test.#TraitRender & _web & {
	parameter: podAffinity: required: [{labelSelector: matchLabels: app: "cache"}]
	expect: error: =~"topologyKey"
}

// type: module (and inline modules) render an owned Application whose aux
// Deployments carry controllers; affinity must reach those pod templates.
_moduleApp: {
	definition: "affinity"
	workload: {
		apiVersion: "core.oam.dev/v1beta1"
		kind:       "Application"
		metadata: name: "module-provider-kit"
		spec: components: [{
			name: "provider-kit-aux"
			type: "k8s-objects"
			properties: objects: [{
				apiVersion: "apps/v1"
				kind:       "Deployment"
				metadata: name: "provider-ctrl"
				spec: template: spec: {
					containers: [{name: "ctrl", image: "provider:1"}]
				}
			}, {
				apiVersion: "apiextensions.crossplane.io/v1"
				kind:       "CompositeResourceDefinition"
				metadata: name: "xbuckets.aws.platform.io"
				spec: {}
			}]
		}]
	}
}

"module owned Application aux Deployment gets nodeSelector": test.#TraitRender & _moduleApp & {
	parameter: nodeSelector: "node-type": "infra"
	expect: output: spec: components: [{
		name: "provider-kit-aux"
		type: "k8s-objects"
		traits: [{
			type: "affinity"
			properties: nodeSelector: "node-type": "infra"
		}]
		properties: objects: [{
			metadata: name: "provider-ctrl"
			spec: template: spec: {
				nodeSelector: "node-type": "infra"
				containers: [{name: "ctrl", image: "provider:1"}]
			}
		}, {
			metadata: name: "xbuckets.aws.platform.io"
			spec: {}
		}]
	}]
}

// type: addon owned Application: child webservice must receive the affinity trait
// so its Deployment is scheduled (object-walk alone cannot see that workload).
_addonApp: {
	definition: "affinity"
	workload: {
		apiVersion: "core.oam.dev/v1beta1"
		kind:       "Application"
		metadata: name: "addon-kube-state-metrics"
		spec: components: [{
			name: "addon-kube-state-metrics-ns"
			type: "k8s-objects"
			properties: objects: [{
				apiVersion: "v1"
				kind:       "Namespace"
				metadata: name: "o11y-system"
			}]
		}, {
			name: "kube-state-metrics"
			type: "webservice"
			properties: image: "registry.k8s.io/kube-state-metrics/kube-state-metrics:v2.13.0"
		}]
	}
}

"addon owned Application upserts affinity onto webservice, skips ns helper": test.#TraitRender & _addonApp & {
	parameter: nodeSelector: {
		disktype:    "ssd"
		"node-type": "infra"
	}
	expect: output: spec: components: [{
		name: "addon-kube-state-metrics-ns"
		type: "k8s-objects"
		traits?: _|_
	}, {
		name: "kube-state-metrics"
		type: "webservice"
		traits: [{
			type: "affinity"
			properties: nodeSelector: {
				disktype:    "ssd"
				"node-type": "infra"
			}
		}]
	}]
}

"addon owned Application upserts wrapped nodeAffinity onto webservice": test.#TraitRender & _addonApp & {
	parameter: nodeAffinity: required: nodeSelectorTerms: [{
		matchExpressions: [{key: "node-type", values: ["infra"]}]
	}]
	expect: output: spec: components: [{
		name: "addon-kube-state-metrics-ns"
		traits?: _|_
	}, {
		name: "kube-state-metrics"
		type: "webservice"
		traits: [{
			type: "affinity"
			properties: nodeAffinity: required: nodeSelectorTerms: [{
				matchExpressions: [{key: "node-type", values: ["infra"]}]
			}]
		}]
	}]
}

"addon owned Application upserts combo nodeSelector+tolerations": test.#TraitRender & _addonApp & {
	parameter: {
		nodeSelector: disktype: "ssd"
		tolerations: [{key: "dedicated", operator: "Exists", effect: "NoSchedule"}]
	}
	expect: output: spec: components: [{
		name: "addon-kube-state-metrics-ns"
		traits?: _|_
	}, {
		name: "kube-state-metrics"
		traits: [{
			type: "affinity"
			properties: {
				nodeSelector: disktype: "ssd"
				tolerations: [{key: "dedicated", operator: "Exists", effect: "NoSchedule"}]
			}
		}]
	}]
}

"module aux Deployment gets raw affinity and tolerations": test.#TraitRender & _moduleApp & {
	parameter: {
		affinity: nodeAffinity: requiredDuringSchedulingIgnoredDuringExecution: nodeSelectorTerms: [{
			matchExpressions: [{key: "node-type", operator: "In", values: ["infra"]}]
		}]
		tolerations: [{key: "dedicated", operator: "Exists", effect: "NoSchedule"}]
	}
	expect: output: spec: components: [{
		name: "provider-kit-aux"
		traits: [{
			type: "affinity"
			properties: {
				affinity: nodeAffinity: requiredDuringSchedulingIgnoredDuringExecution: nodeSelectorTerms: [{
					matchExpressions: [{key: "node-type", operator: "In", values: ["infra"]}]
				}]
				tolerations: [{key: "dedicated", operator: "Exists", effect: "NoSchedule"}]
			}
		}]
		properties: objects: [{
			metadata: name: "provider-ctrl"
			spec: template: spec: {
				affinity: nodeAffinity: requiredDuringSchedulingIgnoredDuringExecution: nodeSelectorTerms: [{
					matchExpressions: [{key: "node-type", operator: "In", values: ["infra"]}]
				}]
				tolerations: [{key: "dedicated", operator: "Exists", effect: "NoSchedule"}]
				containers: [{name: "ctrl", image: "provider:1"}]
			}
		}, {
			metadata: name: "xbuckets.aws.platform.io"
			spec: {}
		}]
	}]
}

"primary Deployment patch overlays existing nodeSelector": test.#TraitRender & {
	definition: "affinity"
	workload: {
		apiVersion: "apps/v1"
		kind:       "Deployment"
		metadata: name: "chart-ctrl"
		spec: template: spec: {
			nodeSelector: disktype: "hdd"
			containers: [{name: "c", image: "chart:1"}]
		}
	}
	parameter: nodeSelector: {
		disktype:    "ssd"
		"node-type": "infra"
	}
	expect: output: spec: template: spec: {
		nodeSelector: {
			disktype:    "ssd"
			"node-type": "infra"
		}
		containers: [{name: "c", image: "chart:1"}]
	}
}

"module aux Deployment overlays prior nodeSelector without CUE conflict": test.#TraitRender & {
	definition: "affinity"
	workload: {
		apiVersion: "core.oam.dev/v1beta1"
		kind:       "Application"
		metadata: name: "module-provider-kit"
		spec: components: [{
			name: "provider-kit-aux"
			type: "k8s-objects"
			properties: objects: [{
				apiVersion: "apps/v1"
				kind:       "Deployment"
				metadata: name: "provider-ctrl"
				spec: template: spec: {
					nodeSelector: disktype: "hdd"
					containers: [{name: "ctrl", image: "provider:1"}]
				}
			}]
		}]
	}
	parameter: nodeSelector: {
		disktype:    "ssd"
		"node-type": "infra"
	}
	expect: output: spec: components: [{
		name: "provider-kit-aux"
		properties: objects: [{
			metadata: name: "provider-ctrl"
			spec: template: spec: {
				nodeSelector: {
					disktype:    "ssd"
					"node-type": "infra"
				}
				containers: [{name: "ctrl", image: "provider:1"}]
			}
		}]
	}]
}

"Application walk preserves inputs/outputs on child components": test.#TraitRender & {
	definition: "affinity"
	workload: {
		apiVersion: "core.oam.dev/v1beta1"
		kind:       "Application"
		metadata: name: "addon-with-meta"
		spec: components: [{
			name: "web"
			type: "webservice"
			inputs: [{from: "cfg", parameterKey: "env"}]
			outputs: [{name: "url", valueFrom: "status.url"}]
			properties: image: "web:1"
		}]
	}
	parameter: nodeSelector: disktype: "ssd"
	expect: output: spec: components: [{
		name: "web"
		type: "webservice"
		inputs: [{from: "cfg", parameterKey: "env"}]
		outputs: [{name: "url", valueFrom: "status.url"}]
		traits: [{
			type: "affinity"
			properties: nodeSelector: disktype: "ssd"
		}]
	}]
}

"schedulerName and priorityClassName land on the pod spec": test.#TraitRender & _web & {
	parameter: {
		schedulerName:      "default-scheduler"
		priorityClassName:  "system-cluster-critical"
		nodeSelector: zone: "a"
	}
	expect: output: spec: template: spec: {
		schedulerName:     "default-scheduler"
		priorityClassName: "system-cluster-critical"
		nodeSelector: zone: "a"
	}
}

"raw affinity.podAffinity and podAntiAffinity pass through": test.#TraitRender & _web & {
	parameter: affinity: {
		podAffinity: requiredDuringSchedulingIgnoredDuringExecution: [{
			labelSelector: matchLabels: app: "cache"
			topologyKey: "kubernetes.io/hostname"
		}]
		podAntiAffinity: preferredDuringSchedulingIgnoredDuringExecution: [{
			weight: 10
			podAffinityTerm: {
				labelSelector: matchLabels: app: "web"
				topologyKey: "kubernetes.io/hostname"
			}
		}]
	}
	expect: output: spec: template: spec: {
		affinity: {
			podAffinity: requiredDuringSchedulingIgnoredDuringExecution: [{
				labelSelector: matchLabels: app: "cache"
				topologyKey: "kubernetes.io/hostname"
			}]
			podAntiAffinity: preferredDuringSchedulingIgnoredDuringExecution: [{
				weight: 10
				podAffinityTerm: {
					labelSelector: matchLabels: app: "web"
					topologyKey: "kubernetes.io/hostname"
				}
			}]
		}
	}
}

"topologySpreadConstraints land on the pod spec": test.#TraitRender & _web & {
	parameter: topologySpreadConstraints: [{
		maxSkew:           1
		topologyKey:       "kubernetes.io/hostname"
		whenUnsatisfiable: "DoNotSchedule"
		labelSelector: matchLabels: app: "web"
		minDomains:          2
		matchLabelKeys: ["pod-template-hash"]
		nodeAffinityPolicy: "Honor"
		nodeTaintsPolicy:   "Honor"
	}]
	expect: output: spec: template: spec: {
		topologySpreadConstraints: [{
			maxSkew:           1
			topologyKey:       "kubernetes.io/hostname"
			whenUnsatisfiable: "DoNotSchedule"
			labelSelector: matchLabels: app: "web"
			minDomains:          2
			matchLabelKeys: ["pod-template-hash"]
			nodeAffinityPolicy: "Honor"
			nodeTaintsPolicy:   "Honor"
		}]
	}
}

