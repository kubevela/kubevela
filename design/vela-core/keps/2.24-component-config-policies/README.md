> ⚠️ **Early concept draft.** This KEP is an early-stage exploration. It is **incomplete and may be inaccurate**, its direction is unsettled, and it should not be relied upon for implementation or as a description of committed behaviour. Expect substantial change.

# KEP-2.24: Component Configuration & Policies as Transforms

**Status:** Drafting (Not ready for consumption)
**Parent:** [vNext Roadmap](../README.md)

Policy is not a category of features. It is **one mechanism, cross-cutting override**,
that became a namespace for eleven unrelated ones. This KEP moves per-resource
configuration onto the component where it belongs, and reduces policy to a CUE transform
over component structure.

## The Three-Layer Model

| Layer | Drives | By |
|---|---|---|
| **Annotation** | controller behaviour | being the only thing the controller reads off a resource |
| **Trait** | component behaviour | applying the controller annotations to that component's resources |
| **Policy** | application behaviour | overriding the traits of one or more components |

Scope widens going down the table, from one resource to one component to the whole
application. Mechanism narrows going up: each layer is expressed purely in terms of the
one above it.

**No layer skips a level.** A policy cannot stamp an annotation; it can only attach or
alter a trait. A trait cannot call the controller; it can only emit an annotation. That
single constraint is what produces most of the properties this KEP argues for elsewhere:
third-party CUE stays off the deletion path because it can only write a value that fixed
Go interprets, the spoke can be sent traits and never need to know a policy existed, and
"why did this resource survive deletion" is answerable with `kubectl get -o yaml` instead
of by reading eight files and a selector.

