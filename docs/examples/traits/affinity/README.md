# Affinity trait examples

The `affinity` trait overlays PodSpec scheduling fields (`nodeSelector`,
`affinity`, `tolerations`, ...) onto workloads. It also works on
`type: addon`, `type: module`, and `type: helmchart` by walking Application
outputs and overlaying chart resources.

| Example | What it shows |
| --- | --- |
| [`example.yaml`](example.yaml) | webservice + podAffinity |
| [`addon.yaml`](addon.yaml) | `type: addon` + affinity |
| [`module.yaml`](module.yaml) | `type: module` + affinity (aux controllers) |
| [`helmchart.yaml`](helmchart.yaml) | `type: helmchart` + affinity (overlays chart values) |
| [`addon-with-embedded-modules.yaml`](addon-with-embedded-modules.yaml) | embedded modules + outer affinity |

CLI ([#5362](https://github.com/kubevela/kubevela/issues/5362)):

```bash
vela addon enable <name> --nodeSelector disktype=ssd
vela addon enable <name> --scheduling-file scheduling.yaml
```

Permutation fixtures: [`regression/`](regression/).
