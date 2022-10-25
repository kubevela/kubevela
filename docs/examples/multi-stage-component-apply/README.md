# MultiStageComponentApply

This example shows how to enable MultiStageComponentApply, the MultiStageComponentApply feature will be combined with the stage field in TraitDefinition to complete the multi-stage apply. Currently, the stage field in TraitDefinition is an optional parameter, which provides `PreDispatch` and `PostDispatch`.

## How to use multi-stage
> The future-gate is still in alpha stage, and it is recommended to use it only in short-term test clusters.

The `MultiStageComponentApply` is not enabled by default, you need some extra works to use it. 

1. Add an args `--feature-gates=MultiStageComponentApply=ture` in KubeVela controller's deployment like:

```yaml
    spec:
      containers:
        - args:
            - --feature-gates=MultiStageComponentApply=true
          ...
```

2. Sometime, you have multi-stage apply requirements inside the component, and it is the `outputs` resource defined in the trait. In this case,  you can use the `stage` with the value `PreDispatch` or `PostDispatch` like:

```yaml
  apiVersion: core.oam.dev/v1beta1
  kind: TraitDefinition
  metadata:
    annotations:
      definition.oam.dev/description: Add storages on K8s pod for your workload which follows the pod spec in path 'spec.template'.
    name: storage
    namespace: vela-system
  spec:
    appliesToWorkloads:
      - deployments.apps
      - statefulsets.apps
      - daemonsets.apps
      - jobs.batch
    podDisruptive: true
    stage: PreDispatch
    schematic:
      cue:
        template: |
          ...
```

