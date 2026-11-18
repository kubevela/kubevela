# Definition inheritance: a worked example

A platform team publishes a hardened web service type. Product teams want their own flavour of it: their labels, their image, their replica count, sometimes a different network policy. With `spec.extends`, a team writes a definition that is the platform's one with more on it, and the platform's guarantees hold for every type built on top.

This walkthrough uses one platform type, three team types built on it, a trait chain, a storage type and the mistakes a team might make. **Every message and every rendered object below was captured from a real cluster**, not written by hand.

```
definitions/
  platform-web.yaml      the platform's web service: non-root, a Service, a default-deny NetworkPolicy
  payments-web.yaml      the payments team's flavour: image, labels, three replicas
  checkout-web.yaml      built on payments-web, so platform-web two levels up
  public-web.yaml        swaps the default-deny NetworkPolicy for one that admits ingress
  platform-expose.yaml   an Ingress trait, and internal-expose built on it
  team-volume.yaml       storage whose class is fixed once set, through a child that picks a tier
  team-mistakes.yaml     five things a team might try, all refused
apps/                    the Applications below
```

The feature gate `EnableDefinitionInheritance` must be on (`--set featureGates.enableDefinitionInheritance=true`).

## The rules in one table

| A definition that extends another may | It may not |
|---|---|
| add fields its parent did not set | change a field its parent set, even one from a defaulted parameter |
| fill a field its parent left open, such as `paused: bool` | replace its parent's output: `$inherit: false`, `$inherit: {output: false}` |
| add outputs of its own | leave out an output the parent marks `// +required` |
| leave out one of its parent's outputs, `$inherit: {outputs: {<name>: false}}`, and write its own | add to a field the parent marks `// +final`, at any depth |
| change its parent through `$super: properties` | declare a workload other than its parent's |

Every rule is checked when the definition is applied, and again when an Application renders it.

---

## 1. The platform type

[`definitions/platform-web.yaml`](definitions/platform-web.yaml) renders a Deployment, a Service and a NetworkPolicy. Two doc comments carry the platform's guarantees:

```cue
spec: template: spec: {
	// +final
	securityContext: {
		runAsNonRoot: true
		seccompProfile: type: "RuntimeDefault"
	}
	...
}
outputs: {
	// +required
	service: {...}
	netpol: {...}      // default-deny ingress
}
```

- `+final` on the security context: nothing built on `platform-web` may add to it, at any level.
- `+required` on the Service: no team may leave it out. The NetworkPolicy is not marked, so a team may replace it.

## 2. A team's flavour

[`definitions/payments-web.yaml`](definitions/payments-web.yaml) adds what the payments team needs and nothing else:

```cue
parameter: {
	image:  *"nginxinc/nginx-unprivileged:1.27" | string
	tenant: string
}
// What platform-web receives. Replicas go up here, never by writing
// output.spec.replicas.
$super: properties: {
	image:    parameter.image
	replicas: 3
}
output: metadata: {
	labels: team: "payments"
	annotations: "example.com/cost-centre": "cc-4410"
}
output: spec: template: metadata: labels: {
	team:   "payments"
	tenant: parameter.tenant
}
```

It declares its own parameters, `image` and `tenant`. It does not inherit `platform-web`'s: it decides what `platform-web` receives through `$super: properties`.

[`definitions/checkout-web.yaml`](definitions/checkout-web.yaml) builds on `payments-web` and only adds an environment variable to the container.

## 3. What renders

[`apps/payments.yaml`](apps/payments.yaml) runs `checkout` (a `checkout-web`) with the `internal-expose` trait, and `ledger` (a `payments-web`). The `checkout` Deployment carries something from every level:

```json
{
  "labels": {"team": "payments"},
  "annotations": {"example.com/cost-centre": "cc-4410"},
  "replicas": 3,
  "podLabels": {"team": "payments", "tenant": "acme"},
  "securityContext": {"runAsNonRoot": true, "seccompProfile": {"type": "RuntimeDefault"}},
  "container": {
    "image": "nginxinc/nginx-unprivileged:1.27",
    "env": [{"name": "CHECKOUT_MODE", "value": "live"}],
    "ports": [{"containerPort": 8080, "protocol": "TCP"}]
  }
}
```

| From | Fields |
|---|---|
| platform-web | the Deployment, the security context, the port, the Service, the NetworkPolicy |
| payments-web | the image, `replicas: 3`, the team and tenant labels, the cost centre |
| checkout-web | `CHECKOUT_MODE` |

