# KEP-2.13 Design Notes (companion exploration)

These are companion design notes to [KEP-2.13](../README.md), exploring an evolved
architecture for delivering addons (and, with [KEP-2.20](../../2.20-module-versioning/README.md),
modules). They are an **active exploration**, not accepted design; the KEPs remain the
baseline. This index is the front door: read it first.

## Reading order

Read in sequence; each builds on the previous.

1. **[Design 01: Per-Component Topology Placement](./01-per-component-topology-placement.md)**
   — the primitive. A workflow step can send different components to different clusters
   (hub vs spokes). Implemented in #7213. Everything else rests on this.
2. **[Design 02: Application-Managed Resources](./02-application-managed-resources.md)**
   — "everything is an Application." Because of Design 01, *all* addon resources move inside
   the generated Application, so the lifecycle controller owns intent and the Application owns
   deployment (dispatch, drift, GC, health, VelaUX).
3. **[Design 03: CueX Components Instead of CRs](./03-cuex-components-instead-of-crs.md)**
   — the component model. If the Application owns deployment, an addon/module can be a
   CueX-backed *component* that renders a nested Application, needing **no CRD and no
   controller**. Includes the investigation comparing this to the CRD model.

Related, in the KEP-2.20 folder:
[Module CRD](../../2.20-module-versioning/design/01-module-crd.md),
[Namespaces & Tenancy](../../2.20-module-versioning/design/02-namespace-and-tenancy.md),
[API Line Investigation](../../2.20-module-versioning/design/03-api-line-investigation.md).

## The central fork

One directional decision runs through the whole set, and it is a **fork with two
alternatives, not two complementary layers**:

- **CR + controller model** — an Addon/Module is a CR reconciled by a dedicated controller
  that manages an Application. This is the KEP baseline; the Module form is written up in
  [KEP-2.20 Design 01](../../2.20-module-versioning/design/01-module-crd.md).
- **Component model** — an Addon/Module is a component whose CueX provider renders a nested
  Application; **no CRD, no controller**. Written up in
  [Design 03](./03-cuex-components-instead-of-crs.md).

These are **mutually exclusive end-states**, documented side by side so the choice can be
made deliberately. Addon and Module should share the decision (same rationale at both
layers). Design 02's "thin controller" and Design 03's "no controller" are the two sides of
this same fork — not a controller plus a component on top of it.

**What is *not* forked** (holds under either model): per-component placement (Design 01);
everything-is-an-Application (Design 02); Module identity, namespace-per-module, and the API
line model (KEP-2.20 Designs 01–03). Those stand regardless of how the fork lands.

## Status of each idea

| Topic | Status |
|---|---|
| Per-component placement (Design 01) | Implemented (#7213) |
| Everything-is-an-Application (Design 02) | Exploration; foundation for both fork sides |
| Component model (Design 03) | Exploration; alternative to the CR baseline |
| CR model (KEP-2.20 Design 01) | Baseline |
| API line: encapsulate vs own identity | Open investigation (KEP-2.20 Design 03) |

Each doc ends with an **"Open Discussions & Spikes"** section listing what that doc leaves
for the team to resolve.
