# Source library examples

The source library itself ships with vela-core. Its definitions live in
[`vela-templates/definitions/internal/source/`](../../vela-templates/definitions/internal/source)
and are rendered into the Helm chart by `make manifests`, so `configmap-local`,
`git-file`, `http-get`, `vela-addon`, `vela-app`, `vela-component`,
`vela-config` and `vela-env` are available in any cluster running KubeVela.

What is left here are Applications that read them, one per source, small enough
to follow in a sitting:

| App | Reads |
| --- | --- |
| `configmap-app.yaml` | A ConfigMap's data from the Application's own namespace, into a component's properties |
| `git-file-app.yaml` | One file from a registry, at three different refs, and an optional file that may not exist |
| `http-get-app.yaml` | A JSON endpoint, with fields read out of the response |
| `vela-config-app.yaml` | A KubeVela Config's properties and the references it produced |

Apply one against a cluster with the chart installed:

```bash
kubectl apply -f docs/examples/source-library/configmap-app.yaml
vela status <app> --sources
```

To read the definitions themselves, read the CUE rather than the rendered YAML -
the YAML in the chart is generated, `$internal` included.
