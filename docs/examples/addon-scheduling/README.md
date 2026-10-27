# Affinity scheduling examples

Attach the `affinity` trait on `type: addon`, `type: module`, `type: helmchart`, or a normal workload.
It overlays PodSpec scheduling (`nodeSelector`, `affinity`, `tolerations`, ...) onto workloads,
helmchart chart resources, and (when the component output is an Application) child components
via an Application walk.

| Example | Component |
| --- | --- |
| [`addon.yaml`](addon.yaml) | `type: addon` + `affinity` trait |
| [`module.yaml`](module.yaml) | `type: module` + `affinity` trait |
| [`helmchart.yaml`](helmchart.yaml) | `type: helmchart` + `affinity` trait |
| [`addon-with-embedded-modules.yaml`](addon-with-embedded-modules.yaml) | embedded modules + `affinity` trait |

CLI ([#5362](https://github.com/kubevela/kubevela/issues/5362)):

```bash
vela addon enable <name> --nodeSelector disktype=ssd
vela addon enable <name> --scheduling-file scheduling.yaml
```

Live permutation fixtures: [`regression/`](regression/).
