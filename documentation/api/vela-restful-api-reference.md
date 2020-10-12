# vela Restful API Reference

# API response introduction
###  The API response contains two keys: `code` and `data`
The API response is in `json` format which contains two keys: `code` and `data`.
```json
{
	"code": RESPONSE_CODE,
	"data": RESPONSE_DATA
}
```

### `code` represents the status of a request
The type of `code` is `Number`, currently it allows two codes.
- 200 - StatusOK
- 500 - StatusInternalServerError

### `data` represents the response data.
- code == 500  

`data` contains the error message.

- code == 200

The type of `data` is `Object` or `Object` list if method is `Get`, or is `String` type the content of which represents
the successful message.

# API list
## Env
### POST /api/envs/ (env init)
- example

sample request:
```json
{
  "envName": "e2",
  "namespace": "e2",
  "email": "e@gmail.com",
  "domain": "d.com"
}
```

sample response:
```json
{"code":200,"data":"environment e2 created, Namespace: e2, Email: e@gmail.com."}
```

### GET /api/envs/ (env list)
- Response
`current` of the elment of `data` indicates the currently using environment.

- example

sample response:
```json
{
	"code": 200,
	"data": [{
		"envName": "ccc",
		"current": "*",
		"namespace": "ccc"
	}, {
		"envName": "default",
		"namespace": "default"
	}, {
		"envName": "env-application",
		"namespace": "env-application"
	}, {
		"envName": "env-hello",
		"namespace": "env-hello"
	}, {
		"envName": "env-poc",
		"namespace": "env-poc"
	}, {
		"envName": "env-trait",
		"namespace": "env-trait"
	}, {
		"envName": "env-workload",
		"namespace": "env-workload"
	}]
}
```

### Get /api/envs/:envName (env description)
- example
sample response
```json
{"code":200,"data":[{"envName":"ccc","namespace":"ccc"}]}
```

### DELETE /api/envs/:envName (env delete)
- example
sample response
```json
{
"code": 200,
"data": "abcd deleted"
}
```

```json
{
"code": 500,
"data": "you can't delete current using env abc"
}
```

### PATCH /api/envs/:envName (env set)
- example
sample response
```json
{"code":200,"data":"Set environment succeed"}
```

```json
{"code":500,"data":"abcd not exist"}
```

### PUT /api/envs/:envName (env update)
- example

sample request:
```json
{"namespace":"ccc"}
```

sample response:
```json
{
"code": 200,
"data": "Update env succeed"
}
```

```json
{
"code":500,
"data":"env <envName> does not exist"
}
```

## Application
### GET /api/envs/default/apps/ (app list)
- response
`status` of the element of `data` represents the status of the application
    * True：normal
    * False：fatal
    * UNKNOWN：deploying

- example
sample response
```json
{
	"code": 200,
	"data": [{
		"name": "poc2040",
		"workload": "ContainerizedWorkload",
		"status": "True",
		"created": "2020-08-17 15:09:27 +0800 CST"
	}]
}
```

```json
{
	"code": 500,
	"data": "hit some issues"
}
```

### GET /api/envs/:envName/apps/:appName (app description)
- example
sample response
```json
{
    "code": 200,
    "data": {
        "Status": "True",
        "Components": [
            {
                "name": "web-comp",
                "Status": "True",
                "workload": {
                    "apiVersion": "standard.oam.dev/v1alpha1",
                    "kind": "PodSpecWorkload",
                    "metadata": {
                        "name": "web-comp"
                    },
                    "spec": {
                        "podSpec": {
                            "containers": [
                                {
                                    "image": "nginx:1.9.4",
                                    "name": "web-comp",
                                    "ports": [
                                        {
                                            "containerPort": 80,
                                            "name": "default",
                                            "protocol": "TCP"
                                        }
                                    ]
                                }
                            ]
                        },
                        "replicas": 1
                    }
                },
                "Traits": [
                    {
                        "trait": {
                            "apiVersion": "standard.oam.dev/v1alpha1",
                            "kind": "Route",
                            "metadata": {
                                "annotations": {
                                    "trait.oam.dev/name": "route"
                                }
                            },
                            "spec": {
                                "backend": {
                                    "port": 0
                                },
                                "host": "web-comp.poc.oam.dev",
                                "path": "",
                                "tls": {
                                    "issuerName": "oam-env-default"
                                }
                            }
                        }
                    }
                ]
            },
            {
                "name": "comp1",
                "Status": "True",
                "workload": {
                    "apiVersion": "standard.oam.dev/v1alpha1",
                    "kind": "PodSpecWorkload",
                    "metadata": {
                        "name": "comp1"
                    },
                    "spec": {
                        "podSpec": {
                            "containers": [
                                {
                                    "image": "nginx:1.9.4",
                                    "name": "comp1",
                                    "ports": [
                                        {
                                            "containerPort": 6379,
                                            "name": "default",
                                            "protocol": "TCP"
                                        }
                                    ]
                                }
                            ]
                        },
                        "replicas": 1
                    }
                }
            }
        ]
    }
}
```
### DELETE /api/envs/:envName/apps/:appName (app delete)
- example
sample response
```json
{
    "code": 200,
    "data": "delete apps succeed <appName> from <envName>"
}
```

