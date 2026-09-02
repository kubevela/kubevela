# Logging and Verbosity

## Controller flags

| Flag | Effect |
|---|---|
| `-v=<N>` | klog verbosity level. Default is `0`; most diagnostic detail lives at `2`-`4`. |
| `--dev-logs=true` | Human-readable, colorized log output instead of structured JSON. Easier to read locally. |
| `--log-debug=true` | Enables debug-level application logs (separate from klog's `-v`). |
| `--log-file-path=<path>` | Also write logs to a file, in addition to stdout. |

Combine them for a readable local session, e.g. `-v=3 --dev-logs=true
--log-file-path=./vela.log`.

## Where to look

- **Running from an IDE**: the IDE's own console/Debug Console shows stdout
  in real time.
- **Running in a cluster** (k3d or remote):

  ```bash
  kubectl logs -n vela-system -l app.kubernetes.io/name=vela-core -f
  ```

## Raising verbosity on an already-running deployment

You don't need to rebuild or redeploy to turn up verbosity temporarily. Patch
the deployment's args directly:

```bash
kubectl -n vela-system patch deployment vela-core --type=json -p='[
  {"op": "add", "path": "/spec/template/spec/containers/0/args/-", "value": "--v=4"}
]'
kubectl -n vela-system rollout status deployment/vela-core --timeout=60s
```

This survives a later [image-only rebuild](./k3d-workflow.md#7-iterate-after-code-changes),
since rebuilding only swaps the image and force-restarts the pod without
touching the deployment's arg list. You don't need to re-apply it after
every code change, only after `helm upgrade`/`helm install` resets the
deployment spec.
