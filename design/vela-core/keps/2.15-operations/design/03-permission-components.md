# Design 03: Permission Components

**Status:** Illustrative. These are `ComponentDefinition`s somebody writes, not API this KEP adds. Nothing here needs a controller change; it is the permission model of [KEP-2.15](../README.md) expressed in objects that already exist.

**Companion to:** [KEP-2.15](../README.md), in particular [Permissions](../README.md#permissions) and [Declaring what a template needs](../README.md#declaring-what-a-template-needs-without-minting-it). Worked scenarios are in [Design 02](./02-permission-scenarios.md).

> **TL;DR**
> - Two components, split by who owns them: `operation-permissions` provisions the identity, `operation-access` grants people the right to use it.
> - The result locks down tightly, and stays flexible in how it is administered. Every grant is an ordinary `Role`, so a platform can be centrally governed, fully delegated, or anywhere between, without the KEP taking a position.
> - Three subjects end up holding three disjoint sets of permissions. That disjointness is the whole point and it is worth checking against the rendered output rather than taking on trust.
> - The invoke grant necessarily lands in the template's namespace, so a team cannot self-grant. That is deliberate and it has an administrative cost.

## Why two components

| | `operation-permissions` | `operation-access` |
|---|---|---|
| Answers | what may this operation do | who may run it, and against what |
| Creates | the `ServiceAccount`, its `Role`s, the `use` grant | `invoke`, `operate` and `create` grants |
| Owned by | the platform administrator | the team lead |
| Changes when | a runbook needs a new API | somebody joins the on-call rota |
| Ships with | the module, next to the `OperationTemplate` | the team, next to its Applications |

Rolling them into one object would tie a rota change to a re-review of what the service account may touch. They are shown together in [one Application](#both-in-one-application) below because it reads more clearly that way, not because it is the recommendation.

## `operation-permissions`

Provisions the identity an `OperationTemplate` names in `runAs`, the permissions it holds in each namespace it serves, and the grant that lets an author point a template at it.

```cue
// operation-permissions.cue
"operation-permissions": {
  type:        "component"
  description: "Provisions the service account an OperationTemplate runs as"
  attributes: workload: definition: {apiVersion: "v1", kind: "ServiceAccount"}
}

template: {
  parameter: {
    // +usage=The account an OperationTemplate names in runAs
    serviceAccountName: string
    // +usage=Namespaces this account may operate in
    namespaces: [...string]
    // +usage=What it may do there, the same shape as the template's requires:
    rules: [...{
      apiGroups: [...string]
      resources: [...string]
      verbs: [...string]
      resourceNames?: [...string]
    }]
    // +usage=Who may name this account in an OperationTemplate
    templateAuthors?: [...{kind: string, name: string}]
  }

  // the account itself, in this Application's namespace
  output: {
    apiVersion: "v1"
    kind:       "ServiceAccount"
    metadata: name: parameter.serviceAccountName
  }

  outputs: {
    // what it may do, granted separately in each namespace it serves
    for ns in parameter.namespaces {
      "role-\(ns)": {
        apiVersion: "rbac.authorization.k8s.io/v1"
        kind:       "Role"
        metadata: {name: "\(parameter.serviceAccountName)-ops", namespace: ns}
        rules: parameter.rules
      }
      "binding-\(ns)": {
        apiVersion: "rbac.authorization.k8s.io/v1"
        kind:       "RoleBinding"
        metadata: {name: "\(parameter.serviceAccountName)-ops", namespace: ns}
        roleRef: {apiGroup: "rbac.authorization.k8s.io", kind: "Role", name: "\(parameter.serviceAccountName)-ops"}
        subjects: [{
          kind:      "ServiceAccount"
          name:      parameter.serviceAccountName
          namespace: context.namespace
        }]
      }
    }

    // who may point a template at it
    if parameter.templateAuthors != _|_ {
      "use-role": {
        apiVersion: "rbac.authorization.k8s.io/v1"
        kind:       "Role"
        metadata: {name: "\(parameter.serviceAccountName)-use", namespace: context.namespace}
        rules: [{
          apiGroups:     [""]
          resources:     ["serviceaccounts"]
          resourceNames: [parameter.serviceAccountName]
          // grants no API access; only the OperationTemplate webhook consults it
          verbs:         ["use"]
        }]
      }
      "use-binding": {
        apiVersion: "rbac.authorization.k8s.io/v1"
        kind:       "RoleBinding"
        metadata: {name: "\(parameter.serviceAccountName)-use", namespace: context.namespace}
        roleRef: {apiGroup: "rbac.authorization.k8s.io", kind: "Role", name: "\(parameter.serviceAccountName)-use"}
        subjects: parameter.templateAuthors
      }
    }
  }
}
```

`namespaces` is the bounded alternative to a `ClusterRole`. One `Role` and `RoleBinding` pair renders into each listed namespace, so a namespace that is not on the list refuses the operation. A `ClusterRoleBinding` would grant the account its powers everywhere, including namespaces that never wanted operations running in them.

## `operation-access`

Grants a set of subjects the three things an operator needs: `invoke` on the templates, `operate` on the targets, and `create` where the `Operation` record lands.

```cue
// operation-access.cue
"operation-access": {
  type:        "component"
  description: "Grants a team the ability to run operations against its applications"
  attributes: workload: definition: {
    apiVersion: "rbac.authorization.k8s.io/v1"
    kind:       "Role"
  }
}

template: {
  parameter: {
    // +usage=Who is being granted. Users, Groups or ServiceAccounts
    subjects: [...{kind: string, name: string, namespace?: string}]
    // +usage=Where the team's Applications live and its Operations are created
    namespace: *context.namespace | string
    // +usage=Applications they may operate on. Omit to mean every one in the namespace
    applications?: [...string]
    // +usage=Also allow steps declaring impact: Irreversible
    allowIrreversible: *false | bool
    // +usage=Templates they may invoke, grouped by the namespace each lives in
    templates: [...{namespace: string, names: [...string]}]
  }

  // one Role per namespace, gathering every template name listed for it. Keying the
  // output on t.namespace alone would collide when two entries share a namespace, and
  // CUE fails the render rather than merging the conflicting resourceNames.
  _invokeNames: {
    for t in parameter.templates {
      "\(t.namespace)": [for u in parameter.templates if u.namespace == t.namespace for n in u.names {n}]
    }
  }

  // Group and User need the rbac apiGroup, ServiceAccount needs a namespace instead
  _subjects: [for s in parameter.subjects {
    kind: s.kind
    name: s.name
    if s.kind == "ServiceAccount" {namespace: s.namespace}
    if s.kind != "ServiceAccount" {apiGroup: "rbac.authorization.k8s.io"}
  }]

  // what they may do in their own namespace: operate on targets, create the record
  output: {
    apiVersion: "rbac.authorization.k8s.io/v1"
    kind:       "Role"
    metadata: {name: "\(context.name)-operate", namespace: parameter.namespace}
    rules: [
      {
        apiGroups: ["core.oam.dev"]
        resources: ["applications"]
        if parameter.applications != _|_ {resourceNames: parameter.applications}
        verbs: ["operate", if parameter.allowIrreversible {"operate-irreversible"}]
      },
      {
        apiGroups: ["core.oam.dev"]
        resources: ["operations"]
        verbs:     ["create", "get", "list", "watch"]
      },
    ]
  }

  outputs: {
    "operate-binding": {
      apiVersion: "rbac.authorization.k8s.io/v1"
      kind:       "RoleBinding"
      metadata: {name: "\(context.name)-operate", namespace: parameter.namespace}
      roleRef: {apiGroup: "rbac.authorization.k8s.io", kind: "Role", name: "\(context.name)-operate"}
      subjects: _subjects
    }

    // invoke is granted where the template lives, not where the team works
    for ns, names in _invokeNames {
      "invoke-role-\(ns)": {
        apiVersion: "rbac.authorization.k8s.io/v1"
        kind:       "Role"
        metadata: {name: "\(context.name)-invoke", namespace: ns}
        rules: [{
          apiGroups:     ["core.oam.dev"]
          resources:     ["operationtemplates"]
          resourceNames: names
          verbs:         ["invoke"]
        }]
      }
      "invoke-binding-\(ns)": {
        apiVersion: "rbac.authorization.k8s.io/v1"
        kind:       "RoleBinding"
        metadata: {name: "\(context.name)-invoke", namespace: ns}
        roleRef: {apiGroup: "rbac.authorization.k8s.io", kind: "Role", name: "\(context.name)-invoke"}
        subjects: _subjects
      }
    }
  }
}
```

Two details worth pulling out.

**The `namespace` parameter, rather than `context.namespace` alone.** Without it the operate grant lands wherever the Application sits, and this Application sits in `vela-system` while the team works in `payments-prod`. Defaulting to `context.namespace` keeps the common single-namespace case quiet.

**The `_subjects` transform.** A `RoleBinding` subject needs `apiGroup: rbac.authorization.k8s.io` for a `Group` or `User` and a `namespace` for a `ServiceAccount`. Getting that wrong produces a binding that applies cleanly and silently matches nobody, which is the worst failure mode available to a permission object.

## Both in one Application

```yaml
apiVersion: core.oam.dev/v1beta1
kind: Application
metadata:
  name: payments-operations
  namespace: vela-system
spec:
  components:
    # 1. the identity the operation runs as, and who may point a template at it
    - name: op-backup
      type: operation-permissions
      properties:
        serviceAccountName: op-backup
        namespaces: [payments-prod]              # where it may operate
        rules:
          - {apiGroups: [""], resources: [secrets], verbs: [get]}
          - {apiGroups: ["batch"], resources: [jobs], verbs: [create, get, list]}
        templateAuthors:
          - {kind: Group, name: platform-engineers}

    # 2. who may run the resulting templates, and against what
    - name: payments-oncall
      type: operation-access
      properties:
        namespace: payments-prod                 # where they work
        subjects:
          - {kind: Group, name: payments-oncall}
        applications: [payments]
        templates:
          - {namespace: vela-system, names: [s3-backup, restart-workload]}
```

## What lands where

```
vela-system                                    payments-prod
├── ServiceAccount/op-backup                    ├── Role/op-backup-ops
│     identity only, zero permissions           │     get secrets, create jobs
│                                               ├── RoleBinding/op-backup-ops
├── Role/op-backup-use                          │     subject: vela-system/op-backup
│     serviceaccounts/op-backup: [use]          │
├── RoleBinding/op-backup-use                   ├── Role/payments-oncall-operate
│     subject: Group/platform-engineers         │     applications/payments: [operate]
│                                               │     operations: [create get list watch]
├── Role/payments-oncall-invoke                 └── RoleBinding/payments-oncall-operate
│     operationtemplates/s3-backup,                   subject: Group/payments-oncall
│     restart-workload: [invoke]
└── RoleBinding/payments-oncall-invoke
      subject: Group/payments-oncall
```

**Three subjects, three disjoint sets.** `platform-engineers` can name `op-backup` in a template and cannot create a Job. `payments-oncall` can run `s3-backup` against `payments` and holds no grant that mentions `op-backup` at all. `op-backup` can create Jobs and read Secrets and is an identity nobody logs in as, so it does nothing until an operation borrows it. Deleting the Application removes all nine objects, which is the part a hand-rolled set of bindings across several namespaces tends not to get right.

## The administrative shape this implies

The invoke grant lands in `vela-system` because RBAC grants live where the resource lives, and the template lives there. So a team's access is split across two namespaces and a team administrator cannot grant the invoke half alone. That is correct, since otherwise a team could grant itself any template on the cluster, and it has a real cost: adding a template to a team's rota needs somebody with write access to `vela-system`.

What it does not do is force a particular operating model. Every grant here is an ordinary `Role`, so the same two components support:

| Model | How |
|---|---|
| Centrally governed | both components applied by the platform team, teams request changes |
| Delegated targets | platform owns `operation-permissions` and the invoke grants, teams own their own `operation-access` for `operate` and `create` |
| Fully self-service within a boundary | teams hold `invoke` on a whole namespace of templates via a wildcard `Role`, and pick from it freely |
| Locked down per procedure | `resourceNames` on every rule, plus `requireDirectGrant` on the templates that warrant it |

The KEP takes no position between those, and it should not. The verbs and the two checks are the fixed part; how a platform arranges them is a matter for the platform.

## Caveats

**Only meaningfully safe under the authenticated posture.** Kubernetes stops a subject creating a `Role` that grants more than it holds, and that binds the *applier* only when the platform impersonates them. With `authentication.enabled` off, an Application applies with the controller's identity and the escalation check does not bite. True of any Application carrying RBAC objects, so it is a property to know rather than one introduced here.

**These are examples, not deliverables.** Nothing in KEP-2.15 depends on them existing. They are included to show the permission model resolves to objects a platform administrator already recognises, and that the pairing works: the template *declares* what it needs in `requires:`, the component *provisions* it, and the two are the same list.

## For next week

- Whether `operation-access` should be able to grant `invoke` by label selector rather than by name, so a module can ship templates that a team's existing grant picks up. Convenient, and it quietly removes the review step that naming forces.
- Whether `allowIrreversible` is the right granularity, or whether `operate-irreversible` should be per-Application like `operate` is.
- Whether the `create` on `operations` rule should carry `resourceNames`, given a name is chosen by the invoker at creation time and cannot be predicted.
- Whether these belong in an addon at all, or stay as reference material in this KEP.