## Components
### GET /api/envs/:envName/apps/:appName/components/:compName (component details)
- example
sample response
```json
{
    "code": 200,
    "data": {
        "name": "web-comp",
        "Status": "True",
        "workload": {
            "apiVersion": "standard.oam.dev/v1alpha1",
            "kind": "PodSpecWorkload",
            "metadata": {
                "name": "web-comp"
            },
            "spec": {
                "podSpec": {
                    "containers": [
                        {
                            "image": "nginx:1.9.4",
                            "name": "web-comp",
                            "ports": [
                                {
                                    "containerPort": 80,
                                    "name": "default",
                                    "protocol": "TCP"
                                }
                            ]
                        }
                    ]
                },
                "replicas": 1
            }
        },
        "Traits": [
            {
                "trait": {
                    "apiVersion": "standard.oam.dev/v1alpha1",
                    "kind": "Route",
                    "metadata": {
                        "annotations": {
                            "trait.oam.dev/name": "route"
                        }
                    },
                    "spec": {
                        "backend": {
                            "port": 0
                        },
                        "host": "web-comp.poc.oam.dev",
                        "path": "",
                        "tls": {
                            "issuerName": "oam-env-default"
                        }
                    }
                }
            }
        ]
    }
}
```

### GET /api/envs/:envName/apps/:appName/components/ (component list)
Same as `GET /api/envs/:envName/apps/:appName (app description)`.

### DELETE /api/envs/:envName/apps/:appName/components/:compName (component delete)
- example
sample response
```
{
"code": 200,
"data": "delete apps succeed a1 from default"
}
```


## Workloads
### POST /api/workloads/ (workload create, component create)
- parameters
```go
type WorkloadRunBody struct {
	EnvName      string       `json:"env_name"`
	WorkloadType string       `json:"workload_type"`
	WorkloadName string       `json:"workload_name"`
	AppGroup     string       `json:"app_group,omitempty"`
	Flags        []CommonFlag `json:"flags"`
	Staging      bool         `json:"staging,omitempty"`
	Traits       []TraitBody  `json:"traits,omitempty"`
}
```

The parameter should in `TraitBody` format and `Trait` should be in the following format.
```go
type TraitBody struct {
	EnvName      string       `json:"env_name"`
	Name         string       `json:"name"`
	Flags        []CommonFlag `json:"flags"`
	WorkloadName string       `json:"workload_name"`
	AppGroup     string       `json:"app_group,omitempty"`
}
```

- example
sample request
```json
{
  "env_name": "default",
  "workload_type": "podspecworkload",
  "workload_name": "poc2",
  "flags": [
    {
      "name": "image",
      "value": "nginx:1.9.4"
    },
    {
      "name": "port",
      "value": "80"
    }
  ],
}
```
sample response
```json
{
"code": 200,
"data": "Creating App poc2 SUCCEED"
}
```

Please also specify `traits` values if need to attach a trait to several traits to the application during workload creation.
```json
{
  "env_name": "default",
  "workload_type": "podspecworkload",
  "workload_name": "poc5",
  "app_group": "",
  "flags": [
    {
      "name": "port",
      "value": "80"
    },
    {
      "name": "image",
      "value": "nginx:1.9.4"
    }
  ],
  "staging": false,
  "traits": [
    {
      "name": "scale",
      "env_name": "default",
      "workload_name": "poc5",
      "flags": [
        {
          "name": "replica",
          "value": "4"
        }
      ]
    }
  ]
}
```

