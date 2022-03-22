# Requirement

Here are some configurations, like image registry、Helm Chart repository、Dex Connector and Terraform Provider, which
need to be preconfigured.

- Image registry

When a user is about to create a [webservice](https://kubevela.net/docs/next/end-user/components/cue/webservice) type application,
a preconfigured image registry will help him/her to quickly choose an image and its tag from the registry.

- Helm Chart repository

When a user is about to create a [helm](https://kubevela.net/docs/next/end-user/components/helm) type application,
a preconfigured helm chart repository will help him/her to quickly choose a helm chart and its version from the repository.

- Dex Connector

To preconfigure a [Dex Connector](https://dexidp.io/docs/connectors/).

- Terraform provider

End users need to authenticate multiple Terraform cloud providers to catalog different cloud resources in different tenants.

# Proposal

## Define all configuration with different ComponentDefinitions

Use four different ComponentDefinitions to define those configurations above, and all of them will be defined in vela core
chart except for Terraform providers which will be delivered with Terraform provider addons.

### 1. Admins: store configuration

Admins are responsible for storing configurations image registry, Helm Chart repository, Dex Connector and Terraform Provider,
which will generated a secret respectively.

All secrets are shared with these labels:

```yaml
labels: {
				"config.oam.dev/catalog": "velacore-config"
				"config.oam.dev/config-type": "image-registry"
                "config.oam.dev/multi-cluster": "true"
			}
```

`config.oam.dev/catalog` indicates the secret is belongs to configurations.
`config.oam.dev/config-type` indicates the secret is kind of configurations, like image registry.
If `config.oam.dev/multi-cluster` is `true`, it indicates the secret will be delivered to working clusters if needed.

Besides, here is a custom for each configuration type. For example, the label before marks the URL for
an image registry.
```yaml
				"config.oam.dev/registry-url": "reg.docker.alibaba-inc.com/some-org"
```

- Image registry


```
import (
	"encoding/base64"
	"encoding/json"
)

"config-image-registry": {
	type: "component"
	annotations: {}
	labels: {
		"catalog.config.oam.dev":       "velacore-config"
		"type.config.oam.dev":          "image-registry"
		"multi-cluster.config.oam.dev": "true"
	}
	description: "Config information to authenticate image registry"
}

template: {
	output: {
		apiVersion: "v1"
		kind:       "Secret"
		metadata: {
			name:      parameter.name
			namespace: "vela-system"
			labels: {
				"config.oam.dev/catalog":       "velacore-config"
				"config.oam.dev/type":          "image-registry"
				"config.oam.dev/multi-cluster": "true"
				if parameter.accountAuth != _|_ {
					"config.oam.dev/identifier": parameter.accountAuth.registry
				}
				if parameter.noAuth != _|_ {
					"config.oam.dev/identifier": parameter.noAuth.registry
				}
			}
		}
		type: "kubernetes.io/dockerconfigjson"
		stringData: {
			if parameter.accountAuth != _|_ {
				".dockerconfigjson": accountAuthDockerConfig
			}
		}
	}

	accountAuthDockerConfig: json.Marshal({
		"auths": "\(reg)": {
			"username": parameter.accountAuth.username
			"password": parameter.accountAuth.password
			"email":    parameter.accountAuth.email
			"auth":     base64.Encode(null, (parameter.accountAuth.username + ":" + parameter.accountAuth.password))
		}
	})

	aaa: base64.Encode(null, parameter.accountAuth.username)

	reg: parameter.accountAuth.registry

	parameter: {
		name: string
		noAuth?: {
			registry: string
		}
		accountAuth?: {
			// +usage=Private Image registry FQDN
			registry: string
			// +usage=Image registry username.
			username: string
			// +usage=Image registry password.
			password: string
			// +usage=Image registry email.
			email: string
		}
	}

}

```

- Helm chart repository

```shell
import (
	"encoding/base64"
)

"config-helm-repository": {
	type: "component"
	annotations: {}
	labels: {
		"catalog.config.oam.dev": "velacore-config"
		"multi-cluster.config.oam.dev": "true"
		"type.config.oam.dev":    "helm-repository"
	}
	description: "Config information to authenticate helm chart repository"
}

template: {
	output: {
		apiVersion: "v1"
		kind:       "Secret"
		metadata: {
			name:      parameter.name
			namespace: "vela-system"
			labels: {
				"config.oam.dev/catalog": "velacore-config"
				"config.oam.dev/type":    "helm-repository"
				"config.oam.dev/multi-cluster": "true"
				"config.oam.dev/identifier":  parameter.registry
			}
		}
		type: "Opaque"

		if parameter.https != _|_ {
			stringData: parameter.https
		}
		if parameter.ssh != _|_ {
			stringData: parameter.ssh
		}
	}

	parameter: {
		oss?: {
			bucket:   string
			endpoint: string
		}
		https?: {
			url:      string
			username: string
			password: string
		}
		// +usage=https://fluxcd.io/legacy/helm-operator/helmrelease-guide/chart-sources/#ssh
		ssh?: {
			url:      string
			identity: string
		}
	}
}


```

- Dex Connector

```shell
"config-dex-connector": {
	type: "component"
	annotations: {}
	labels: {
		"catalog.config.oam.dev": "velacore-config"
		"type.config.oam.dev":    "dex-connector"
		"multi-cluster.config.oam.dev": "false"
	}
	description: "Config information to authenticate Dex connectors"
}

template: {
	output: {
		apiVersion: "v1"
		kind:       "Secret"
		metadata: {
			name:      parameter.name
			namespace: "vela-system"
			labels: {
				"config.oam.dev/catalog": "velacore-config"
				"config.oam.dev/type":    "dex-connector"
				"config.oam.dev/multi-cluster": "false"
				"config.oam.dev/identifier":             parameter.name
			}
		}
		type: "Opaque"

		if parameter.github != _|_ {
			stringData: parameter.github
		}
		if parameter.ldap != _|_ {
			stringData: parameter.ldap
		}
	}

	parameter: {
		name: string
		github?: {
			clientID:     string
			clientSecret: string
			callbackURL:  string
		}
		ldap?: {
			host:               string
			insecureNoSSL:      *true | bool
			insecureSkipVerify: bool
			startTLS:           bool
			usernamePrompt:     string
			userSearch: {
				baseDN:    string
				username:  string
				idAttr:    string
				emailAttr: string
				nameAttr:  string
			}
		}
	}
}


```

- Terraform provider

Let's take an example for Terraform Provider for Baidu Cloud.

```shell
import "strings"

"terraform-baidu": {
	type: "component"
	annotations: {}
	labels: {
		"type.config.oam.dev": "terraform-provider"
	}
	description: "Terraform Provider for Baidu Cloud"
}

template: {
	output: {
		apiVersion: "terraform.core.oam.dev/v1beta1"
		kind:       "Provider"
		metadata: {
			name:      parameter.name
			namespace: "default"
			labels: l
		}
		spec: {
			provider: "baidu"
			region:   parameter.BAIDUCLOUD_REGION
			credentials: {
				source: "Secret"
				secretRef: {
					namespace: "vela-system"
					name:      parameter.name + "-account-creds"
					key:       "credentials"
				}
			}
		}
	}

	outputs: {
		"credential": {
			apiVersion: "v1"
			kind:       "Secret"
			metadata: {
				name:      parameter.name + "-account-creds"
				namespace: "vela-system"
				labels: l
			}
			type: "Opaque"
			stringData: credentials: strings.Join([
							"accessKey: " + parameter.BAIDUCLOUD_ACCESS_KEY,
							"secretKey: " + parameter.BAIDUCLOUD_SECRET_KEY,
			], "\n")
		}
	}

	l: {
		"config.oam.dev/type": "terraform-provider"
		"config.oam.dev/provider": "terraform-baidu"
	}

	parameter: {
		//+usage=The name of Terraform Provider for Baidu Cloud, default is `baidu`
		name: *"baidu" | string
		//+usage=Get BAIDUCLOUD_ACCESS_KEY per this guide https://cloud.baidu.com/doc/Reference/s/9jwvz2egb
		BAIDUCLOUD_ACCESS_KEY: string
		//+usage=Get BAIDUCLOUD_SECRET_KEY per this guide https://cloud.baidu.com/doc/Reference/s/9jwvz2egb
		BAIDUCLOUD_SECRET_KEY: string
		//+usage=Get BAIDUCLOUD_REGION by picking one RegionId from Baidu Cloud region list https://cloud.baidu.com/doc/Reference/s/2jwvz23xx
		BAIDUCLOUD_REGION: string
	}
}

```


### 2. End-users: retrieve and use configuration

For VelaUX users, an OpenAPI will help users to choose a proper configuration and use it.


### 3. Deliver to working clusters

Configuration with label `"multi-cluster.config.oam.dev": "true"`, ie, image registry and Helm Chart Repository, will be delivered to working clusters, in which business
applications will be deployed.

Using the following application to deliver configuration secrets to specific namespaces in working clusters.

```shell
apiVersion: core.oam.dev/v1beta1
kind: Application
metadata:
  name: config-$Project
  namespace: pro
  lables: 
spec:
  components:
    - name: ns
      type: ref-objects
      properties:
        objects:
          - apiVersion: v1
            kind: Secret
            name: config-image-registry-1
  policies:
    - type: topology
      name: beijing-clusters
      properties:
        clusters: ["beijing-1"]
        namespace: ["ns1", "ns2"]
    - type: topology
      name: hangzhou-clusters
      properties:
        clusters: [ "hangzhou-1", "hangzhou-2" ]
        namespace: ["ns1", "ns2"]
```



