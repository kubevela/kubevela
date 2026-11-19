# Definition inheritance: a platform type and the teams that build on it

A platform team publishes a hardened web service type. Product teams want their own flavour of it: their labels, their replica count, a different network policy. With `spec.extends`, a team writes a definition that is the platform's one with more on it, and the platform's guarantees hold for every type built on top.

| File | What is in it |
|---|---|
| [`platform.yaml`](platform.yaml) | what the platform team publishes: `platform-web`, the `platform-expose` trait, `platform-volume` |
| [`teams.yaml`](teams.yaml) | what teams build on it: `payments-web`, `checkout-web`, `public-web`, `legacy-web`, `internal-expose`, `team-volume` |
| [`mistakes.yaml`](mistakes.yaml) | thirteen things a team or the platform team might try; every one is refused |
| [`apps.yaml`](apps.yaml) | the Applications |

## The rules

| A definition that extends another may | It may not |
|---|---|
| add fields its parent did not set | change a field its parent set, even one from a defaulted parameter |
| change a parent value through `$super: properties` | replace what the parent renders: `$inherit: false`, `{output: false}`, `{outputs: false}` |
| leave out one named output, `$inherit: {outputs: {<name>: false}}`, and write its own | leave out an output the parent marks `// +required` |
| add outputs and a health check of its own | add anything to a field the parent marks `// +final` |
| leave `spec.workload` unset to inherit it | declare a workload other than its parent's |

A parent's `// +immutable` parameter also holds through a child that forwards or renames it. Every rule is checked when a definition is applied and again when an Application renders it, so a marker added to a parent later still applies to the types already built on it.

## Run it

Inheritance is alpha and off by default:

```bash
helm upgrade --install kubevela kubevela/vela-core -n vela-system --create-namespace \
  --set featureGates.enableDefinitionInheritance=true
```

Then:

```bash
kubectl apply -f platform.yaml
kubectl -n vela-system wait --for=create definitionrevision/platform-web-v1   # legacy-web pins it
kubectl apply -f teams.yaml
kubectl apply -f mistakes.yaml    # thirteen errors, one per object
kubectl apply -f apps.yaml
```

## The platform type

`platform-web` renders a Deployment, a Service and a NetworkPolicy. It is `abstract: true`, so teams build on it rather than use it. Two doc comments carry the platform's guarantees:

```cue
output: spec: template: spec: {
	// +final
	securityContext: {runAsNonRoot: true, seccompProfile: type: "RuntimeDefault"}
	...
}
outputs: {
	// +required
	service: {...}
	netpol: {...}      // default-deny, labelled example.com/policy: default-deny
}
```

Its status says which ingress policy applies, read from that NetworkPolicy: `ingress: default-deny`.

`+final` works on fields reached through structs, like the pod's security context above. It cannot be put on a field inside a list item, such as one container's `securityContext`: the check could not enforce it there, so such a definition is refused when it is applied (the last entry in `mistakes.yaml`). To lock something inside the containers, mark the `containers` list itself, which locks the whole list.

## What teams build

**`payments-web`** declares its own parameters and decides what `platform-web` receives:

```cue
parameter: {
	image:  *"nginxinc/nginx-unprivileged:stable" | string
	tenant: string
}
$super: properties: {
	image:    parameter.image
	replicas: 3            // replicas change here, never by writing output.spec.replicas
}
output: metadata: labels: team: "payments"
outputs: config: {
	kind: "ConfigMap"
	data: image: $super.output.spec.template.spec.containers[0].image   // what platform-web rendered
	...
}
```

**`checkout-web`** extends `payments-web`, so `platform-web` two levels up. It adds an environment variable and a memory limit to the container, and a health check that is combined with the one it inherits.

**`public-web`** must accept traffic from the ingress controller. It leaves out the default-deny policy and writes its own. The inherited status message reads that policy, so it says its own message too:

```cue
$inherit: outputs: netpol: false
outputs: netpol: {... ingress: [{from: [{namespaceSelector: matchLabels: "kubernetes.io/metadata.name": "ingress-nginx"}]}]}
```
```yaml
status:
  customStatus: |
    message: "ingress: from ingress-nginx"
```

**`legacy-web`** extends `platform-web@v1`. When the platform team changes `platform-web`, `legacy-web` keeps rendering what revision 1 rendered.

**`internal-expose`** is the `platform-expose` trait with the internal ingress class and an allowlist. A trait extends a trait the same way.

**`team-volume`** lets a team pick a `tier`, which becomes `platform-volume`'s immutable `storageClass`.

## What renders

In namespace `default`:

