# Data sources and property expressions

A platform team exposes three data sources. An application author consumes them
across a component, a trait, a workflow step and a policy — without learning where
any of the data comes from.

Every value in this README was produced by applying these files to a cluster.

## The shape of it

```
                    reads a ConfigMap            reads namespace labels
                            │                             │
                  ┌─────────▼──────────┐        ┌─────────▼─────────┐
                  │ platform-registry  │        │  tenant-profile   │
                  │  keyed per cluster │        │ per cluster + ns  │
                  └─────────┬──────────┘        └─────────┬─────────┘
                            │                             │
                            └────────────┬────────────────┘
                                         │  chained
                               ┌─────────▼─────────┐
                               │  service-catalog  │  naming + placement rules
                               └─────────┬─────────┘
                                         │
        ┌────────────────┬───────────────┼──────────────┬────────────────┐
        ▼                ▼               ▼              ▼                ▼
    component          trait        workflow step     policy         (status)
```

## What the author writes

```yaml
sources:
  - {name: registry, type: platform-registry, properties: {}}
  - {name: tenant,   type: tenant-profile,    properties: {}}
  - name: catalog
    type: service-catalog
    properties:                                   # a source fed from two others
      registryHost: '$(source.registry.host)'
      project:      '$(source.registry.project)'
      team:         '$(source.tenant.team)'
      environment:  '$(source.tenant.environment)'
      component:    nginx
```

They never mention the `registry-info` ConfigMap, the namespace labels, the cache,
or the cluster. Those are the definition's business.

## What it produces

**Component** — concatenation, a scalar, an int that stays an int, and a struct
and a list substituted whole:

```yaml
image:            '$(source.catalog.image + ":1.25.0")'
imagePullPolicy:  '$(source.registry.pullPolicy)'
port:             '$(source.catalog.httpPort)'
labels:           '$(source.catalog.standardLabels)'
imagePullSecrets: '$(source.catalog.pullSecrets)'
```

```
image=docker.io/library/nginx:1.25.0    replicas=3   ready=3
pullPolicy=IfNotPresent
pullSecrets=[{"name":"platform-registry-creds"}]
port=8080
podLabels={"platform.io/team":"payments","platform.io/environment":"production",
           "platform.io/managed-by":"kubevela", ...}
```

**Environment** — interpolation, context, and two optional fields that are genuinely
absent, each with a fallback:

```
SERVICE_NAME=payments-nginx
MESH_DOMAIN=no-mesh                  # tenant has not opted into mesh
OWNER=payments-oncall                # from context.appLabels
WHERE=checkout.checkout-prod
MIRROR=none                          # platform publishes no mirror on this cluster
```

**Trait** — integer arithmetic on a source value:

```yaml
- type: scaler
  properties:
    replicas: '$(source.tenant.maxReplicas / 2)'      # 6 div 2 -> 3
```

**Workflow step** — the same sources, in a step that runs after the deploy:

```
{"image":"docker.io/library/nginx:1.25.0","service":"payments-nginx",
 "tier":"production","costCentre":"CC-4471","replicas":"3"}
```

**Policy** — Application-scoped policies render *before* the appfile exists, so they
read `context` but not `source`. Reading a source there is refused with a reason,
rather than silently doing nothing:

```
platform.io/owner=payments-oncall
platform.io/app=checkout--checkout-prod
```

**Status** — each binding reports the cache entry it resolved against. Note the
key shapes: per cluster, per cluster+namespace, and shared:

```
registry -> platform-registry-local-049ff91a
tenant   -> tenant-profile-local-checkout-prod-b2b559ad
catalog  -> service-catalog-12824cb5
```

## Why the keys look like that

Nobody wrote them. `vela def` infers the cache key from the context each template
reads and writes it into a `$internal` block; admission re-derives it and rejects a
mismatch.

| Definition | Reads | Generated key | Sharing |
|---|---|---|---|
| `platform-registry` | `context.cluster` | `platform-registry-\(context.cluster)` | one entry per cluster |
| `tenant-profile` | `context.cluster`, `context.namespace` | `tenant-profile-\(context.cluster)-\(context.namespace)` | one per namespace |
| `service-catalog` | nothing | `service-catalog` | shared by every consumer with the same inputs |

The author cannot get this wrong, because they do not write it.

## What is checked before the Application is admitted

Not at render — at admission, against the consuming parameter:

| Written | Rejected with |
|---|---|
| `port: '$(source.catalog.image)'` | *type mismatch: … is string but component "webservice" parameter expects int* |
| `image: '$(source.catalog.standardLabels)'` | *… is object but … expects string* |
| `image: '$(source.catalog.standardLabels)-x'` | *cannot be combined with text* |
| `image: '$(source.registry.mirror)'` | *may be absent and feeds required … supply a default with `*… \| <fallback>`* |
| `image: '$(source.registry.nope)'` | *not declared in the source's schema* |
| `image: '$(parameter.image)'` | *unknown identifier "parameter"* |
| `owner: '$(source.registry.host)'` in a scoped policy | *"source" cannot be read here; this surface permits "context"* |

## Run it

> Expressions are off by default. The controller needs
> `--feature-gates=EnableCelExpressions=true`, and each Application here carries
> `app.oam.dev/cel-expressions: "true"` to opt in, which is what the default
> opt-in mode requires.

```bash
kubectl apply -f resources/platform.yaml
kubectl apply -f definitions/
kubectl apply -f app/checkout.yaml

kubectl get deploy -n checkout-prod api -o yaml
kubectl get cm -n checkout-prod checkout-deployment-summary -o jsonpath='{.data}'
kubectl get app -n checkout-prod checkout -o jsonpath='{.status.services[*].sources[*]}'
```

The policy needs `--feature-gates=EnableApplicationScopedPolicies=true` on the
controller; without it that part is silently skipped.

## The point

The platform decides what exists, what it costs to fetch, and how widely it is
shared. The author decides what they need and where it goes. Neither has to know
the other's job — and the parts that could go wrong (a wrong type, a missing
optional, a key that under-discriminates) are refused before the Application is
accepted rather than surfacing as a render failure later.