### GET /api/workloads/:workloadName (workload description)
- example
sample response
```json
{
	"code": 200,
	"data": {
		"name": "podspecworkload",
		"type": "workload",
		"template": "#Template: {\n\tapiVersion: \"core.oam.dev/v1alpha2\"\n\tkind:       \"PodSpecWorkload\"\n\tmetadata: name: podspecworkload.name\n\tspec: {\n\t\tcontainers: [{\n\t\t\timage: containerized.image\n\t\t\tname:  containerized.name\n\t\t\tports: [{\n\t\t\t\tcontainerPort: containerized.port\n\t\t\t\tprotocol:      \"TCP\"\n\t\t\t\tname:          \"default\"\n\t\t\t}]\n\t\t}]\n\t}\n}\ncontainerized: {\n\tname: string\n\t// +usage=specify app image\n\t// +short=i\n\timage: string\n\t// +usage=specify port for container\n\t// +short=p\n\tport: *6379 | int\n}\n",
		"parameters": [{
			"name": "name",
			"required": true,
			"default": "",
			"type": 16
		}, {
			"name": "image",
			"short": "i",
			"required": true,
			"default": "",
			"usage": "specify app image",
			"type": 16
		}, {
			"name": "port",
			"short": "p",
			"default": 6379,
			"usage": "specify port for container",
			"type": 4
		}],
		"definition": "/Users/zhouzhengxi/.vela/capabilities/containerizedworkloads.core.oam.dev.cue",
		"crdName": "containerizedworkloads.core.oam.dev",
		"crdInfo": {
			"apiVersion": "core.oam.dev/v1alpha2",
			"kind": "ContainerizedWorkload"
		}
	}
}
```

### GET /api/workloads/ (workloads list)
- example
sample response
```json
{
	"code": 200,
	"data": [{
		"name": "podspecworkload",
		"parameters": [{
			"name": "name",
			"required": true,
			"default": "",
			"type": 16
		}, {
			"name": "image",
			"short": "i",
			"required": true,
			"default": "",
			"usage": "specify app image",
			"type": 16
		}, {
			"name": "port",
			"short": "p",
			"default": 6379,
			"usage": "specify port for container",
			"type": 4
		}]
	}, {
		"name": "deployment",
		"parameters": [{
			"name": "name",
			"required": true,
			"default": "",
			"type": 16
		}, {
			"name": "env",
			"type": 128
		}, {
			"name": "image",
			"required": true,
			"default": "",
			"type": 16
		}, {
			"name": "port",
			"default": 8080,
			"type": 4
		}]
	}]
}
```

## Trait
### POST /envs/:envName/apps/:appName/components/:compName/traits/ (attach a trait)
- example
sample request
```json
{
  "name": "scale",
  "flags": [
    {
      "name": "replica",
      "value": "4"
    }
  ]
}
```
sample response
```json
{
"code": 200,
"data": "Succeeded!"
}
```
### GET /api/traits/:traitName (trait description)
- example
sample response
```json
{
	"code": 200,
	"data": {
		"name": "manualscaler",
		"type": "trait",
		"template": "#Template: {\n\tapiVersion: \"core.oam.dev/v1alpha2\"\n\tkind:       \"ManualScalerTrait\"\n\tspec: {\n\t\treplicaCount: manualscaler.replica\n\t}\n}\nmanualscaler: {\n\t//+short=r\n\treplica: *2 | int\n}\n",
		"parameters": [{
			"name": "replica",
			"short": "r",
			"default": 2,
			"type": 4
		}],
		"definition": "/Users/zhouzhengxi/.vela/capabilities/manualscalertraits.core.oam.dev.cue",
		"crdName": "manualscalertraits.core.oam.dev",
		"appliesTo": ["containerizedworkload"],
		"crdInfo": {
			"apiVersion": "core.oam.dev/v1alpha2",
			"kind": "ManualScalerTrait"
		}
	}
}
```
### GET /api/traits/ (traits list) 

- response
`applies_to` shows the workload list which the trait can be attached.

- example
sample response
```json
{
	"code": 200,
	"data": [{
		"name": "manualscaler",
		"definition": "manualscalertraits.core.oam.dev",
		"applies_to": ["podspecworkload"]
	}, {
		"name": "rollout",
		"definition": "simplerollouttraits.extend.oam.dev",
		"applies_to": ["podspecworkload", "deployment"]
	}, {
		"name": "scale",
		"definition": "manualscalertraits.core.oam.dev",
		"applies_to": ["podspecworkload", "deployment"]
	}]
}
```

