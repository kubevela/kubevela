# KubeVela 1.10.6 → 1.11 CUE upgrade behaviour — investigation design

## Context

A ComponentDefinition (CD) written when KubeVela 1.10.6 shipped uses CUE
v0.9. After upgrading KubeVela to 1.11.x, the embedded CUE engine is
v0.14, which has a documented breaking change: the `+` operator no
longer concatenates lists. CDs that relied on list concatenation now
fail to compile.

The product question is what happens to an Application that was created
under the old CD: does it survive the upgrade, does reconciliation
explode, what does the validating webhook do, where is the CUE template
actually read from on the reconcile path, and what does the health
status look like across all of this.

## Hypothesis (to confirm / refute)

1. **Untouched reconcile keeps running.** The reconciler does not
   recompile the CD's CUE every loop. It rides the snapshot held by
   the ApplicationRevision and the cached manifests held by the
   ResourceTracker. Pods do not restart.
2. **Webhook rejects spec edits.** Editing the Application's spec
   re-triggers Application admission, which evaluates the CD template
   against new parameters. That evaluation explodes on CUE 0.14 and the
   webhook returns a 400.
3. **ApplicationRevision is the snapshot.** The CD's full CUE template
   is copied into the AppRev `componentDefinitions` map at the time of
   the App's last accepted change. The reconciler reads from the AppRev,
   not from the live CD object.
4. **Health stays whatever it was last set to.** Health computation
   compiles a separate `healthPolicy` CUE block, not the workload
   template, and any error from that path is swallowed. Existing
   `services[*].healthy` value is the carryover.

The cluster work is to prove or break each of these four.

## Method

### Cluster

- Use the `k3d-kubevela` kubeconfig context the user supplied.
- Confirm reachability, confirm no prior vela install, confirm CRDs are
  absent. If any are present, document it before proceeding.

### Versions

- 1.10.6 installed via `vela install --version 1.10.6`.
- 1.11.0-alpha.3 installed via `vela install --set
  controllerArgs.reSyncPeriod=30s --version 1.11.0-alpha.3`. Same
  command, vela CLI handles the upgrade path. The 30s resync gives
  observations a tight loop instead of waiting on the 5-min default.

### Fixtures

One CD and one Application, written inline in the doc so anyone can
reproduce them without `localtest/rttest/audit/`.

- **CD `bad-cd`.** Renders one Deployment. The template uses the CUE
  `+` operator to concatenate two `[...string]` lists into the
  container's `command` field. Compiles under 0.9, fails to compile
  under 0.14.
- **App `victim`.** References `bad-cd` once. Supplies the two list
  parameters.

### Observations

Each observation captures: the question, the kubectl invocation, the
verbatim output trimmed to the relevant bytes, the verdict.

- **O1 — Untouched reconcile after upgrade.** Watch two reconcile
  cycles, ~90s. Snapshot the controller log, the Deployment's UID and
  pod restart count, and the App's status before and after. Verdict:
  did anything change.
- **O2 — Spec edit after upgrade.** `kubectl patch app victim -p
  '{"spec":{"components":[...]}}'` flipping one parameter. Capture the
  apiserver response verbatim. If accepted, capture the next
  reconcile.
- **O3 — Where is the CUE read from.** Two probes: (a) `kubectl get
  apprev -o yaml | grep -A 40 "bad-cd"` to prove the template is in the
  snapshot; (b) `kubectl delete componentdefinition bad-cd`, then bounce
  the kubevela-vela-core controller pod to force a fresh reconcile
  without touching the App's spec. Observe whether the App still
  reconciles cleanly. If it does, the live CD is not on the reconcile
  path. Touching App spec for this probe would be wrong — it triggers
  the webhook and confounds the result.
- **O4 — Health across O1, O2, O3.** Read `.status.services[].healthy`
  and `.status.conditions` at each step.

## Output

A single Markdown file at
`docs/superpowers/specs/2026-06-05-kubevela-cue-upgrade-behavior.md`
with:

1. TL;DR (two sentences).
2. Setup (versions, fixtures, repro).
3. Answers to the five user questions, one paragraph each.
4. Observations O1–O4 with raw evidence inline.
5. Why it works this way — short architecture note (AppRev as
   snapshot, ResourceTracker as manifest cache, webhook as the door,
   reconciler as the indoor traffic).
6. What's still risky — silent-failure surfaces that matter for prod.

No matrix. No appendix. No remediation playbook. No CI test harness
section. If we want any of that, we add it in Approach 2 later.

## Out of scope (Approach 2)

- Recovery test: rewriting the CD with `list.Concat` to see whether
  the App self-heals.
- `app.oam.dev/autoUpdate` and `publishVersion` interaction.
- Multi-output CDs (Deployment + ConfigMap) and what happens when one
  output is added during the broken window.
- Validating webhook on CD admission (compile-time check) vs.
  Application admission (eval-time check).
- The three upstream bugs identified in the prior 2026-05-25 doc.

## Success criteria

The doc, read cold by someone who has never seen this codebase, must
let them answer:

- Will my running app break on upgrade?
- What happens if I try to change it after?
- Where does the controller actually read CUE from?
- Why isn't my health status reflecting reality?

All four answered with evidence, no hedging, no extra material.