It also says what is *not* here. `placement` and `dispatcher` fit no row, because the
controller needs them before there is a rendered resource to annotate. Those stay
first-class fields on the component; see [Trait or field](#trait-or-field).

| Mechanism | What it does | Status today |
|---|---|---|
| Component properties | placement, GC intent, drift paths, update semantics, sharing, ownership | expressed as app-scoped policies with resource selectors |
| **Override** | cross-cutting mutation of those properties, per environment | exists (`override`), and is the only structurally necessary policy |
| **Inject** | app-scoped additions attached to no component | exists, as a rendered `PolicyDefinition` |

## Motivation

`pkg/appfile/policy_kind.go` documents the problem in its own header comments:

> The distinction was previously implicit, a switch in `parsePolicies`, a scope lookup
> elsewhere, which is why all three shared one set of rules and got the narrowest of them.

> `parsePoliciesFromRevision` carries a *different* list, it omits replication, so an
> Application parsed from a revision classifies that one policy differently from the same
> Application parsed fresh. That inconsistency predates this file and is not resolved here.

> `appScoped` cannot be derived from the type alone... Five places need this answer.

Eleven built-in policy types, with behaviour scattered across `appfile/parser.go`,
`workflow/step/generator.go`, `workflow/providers/multicluster/deploy.go`, `pkg/policy/`,
`config/factory.go`, `addon/render.go`, and eight files in `resourcekeeper/`.

### Origins

`PolicyDefinition` can only do one thing: **render resources**. It can add objects to the
output. It cannot change what the controller does.

So every policy that changes *behaviour* rather than adding resources has no expressible
form, and is therefore hardcoded into whichever subsystem consumes it. The dumping ground
is not carelessness. It is the only door that was open.

## The Selector Diagnostic

Six of the eleven types embed `ResourcePolicyRuleSelector`: `apply-once`,
`garbage-collect`, `resource-update`, `read-only`, `take-over`, `shared-resource`. Its
fields are:

```go
CompNames, CompTypes, OAMResourceTypes, TraitTypes, ResourceTypes, ResourceNames
```

Every one reaches *down* from app scope to pick out individual resources. **The selector
exists purely to undo a scope mismatch.** If a policy needs one, it is a per-resource
property declared at the wrong level.

The specs confirm it. `apply-once` carries field paths like
`spec.template.spec.containers[0].resources` that are permitted to drift.
`resource-update` carries `RecreateFields`. Those are facts about a resource *type*, not
about an application: a definition supporting HPA knows `replicas` is externally managed;
a definition emitting a Service knows `clusterIP` is immutable. No application author
should be restating either.

## Verdict Per Policy

| Policy | Verdict | Expressed instead as |
|---|---|---|
| `override` | **Genuine policy.** Varies components from outside, expressing what the author did not anticipate. | A `#TransformSpec` returning a modified component, evaluated per cluster. Writes the same fields an author would |
| `topology` | **Becomes a component field plus override.** Placement must resolve before rendering, so it cannot be a trait. See below. | `placement` on the component, varied by `override` |
| `apply-once` | Definition default, author override. It knows which paths are externally managed; the author knows what the cluster does to them. | The `apply-once` trait. [Worked below](#worked-example-apply-once). |
| `resource-update` | Definition. Immutable fields are a type property. | The `apply-once` trait's `recreateFields` |
| `shared-resource` | Resource. It is or is not shared. | The `ownership` trait, `mode: shared` |
| `read-only` | Resource. It is or is not externally owned. | The `ownership` trait, `mode: read-only`, usually on a `from*`-resolved external resource |
| `garbage-collect` | Definition default (it knows whether it holds data); situational variance stays an override. | The `gc-strategy` trait |
| `replication` | Placement in disguise. Its own doc comment says it must be used with `override`. | The `placement` field, varied by `override`. No separate concept |
| `take-over` | Not a policy, an operation. Adoption is a migration mode, not steady state. | `vela adopt`, or a workflow step. Not a standing field in the Application |
| `debug` | Not a policy, a runtime flag. | A controller flag or a CLI flag |
| `env-binding` | Deprecated. | Removed |

The principle: **a policy is legitimate when it expresses something the component author
cannot know and should not decide.** Only two categories qualify, environment binding and
cross-cutting variation. Everything else drifted in for want of anywhere else to go.

## Topology and Placement

`Placement` already carries `ClusterLabelSelector map[string]string`, and cluster labels
are already first class: `NewVirtualClusterFromSecret` copies every label off the cluster
Secret onto `VirtualCluster.Labels`, and `FindVirtualClustersByLabels` selects on them.

So a component can carry its own placement and resolve against real clusters with code
that exists today. Topology-as-policy is not performing resolution only a policy can do;
it is where the selector happens to be written.

The real question is what the component names:

```cue
// concrete: simple, resolves today, couples app artefacts to fleet label conventions
placement: clusterLabelSelector: {tier: "prod", region: "eu"}

// abstract: apps stay portable, needs a resolver
placement: "regional"
```

The concrete form puts fleet knowledge into the app artefact, so a change to the
labelling scheme breaks every application. The abstract form keeps apps portable at the
cost of a resolver, but that resolver is a single fleet-level artefact rather than a
policy per app, and it is a natural extension point
([KEP-2.23](../2.23-plugins/README.md)).

Expected answer: support both, recommend the abstract form, make the resolver pluggable.

## Trait or Field

Component-level behaviour has two possible carriers, and picking between them is the
question this section exists to answer.

**A behaviour that operationally affects a resource is a trait. A behaviour KubeVela
itself needs in order to function is a first-class field.**

The mechanical form of that: **can the effect be carried by an annotation on a rendered
resource?** If it can, it is a trait. If it cannot, the controller needs it before there
is anything to annotate, and it is a field.

| | Trait | Component field |
|---|---|---|
| Answers | how should this resource be treated once it exists | what must KubeVela do to produce or deliver it at all |
| Carrier | `component.oam.dev/*` on the rendered object | read by the controller directly, never rendered |
| Consumed by | `resourcekeeper`, apply, GC | placement resolution, dispatcher selection |
| Extensible by an organisation | yes, with a `TraitDefinition` | no, the controller switches on it |
| Examples | `gc-strategy`, `ownership`, `apply-once`, `resource-update` | `placement`, `dispatcher` |

`gc-strategy: never` is a trait because it says what to do with a PVC that already
exists. `placement` cannot be, because it decides whether that PVC exists in a given
cluster at all, and it has to resolve before any per-cluster rendering: this KEP's own
transform signature is `(component, clusterContext) → component`, which presupposes the
cluster. `dispatcher` cannot be either, because it names the mechanism that carries the
resource rather than any property of it, and the choice is consumed by code that runs
outside the render.

**The risk is field growth.** "Central to how KubeVela functions" is a judgement, and
judgements of that shape trend towards yes. The annotation test is the guard: a field has
to be read before render by a named controller, and anything that fails that is a trait.

## Component Behaviour

```yaml
components:
  - name: db
    type: stateful-service
    properties:
      image: postgres:16
    placement:                        # field, resolved before rendering
      abstract: regional
    traits:
      - type: gc-strategy
        properties: {strategy: never}
      - type: ownership
        properties: {mode: shared}
```

`properties` is what the component *is*, `traits` are how it should be treated, and
`placement` is what KubeVela must know before it can treat it at all. None of them belong
in a separate object that reaches back and names the component by string.

This is the umbrella roadmap's mechanism rather than a new one. "Traits own behaviour"
already names `apply-once`, `gc`, `read-only` and `ownership` as traits, and the trait
`pre`/`default`/`post` phases already express the pre-render versus post-render split
this KEP would otherwise have to invent.

### Layering and Precedence

The trait on the component is not the only source. A definition still knows structural
facts an author should not have to restate, and a policy still exists to vary things
across many components at once. They compose as a strict precedence chain:

```
ComponentDefinition default   the definition knows a PVC holds data
        ↓ overridden by
trait on the component        the author declares it for this instance
        ↓ overridden by
policy injects a trait        cross-cutting, per-cluster variation
        ↓ all emit
component.oam.dev/* annotations   the wire contract; resourcekeeper reads only this
```

Each layer only ever overrides the one above, which is this KEP's thesis applied to
itself: **policies override, they do not originate.** The roadmap's "policies inject
traits" supplies the third layer with no separate merge story, and its consequence,
**policies never reach the spoke**, follows directly: the spoke sees traits and cannot
tell which were author-declared and which were injected.

> **This reverses two earlier drafts, and both errors are recorded here.** The first had
> the definition stamp annotations and claimed no new API was needed, which left an author
> with no way to express intent the definition had not anticipated. The second put a closed
> `policies` map on the component, which fixed that but invented a parallel mechanism for a
> subset of what traits already do. That is the same mistake this KEP attributes to
> policies, made one level down.

### Vocabulary

Three behaviours, taken from the roadmap rather than invented here. Each is a trait, and
each emits an annotation of the same name:

| Trait | Annotation value | Replaces |
|---|---|---|
| `ownership` | `exclusive` (default), `shared`, `read-only` | `shared-resource`, `read-only` |
| `gc-strategy` | `onAppDelete` (default), `onAppUpdate`, `never` | `garbage-collect` |
| `apply-once` | `{skipPaths: [...], recreateFields: [...]}` | `apply-once`, `resource-update` |

**Nobody writes the annotation.** It is emitted, and it exists so `resourcekeeper` has one
thing to read. That matters most for `apply-once`, which carries field-path lists and so
cannot be an enum: as an annotation it is JSON with no API server validation, but the
trait's `parameter` schema is validated at admission like any other definition. The
authoring surface being a definition rather than a raw annotation is what removes that
problem instead of living with it.

**These three are shipped traits, not special cases.** They have no privileges an
organisation's own trait would not have, which is what makes the vocabulary open: a
fourth behaviour needs a `TraitDefinition`, not a KubeVela release.

### Mechanism

A trait does not call anything. It stamps an annotation, and the annotation is read by
whichever Go code path already handles that concern:

```
TraitDefinition
  patch: metadata: annotations: "component.oam.dev/gc-strategy": "never"
        ↓ CUE unification during render
rendered object carries the annotation
        ↓ travels through dispatch as ordinary metadata, nothing special
applicator / resourcekeeper reads it off the object and branches
```

**This is not speculative. One of the eleven already works exactly this way.**
`shared-resource` is annotation-driven end to end today: `app.oam.dev/shared-by` is
written onto the manifest in `resourcekeeper/gc.go` and `utils/apply/apply.go`, and read
back in `apply.go` and `resourcekeeper/cache.go`. The only thing this KEP changes is where
the annotation *originates*. Today a policy computes it through
`SharedResourcePolicySpec.FindStrategy`; under this a trait emits it. The consumption half
needs no invention, which is the strongest evidence available that the contract is
workable.

The work on the consumption side is subtraction. Seven files in `pkg/resourcekeeper`
import the policy types and switch on them, and each becomes a read of a field it already
has on an object it already holds.

### Gap: Annotating a Component's Full Output

This is the mechanism's weakest link and it is a real blocker, not a detail.

`patch` reaches the base workload only. `patchOutputs` iterates the component's auxiliary
outputs and looks each one up **by name**, skipping any it does not name:

```go
for _, auxiliary := range auxiliaries {
    target := outputsPatcher.LookupPath(value.FieldPath(auxiliary.Name))
    if !target.Exists() { continue }
    ...
}
```

There is no wildcard. So a generic `gc-strategy` trait used across twenty
`ComponentDefinition`s would have to enumerate the internal output names of all twenty,
which couples every behaviour trait to every definition it is used with and breaks the
moment a definition renames an output. That is unusable for exactly the behaviours this
KEP wants traits to carry, because those apply to *the component's resources* rather than
to one named resource.

Three ways out, none chosen:

| Option | Cost |
|---|---|
| A wildcard in `patchOutputs` | Smallest change, but "patch everything" is a sharp tool to hand every trait author |
| The controller propagates after render | Matches the roadmap's `post` phase, but the behaviour is then not expressed in the trait's own CUE, which weakens "traits own behaviour" |
| The annotation is set once on the component and stamped by the controller onto everything it rendered | Cleanest to reason about, and closest to a field again, which reopens the trait-or-field question for exactly these three |

Whichever wins, it should be settled before any of the three traits are written, because
all three depend on it and the answer changes what they look like.

### Worked Example: `apply-once`

This is the least obvious of the eleven, because `apply-once` looks situational and the
application author is the one who currently sets it. It needs working through, since
if the answer is unconvincing here the whole verdict table is unconvincing.

Today the author must know that an HPA will fight KubeVela over `spec.replicas`, and say
so in a policy that names the component by string:

```yaml
policies:
  - name: no-drift
    type: apply-once
    properties:
      enable: true
      rules:
        - selector: {componentNames: ["backend"]}
          strategy: {path: ["spec.replicas"]}
```

Under this KEP nobody writes that. The definition already knows: if it emitted an HPA, it
knows something else owns `replicas`.

```cue
// ComponentDefinition: webservice
output: {
    apiVersion: "apps/v1"
    kind:       "Deployment"
    metadata: annotations: {
        if parameter.autoscaling != _|_ {
            "component.oam.dev/apply-once": json.Marshal({skipPaths: ["spec.replicas"]})
        }
    }
    spec: {
        if parameter.autoscaling == _|_ { replicas: parameter.replicas }
        ...
    }
}

parameter: {
    autoscaling?: {minReplicas: int, maxReplicas: int}
    replicas:     *1 | int
}
```

The application author asks for autoscaling and never mentions drift:

```yaml
components:
  - name: backend
    type: webservice
    properties:
      image: acme/backend:1.4
      autoscaling: {minReplicas: 2, maxReplicas: 10}
```

**That answers the objection that users only interact through components.** They still
do. What changes is that the intent is derived from what they asked for rather than
restated in a second place, in a policy that names the component by string and can
silently stop matching when the component is renamed.

Now the two cases the definition cannot know, and where each lands:

**A path the definition never anticipated.** A service mesh injects a sidecar and
KubeVela reverts its `resources`. No definition foresees that. The author says so on the
component:

```yaml
- name: backend
  type: webservice
  properties: {image: acme/backend:1.4}
  traits:
    - type: apply-once
      properties:
        skipPaths: ["spec.template.spec.containers[1].resources"]
```

Which merges with, rather than replaces, whatever the definition contributed. This is the
layer whose absence made the earlier draft unworkable.

**The same thing across forty components in one cluster.** That is a policy, and it is
what policies are for:

```cue
// PolicyDefinition: mesh-drift  (pre-render #TransformSpec)
$returns: component: $params.component & {
    if context.cluster == "prod-mesh" {
        traits: [{
            type: "apply-once"
            properties: skipPaths: ["spec.template.spec.containers[1].resources"]
        }]
    }
}
```

The policy attaches the same trait the author would have attached. It does not reach past
the component into the rendered resource, and it needs no selector vocabulary to say
which components it applies to, because it is invoked per component already.

### The Annotation Contract

The component field is what people write. The annotation is what the machinery reads, and
the two are deliberately different surfaces:

```
definition default → author-declared trait → policy-injected trait
                                        ↓
                            stamped onto each rendered resource
                                        ↓
resourcekeeper / dispatch      →  read annotations only, know nothing about policy types
```

`resourcekeeper` references policies in eight files and imports the policy types directly
in seven of them, switching on them.
Under this it reads declared intent off each resource and has no idea a policy exists.
The decoupling matters because **policy authoring and policy consumption are currently the
same code.**

Two consequences follow:

- **Third-party code stays off the deletion path.** A transform can only write an
  annotation that a fixed Go code path interprets. It cannot decide to delete anything.
- **Policy effects become inspectable.** Working out why a resource survived deletion is
  `kubectl get pvc -o yaml`, not reading a policy's selectors and knowing which of eight
  files consumed it. Dry-run becomes meaningful for policies for the first time.

Precedent: `helm.sh/resource-policy: keep` and Argo's `Prune=false` are the same pattern
for the same reason. KubeVela has its own, too: `app.oam.dev/shared-by` is already a
resource annotation recording cross-application intent, so the contract described here is
an extension of something in tree rather than a new idea.

## Policies as Transforms

A policy becomes a pure function over component structure, written in CUE.

**There are two stages, not one.** `override` mutating `properties.image` must run before
rendering or the template never sees it. Stamping an annotation on a PVC must run after,
because the PVC does not exist until then.

```cue
#TransformSpec: {
    $params:  {component: <spec>,      context: {cluster: string, ...}}
    $returns: {component: <spec>}
}

#TransformOutput: {
    $params:  {resources: [...],       context: {cluster: string, ...}}
    $returns: {resources: [...]}
}
```

| Stage | Policies |
|---|---|
| Pre-render (`#TransformSpec`) | `override`, `replication` |
| Post-render (`#TransformOutput`) | `garbage-collect`, `apply-once`, `read-only`, `shared-resource`, `resource-update` |

That the split is clean is evidence it is the real seam.

### Per-Cluster Variation

Signature is `(component, clusterContext) → component`, evaluated once per
(component, cluster) after placement resolves. Not a component carrying a
`clusters[name]{...}` tree, which would grow with the fleet and make it ambiguous what a
transform is modifying. `override`'s per-placement variation then needs no special case, because
the transform reads `context.cluster`.

### Rationale

A transform is a **pure function**, so a policy can be unit tested with no cluster, no
Application and no controller. Testing `apply-once` behaviour today means exercising
`resourcekeeper`. It is also the idiom already in use: `ComponentDefinition` is
`parameter → resources`; a policy is `component → component`.

The eleven built-ins become eleven shipped CUE transforms, readable in the repository and
copyable as a starting point. Today `apply-once`'s semantics live across
`resourcekeeper/statekeep.go` and its neighbours, and you must read Go to learn them.

## Constraints

**Ordering becomes explicit, and nobody has decided it.** Today the interaction between
`override`, `replication` and the resource policies is implicit in Go execution order.
Chained transforms force the question open. Declaration order in `spec.policies` is the
obvious default.

**Idempotency is a requirement, not an expectation.** A transform that appends to a list
works once and corrupts on the second reconcile, and the failure is intermittent. This is
more acute now that `skipPaths` merges across the three layers rather than replacing: merge is
exactly the operation that is not naturally idempotent. Test it mechanically: run every
transform twice, assert the result is identical.

**This puts CUE on a hot path where Go is today.** `garbage-collect` is currently a
selector match in Go; under this it is a CUE evaluation per component per cluster per
reconcile. That is KEP-2.23's budget and caching problem arriving in the most
performance-sensitive place available. **Measure before committing, not after.**

**Provenance, or debugging becomes miserable.** With three layers and N chained
transforms, "why does this have 3 replicas" needs an answer, and so does "why is this PVC
still here when I set `gc-strategy: onAppDelete`". Provenance has to record the layer as
well as the policy, since "the definition already said `never` and nothing overrode it" is
a different answer from "a policy set it back". Feasible, arguably a bigger win than the
extensibility, and much harder to add later.

## Migration

Eleven behaviour-preserving rewrites. The success criterion is testable and the existing
suites are the oracle: behaviour must not change. Also resolved as a side effect:

- the `builtinPolicyTypes` map disappears, because everything renders
- the divergent `parsePoliciesFromRevision` list disappears with it
- the built-in / rendered / application-scoped trichotomy collapses
- "five places need this answer" stops being true

## Relationship to Other KEPs

| KEP | Relationship |
|---|---|
| [2.23 Plugins](../2.23-plugins/README.md) | Supplies the extension mechanism. This KEP reduces the surface from eleven candidate points to two: placement resolution and render-time transformation. |
| [2.2 Spoke controller](../2.2-spoke-controller/README.md) | **Shared seam.** Transforms produce what a dispatched `Component` CR carries, and the annotation contract is what the spoke consumes. The contract becomes a cross-cluster API rather than an in-process one, so version skew matters far more. |
| [2.4 Dispatchers](../2.4-dispatchers/README.md) | `topology` moving to components changes `targetsTemplate`'s inputs. Placement resolution becomes shared rather than dispatcher-owned. |
| [2.9 App templates](../2.9-app-templates/README.md) | Application-scoped `PolicyDefinition` is the `Inject` mechanism this KEP keeps. |
| [2.21 `from*` resolution](../2.21-from-resolution/README.md) | Component-declared placement resolved by a fleet-level resolver is the same declare-then-resolve idiom. |

## Relationship to the vNext Roadmap

> **This KEP overlaps the umbrella README and has not yet been reconciled with it.**
> Recorded here so the overlap is visible and the merge is scoped, rather than two
> documents describing the same direction differently.

The roadmap already states the direction in four lines:

> **Annotations as behaviour contract** — `component.oam.dev/apply-strategy`,
> `component.oam.dev/ownership`, `component.oam.dev/gc-strategy` replace feature flags,
> policy-driven apply options, and per-manifest cluster routing labels

> **Traits own behaviour** — anything that modifies a component's operational behaviour is
> a trait (`apply-once`, `gc`, `read-only`, `ownership`)

> **Three phases** — `pre` (mutate inputs before CUE evaluation), `default` (CUE
> unification with template), `post` (dispatched after health check)

> **Policies inject traits** — purely application-layer concerns; pure functions over
> application state that attach traits to matching Components before dispatch

**The roadmap's mechanism is better than this KEP's earlier drafts and has been adopted
above.** A trait is the authoring surface and the annotation is the wire contract it
emits. That is more KubeVela-native, gives behaviour a definition with parameters and CUE
like anything else, and answers "who stamps the annotation, and with what authority"
cleanly. The trait `pre`/`default`/`post` phases already express the pre-render versus
post-render split this KEP would otherwise invent. Two earlier drafts here got it wrong,
first by having definitions stamp annotations directly and then by adding a closed
`policies` map to the component; both are recorded above rather than quietly dropped.

What this KEP adds beyond the roadmap:

| Addition | Why it matters |
|---|---|
| The `ResourcePolicyRuleSelector` diagnostic | A mechanical test for which policies are misplaced, covering all eleven rather than four examples |
| The `policy_kind.go` evidence | The trichotomy, the divergent `parsePoliciesFromRevision` list, "five places need this answer". That file postdates the roadmap |
| Topology moves too | The roadmap keeps `topology` as a policy. `ClusterLabelSelector` plus cluster labels already make component-declared placement resolvable today |
| Ordering, idempotency, provenance | Consequences of chained transforms that nothing currently records |
| CUE on the reconcile hot path | The cost of moving Go selector matching into CUE, unmeasured and the largest risk here |
| The trait-or-field test | The roadmap says traits own behaviour but does not say what is left over. `placement` and `dispatcher` are read before render, so no annotation can carry them |

**Merge direction when reconciled:** adopt traits as the carrier, keep the diagnostic, the
evidence, the topology argument and the four constraints as the detailed case for a
direction the roadmap states in a line.

## Open Questions

- **Concrete selector, abstract shape, or both** for component-declared placement, and
  who owns the abstraction vocabulary.
- **Can a definition express a hard constraint** rather than a default, so a policy cannot
  silently override "this PVC holds data"? Needs a real case; Helm and Argo both let the
  operator win.
- **Annotation authority.** Anyone who can write a resource can edit its annotations. Is
  hub-stamped intent authoritative and drift-corrected, or advisory? The question sharpens
  once the annotation crosses a cluster boundary and lands where other actors can edit it;
  see [KEP-2.2](../2.2-spoke-controller/README.md).
- **The app-scoped residue.** Revision limits and workflow behaviour do not fit an
  annotation on a rendered object. The model must not pretend everything does.
- **Two policies stamping the same annotation differently** needs explicit precedence.
  The conflict exists today, buried in execution order; this surfaces it.
- **`take-over` as an operation.** If adoption is not a policy, what is it?
  [KEP-2.15](../2.15-operations/README.md) may be the right home.
- **Where is the line for a first-class field?** `placement` and `dispatcher` are both
  read before render, so both pass the test above. Nothing else currently does, and the
  next candidate should be argued against the test rather than added by analogy. The
  failure mode is a component spec that accretes fields for a decade.
- **Merge or replace, per trait.** Two `apply-once` traits, one from the definition and
  one from the author, plainly want their `skipPaths` merged. Two `gc-strategy` traits
  plainly want the later to win. So the rule is per trait rather than uniform, and a trait
  needs somewhere to declare which it is. Traits have no such field today.
- **Can a trait be open to the deletion path?** These three emit annotations that
  `resourcekeeper` obeys. Any `TraitDefinition` can emit the same annotation, so any trait
  author can make a resource undeletable. That may be correct, since it is the same grant
  as writing the annotation directly, but it should be a decision rather than a
  consequence.
- **Can a definition forbid an override?** A definition that knows a PVC holds data may
  want `gc-strategy: never` to be a floor rather than a default. This is the same question
  as hard constraints above, now with a concrete field to hang it on, and the two should
  be settled together.

## Verified Versus Asserted

**Verified by reading code.** That `app.oam.dev/shared-by` is already an annotation-driven
behaviour end to end, written in `resourcekeeper/gc.go` and `utils/apply/apply.go` and read
in `apply.go` and `resourcekeeper/cache.go`, which is the proposed contract already
shipping for one of the eleven; that `patchOutputs` resolves auxiliaries by name with no
wildcard (`cue/definition/template.go`), so a trait cannot annotate every resource a
component produced; that eight files in `pkg/resourcekeeper` reference policies and seven
import the types directly. The eleven built-in policy type constants; that six embed
`ResourcePolicyRuleSelector` and what its fields are; `ApplyOncePolicySpec`'s drift paths
and affect stages; `ResourceUpdatePolicySpec`'s `RecreateFields`;
`GarbageCollectStrategy`'s three values (`never`, `onAppUpdate`, `onAppDelete`), which
the `gc-strategy` key reuses verbatim; that `ReadOnlyPolicySpec`,
`SharedResourcePolicySpec` and `TakeOverPolicySpec` carry nothing but a selector, which
is what makes them resource
facts rather than policies; that `app.oam.dev/shared-by` already exists as a resource
annotation; that `Placement` carries
`ClusterLabelSelector`; that cluster Secret labels are preserved onto `VirtualCluster` and
selectable; that `ReplicationPolicySpec`'s own comment requires `override`; the scattering
of policy handling across `appfile`, `workflow`, `policy`, `config`, `addon` and eight
files in `resourcekeeper`; the `policy_kind.go` comments quoted above.

**Asserted, not verified.** That the pre/post-render split covers every built-in cleanly
has been reasoned from each spec, not proven by rewriting them. Performance claims about
CUE on the reconcile path are unmeasured, and are the largest risk in this KEP. That
per-component placement is sufficient for every case `topology` serves today has not been
checked against real Applications.
