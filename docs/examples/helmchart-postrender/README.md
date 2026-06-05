# helmchart postRender examples

The native `helmchart` component supports Kustomize post-rendering via
`options.postRender.kustomize`. Patches execute **after** Helm renders
manifests and **before** KubeVela stamps its ownership labels, so they
see the raw chart output and cannot accidentally remove KubeVela tracking
labels.

## Quick reference

```yaml
components:
  - type: helmchart
    properties:
      chart:
        source: podinfo
        repoURL: https://stefanprodan.github.io/podinfo
        version: "6.5.0"
      options:
        postRender:
          kustomize:
            patches:           # JSON 6902 or strategic-merge (recommended)
              - target:
                  kind: Deployment
                  name: podinfo
                patch: |
                  - op: add
                    path: /spec/template/spec/tolerations
                    value:
                      - key: node-role.kubernetes.io/control-plane
                        operator: Exists
                        effect: NoSchedule
            patchesStrategicMerge: [...]  # strategic-merge patches (see below)
            patchesJson6902: [...]        # RFC 6902 patches (deprecated; prefer patches)
            images: [...]                 # image tag/digest replacements
            replicas: [...]               # replica count overrides
```

## Patch types

### `patches` (recommended)

Handles both JSON 6902 operations and strategic-merge patches in one field.
Each entry needs a `target` selector and either an inline `patch` string or
a `path` to a file.

```yaml
patches:
  - target:
      kind: Deployment
      name: my-app
    patch: |
      - op: replace
        path: /spec/replicas
        value: 3
```

### `patchesStrategicMerge`

Each item is either a YAML string or a map object shaped like the resource
being patched. Kustomize upstream has deprecated this field in favour of
`patches`; a deprecation advisory is emitted to the controller log but
correctness is not affected.

```yaml
patchesStrategicMerge:
  - |
    apiVersion: apps/v1
    kind: Deployment
    metadata:
      name: my-app
    spec:
      template:
        spec:
          containers:
            - name: my-app
              resources:
                requests:
                  cpu: 50m
```

### `images`

Replace container image names, tags, or digests without patching the
Deployment manifest by hand.

```yaml
images:
  - name: ghcr.io/my-org/my-app
    newTag: "v2.1.0"
  - name: ghcr.io/my-org/sidecar
    digest: sha256:abc123...
```

### `replicas`

Override replica counts by workload name.

```yaml
replicas:
  - name: my-app
    count: 3
```

## Ordering contract

```
Helm render → kustomize patches → KubeVela ownership labels
```

KubeVela's `app.oam.dev/*` labels are always stamped last. Patches that
set or remove labels on resources will not affect KubeVela's tracking labels
because those are applied in a separate pass after kustomize exits.

## Files in this folder

- [`patches-tolerations-resources.yaml`](./patches-tolerations-resources.yaml) —
  adds a control-plane toleration and overrides resource requests on a Deployment.
- [`image-replacement.yaml`](./image-replacement.yaml) —
  pins a container image to a specific tag and overrides replica count.

Apply any example with:

```bash
kubectl apply -f patches-tolerations-resources.yaml
# or
kubectl apply -f image-replacement.yaml
```
