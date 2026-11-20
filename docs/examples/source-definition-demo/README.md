# Authoring a SourceDefinition

Two demos, each self-contained. The first writes a `SourceDefinition` and reads
it; the second chains one source into another.

For the expression language itself see `../source-expressions-demo/`, and for the
sources that ship with KubeVela see `../source-library/`.

> Expressions are off by default. The controller needs
> `--feature-gates=EnableCelExpressions=true`, and every Application here carries
> `app.oam.dev/cel-expressions: "true"` to opt in, which the default opt-in mode
> requires.

## 1. Reading live cluster state

Two purpose-built definitions read the cluster through the CueX `kube` provider,
and one Application turns their values into a Deployment and a ConfigMap.

| File | Is |
|---|---|
| `definitions/cluster-lookup.cue` | reads the `cluster-info` ConfigMap in `kube-system`; surfaces `region`, `zone`, `provider`. Keyed per cluster |
| `definitions/tenant-data.cue` | reads the current namespace's labels; surfaces tenant `name`, `department`, `environment` and an optional `costCenter`. Keyed per cluster and namespace |
| `definitions/get-random.cue` | GETs an integer from random.org over HTTPS. Keyed per `(min,max)`, TTL 10s |
| `definitions/demo-deployment.cue` | a plain Deployment component, so the demo needs no other definitions |
| `apps/tenant-workload.yaml` | binds all three and writes a Deployment and a ConfigMap |
| `resources/` | the `source-demo` namespace with tenant labels, and the `cluster-info` ConfigMap |

```bash
kubectl apply -f docs/examples/source-definition-demo/resources/
vela def apply docs/examples/source-definition-demo/definitions/
kubectl apply -f docs/examples/source-definition-demo/apps/tenant-workload.yaml
```

`vela def apply` rather than `kubectl apply`: these are authored in the `vela def`
CUE format. With no `-n` they install to `vela-system`, and the controller reads
them whatever namespace the Application is in.

```bash
kubectl get deploy -n source-demo -o custom-columns=NAME:.metadata.name,REPLICAS:.spec.replicas
# us-east-1-us-east-1a-platform-acme-web   3

kubectl get cm tenant-config -n source-demo -o jsonpath='{.data}'
```

> `get-random` polls `https://www.random.org` from the controller process, so it
> needs outbound access. There is no in-cluster service to run.

### What to notice

**Cache scope follows what the template reads.** `cluster-lookup` reads
`context.cluster`, so there is one entry per cluster and every Application on it
shares the entry. `tenant-data` also reads `context.namespace`, so it has one per
namespace. Nothing declares this; it is inferred from the template.

```bash
vela config list | grep -E 'cluster-lookup|tenant-data'
```

**An expression composes across sources.** The Deployment's name is built in one
expression rather than by a definition whose only job would be joining strings:

```
$((source.cluster.region + "-" + source.cluster.zone + "-" +
   source.tenant.department + "-" + source.tenant.name + "-web").lowerAscii())
```

**Optional fields need a guard only where the target is required.** `costCenter`
is optional in `tenant-data`. Here it feeds a free-form ConfigMap field, so
admission does not insist on a default, but the example supplies one so an absent
label produces `unassigned` rather than dropping the key.

**A missing required input fails the source, not the render.** Remove a label and
the source reports `Failed`:

```bash
kubectl label ns source-demo tenant.example.com/name-
```

**`autoUpdate` re-dispatches on a changed value.** Each binding sets it. When a
source re-resolves differently the components reading it are re-dispatched even
though their spec is unchanged, because the controller stamps
`source.oam.dev/resolved-hash` on the workload and compares. Unset, it follows the
`EnableSourceAutoUpdate` gate; `app.oam.dev/publishVersion` suppresses it either
way. Re-dispatch happens on reconcile, so nudge it if you do not want to wait:

```bash
vela config delete get-random-1-5     # force a re-roll
kubectl annotate app tenant-workload -n source-demo demo/nudge=$(date +%s) --overwrite
```

## 2. Chaining one source into another

`source-chain-app.yaml` is a single file: two `SourceDefinition`s and an
Application. The second source's *properties* are fed by expressions reading the
first, which is what chaining is for. Joining strings is not, and needs no
chaining at all.

```bash
kubectl apply -f docs/examples/source-definition-demo/source-chain-app.yaml
kubectl get deploy web-chain -o jsonpath='{.spec.template.spec.containers[0].image}{"\n"}'
# nginx:1.25.2
```

### What to notice

**Forward-only ordering.** A source may only read one declared before it;
admission refuses a reference to a source at or after its own position.

**Nested paths resolve.** `$(source.cluster.nested.image.repo)` reads through the
structure the definition returns.

**A binding can mask what status records.** `clusterInfo` sets
`statusPolicy.maskPaths: [nested.image.tag]`, so the read is reported as `***`
while the container still receives the real value:

```bash
kubectl get app source-chain-app -o jsonpath='{.status.sources[*].consumedBy[*].values[*]}'
# clusterInfo.nested.image.tag -> ***      rendered.resolved.image -> nginx:1.25.2

kubectl get deploy web-chain -o jsonpath='{.spec.template.spec.containers[0].env[0].value}'
# 1.25.2
```

That is a status-reporting choice made by the Application. A definition that
should never expose a value marks it `+sensitive` instead, which no binding can
override.