| Command | Result |
|---|---|
| `kubectl get deploy checkout -o jsonpath='{.metadata.labels.team} {.spec.replicas} {.spec.template.spec.containers[0].env[0].value} {.spec.template.spec.containers[0].resources.limits.memory}'` | `payments 3 live 256Mi`: one value from each level |
| `kubectl get deploy checkout -o jsonpath='{.spec.template.spec.securityContext}'` | `{"runAsNonRoot":true,"seccompProfile":{"type":"RuntimeDefault"}}`, the platform's |
| `kubectl get configmap ledger-config -o jsonpath='{.data.image}'` | `nginxinc/nginx-unprivileged:stable`, read from what `platform-web` rendered |
| `kubectl get networkpolicy shop -o jsonpath='{.spec.ingress[0].from[0].namespaceSelector.matchLabels}'` | `{"kubernetes.io/metadata.name":"ingress-nginx"}`, `public-web`'s own |
| `kubectl get networkpolicy ledger -o jsonpath='{.metadata.labels.example\.com/policy}'` | `default-deny`, the platform's |
| `kubectl get ingress checkout -o jsonpath='{.spec.ingressClassName}'` | `internal`, from the child trait |
| `kubectl get app payments storefront -o jsonpath='{range .items[*].status.services[*]}{.name}: {.message}{"\n"}{end}'` | `checkout`, `ledger` and `archive` say `ingress: default-deny`; `shop` says `ingress: from ingress-nginx` |

## What a team cannot do

Each object in [`mistakes.yaml`](mistakes.yaml) is refused when it is applied:

| Attempt | Refused with |
|---|---|
| swap the Deployment for a Job | `` `$inherit: {output: false}` is not allowed in nightly-job: a definition that extends another adds to its parent's output and cannot replace it; a definition that needs something else in its place is a different definition `` |
| throw away everything the parent renders | `` `$inherit: false` is not allowed in bare-web: ... leave out one output at a time with `$inherit: {outputs: {<name>: false}}` `` |
| set `replicas: 10` on the Deployment | ``component big-web sets output.spec.replicas, which platform-web already sets; pass a value to platform-web through `$super: properties` instead`` |
| change the Service port | `component https-web sets outputs.service.spec.ports[0].port, but platform-web fixes it to 80 and a child cannot change it; to use another value, write a definition that does not extend platform-web` |
| add `runAsUser: 0` to the security context | `component root-web adds to output.spec.template.spec.securityContext, which platform-web marks +final` |
| leave out the Service | `component headless-web leaves out outputs.service, which platform-web marks +required` |
| leave out the NetworkPolicy and keep the message that reads it | `component open-web leaves out outputs.netpol, which the customStatus it inherits from platform-web reads; keep the output, or set its own customStatus` |
| declare a StatefulSet | `component stateful-web declares workload apps/v1 StatefulSet, but platform-web declares apps/v1 Deployment; leave spec.workload unset to inherit it` |
| never call the parent | ``component lazy-web extends platform-web but its template never calls it; add a `$super: properties: {...}` block naming what platform-web should receive`` |
| extend it from another namespace | `component definition platform-web is in namespace vela-system, and a definition may only extend one in its own namespace (default); ...` |
| make `platform-web` extend a type built on it | `definition payments-web extends platform-web, which is already in its own inheritance chain: platform-web -> checkout-web -> payments-web -> platform-web` |
| use `platform-web` directly in an Application | `component platform-web is abstract and cannot be used directly; name a component that extends it.` |
| (platform team) lock one container's security context with `+final` inside the `containers` list | `component locked-web marks output.spec.template.spec.containers[0].securityContext +final, but +final cannot be enforced on a field inside a list item; mark the list field, output.spec.template.spec.containers, instead` |

## `+immutable` through a child

`ledger-data` has a `fast` volume. Changing its tier would change `platform-volume`'s storage class, so it is refused, although the field the Application changes is `tier`:

```bash
kubectl patch app ledger-data --type=json \
  -p '[{"op":"replace","path":"/spec/components/0/properties/tier","value":"standard"}]'
# ... platform-volume's immutable parameter storageClass cannot be changed (current: "fast", new: "standard").

kubectl patch app ledger-data --type=json \
  -p '[{"op":"add","path":"/spec/components/0/properties/size","value":"20Gi"}]'
# accepted: size is not immutable
```

The volume stays `Pending` unless the cluster has a StorageClass named `fast`. The update is judged by the admission webhook either way.

## Clean up

```bash
kubectl delete -f apps.yaml
kubectl delete -f teams.yaml
kubectl delete -f platform.yaml
```
