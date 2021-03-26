# Component Definition

In the following tutorial, you will learn about define your own Component Definition to extend KubeVela.

Before continue, make sure you have learned the basic concept of [Definition Objects](definition-and-templates.md) in KubeVela.

Usually, there are general two kinds of capability resources you can find in K8s ecosystem.

1. Compose K8s built-in resources: in this case, you can easily use them by apply YAML files.
   This is widely used as helm charts. For example, [wordpress helm chart](https://bitnami.com/stack/wordpress/helm), [mysql helm chart](https://bitnami.com/stack/mysql/helm). 
2. CRD(Custom Resource Definition) Operator: in this case, you need to install it and create CR(Custom Resource) instance for use.
   This is widely used such as [Promethus Operator](https://github.com/prometheus-operator/prometheus-operator), [TiDB Operator](https://github.com/pingcap/tidb-operator), etc.

For both cases, they can all be extended into KubeVela as Component type.

## Extend helm chart as KubeVela Component

In this case, it's very straight forward to register a helm chart as KubeVela capabilities.

KubeVela will deploy the helm chart, and with the help of KubeVela, the extended helm charts can use all the KubeVela traits. 

Refer to ["Use Helm To Extend a Component type"](https://kubevela.io/#/en/helm/component) to learn details in this case.

## Extend CRD Operator as KubeVela Component

In this case, you're more likely to make a CUE template to do the abstraction and encapsulation.
KubeVela will render the CUE template, and deploy the rendered resources. This is the most native and powerful way in KubeVela.

Refer to ["Use CUE to extend Component type"](https://kubevela.io/#/en/cue/component) to learn details in this case.


