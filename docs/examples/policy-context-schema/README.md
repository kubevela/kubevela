# Typed policy context: a worked example

An Application-scoped policy can put data in `context.custom` for components and traits to read. Without a schema, nothing describes that data:
- A definition reading a key the policy never sets is admitted, and only fails later, at render.
- So is a definition using a key as the wrong type, or reading an optional key with no guard.
- A policy producing a bad value is only caught if something downstream happens to choke on it.

A policy declares the shape of what it supplies in a `schema: ctx` block, and both ends use it:

| When | Who checks | Against what |
|---|---|---|
| `kubectl apply` of an Application | the Application webhook | the schemas of the policies that apply. No policy is evaluated, so no provider (`kube.#Get`, `http`) is called during admission |
| every render | the controller | each policy's actual `output.ctx` against its own schema |

This walkthrough uses one platform policy and a handful of definitions and Applications. **Every message below was captured from the real code**, not written by hand (see [How the output was captured](#how-the-output-was-captured)).

```
definitions/
  tenant-identity.yaml       the platform's global policy, with its schema
  tagged-service.yaml        a ComponentDefinition reading the context correctly
  broken-definitions.yaml    definitions making the mistakes admission catches
  policy-mistakes.yaml       PolicyDefinitions the PolicyDefinition webhook pushes back on
apps/                        one Application per case below
```

The feature gates `EnableApplicationScopedPolicies` and `EnableGlobalPolicies` must be on, as for any global policy.

---

## 1. The policy

[`definitions/tenant-identity.yaml`](definitions/tenant-identity.yaml) is a global policy. It reads the team and cost centre from labels the platform puts on each tenant namespace, and hands them to every Application in that namespace:

```cue
import (
	"strings"
	"vela/kube"
)

schema: ctx: close({
	team:        =~"^[a-z][a-z0-9-]*$"
	costCentre:  strings.MinRunes(3)
	// Only tenants opted into the mesh have one.
	meshDomain?: string
})

parameter: {}

_ns: kube.#Get & {
	$params: resource: {
		apiVersion: "v1"
		kind:       "Namespace"
		metadata: name: context.namespace
	}
}
_labels: _ns.$returns.metadata.labels

output: ctx: {
	team:       _labels["platform.io/team"]
	costCentre: _labels["platform.io/cost-centre"]
	if _labels["platform.io/mesh-domain"] != _|_ {
		meshDomain: _labels["platform.io/mesh-domain"]
	}
}
```

How to read the schema:

- **`team` and `costCentre` are required.** The policy promises to set them.
- **`meshDomain?` is optional.** Only some tenants have one, so a template must not rely on it.
- **`close({...})` makes the schema the whole contract.** The policy supplies nothing it doesn't declare, so admission can also refuse a template that reads an undeclared key. Without `close`, the schema is open: declared keys are still typed, but other keys may exist, and reading them is not refused.
- **The block must stand on its own.** Admission reads it without the rest of the template, so:
  - the CUE standard library (`strings`, `list`, `net`, ...) is fine;
  - provider packages and references to other fields are not (see [step 5](#5-mistakes-in-the-policy-itself)).

## 2. A definition that reads the context correctly

[`definitions/tagged-service.yaml`](definitions/tagged-service.yaml) uses the two patterns a definition author has:

```cue
parameter: {
	image: string
	// The policy suggests an owner; an Application may name another.
	owner: *context.custom.team | string
}
output: {
	metadata: {
		labels: {
			// Read directly: no properties value changes them.
			"platform.io/team":        context.custom.team
			"platform.io/cost-centre": context.custom.costCentre
			// From the parameter: the Application may name another owner.
			"platform.io/owner": parameter.owner
		}
		// Optional: guarded, so tenants without a mesh still validate.
		if context.custom.meshDomain != _|_ {
			annotations: "platform.io/mesh-domain": context.custom.meshDomain
		}
	}
	...
}
```

| Pattern | Use it when | Who decides |
|---|---|---|
| `labels: team: context.custom.team` (direct) | the value is governance: identity, cost, compliance | the policy: no `properties` value changes it. A trait the Application attaches can still patch the rendered label, so this is not an access control |
| `owner: *context.custom.team \| string` (parameter default) | the policy suggests a value | the Application may override it in `properties` |

Both Applications using it are admitted:

| Application | Result |
|---|---|
| [`apps/good.yaml`](apps/good.yaml): just an image | admitted |
| [`apps/owner-override.yaml`](apps/owner-override.yaml): sets `owner: data-platform` | admitted. The owner label is the Application's; the team and cost-centre labels are still the policy's |

## 3. Mistakes admission catches

Each definition in [`definitions/broken-definitions.yaml`](definitions/broken-definitions.yaml) reads the context in a way the schema says will fail. Applying an Application that uses one is refused immediately. `kubectl` prints each message after its usual `admission webhook "validating.core.oam.dev.v1beta1.applications" denied the request:`.

### An optional key read without a guard

`mesh-service` builds a URL from `meshDomain`, which the schema marks optional:

```cue
annotations: "platform.io/mesh-url": "https://\(context.custom.meshDomain)/\(context.name)"
```

[`apps/mesh-service.yaml`](apps/mesh-service.yaml) is refused:

```
1) "schematic": component "gateway": context.custom.meshDomain is optional in policy "tenant-identity"'s schema.ctx; guard it (if context.custom.meshDomain != _|_ {...}) or give it a default.
```

The fix is to guard it, as `tagged-service` does:

```cue
if context.custom.meshDomain != _|_ {
	annotations: "platform.io/mesh-url": "https://\(context.custom.meshDomain)/\(context.name)"
}
```

The same read in a **trait** is caught the same way. [`apps/mesh-sidecar.yaml`](apps/mesh-sidecar.yaml) adds the `mesh-sidecar` trait to a correct component:

```
1) "schematic": trait "mesh-sidecar": context.custom.meshDomain is optional in policy "tenant-identity"'s schema.ctx; guard it (if context.custom.meshDomain != _|_ {...}) or give it a default.
```

### A key no policy supplies

`regional-service` reads `context.custom.region`. `tenant-identity`'s schema is closed and doesn't declare `region`, so no policy will ever set it. [`apps/regional-service.yaml`](apps/regional-service.yaml):

```
1) "schematic": component "regional": reads context.custom.region, but no applicable policy declares it in schema.ctx; guard it (if context.custom.region != _|_ {...}) or give it a default.
```

This check only runs when **every** policy that applies to the Application declares a closed schema. If any of them is open, or declares no schema at all, it could supply `region`, and admission lets the read through.

### A key used against its type

`sized-service` uses the cost centre, a string, as a replica count:

```cue
replicas: int & context.custom.costCentre
```

[`apps/sized-service.yaml`](apps/sized-service.yaml):

```
1) "schematic": component "sized": uses context.custom.costCentre against its type in policy "tenant-identity"'s schema.ctx: evaluate base template app=payments-sized in namespace=payments: validation failed for workload sized:

Template errors:
  output.spec.replicas: conflicting values int and strings.MinRunes(3) (mismatched types int and string).
```

The first line names the key and the policy. The `Template errors` block is CUE's own report, naming the output field where the conflict showed up.

## 4. When the policy's own output is wrong

Admission trusts the schema; the render checks that the policy keeps it. Suppose the platform labels a namespace badly:

```
kubectl label namespace payments platform.io/team=Payments platform.io/cost-centre=cc --overwrite
```

`tenant-identity` now outputs `team: "Payments"` and `costCentre: "cc"`. The schema refuses both, so the policy fails to render. Global policies are mandatory by default (the controller's `--allow-skip-global-policies` is off), so nothing is deployed with bad identity tags:

- an Application that already has a revision is held at it: it keeps running, cuts no new revision, and reports a `GlobalPolicies` condition naming the policy;
- an Application with no revision yet fails until the policy renders.

The condition reads:

```
failed to render global policy "tenant-identity", which is mandatory: output.ctx does not match schema.ctx: output.ctx.costCentre: invalid value "cc" (does not satisfy strings.MinRunes(3)); output.ctx.team: invalid value "Payments" (out of bound =~"^[a-z][a-z0-9-]*$")
```

Every mismatch is reported at once, in `output.ctx` terms: what the policy produced, against what its schema says.

| Namespace labels | Render |
|---|---|
| `team=payments`, `cost-centre=cc-1042` | renders; `meshDomain` is absent, which its `?` allows |
| the same plus `mesh-domain=payments.mesh.internal` | renders, with all three keys |
| `team=Payments` | refused: `output.ctx.team: invalid value "Payments" (out of bound =~"^[a-z][a-z0-9-]*$")` |
| `cost-centre=cc` | refused: `output.ctx.costCentre: invalid value "cc" (does not satisfy strings.MinRunes(3))` |
| no `cost-centre` label at all | refused, but before the schema check: the template itself can't produce the value (`failed to decode output: output.ctx.costCentre: undefined field: "platform.io/cost-centre"`) |

Other mismatches the render check catches, with their message text:
- **a required key the output doesn't set:** `output.ctx.costCentre: required by schema.ctx (string), not set`
- **a key a closed schema doesn't declare:** `output.ctx.region: not declared by schema.ctx, which is closed`

## 5. Mistakes in the policy itself

The PolicyDefinition webhook checks the schema where it's written. [`definitions/policy-mistakes.yaml`](definitions/policy-mistakes.yaml) has two policies:

**A schema leaning on the rest of the template is refused.** `tenant-identity-leaning` declares `schema: ctx: close({team: _teamPattern})`, but admission reads the schema without the rest of the template, where `_teamPattern` is defined:

```
invalid PolicyDefinition: [schema.ctx must stand on its own, using at most the CUE standard library and nothing else of the template, so that admission can read it: schema.ctx: ctx.team: reference "_teamPattern" not found]
```

**Context without a schema is admitted, with a warning.** `tenant-identity-untyped` sets `output.ctx` and declares nothing:

```
Warning: output.ctx is set without a schema.ctx declaring it, so Applications cannot be checked against it at admission; declare schema: ctx (and close() it to make it the whole contract)
```

A policy like that works like any other. Keys it might supply are simply unknown to admission: any read of them is let through, and the undeclared-key check from step 3 is off for every Application it applies to.

## 6. The same policy in defkit

```go
defkit.NewPolicy("tenant-identity").ApplicationScoped().Global().Priority(10).
	ClosedCtxSchema(
		defkit.String("team").Pattern("^[a-z][a-z0-9-]*$"),
		defkit.String("costCentre").MinLen(3),
		defkit.String("meshDomain").Optional(),
	).
	Template(func(tpl *defkit.PolicyTemplate) {
		tpl.Output().
			Ctx("team", defkit.Lit("payments")).
			Ctx("costCentre", defkit.Lit("cc-1042"))
	})
```

The generated template carries the schema:

```cue
schema: ctx: close({
	team: string & =~"^[a-z][a-z0-9-]*$"
	costCentre: string & strings.MinRunes(3)
	meshDomain?: string
})
```

- **Required by default:** a schema field is required unless it is `Optional()`. `CtxSchema(...)` declares an open schema; `ClosedCtxSchema(...)` a closed one.
- **Imports are added for you:** a field using `MinLen` (or another `strings` constraint) pulls in `"strings"`, as do the policy's parameters and output values. `WithImports(...)` is only needed for CUE you write by hand.
- **Register catches typos** in a closed schema's outputs:

  ```
  defkit: invalid definition 'tenant-identity-typo': Output().Ctx("teem") is not declared by the closed CtxSchema
  ```

## Rules of thumb

For **policy** authors:
- **Declare `schema: ctx`** for anything you put in `output.ctx`.
- **`close()` it** for identity and governance data. That gives Applications the strongest checking, including the undeclared-key check.
- **Mark a key `?`** only if it really can be absent.
- **Keep the schema self-contained:** the standard library, no references, no providers.
- **A schema default is a fallback the policy gives.** With `team: *"platform" | string`, an output that leaves `team` out passes the check, and components receive `team: "platform"`. A value the output does set wins. A default on an optional key (`zone?: *"z" | string`) fills nothing. Don't default a key that must fail closed when the policy can't work it out.

For **definition** authors:
- **Read governance values directly** (`labels: team: context.custom.team`), never through a parameter an Application can override.
- **Guard optional keys** with `if context.custom.x != _|_ {...}`, or give them a default.
- **Don't default a governance key in your own template** if it must fail closed. `context: custom: team: *"unknown" | string` means a broken policy quietly tags everything `unknown`.

## How the output was captured

Each block above came from a run of the code on the branch against the YAML in this folder:
- **Admission (step 3):** the Application webhook's `Handle`, with the definitions loaded into a client.
- **Render (step 4):** the controller's output decoding and schema check, on `tenant-identity`'s real template with the namespace labels supplied directly, since `kube.#Get` needs a cluster. The condition's `failed to render global policy ..., which is mandatory:` prefix is as the controller's envtest suite reports it for a policy failing its schema.
- **PolicyDefinition (step 5) and defkit (step 6):** the PolicyDefinition webhook's `Handle`, and defkit's `ToCue` and `Register`.

Two things are not shown verbatim:
- **The `kubectl` prefix**, which wraps each message.
- **The `(requestUID=...)` suffix**, which the webhook appends and is left out above.

## Limits

- **Definitions that extend another:** the view of the context is given without types that need an import, and reads in the parent's template are not checked for guards.
- **Dynamic reads:** `context.custom[k]` with a computed `k` is not checked.
- **Disjunction schemas:** a schema that is a disjunction (`close({...}) | close({...})`) is not read at admission. That policy counts as unknown.
- **Spec-rewriting policies:** a plain required parameter that a policy fills by rewriting the Application's spec is still not visible to admission.
