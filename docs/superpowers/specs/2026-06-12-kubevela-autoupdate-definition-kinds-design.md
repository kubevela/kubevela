# KubeVela autoUpdate — which definition kinds does it cover? — investigation design

## Context

KubeVela 1.10+ ships an `app.oam.dev/autoUpdate: "true"` annotation on
Applications. When set, the controller is supposed to roll the App
forward to the latest version of every definition it references,
without the user having to bump `publishVersion` or edit the spec.

The published behaviour for ComponentDefinitions is well known: change
the CD, the next reconcile cuts a new ApplicationRevision and re-renders.

The question this investigation answers is whether the same
auto-roll-forward applies to the three other definition kinds an
Application can reference: TraitDefinition, PolicyDefinition,
WorkflowStepDefinition. The user wants a code-grounded answer first,
then live verification.

## Hypothesis

`DeepEqualRevision` (referenced in `revision.go` around the autoUpdate
gate) compares full ApplicationRevision specs, including the snapshotted
`componentDefinitions`, `traitDefinitions`, `policies`, and
`workflowStepDefinitions` maps. If that hypothesis holds, all four
kinds participate in autoUpdate. To prove or refute we have to:

1. Read `DeepEqualRevision` and confirm which fields it walks.
2. Read the AppRev composition path and confirm all four kinds get
   snapshotted at reconcile time.
3. Run isolated cluster tests, one per kind, to confirm an actual
   re-render happens when only that kind is mutated.

## Method

### Cluster

User-supplied k3d cluster, KubeVela already installed. Kubeconfig
points at `0.0.0.0:53536`, repointed to `host.docker.internal:53536`
for reachability from this dev container.

### Fixtures

A new directory `localtest/rttest/autoupdate-multi/` with these files:

- `10-cd-v1.yaml` — `ComponentDefinition` named `multi-cd`. Renders a
  ConfigMap with a single field `data.cdMarker: "v1"`.
- `11-trait-v1.yaml` — `TraitDefinition` named `multi-trait`. Patches
  the same ConfigMap with `data.traitMarker: "v1"`.
- `12-policy-v1.yaml` — `PolicyDefinition` named `multi-policy`.
  Application-scoped. Writes `policyMarker: "v1"` to something
  observable (a `gc-policy` variant or a `topology` policy with an
  identifying label, depending on what's idiomatic on this build).
- `13-wfstep-v1.yaml` — `WorkflowStepDefinition` named `multi-step`.
  Defines a step that emits `stepMarker: "v1"` into App status, or
  applies the component with a custom step name.
- `20-app.yaml` — `Application` named `multi-app`. Uses all four kinds,
  carries `app.oam.dev/autoUpdate: "true"`.
- `30-cd-v2.yaml` … `33-wfstep-v2.yaml` — same names as the v1 files,
  marker flipped to `"v2"`.

If a particular kind turns out to be impractical to render observable
output (workflow steps in particular can be fiddly), the design falls
back to inspecting the AppRev's snapshotted body for that kind instead
of an in-cluster rendered marker. The code-investigation step will
tell us whether AppRev snapshots are sufficient on their own.

### Test sequence

Baseline first: apply all v1 fixtures, apply the App, wait for the
first AppRev (`multi-app-v1`). Snapshot:

- AppRev list with hashes.
- ConfigMap data fields.
- App `.status` (revision, services, conditions).
- Controller log for the reconcile that produced AppRev v1.

Then, for each kind in isolation:

1. Apply that kind's v2 file. Do not touch the App.
2. Wait the default resync.
3. Capture: new AppRev cut yes/no, ConfigMap changes, App status delta,
   controller log lines for the reconcile that fired.
4. If no new AppRev fired, also confirm by dumping the live definition
   to verify it really did update on the cluster.
5. Revert that kind to v1 (so the next test starts from a clean
   baseline).

This isolates each kind. Combined matrix at the end.

### Observations

- **O1 — Code path:** `revision.go` autoUpdate branch + `DeepEqualRevision`
  + `applicationrevision_types.go`. Captured as inline excerpts in the
  findings doc with file:line refs.
- **O2 — CD v2 applied:** does autoUpdate roll forward.
- **O3 — Trait v2 applied:** does autoUpdate roll forward.
- **O4 — Policy v2 applied:** does autoUpdate roll forward.
- **O5 — WorkflowStep v2 applied:** does autoUpdate roll forward.

## Output

A single Markdown file at
`docs/superpowers/specs/2026-06-12-kubevela-autoupdate-definition-kinds.md`
with:

1. **TL;DR** — generalised statement of which kinds participate.
2. **Setup** — cluster, fixtures, repro.
3. **Code findings** — what `DeepEqualRevision` actually compares,
   with file:line references. This is the load-bearing section; the
   cluster tests are confirmation.
4. **Summary matrix** — one row per definition kind. Columns:
   kind / new AppRev cut on v2 / re-render visible / managed resource
   updates / user-visible signal.
5. **Per-kind evidence** — for each kind, the verbatim cluster output
   sufficient to convince a skeptical reader.

No "what's still risky" section, no observations appendix, no
remediation playbook unless the user asks.

## Out of scope

- Rollback semantics. Whether older AppRevs are usable for `vela revert`
  or similar is a separate question.
- `publishVersion` interaction. We already proved publishVersion bumps
  cut new AppRevs in the prior investigation; we will not re-prove it.
- Multi-cluster autoUpdate. Single cluster only.
- TraitDefinition `appliesToWorkloads` filtering. Out of scope unless
  it bites us during the test.

## Success criteria

The doc, read cold, answers:

- Does autoUpdate cover TraitDefinitions? Yes/no with evidence.
- Does autoUpdate cover PolicyDefinitions? Yes/no with evidence.
- Does autoUpdate cover WorkflowStepDefinitions? Yes/no with evidence.
- If any kind is excluded, what is the user expected to do instead?