### DELETE /envs/:envName/apps/:appName/components/:compName/traits/:traitName (detach a trait)
- example
sample response
```json
{"code":200,"data":"Succeeded!"}
```

## Capability
### GET /capability-centers/ (Capability Center list)
- example
sample response
```json
{
	"code": 200,
	"data": [{
		"name": "poc",
		"url": "https://github.com/wonderflow/catalog/tree/repos/repos"
	}, {
		"name": "abc",
		"url": "http://abc.com"
	}]
}
```
### PUT /capability-centers/ (Capability Center add)
- example
sample request
```json
{"Name":"c1","Address":"https://github.com/wonderflow/catalog/tree/repos/repos"}
```
### DELETE /capability-centers/:capabilityCenterName/ (Capability Center delete)
- example
sample response
```json
{
"code": 200,
"data": "mycenter capability center removed successfully"
}
```

### PUT /capability-centers/:capabilityCenterName/capabilities/  (Capability Center sync)

- example
   - response
```json
{
"code": 200,
"data": "sync finished"
}
```


### PUT /capability-centers/:capabilityCenterName/capabilities/:capabilityName  (install a capability)
- example
sample API url
`/api/capability-centers/c1/capabilities/rollout`
sample response
```json
{"code":200,"data":"Successfully installed capability rollout from c1"}
```

### DELETE /api/capabilities/:capabilityName (delete the capability)
- example
sample response
```json
{"code":200,"data":"podspecworkload removed successfully"}
```

### GET /api/capabilities/ (capability list)
- example
sample response
```json
{
	"code": 200,
	"data": [{
		"name": "podspecworkload",
		"type": "workload",
		"template": "#Template: {\n\tapiVersion: \"core.oam.dev/v1alpha2\"\n\tkind:       \"ContainerizedWorkload\"\n\tmetadata: name: podspecworkload.name\n\tspec: {\n\t\tcontainers: [{\n\t\t\timage: containerized.image\n\t\t\tname:  containerized.name\n\t\t\tports: [{\n\t\t\t\tcontainerPort: containerized.port\n\t\t\t\tprotocol:      \"TCP\"\n\t\t\t\tname:          \"default\"\n\t\t\t}]\n\t\t}]\n\t}\n}\ncontainerized: {\n\tname: string\n\t// +usage=specify app image\n\t// +short=i\n\timage: string\n\t// +usage=specify port for container\n\t// +short=p\n\tport: *6379 | int\n}\n",
		"parameters": [{
			"name": "name",
			"required": true,
			"default": "",
			"type": 16
		}, {
			"name": "image",
			"short": "i",
			"required": true,
			"default": "",
			"usage": "specify app image",
			"type": 16
		}, {
			"name": "port",
			"short": "p",
			"default": 6379,
			"usage": "specify port for container",
			"type": 4
		}],
		"definition": "/Users/zhouzhengxi/.vela/centers/c1/.tmp/containerizedworkloads.core.oam.dev.cue",
		"crdName": "containerizedworkloads.core.oam.dev",
		"center": "c1",
		"status": "uninstalled"
	},{
		"name": "rollout",
		"type": "trait",
		"template": "#Template: {\n\tapiVersion: \"extend.oam.dev/v1alpha2\"\n\tkind:       \"SimpleRolloutTrait\"\n\tspec: {\n\t\treplica:        rollout.replica\n\t\tmaxUnavailable: rollout.maxUnavailable\n\t\tbatch:          rollout.batch\n\t}\n}\nrollout: {\n\treplica:        *3 | int\n\tmaxUnavailable: *1 | int\n\tbatch:          *2 | int\n}\n",
		"parameters": [{
			"name": "replica",
			"default": 3,
			"type": 4
		}, {
			"name": "maxUnavailable",
			"default": 1,
			"type": 4
		}, {
			"name": "batch",
			"default": 2,
			"type": 4
		}],
		"definition": "/Users/zhouzhengxi/.vela/centers/poc/.tmp/simplerollouttraits.extend.oam.dev.cue",
		"crdName": "simplerollouttraits.extend.oam.dev",
		"center": "poc",
		"appliesTo": ["podspecworkload", "deployment"],
		"status": "uninstalled"
	}]
}
```

## Others
- applications search
