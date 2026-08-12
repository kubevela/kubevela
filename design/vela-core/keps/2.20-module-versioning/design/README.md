# KEP-2.20 Design Notes (companion exploration)

These are companion design notes to [KEP-2.20](../README.md), exploring the Module and API
line versioning model. They are an **active exploration**, not accepted design; the KEP
remains the baseline. This index is the front door: read it first.

## Reading order

1. **[Design 01: Module CRD, Registry Publishing & Module-Owned Application](./01-module-crd.md)**
   — what a Module is (an independently installable/publishable API unit: definitions + the
   auxiliary resources that back them), its identity, and how it is delivered. Describes the
   **CR + controller** baseline while flagging the component alternative.
2. **[Design 02: Module Namespaces, Tenancy & `vela-system` Coexistence](./02-namespace-and-tenancy.md)**
   — the namespace model (`vela-module-<name>`), why namespaced definitions make it clean,
   resolution, tenancy/RBAC, and coexistence with legacy `vela-system` definitions. Hub doc for
   anything namespace/resolution.
3. **[Design 03: API Line Investigation](./03-api-line-investigation.md)**
   — the largest **open** question: should an API line be encapsulated as state within the
   Module, or have its own durable identity? Presented neutrally; deliberately undecided.
4. **[Design 04: Cluster Context](./04-cluster-context.md)**
   — how `enabled` gating gets its context values: baseline cluster labels/annotations now, the
   richer `Config` model as a merging stretch goal, and the blocking hub-metadata prerequisite.
   Fork-neutral (applies under either the CR or component model).

Foundational context lives in the KEP-2.13 folder — read those first if you have not:
[Per-Component Placement](../../2.13-addons/design/01-per-component-topology-placement.md),
[Everything-is-an-Application](../../2.13-addons/design/02-application-managed-resources.md),
[CueX Components Instead of CRs](../../2.13-addons/design/03-cuex-components-instead-of-crs.md).

## The central fork

One directional decision runs through the whole set, and it is a **fork with two
alternatives, not two complementary layers**:

- **CR + controller model** — a Module is a CR reconciled by a controller that manages an
  Application. This is the baseline, written up in [Design 01](./01-module-crd.md).
- **Component model** — a Module is a component whose CueX provider renders a nested
  Application; **no CRD, no controller**. Written up in
  [KEP-2.13 Design 03](../../2.13-addons/design/03-cuex-components-instead-of-crs.md).

These are **mutually exclusive**; Addon and Module should share the outcome. Design 01 is
written CR-first but marks which of its content is **model-agnostic** (namespace, naming,
Application-as-engine, VelaUX — all hold either way) versus **CR-specific** (finalizer,
controller loop). The API line investigation (Design 03) decides only *whether* a line has
its own identity; the *form* of that identity (CR vs component) follows this same fork.

**What is *not* forked** (holds under either model): Module identity and the
`<module>/<apiLine>/<definition>` reference model; namespace-per-module and its resolution
behaviour; module-mediated publishing; the API line naming and coexistence rules.

## Divergences from the published KEP-2.20 (reconcile on merge-back)

These notes intentionally diverge from KEP-2.20 in a few places; each is flagged in-doc:

- **Definition naming** drops the module prefix (`v1-bucket`, not `aws-s3-v1-bucket`) because
  the namespace carries the module (Design 02).
- **Resolution** keeps only two reference forms (legacy `bucket`, canonical
  `aws-s3/v1/bucket`); the two-segment `v1/bucket` form is dropped (Design 01 identity brief).
- **Definitions are Namespaced, not cluster-scoped** — a factual correction the KEP prose
  needs (Design 02).

## Status of each idea

| Topic | Status |
|---|---|
| Module as independently installable/publishable API unit | Exploration |
| Namespace-per-module + deterministic resolution | Exploration; model-agnostic |
| Module-mediated line publishing (`vela module publish-line`) | Settled within these notes |
| API line: encapsulate vs own identity | Open investigation (Design 03) |
| Immutable revisions (`ModuleRevision`/`APILineRevision`) | Open, conditional on the CR path |
| Full identity/resolution model (locks, override, `vela` module) | Deferred to a future identity design (brief in Design 01) |

Each doc ends with an **"Open Discussions & Spikes"** section listing what it leaves for the
team to resolve.
