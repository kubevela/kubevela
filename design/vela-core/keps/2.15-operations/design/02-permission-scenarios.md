# Design 02: Permission Scenarios

**Status:** Illustrative. Not a proposal; it works through the model [KEP-2.15](../README.md) specifies to check it behaves sensibly against realistic teams and grants.

**Companion to:** [KEP-2.15](../README.md), in particular [Permissions](../README.md#permissions).

> **TL;DR**
> - Five walkthroughs of one cluster, three roles and four templates, showing what runs, what is refused, and where.
> - Every *permission* refusal lands at `kubectl apply`, not mid-run, including the direct-grant check on children. What can still fail at run time is the service account's own RBAC, as scenarios 4 and 5 show.
> - A central template with a central service account is already bounded team by team, through ordinary `RoleBinding`s, with no field in the KEP for it.
> - Scenario 5 is the honest cost of `OperationsRunAsInvoker`: it works exactly as designed, and the platform stops being self-service.

## The cluster

```
vela-system
├── ServiceAccount/op-backup       can create Jobs, read db secrets
├── ServiceAccount/op-failover     can patch postgres CRs, write DNS records
├── ServiceAccount/op-local        holds nothing by itself, see scenario 4
│
├── OperationTemplate/s3-backup
│     runAs:  {mode: Platform, serviceAccountName: op-backup}
│     attach: {scope: Component, allowedComponentTypes: [aws-s3-bucket]}
│     steps:  backup (impact: Safe)
│
├── OperationTemplate/dr-failover
│     runAs:  {mode: Platform, serviceAccountName: op-failover}
│     attach: {scope: Application, selector: {matchLabels: {dr.oam.dev/enabled: "true"}}}
│     steps:  dispatch pause-writes, dispatch promote-replica
│
├── OperationTemplate/promote-replica
│     runAs:  {mode: Platform, serviceAccountName: op-failover}
│     requireDirectGrant: true
│     steps:  promote (impact: Irreversible)
│
└── OperationTemplate/restart-workload
      runAs:  {mode: Platform, serviceAccountName: op-local}
      steps:  restart (impact: Disruptive)
```

```
payments-prod
├── Application/payments            labels: {dr.oam.dev/enabled: "true"}
│   ├── Component/payments-db       type: postgres
│   └── Component/payments-assets   type: aws-s3-bucket
└── (Operations are created here)
```

Three roles. Each is really a **pair** of `Role`s, because an RBAC grant lives in the namespace of the resource it protects, and these two resources live in different namespaces:

| Role | in `vela-system`, `invoke` on `operationtemplates` | in `payments-prod`, on `applications` | Held by |
|---|---|---|---|
| `dev` | `s3-backup`, `restart-workload` | `operate` | Alice |
| `oncall` | the above, plus `dr-failover` | `operate` | Bob |
| `oncall-senior` | the above, plus `promote-replica` | `operate` | Carol |

The `invoke` half cannot sit in `payments-prod`: the templates are in `vela-system`, so a `Role` in the team's own namespace would match nothing. That split is the administrative cost of publishing templates centrally, and it means a team administrator cannot widen the left column alone. See [Design 03](./03-permission-components.md#the-administrative-shape-this-implies).

**None of the three holds `create` on Jobs or `get` on Secrets.** That is the point of the exercise.

All three also hold `operate` on `virtualclusters` with no `resourceNames`, the blanket form of the [cluster gate](../README.md#may-the-invoker-act-on-the-target-there). This platform does not draw environment lines by cluster, so that gate passes always and is not mentioned again below. A platform that did would name clusters here, and Alice's backup would then be refused in production while succeeding in staging.

## 1. Alice runs a backup

```
vela op run s3-backup --app payments --component payments-assets
```

| Gate | Result |
|---|---|
| may she operate on the target? | yes, `operate` on `payments` |
| may she invoke the template? | yes, `dev` names `s3-backup` |
| any child needing a direct grant? | none dispatched |
| does the target match `attach`? | yes, `payments-assets` is `aws-s3-bucket` |
| lease free? | yes |

Runs as `system:serviceaccount:vela-system:op-backup`.

Alice cannot create a Job herself and never needed to. She was granted a procedure, not the permissions it uses, which is the whole argument of the KEP reduced to one command.

## 2. Alice tries a failover

```
vela op run dr-failover --app payments
```

Refused at `kubectl apply`:

```
Error: user alice may not invoke OperationTemplate dr-failover
       (no invoke on operationtemplates/dr-failover in vela-system)
```

She passes the first gate: she holds `operate` on the Application. **Being able to operate on an application is not being able to fail it over**, and that separation is why there are two gates rather than one. A model that checked only the target would have let this through.

## 3. Bob runs the failover and hits the child

```
vela op run dr-failover --app payments from=eu-west-1 to=eu-central-1
```

| Gate | Result |
|---|---|
| may he operate on the target? | yes |
| may he invoke `dr-failover`? | yes, `oncall` |
| any child needing a direct grant? | **no.** `promote-replica` sets `requireDirectGrant: true` and `oncall` lacks it |

Refused, again at apply time.

Note what does *not* block him. `dr-failover` also dispatches `pause-writes`, which Bob holds no grant on at all, and that inherits normally. Only the template that marked *itself* sensitive requires a direct grant.

Carol runs the same command and it proceeds. Its children run as `op-failover`, the account each child's own template names.

**This is the row worth arguing about.** Bob can start a failover in principle but not this one, because of a decision the `promote-replica` author made rather than one his platform administrator made. That is either exactly right, since the person who wrote the destructive procedure knows it is destructive, or it is authority in the wrong place. The KEP takes the first position; a reviewer might reasonably take the second.

## 4. One central account, bounded per team

`restart-workload` is published once, in `vela-system`, naming `op-local`. That account holds nothing on its own. What it can do in a given namespace is whatever that namespace grants it:

```
payments-prod                            reporting-dev
└── RoleBinding/allow-restarts           (no binding for op-local)
      subjects:
        - kind: ServiceAccount
          name: op-local
          namespace: vela-system
      roleRef: can-delete-pods
```

Same template, same command, two outcomes:

| Namespace | Result |
|---|---|
| `payments-prod` | runs, deletes pods, succeeds |
| `reporting-dev` | runs, and fails with `Forbidden: cannot delete pods` |

Nobody escalated and nobody borrowed. Each team got exactly what its own administrator granted, and the template knows neither namespace exists.

An earlier draft of the KEP proposed a `serviceAccountNamespace: Local` mode to achieve this. It was removed because ordinary `RoleBinding`s already do it, in the place a Kubernetes administrator would look for the grant.

## 5. The regulated cluster

Identical cluster, except `OperationsRunAsInvoker` is enabled.

```
Alice runs s3-backup
  -> identity resolves to alice, not op-backup
  -> the backup step creates a Job
  -> Forbidden: alice cannot create jobs in payments-prod
```

The operation is now attributable to a named human and bounded by her own rights. It also does not work.

For this platform to function, Alice needs `create` on Jobs and `get` on Secrets directly, which is precisely the low-level access the abstraction existed to remove. Every developer ends up holding the union of what every operation they might run uses.

That is the trade stated plainly. It is the right posture where every privileged act must trace to a person, and the wrong one for a self-service platform, which is why it is a floor a platform opts into rather than a default.

## Summary

| | Alice (`dev`) | Bob (`oncall`) | Carol (`oncall-senior`) |
|---|---|---|---|
| `s3-backup` | runs as `op-backup` | same | same |
| `restart-workload` | runs as `op-local`, bounded per namespace | same | same |
| `dr-failover` | refused, no `invoke` grant | refused, child needs direct grant | runs |
| `promote-replica` directly | refused | refused | runs as `op-failover` |

Two things to take from the table.

**Every refusal happens at `kubectl apply`.** Scenario 3 included: a child marked `requireDirectGrant` is checked against the invoker when the *parent* is applied, so it is not discovered when the workflow reaches that step. That is the difference between a permission model and a source of incidents. What scenarios 4 and 5 show failing at run time is a different thing, the service account being short a grant, which `requires:` exists to catch at apply time too.

**Nobody in it holds the permissions their operations use**, except in scenario 5, where the platform chose otherwise. The grants are all on templates and targets, and the underlying RBAC stays with accounts nobody logs in as.
