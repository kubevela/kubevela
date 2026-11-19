# Design 04: Cluster Context (baseline metadata + Config stretch goal)

**Status:** In Progress

**Companion to:** [KEP-2.20](../README.md), [KEP-2.13](../../2.13-addons/README.md)
**Related:** [Design 01](./01-module-crd.md) (Module),
[KEP-2.13 Design 03](../../2.13-addons/design/03-cuex-components-instead-of-crs.md) (component model).

> **TL;DR**
> - Cluster context feeds CueX-evaluated `enabled` expressions (e.g. gate an API line on
>   `provider == "aws"`). It is **fork-neutral**: needed whether a Module/Addon is a CR or a
>   component.
> - **Baseline (now):** inject the target cluster's **labels and annotations** into the CUE
>   context. Zero new subsystem; covers the common gating cases.
> - **Stretch goal:** KEP-2.20's richer typed/merged `Config`-based context — sequenced later,
>   and it **merges with** the baseline labels rather than replacing them.
> - **Blocking prerequisite:** the local **hub** has no cluster-metadata entry today, so
>   label-based context resolves on spokes but not the hub. This must be closed before
>   `context.cluster.*` satisfies "same context shape on every target."

## Overview

Cluster context is not specific to either side of the CR-vs-component fork (see the
[design index](../../2.13-addons/design/README.md)). `enabled` gating, label/annotation
injection, the Config stretch goal, and the hub-metadata prerequisite all apply whether a
Module/Addon is delivered as a CR reconciled by a controller or as a component that renders a
child Application. It is extracted here so it reads as the shared concern it is, rather than
living inside one fork's doc.

Where `enabled` is *evaluated* does differ slightly by model — in the component model it lives
naturally in the module component's CueX provider (which already runs CueX to render the child
Application), so no new evaluation path is needed; in the CR model the controller runs the same
CueX evaluation. Either way the *context values* below are the same.

## Baseline and stretch goal

KEP-2.20 specifies a rich cluster-context mechanism (a `cluster-context-schema` ConfigTemplate
plus merged `Config` resources) to feed CueX-evaluated `enabled` expressions in `_version.cue`.
For the context values themselves, the recommendation is to **start with cluster metadata and
treat the richer Config-based model as a stretch goal that merges with it.** Two layers:

- **Baseline (now):** inject the target cluster's **labels and annotations** into the CUE
  context. This requires no new subsystem and enables the common gating cases:

  ```cue
  // baseline - no config subsystem required
  enabled: context.cluster.labels["kubevela.io/provider"] == "aws"
  ```

- **Stretch goal:** the richer typed/merged `Config`-based context from KEP-2.20
  (the `cluster-context-schema` ConfigTemplate plus merged `Config` resources). This is **not
  dropped**; it is sequenced later. When it lands it should **merge with** the injected cluster
  labels/annotations rather than replace them, so labels remain the always-available floor and
  Config augments it with schema-backed, typed values. The exact merge precedence and schema
  are to be revised down the line.

This keeps cluster labels as the zero-plumbing baseline while preserving KEP-2.20's context
model as the richer target, composed on top rather than in competition.

## Implementation prerequisite: the local hub needs a cluster-metadata entry

The baseline (inject cluster labels/annotations) works for **spoke** clusters because they are
registered with metadata the resolver can read. The **local hub does not have an equivalent
cluster-metadata entry today.** This is a concrete, blocking prerequisite, not a general
aspiration: until the hub is representable the same way a spoke is, `context.cluster.labels[...]`
resolves for spokes but has nothing to read on the hub, so an `enabled` expression keyed on
cluster labels would behave differently (or fail) there. The "same context shape on every
target, including the local hub" principle is **unsatisfiable** until this gap is closed.

The required work: give the local hub a cluster-metadata representation (name, labels,
annotations, an `isHub`-style marker) equivalent to a registered spoke, so that the baseline
context resolves identically on hub and spokes. This is a prerequisite for the baseline context
mechanism, ahead of the richer Config model. The exact representation (a self-entry for the hub
cluster, a synthesized `Cluster`-style record, or similar) is an open implementation question,
but that a representation must exist is not optional.

## Open Discussions & Spikes

- **Hub cluster-metadata entry (blocking prerequisite).** Give the local hub a
  cluster-metadata representation equivalent to a registered spoke, so `context.cluster.*`
  resolves identically everywhere. Must be closed before the baseline context mechanism works
  on the hub. Exact representation is open.
- **Config-based context (stretch goal).** The full typed/merged `Config` model from KEP-2.20,
  including merge precedence with the baseline labels and the schema. Sequenced after the
  baseline; candidate for its own "Context Evolution" companion document.