Both components report the platform's workload and its health:

```
checkout	Deployment	healthy=true
ledger	Deployment	healthy=true
```

The trait chain works the same way. [`internal-expose`](definitions/platform-expose.yaml) is `platform-expose` with an ingress class and an allowlist on its Ingress:

```json
{"class": "internal", "allowlist": "10.0.0.0/8", "host": "checkout.internal.example.com"}
```

## 4. Leaving one output out

`platform-web`'s NetworkPolicy denies all inbound traffic. [`definitions/public-web.yaml`](definitions/public-web.yaml) leaves that one output out and writes its own, which admits the ingress controller:

```cue
$inherit: outputs: netpol: false
outputs: netpol: {
	...
	spec: ingress: [{from: [{namespaceSelector: matchLabels: "kubernetes.io/metadata.name": "ingress-nginx"}]}]
}
```

The `shop` component of [`apps/storefront.yaml`](apps/storefront.yaml) gets only the child's NetworkPolicy, while `ledger` keeps the platform's default-deny one:

```
shop:   {"ingress": [{"from": [{"namespaceSelector": {"matchLabels": {"kubernetes.io/metadata.name": "ingress-nginx"}}}]}], ...}
ledger: {"podSelector": {...}, "policyTypes": ["Ingress"]}
```

The Service and the Deployment are still `platform-web`'s. Only the output named in `$inherit` was replaced.

## 5. What a team cannot do

[`definitions/team-mistakes.yaml`](definitions/team-mistakes.yaml) holds five attempts to bend `platform-web`. Each is refused when it is applied:

| Attempt | Refused with |
|---|---|
| swap the Deployment for a Job | `` `$inherit: {output: false}` is not allowed in nightly-job: a definition that extends another adds to its parent's output and cannot replace it; a definition that needs something else in its place is a different definition `` |
| set `replicas: 10` on the Deployment | ``component big-web sets output.spec.replicas, which platform-web already sets; pass a value to platform-web through `$super: properties` instead`` |
| add `runAsUser: 0` to the security context | `component root-web adds to output.spec.template.spec.securityContext, which platform-web marks +final` |
| leave out the Service | `component headless-web leaves out outputs.service, which platform-web marks +required` |
| declare a StatefulSet workload | `component stateful-web declares workload apps/v1 StatefulSet, but platform-web declares apps/v1 Deployment; leave spec.workload unset to inherit it` |

The same checks run again at render. If the platform team adds `+final` or `+required` to `platform-web` after teams have built on it, an Application using a team type that breaks the new marker is refused from then on.

## 6. Immutable through a child

[`definitions/team-volume.yaml`](definitions/team-volume.yaml) has `platform-volume`, whose `storageClass` is `// +immutable`, and `team-volume`, which lets a team pick a tier and maps it onto the class:

```cue
parameter: {
	tier: "fast" | "standard"
	size: *"10Gi" | string
}
$super: properties: {
	storageClass: parameter.tier
	size:         parameter.size
}
```

[`apps/volume.yaml`](apps/volume.yaml) creates a volume with `tier: fast`, so its class is `fast`. Changing the tier on the running Application would change `platform-volume`'s storage class, so it is refused, although the field the Application changed is `tier`:

```
Error from server: admission webhook "validating.core.oam.dev.v1beta1.applications" denied the request:
  1) "spec.components[0].properties": platform-volume's immutable parameter storageClass cannot be changed (current: "fast", new: "standard").
```

Growing the volume to `size: 20Gi` is allowed.

## Try it

```bash
kubectl apply -f definitions/platform-web.yaml -f definitions/payments-web.yaml \
  -f definitions/checkout-web.yaml -f definitions/public-web.yaml \
  -f definitions/platform-expose.yaml -f definitions/team-volume.yaml
kubectl apply -f definitions/team-mistakes.yaml     # all five refused
kubectl apply -f apps/
kubectl patch app ledger-data --type=merge \
  -p '{"spec":{"components":[{"name":"ledger-db","type":"team-volume","properties":{"tier":"standard"}}]}}'   # refused
```

## How the output was captured

The definitions and Applications in this folder were applied to a k3d cluster running vela-core built from this branch, with `EnableDefinitionInheritance` and the other feature gates the e2e suite sets. The refusals are the webhook's messages as `kubectl` printed them, with the request UID removed. The rendered objects were read back with `kubectl get -o json` and trimmed to the fields discussed.
