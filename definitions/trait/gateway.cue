"gateway": {
    type: "trait"
    annotations: {}
    labels: {}
    description: "Enable public web traffic for the component, the ingress API matches K8s v1.20+."
    attributes: {
        podDisruptive: false
        status: {
            customStatus: #"""
                let igs = context.outputs.ingress.status.loadBalancer.ingress
                if igs == _|_ {
                  message: "No loadBalancer found, visiting by using 'vela port-forward " + context.appName + "'\n"
                }
                if len(igs) > 0 {
                  if igs[0].ip != _|_ {
                      message: "Visiting URL: " + context.outputs.ingress.spec.rules[0].host + ", IP: " + igs[0].ip
                  }
                  if igs[0].ip == _|_ {
                      message: "Visiting URL: " + context.outputs.ingress.spec.rules[0].host
                  }
                }
                """#
            healthPolicy: #"""
                isHealth: len(context.outputs.service.spec.clusterIP) > 0
                """#
        }
    }

    template: {
        outputs: service: {
            apiVersion: "v1"
            kind:       "Service"
            metadata: name: context.name
            spec: {
                selector: "app.oam.dev/component": context.name
                ports: [
                    for k, v in parameter.http {
                        port:       v
                        targetPort: v
                    },
                ]
            }
        }

        if parameter.http != _|_ {
            outputs: ingress: {
                apiVersion: "networking.k8s.io/v1"
                kind:       "Ingress"
                metadata: {
                    name: context.name
                    annotations: {
                        "kubernetes.io/ingress.class": "nginx"
                    }
                }
                spec: {
                    ingressClassName: "nginx"
                    rules: [
                        for i, rule in parameter.http {
                            {
                                host: rule.domain
                                http: paths: [
                                    for k, v in rule.paths {
                                        path:     v.path
                                        pathType: "ImplementationSpecific"
                                        backend: service: {
                                            name: context.name
                                            port: number: v.port
                                        }
                                    },
                                ]
                            }
                        },
                    ]
                }
            }
        }

        // Shared ConfigMap handling for TCP with proper merging
        if parameter.tcp != _|_ {
            outputs: tcpConfig: {
                apiVersion: "v1"
                kind:       "ConfigMap"
                metadata: {
                    name:      "tcp-services"
                    namespace: "ingress-nginx"
                }
                // Use patch merge strategy to ensure entries are merged, not replaced
                $patch: "merge"
                data: {
                    // Use existing resource data if available
                    if context.outputs.tcpConfigPrevious != _|_ {
                        // Preserve existing data entries
                        for k, v in context.outputs.tcpConfigPrevious.data {
                            "\(k)": v
                        }
                    }
                    // Add new entries from this application
                    for k, v in parameter.tcp {
                        "\(v.gatewayPort)": context.namespace + "/" + context.name + ":" + v.port
                    }
                }
            }
            
            // Fetch existing ConfigMap for future reference
            outputs: tcpConfigPrevious: {
                apiVersion: "v1"
                kind:       "ConfigMap"
                metadata: {
                    name:      "tcp-services"
                    namespace: "ingress-nginx"
                }
                $type: "raw"
                // Mark for read-only operations, not create/update
                $read: true
            }
        }

        // Shared ConfigMap handling for UDP with proper merging
        if parameter.udp != _|_ {
            outputs: udpConfig: {
                apiVersion: "v1"
                kind:       "ConfigMap"
                metadata: {
                    name:      "udp-services"
                    namespace: "ingress-nginx"
                }
                // Use patch merge strategy to ensure entries are merged, not replaced
                $patch: "merge"
                data: {
                    // Use existing resource data if available
                    if context.outputs.udpConfigPrevious != _|_ {
                        // Preserve existing data entries
                        for k, v in context.outputs.udpConfigPrevious.data {
                            "\(k)": v
                        }
                    }
                    // Add new entries from this application
                    for k, v in parameter.udp {
                        "\(v.gatewayPort)": context.namespace + "/" + context.name + ":" + v.port
                    }
                }
            }
            
            // Fetch existing ConfigMap for future reference
            outputs: udpConfigPrevious: {
                apiVersion: "v1"
                kind:       "ConfigMap"
                metadata: {
                    name:      "udp-services"
                    namespace: "ingress-nginx"
                }
                $type: "raw"
                // Mark for read-only operations, not create/update
                $read: true
            }
        }
    }

    parameter: {
        http?: [...{
            domain: string
            paths: [...{
                path: "/" | string
                port: int
            }]
        }]
        tcp?: [...{
            gatewayPort: int
            port: int
        }]
        udp?: [...{
            gatewayPort: int
            port: int
        }]
    }
}
