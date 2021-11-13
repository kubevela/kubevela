# ComponentTrait Composing and Trait CR Naming

* Owner: Jianbo Sun (@wonderflow), Zhou Zheng Xi (@zzxwill)
* Reviewers: KubeVela/Crossplane Maintainers
* Status: Draft
* Notice: Manual scaler is deprecated. See this [issue](https://github.com/oam-dev/kubevela/issues/2262) to get more information.
## Background

Now definition name is no longer coupled with CRD name, it's align to capability name  in KubeVela,
and two TraitDefinition  resources can both refer to the same CRD. So it's necessary to specify how to assemble trait CR
in ApplicationConfiguration and to set the CR name in a friendly way.

## Several ways to assemble trait CR in ApplicationConfiguration

- If the name of TraitDefinition is the same with the referenced CRD

The normal way to assemble a trait CR works as below.

```
apiVersion: core.oam.dev/v1alpha2
kind: ApplicationConfiguration
metadata:
  name: example-appconfig
spec:
  components:
    - componentName: example-component
      traits:
        - trait:
            apiVersion: core.oam.dev/v1alpha2
            kind: ManualScalerTrait
            spec:
              replicaCount: 3
```

- If the name of TraitDefinition is different to that of Trait CRD

The definition name `autoscale` is different to the trait name `autoscalers.standard.oam.dev`, we have two ways to compose ComponentTrait.

```
apiVersion: core.oam.dev/v1alpha2
kind: TraitDefinition
metadata:
  name: autoscale
spec:
  definitionRef:
    name: autoscalers.standard.oam.dev
```

1) Use label `trait.oam.dev/type`

```
apiVersion: core.oam.dev/v1alpha2
kind: ApplicationConfiguration
metadata:
  name: example-appconfig
spec:
  components:
    - componentName: example-component
      traits:
        - trait:
            apiVersion: standard.oam.dev/v1alpha2
            kind: Autoscalers
            metadata:
              labels:
                trait.oam.dev/type: autoscale
            spec:
              replicaCount: 3
```

When rendering trait, the TraitDefinition could be retrieved by the label.

2) Use `name` field

```
apiVersion: core.oam.dev/v1alpha2
kind: ApplicationConfiguration
metadata:
  name: example-appconfig
spec:
  components:
    - componentName: example-component
      traits:
        - trait:
            apiVersion: standard.oam.dev/v1alpha2
            kind: Autoscalers
            name: autoscale
            spec:
              replicaCount: 3
```

The mutating handler will, at the very beginning,  convert `name:autoscale` to the labels above.

In summary, among these two ways to compose ComponentTrait, using labeling is recommended.

While in KubeVela, you don't need to worry about either `label` or `name`, just use command `vela TraitType` or `vela up` to attach the trait to
a component.

## Set the CR name in a friendly way

Currently, we named all trait name in the format of `${ComponentName}-trait-${HashTag}`. This will lead to confusions
when listing CRs of all Traits of a component. 

In this proposal, we propose to change the naming rule to `${ComponentName}-${TraitDefinitionName}-${HashTag}`.

For example, if the name of TraitDefinition is `autoscale`, we name the trait CR to `example-component-autoscale-xyzfa32r`.

```
apiVersion: core.oam.dev/v1alpha2
kind: ApplicationConfiguration
metadata:
  name: example-appconfig
spec:
  components:
    - componentName: example-component
      traits:
        - trait:
            apiVersion: standard.oam.dev/v1alpha2
            kind: Autoscalers
            metadata:
              labels:
                trait.oam.dev/type: autoscale
            spec:
              replicaCount: 3
```

If the name is the same as the CRD name, like `manualscalertraits.core.oam.dev`, we name it to `example-component-manualscalertraits-rq234rrw`
by choosing the first part.

```
apiVersion: core.oam.dev/v1alpha2
kind: ApplicationConfiguration
metadata:
  name: example-appconfig
spec:
  components:
    - componentName: example-component
      traits:
        - trait:
            apiVersion: core.oam.dev/v1alpha2
            kind: ManualScalerTrait
            spec:
              replicaCount: 3

---
apiVersion: core.oam.dev/v1alpha2
kind: TraitDefinition
metadata:
  name: manualscalertraits.core.oam.dev
spec:
  workloadRefPath: spec.workloadRef
  definitionRef:
    name: manualscalertraits.core.oam.dev
```

After OAM Kubernetes Runtime is upgraded, the old trait CR name `example-component-trait-uewf77eu` will stay unchanged for previous version compatibility.
