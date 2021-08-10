# Apply-Once-Only: Apply workload/trait only when spec is changed

- Owner: Yue Wang(@captainroy-hy), Jianbo Sun(@wonderflow)
- Date: 11/24/2020
- Status: Implemented

## Intro
When an ApplicationConfiguration is deployed, 
vela-core will create(apply) corresponding workload/trait instances and keep them stay align with the `spec` defined in ApplicationConfiguration through periodical reconciliation. 

If we run vela-core with `--apply-once-only` flag enabled, vela-core will never apply the workload and trait instance after they are applied once. Even if they are changed by others (e.g., trait controller, workload controller,etc). 
Since the create operation is the only one apply operation occurring on workload/trait, we call this mechanism as `Apply Once Only`.

## A Motivational Example
Here is a scenario from production environment to demonstrate how `Apply Once Only` works.

```yaml
apiVersion: core.oam.dev/v1alpha2
kind: ApplicationConfiguration
metadata:
  name: example-appconfig
spec:
  components:
    - componentName: example-component
---
apiVersion: core.oam.dev/v1alpha2
kind: Component
metadata:
  name: example-component
spec:
  workload:
    apiVersion: apps/v1
    kind: Deployment
    spec:
      ...
      template:
        spec:
          containers:
            - image: sample/app:1.0
              name: sample-app
```

After deploying above ApplicationConfiguration, vela-core will create a Deployment with corresponding `PodTemplateSpec`. 

In production env, it's possible to change the Deployment according to particular requirements, e.g., RolloutTrait, AutoscalerTrait,etc.
Currently, we just use `kubectl` to simulate workload is changed bypass changing the ApplicationConfiguration. 
Below cmd changes `spec.template.spec.containers[0].image` of the Deployment from `sample/app:1.0` to `sample/app:2.0`.
```shell
cat <<EOF | kubectl patch deployment example-deploy --patch
spec:
  template:
    spec:
      containers:
      - name: sample-app
        image: sample/app:2.0
EOF
```

Above change will trigger recreate of Pods owned by the Deployment. 
But vela-core will change it back to `sample/app:1.0` in the following reconciliation soon or late. 
That's not what we expect in some scenarios.

Instead, we hope vela-core ignore reconciling the workload we changed and leave them as what they are now until we change the `spec` of their parent ApplicationConfiguration .

## Goals

Add a startup parameter for vela-core controller to allow users choose whether to enable apply only once or not. 

If enabled, workload/trait will be applied only one time for each resource generation (only when the corresponding appconfig/component is created or updated).
After workload/trait created/updated and aligned to the generation of appconfig, vela-core will not apply them EXCEPT below situations:

- The `spec` of ApplicationConfiguration is changed (new generation created)
- The revision of Component is changed

By default, the mechanism is disabled, vela-core will always reconcile and apply workload/trait periodically as usual.

## Implementation

In each round of reconciliation, vela-core compares below Labels & Annotations of existing workload/trait with newly rendered ones before applying.

After deploying the ApplicationConfiguration in the motivational example, the Deployment created by vela-core will have such labels and annotations.


```yaml
apiVersion: apps/v1
kind: Deployment
metadata:
  annotations:
    deployment.kubernetes.io/revision: "1" 
    app.oam.dev/generation: "1"
  generation: 1
  labels:
    app.oam.dev/component: example-component
    app.oam.dev/name: example-appconfig
    app.oam.dev/resourceType: WORKLOAD
    app.oam.dev/revision: example-component-v1
  ...
```

- `annotations["app.oam.dev/generation"]:"1" ` refers to the generation of AppConfig
- `labels["app.oam.dev/revision"]:"example-component-v1" ` refers to the revision of Component

These crucial two are propogated from AppConfig and Component during reconciliation.
Any change applied to the Deployment directly has no impact on these labels and annotations, e.g., change the Deployment spec just like what we do in the [motivational example](#a-motivational-example).
If `--apply-once-only` is enabled, since no discrepancy is found on labels and annotations, 
vela-core controller will ignore applying the Deployment and leave it as what it is at that moment.

By contrast, changes on AppConfig (changing `spec` creates new generation) and Component (updating Component creates new revision) will change the value of these labels and annotations.
For example, if we update the spec of AppConfig, newly rendered workload is supposed to contain such labels and annotations.

```yaml
apiVersion: apps/v1
kind: Deployment
metadata:
  annotations:
    deployment.kubernetes.io/revision: "1" 
    app.oam.dev/generation: "2" # generation of AppConfig changed
  generation: 1
  labels:
    app.oam.dev/component: example-component
    app.oam.dev/name: example-appconfig
    app.oam.dev/resourceType: WORKLOAD
    app.oam.dev/revision: example-component-v1
  ...
```
Since discrepancy is found, vela-core controller will apply(update) the Deployment with newly rendered one.
Thus, the changes we made to the Deployment before will also be eliminated.

The same mechanism also works for Trait as well as Workload.

### Apply Once Only Force

Based on the same mechanism as `apply-once-only`, `apply-once-only-force` has a more strict method for apply only once.

It allows to skip re-creating a workload or trait that has already been DELETED from the cluster if its spec is not changed. 

Besides the condition in `apply-once-only`, `apply-once-only-force` has one more condition:

- if the component revision not changed, the workload will not be applied.

## Usage

Three available options are provided to a vela-core runtime setup flag named `apply-one-only`, referring to three modes:

- off - `apply-once-only` is disabled, this is the default option
- on - `apply-once-only` is enabled
- force - `apply-once-only-force` is enabled

You can set it through `helm` chart value `applyOnceOnly` which is "off" by default if omitted, for example

```shell
helm install -n vela-system kubevela ./charts/vela-core --set applyOnceOnly=on
```
or
```
helm install -n vela-system kubevela ./charts/vela-core --set applyOnceOnly=force
```


