# KEP: One Way to Connect an Application to a Provisioned Service

**Authors**: Anoop Gopalakrishnan

**Status**: Draft

**Created**: 2026-11-26

**Last Updated**: 2026-11-26

> **Status**: Design proposal. Names of parameters, keys, traits and fields may change
> during review. Nothing here is committed behaviour. The proposal is written to land
> after the 1.12 release.

## Summary

Today every database component in a KubeVela catalog answers "where are my connection
details" differently. An Aurora component built on ACK copies them into a Secret the
developer names. A CloudNativePG component leaves them in a Secret the operator names
after the cluster, with its own key names. A Crossplane component writes whatever the
provider emits. A Percona component generates one Secret per user. The developer who
wants to point a web service at a database has to know which operator sits behind the
component before they can write a single environment variable, and they have to remember
to add `dependsOn` by hand or the web service starts before the database exists.

This proposal gives every provisioned-service component the same contract, so an
application developer connects to Aurora, CloudNativePG, Percona or anything else the
same way:

- a `credentialsSecret` parameter naming a Secret the developer or platform owns, holding
  the login the service is created with;
- a `<component>-connection` Secret the component emits, carrying the non-secret facts
  (host, port, database, and so on) under one fixed set of key names;
- a `connect` trait on the consumer that takes a component name and renders the
  environment variables or files, and adds the ordering edge for you.

The provider-specific work moves inside each component definition, where the definition
author already has to know it. The developer never sees it.

The proposal has two phases. Phase 1 is a convention plus a trait and needs no change to
vela-core. Phase 2 makes the contract declared on the ComponentDefinition and lets the
controller, the dry run and the diagnosis tools use it, which removes the last cases where
a Secret has to be copied.

## Motivation

### What we saw

On 2026-11-25 we ran a Spring Boot application against a real Aurora PostgreSQL cluster
provisioned through ACK, both delivered by one Application. The database component
reported healthy as soon as the DBCluster was synced. The application started, read its
connection Secret, found no host, and crash-looped with an unknown-host error. The host
only appears once the cluster's first instance exists, and the FieldExport that copies it
into the Secret had not run yet. Nothing in the Application said that the web service
needed the database, and nothing in the diagnosis connected the empty Secret to the
missing instance.

The fix at the time was `dependsOn` on the web service, a health rule on the database
component that waits for its instances, and three pull requests to the MCP diagnosis so it
can follow a Secret back to whoever fills it (kubevela/kubevela#7716, #7717, #7718). Those
are all good things, and they are all workarounds for the same missing piece: there is no
agreed way for a component to say "here is how you connect to me".

### Why the existing pieces do not add up to one

We looked through the repository before writing this (upstream master at 3bfa79c3b):

- `writeConnectionSecretToRef` exists only as a field of the Terraform component
  schematic, which `pkg/appfile` copies onto the generated Configuration. Nothing reads it
  back.
- The `service-binding` trait patches `secretKeyRef` entries into a workload from a Secret
  name and key the developer supplies. It is marked deprecated in favour of `storage`,
  which does the same with mounts. Both assume the Secret is already filled and already
  known.
- `read-object` and `export2secret` are workflow steps that can copy a Secret, but a step
  lives on the Application, so the developer has to add one per database and add
  `dependsOn` as well.
- The resource tree in `pkg/workflow/providers/query/tree.go` has health rules for a
  handful of kinds and reports every other kind, FieldExport included, as healthy without
  looking at it.
- The 2022 design `cloud-resource-data-passing.md` covers how to move a Terraform
  connection Secret to another cluster. It does not cover what the Secret contains or how
  a consumer finds it.

Each piece solves a slice. None of them says what a connection looks like.

### Goals

1. An application developer connects a workload to a provisioned service by naming the
   component, nothing else, and gets the same result whatever operator is behind it.
2. A component definition author follows one written contract and the tooling can check
   it: the dry run, `explain_definition`, `diagnose_application` and the real-cloud test
   suite described in the skills.
3. Credentials never pass through KubeVela's render. The component points the provider at
   a Secret the developer owns; it never reads, emits or copies the password.
4. The ordering problem goes away by construction: connecting to a component implies
   depending on it, and in Phase 2 implies waiting until the declared keys exist.
5. Everything is additive. A definition that ignores the contract keeps working. A
   developer who writes `secretKeyRef` by hand keeps working.

### Non-goals

1. Managing secrets. This is not a vault, a rotation engine or a replacement for External
   Secrets Operator. It says where a Secret is and what its keys are called.
2. Managing database schemas, grants or migrations. Creating extra users is covered only as
   far as naming their Secrets.
3. Implementing the `servicebinding.io` ServiceBinding CRD or its controller. We borrow
   its key names so binding-aware frameworks work, and nothing more.
4. Copying Secrets across clusters. A consumer in another cluster is a follow-up, see
   "Unresolved questions".

## Proposal

### The contract, as the developer sees it

An Application with one web service and one database. Swap the database component for any
other that follows the contract and the web service does not change.

```yaml
apiVersion: core.oam.dev/v1beta1
kind: Application
metadata:
  name: petclinic
spec:
  components:
    - name: orders-db
      type: aurora-postgres          # or cnpg-postgres, percona-postgres, rds-crossplane ...
      properties:
        credentialsSecret: orders-db-credentials
        # provider-specific parameters stay available here, with defaults

    - name: web
      type: webservice
      properties:
        image: petclinic:3.5.0
      traits:
        - type: connect
          properties:
            services:
              - component: orders-db
                prefix: DB_
```

The web service gets `DB_HOST`, `DB_PORT`, `DB_DATABASE`, `DB_USERNAME` and `DB_PASSWORD`,
and it is applied only after `orders-db` is healthy. If the developer prefers files, the
trait can mount the same keys under a directory instead, in the layout Spring Boot and
other binding-aware frameworks already read.

### The two Secrets

Every component that follows the contract works with exactly two Secrets in the
Application's namespace.

| Secret | Keys | Created by | Deleted with the Application |
|---|---|---|---|
| `<component>-credentials` | `username`, `password` (type `kubernetes.io/basic-auth`) | the developer or the platform, before the Application | no |
| `<component>-connection` | `type`, `provider`, `host`, `port`, `database`, and optional `uri`, `ca.crt`, `options` | the component, or the provider's controller on the component's behalf | yes |

The split is deliberate. The credentials Secret carries the only thing that must stay
secret, is never emitted by any template, and so is never adopted or garbage-collected by
KubeVela. Rotating a password is an edit to that Secret plus whatever the provider does on
its own. The connection Secret carries facts that are not secret, so the component can
emit it freely, overwrite it on every reconcile, and let it go when the Application goes.

`uri`, where present, never contains the password. A framework that wants a full URL
composes it from `host`, `port`, `database` and the credentials at start-up; the one we
tested does that already. `ca.crt` is for operators that issue their own server
certificate, such as CloudNativePG. `options` is a free-form string for things like
`sslmode=require`.

The key names come from the well-known names in the `servicebinding.io` specification
(`type`, `provider`, `host`, `port`, `username`, `password`, `database`, `uri`). We use
them because frameworks already recognise them, not because we implement that
specification. `type` is the kind of service (`postgresql`, `mysql`, `redis`, `amqp`,
`s3`), `provider` is who runs it (`aurora`, `cnpg`, `percona`, `rds`). Both are constants
the definition author writes.

This document fixes the vocabulary for relational databases. Other kinds of service get a
row in the same table as they are added; see "Other kinds of service" below.

### What the component does with `credentialsSecret`

`credentialsSecret` is a Secret that already exists. The component never creates it,
never reads its values and never copies it. It hands the Secret's name to whatever field
the provider uses to say "take the password from this Secret", so the credential goes from
the Secret to the database without passing through KubeVela's render.

That field is different for every provider, and that difference is exactly what the
definition hides:

| Component | Where `credentialsSecret` lands | What else follows from it |
|---|---|---|
| Aurora on ACK | `spec.masterUserPassword: {name, key: password}` on the DBCluster | `spec.masterUsername` from the `username` parameter |
| CloudNativePG | `spec.bootstrap.initdb.secret.name` on the Cluster for the first start, and `spec.managed.roles[].passwordSecret` for the same user so later rotations are honoured | nothing more; the operator reads `username` and `password` from the Secret and skips generating its own `<name>-app` Secret |
| RDS through Crossplane | `spec.forProvider.masterPasswordSecretRef: {name, key: password, namespace}` | `masterUsername` from the parameter |
| Terraform addon | a Secret-backed variable, the way the module already takes passwords | as the module defines |
| Percona PostgreSQL | to be checked against the pinned operator version; the Crunchy-based operator generates its own user Secret and accepts a chosen password only by writing into it | if that holds, this is the one case that needs the projection in "When the controller writes its own Secret" |

One seam is worth stating. Aurora and Crossplane take the master username as a spec field,
so those definitions carry a `username` parameter. CloudNativePG reads it from the Secret.
The definitions default `username` to the same value, but a developer can edit one and not
the other. Both values are visible at render time, so the dry run can flag the mismatch
(Phase 1 lint).

### How each component fills `<component>-connection`

How hard this is depends only on how the provider's controller hands the facts out. There
are three cases.

**Case A: the facts are on the resource, or can be worked out at render time.** ACK puts
the endpoint in `status.endpoint` and the port in `spec.port`. CloudNativePG's host is the
`<name>-rw` Service and the port is 5432. The component emits the connection Secret itself
as a second output with `type`, `provider`, `database` and `username` filled at render
time; the remaining keys are either filled at render time too (CloudNativePG) or copied in
by the provider's own per-field exporter once they exist (ACK FieldExport, which needs its
target Secret to exist first, which this gives it). No copying, no renaming.

**Case B: the controller writes its own Secret, in its own vocabulary, often with the
password mixed in.** Crossplane writes `endpoint`, `port`, `username`, `password` to the
`writeConnectionSecretToRef` target. Terraform writes whatever the module's outputs are
called. The component cannot rename keys or drop the password at render time, because
render never touches Secret data.

**Case C: the facts are scattered.** A ConfigMap for the host and a Secret for the
password, or a generated Secret per user whose name the component did not choose.

Case A needs nothing beyond the component template. Cases B and C need a projection: some
step that reads what the controller wrote and produces the two contract Secrets. In Phase 1
there are two ways to do it, both written once by the definition author:

| Projection | How | Cost |
|---|---|---|
| ExternalSecret output | the component emits an `ExternalSecret` using External Secrets Operator's Kubernetes provider, which reads the controller's objects and writes the contract Secrets with renamed keys through its template | needs ESO on the cluster; declarative, owned by the Application, nothing for the developer to add |
| Workflow step | the module ships a `project-connection` WorkflowStepDefinition wrapping `read-object` and `export2secret` with a key map | nothing to install, but the developer has to add the step, which is the leak we are trying to close |

Phase 2 removes the need for projection entirely; see below.

### The `connect` trait

A new trait in the built-in catalog. It applies to every workload with a pod template,
through `podSpecPath`, the same way `storage` does.

```cue
parameter: {
	services: [...{
		// the producer component in this Application
		component: string
		// which of the producer's users to connect as; empty is the default user
		user?: string
		// environment variable prefix; empty means no env injection
		prefix?: string
		// mount the keys as files under this directory instead of, or as well as, env
		mountPath?: string
		// override the contract names if a producer could not follow them
		connectionSecret?: string
		credentialsSecret?: string
	}]
}
```

What it renders, for `component: orders-db, prefix: DB_`:

- `DB_HOST`, `DB_PORT`, `DB_DATABASE` from `orders-db-connection`, through `secretKeyRef`;
- `DB_USERNAME`, `DB_PASSWORD` from `orders-db-credentials`, through `secretKeyRef`;
- `DB_URI`, `DB_OPTIONS`, `DB_CA_CRT` only when the producer declares them (Phase 2) or
  with `optional: true` on the reference (Phase 1), so a missing optional key does not stop
  the pod;
- with `mountPath`, a projected volume combining both Secrets under one directory, one
  file per key, plus a `type` file, which is the directory layout binding-aware frameworks
  read.

In Phase 1 the trait resolves Secret names by convention from the component name. It
cannot see the producer's definition at render time, because a trait only sees its own
component's context. In Phase 2 it reads the producer's declared contract instead.

The trait also adds `orders-db` to the consumer's `dependsOn`. In Phase 1 that is done by
the trait template patching the component's workflow dependency. Today that needs a small
change in the application controller, because traits render after dependencies are
computed; this is the one place where Phase 1 touches vela-core, and if it proves awkward
the fallback is that the trait documents the `dependsOn` and the dry-run lint reports when
it is missing (that lint is kubevela/kubevela#7718).

### Several users and the admin account

"Credential" hides two things: the account the application connects as, and the
privileged account the service was created with. The contract keeps them apart.

- `credentialsSecret` is the application user. It is what `connect` hands out.
- `adminCredentialsSecret` is optional and names the privileged account where the provider
  has one that is distinct from the application user. `connect` never renders it. For
  Aurora the master user is the only account the RDS API can create, so the two coincide
  and the definition says so. For CloudNativePG it maps to `superuserSecret` with
  `enableSuperuserAccess`.
- `users` is an optional list for more application users, each with a name, its own
  credentials Secret and a privilege level the definition understands:

```yaml
properties:
  credentialsSecret: orders-db-credentials
  users:
    - name: reporting
      credentialsSecret: orders-db-reporting-credentials
      privileges: read
```

The consumer picks a user by name, `user: reporting`, and the trait resolves
`orders-db-reporting-credentials`. Connection facts stay in the one `-connection` Secret,
because host, port and database are the same for every user.

How a definition honours `users` depends on the provider: CloudNativePG has
`managed.roles`, each with its own `passwordSecret`; Percona and Crunchy have `users[]`
with a generated Secret per user; the RDS API has nothing, so an Aurora definition either
emits a Job that applies SQL as the master user, or leaves user management to a separate
component that connects to the database component the same way a workload does. The
separate component is the better long-term shape, because it gives the same `users`
experience on top of every database, but it is not required by this proposal.

### Phase 2: declaring the contract on the definition

Phase 1 works by naming convention. It covers Case A natively and makes Cases B and C pay
for a projection. Phase 2 replaces the convention with a declaration, and that removes the
projection for every case.

A new optional block on `ComponentDefinition.spec`, next to `status`:

```yaml
spec:
  connection:
    type: postgresql
    provider: aurora
    # where each vocabulary key lives; CUE, evaluated with the component's context
    keys:
      host:     {secret: "\(context.name)-connection", key: host}
      port:     {secret: "\(context.name)-connection", key: port}
      database: {secret: "\(context.name)-connection", key: database}
      username: {secret: "\(parameter.credentialsSecret)", key: username}
      password: {secret: "\(parameter.credentialsSecret)", key: password}
    users:
      # how a named user's Secret is found
      credentials: {secret: "\(context.name)-\(user)-credentials"}
```

For a Crossplane definition the same block points `host` at key `endpoint` and `password`
at key `password` in the Secret the controller writes. For a controller that uses a
ConfigMap, the entry says `configMap` instead of `secret`. Nothing is copied; the
declaration just says where to look.

What the controller does with it:

- **Resolve for consumers.** When rendering a component whose `connect` trait names a
  producer, the controller evaluates the producer's `connection` block with the producer's
  context and parameters and exposes the result to the trait as
  `context.connections[<component>]`. The trait renders `secretKeyRef` and
  `configMapKeyRef` from that. The two-Secret convention becomes the recommended default
  for Case A rather than a requirement.
- **Order and wait.** Connecting implies `dependsOn`. Beyond that, the controller holds the
  consumer until every declared key exists in the declared object, not merely until the
  producer reports healthy. This closes the "healthy before the endpoint exists" gap at the
  root instead of asking every definition author to get the health rule right.
- **Diagnose.** `diagnose_application` reads the declared contract to know who fills a
  Secret, so the producer-side detection in kubevela/kubevela#7716 becomes the fallback for
  resources that declare nothing, and operators that name Secrets by convention
  (CloudNativePG, Percona) are covered without a per-operator table.
- **Check at dry run.** `dryrun_application` and `explain_definition` report a consumer
  whose producer declares no contract, a declared key that no output or known controller
  Secret can supply, and a `-connection` Secret that would carry a credential key.

The block is validated by the definition webhook: every `secret` and `configMap` name must
be either an output of the template or marked `external: true` with a note saying which
controller writes it; the `-connection` Secret, when the template emits it, may not carry
`password`; `type` must be one of the registered service types.

### Other kinds of service

The vocabulary above is for relational databases. The same contract applies to anything a
workload connects to; only the key table changes. Proposed starting rows, to be confirmed
with the first definition of each kind:

| `type` | Required keys | Optional keys |
|---|---|---|
| `postgresql`, `mysql` | `host`, `port`, `database`, `username`, `password` | `uri`, `ca.crt`, `options` |
| `redis` | `host`, `port`, `password` | `username`, `uri`, `ca.crt`, `tls` |
| `amqp`, `kafka` | `host`, `port`, `username`, `password` | `uri`, `vhost` or `topic`, `ca.crt` |
| `s3` | `endpoint`, `bucket`, `region`, `accessKeyId`, `secretAccessKey` | `prefix`, `pathStyle` |

An object store shows why the credentials half is not always `username` and `password`:
the credentials Secret for `s3` carries `accessKeyId` and `secretAccessKey`. The contract
is that the credentials Secret holds whatever is secret for that `type`, and the vocabulary
table says which keys those are.

### Edge cases and how the contract handles them

**Two Applications in one namespace with a component of the same name.** Both would emit
`orders-db-connection`. This is not new: KubeVela names every resource after its component,
so the Deployments and Services would collide first, and the rule is already that component
names are unique within a namespace across Applications. The dry run reports the collision
when both are in the same Application; `diagnose_application` reports a connection Secret
owned by a different Application through its owner labels.

**Operator-generated names that clash with the contract suffixes.** CloudNativePG creates
`-rw`, `-ro`, `-r`, `-app`, `-superuser`, `-ca`, `-server`, `-replication`. Percona creates
`-pguser-<user>`, `-pgbouncer`, `-cluster-cert` and others. `-connection` and
`-credentials` were chosen to stay clear of those. The `connect` trait's
`connectionSecret` and `credentialsSecret` overrides exist for the day some operator uses
one of them anyway, and the Phase 2 declaration makes the names irrelevant.

**A definition that emits a Secret with the credentials name.** It must not. KubeVela would
adopt it and delete it with the Application, taking the password with it. The Phase 2
webhook rejects a template output whose name equals `parameter.credentialsSecret`; in
Phase 1 the dry-run lint reports it.

**The credentials Secret does not exist yet.** The provider's controller fails to create
the service and says so in its conditions. The component is not healthy, the consumer
waits on `dependsOn`, and the diagnosis names the missing Secret because the provider's
condition message does. Nothing is created half-way. Platforms that want to be stricter
can make the dry run require the Secret to exist with a server check.

**Password rotation.** Editing `-credentials` is the whole developer-side action. Whether
the service picks it up depends on the provider: ACK re-applies a changed master password;
CloudNativePG reads `initdb.secret` only at first start, which is why the definition also
declares the same user under `managed.roles` with `passwordSecret`, which it does watch;
Crossplane re-applies on reconcile. A definition documents which it is, and a definition
for an operator that never re-reads the Secret says rotation is manual.

**A key that appears late.** ACK fills `port` into `spec` only after the first reconcile,
and `host` only after the writer instance exists. In Phase 1 the producer's health rule
must cover its exporters (the rule now in the defkit-author skill) and a consumer that
reads keys through environment variables restarts until they exist, while one that mounts
files picks them up in place. In Phase 2 the controller waits for the declared keys, so
the consumer is not released early at all.

**A key that will never appear.** A FieldExport of a field that is unset never converges.
The definition gates each export on the parameter that makes its source exist, which is
why `database` is only exported when `databaseName` is set. The dry run reports a declared
required key with no source.

**Consumer and producer in different namespaces.** Not supported by the contract. Secrets
do not cross namespaces, and a `secretKeyRef` can only name a Secret in the pod's own
namespace. FieldExport can write to another namespace, but the contract does not use that.
Both components belong to one Application, which has one namespace.

**Consumer and producer in different clusters.** Not supported in this proposal. The
topology policy can place the web service on a managed cluster while the database
component stays local, and then the Secret is in the wrong cluster. The 2022
`cloud-resource-data-passing.md` design describes copying the Secret with a workflow step
and that still works. Making the `connect` trait do it needs a decision on who owns the
copy; see "Unresolved questions".

**A producer that does not follow the contract.** `connect` cannot resolve it. The
developer falls back to `env` with `secretKeyRef`, or `storage`, or workflow
`inputs`/`outputs`, exactly as today. The dry run says which producer has no contract so
it is a known gap rather than a surprise.

**A workload with several containers.** The trait injects into the container named after
the component by default and takes a `containers` list for others, the same way the `env`
trait does.

**Optional keys.** A reference to `ca.crt` on a provider that has no CA would stop the pod
if it were required. The trait renders optional vocabulary keys with `optional: true` in
Phase 1 and omits undeclared keys in Phase 2.

**Several users and ordering.** A consumer connecting as `reporting` waits on the producer
component; in Phase 2 it also waits for that user's credentials Secret to exist, so a user
created by a Job after the cluster is up does not release the consumer early.

**Revision and rollback.** The connection Secret is an ordinary output, so it is part of
the ApplicationRevision and rolls back with the component. The credentials Secret is not
owned by the Application and does not roll back, which is what you want for a password.

### Why not just copy everything into one standard Secret

Copying would give one physical shape for every provider, but it needs something to run
after the operator writes its Secret, and KubeVela has no primitive that derives a Secret
from another Secret at reconcile time. It would mean a new controller or a hard dependency
on External Secrets Operator for every database, and it duplicates every credential into a
Secret the Application owns and garbage-collects. Resolving by declaration happens at
render time, needs only names, never data, and leaves the password where the developer put
it. Projection stays available for the controllers that give no other choice.

### Relationship to the `servicebinding.io` specification

We reuse its key names and its file layout so that frameworks which already read bindings
work without changes. We do not implement its `ServiceBinding` resource, its
`status.binding.name` convention on provisioned services, or its controller; the Red Hat
implementation of that controller was deprecated in 2024, and the KubeVela Application
already has the composition the specification was trying to add. If an operator does set
`status.binding.name`, a Phase 2 definition can point its `connection` block at it like any
other Secret.

## Examples

### CloudNativePG definition, Phase 1, in defkit

```go
creds := defkit.String("credentialsSecret").Required().
	Description("Existing kubernetes.io/basic-auth Secret with username and password for the application user; never created by this component")
database := defkit.String("database").Default("app")
instances := defkit.Int("instances").Default(1).Min(1)

cluster := defkit.NewResource("postgresql.cnpg.io/v1", "Cluster").
	Set("metadata.name", defkit.VelaCtx().Name()).
	Set("spec.instances", instances).
	Set("spec.bootstrap.initdb.database", database).
	Set("spec.bootstrap.initdb.secret.name", creds).
	// the same user under managed roles, so a later password change is honoured
	Set("spec.managed.roles", ...)

connection := defkit.NewResource("v1", "Secret").
	Set("metadata.name", defkit.Sprintf("%s-connection", defkit.VelaCtx().Name())).
	Set("stringData.type", "postgresql").
	Set("stringData.provider", "cnpg").
	Set("stringData.host", defkit.Sprintf("%s-rw.%s.svc", defkit.VelaCtx().Name(), defkit.VelaCtx().Namespace())).
	Set("stringData.port", "5432").
	Set("stringData.database", database)

tpl.Output(cluster)
tpl.Outputs("connection", connection)
```

### The same definition, Phase 2, adding the declaration

```go
def.Connection(defkit.Connection("postgresql", "cnpg").
	Key("host", defkit.SecretKey(connectionName, "host")).
	Key("port", defkit.SecretKey(connectionName, "port")).
	Key("database", defkit.SecretKey(connectionName, "database")).
	Key("username", defkit.SecretKey(creds, "username")).
	Key("password", defkit.SecretKey(creds, "password")).
	Optional("ca.crt", defkit.SecretKey(defkit.Sprintf("%s-ca", name), "ca.crt")))
```

### Aurora on ACK, the parts that differ

The DBCluster takes `spec.masterUserPassword` from the credentials Secret and
`spec.masterUsername` from the `username` parameter. The template emits the `-connection`
Secret with `type`, `provider`, `database` and `username`. Two FieldExports copy
`status.endpoint` to `host` and `spec.port` to `port` into that Secret once they exist, and
the health rule waits for every instance so the component is not healthy before the
endpoint resolves. This is the module we tested on 2026-11-25, renamed to the contract.

### Crossplane RDS, Phase 2

```yaml
spec:
  connection:
    type: postgresql
    provider: rds
    keys:
      host:     {secret: "\(context.name)-conn", key: endpoint, external: true}
      port:     {secret: "\(context.name)-conn", key: port, external: true}
      database: {secret: "\(context.name)-connection", key: database}
      username: {secret: "\(parameter.credentialsSecret)", key: username}
      password: {secret: "\(parameter.credentialsSecret)", key: password}
```

The template sets `writeConnectionSecretToRef` to `<name>-conn`, marks those keys
`external` because Crossplane writes them, and emits its own `-connection` Secret for the
facts it knows at render time. No projection, no copy.

## Alternatives considered

- **Physical normalisation through External Secrets Operator for every provider.** One
  shape, but a hard dependency, a copy of every credential, and nothing for the ordering
  problem. Kept as the projection option for Cases B and C only.
- **Workflow `inputs` and `outputs` between components.** Already possible, carries only
  rendered fields and not Secret data, and puts the plumbing in the Application, which is
  where we do not want it.
- **Adopting the `servicebinding.io` ServiceBinding resource.** Adds a second composition
  model next to the Application and depends on a controller whose main implementation is
  deprecated. The key names and the file layout are kept.
- **A convention table of operator Secret names inside the diagnosis tools.** Covers the
  diagnosis gap for CloudNativePG and Percona but nothing else, and rots with every operator
  release. The declared contract replaces it.
- **Do nothing and document `dependsOn` and `secretKeyRef`.** This is the current state,
  and the skills do document it. The developer still has to know the operator.

## Risks

- **Convention drift in Phase 1.** A definition can claim the contract and emit the wrong
  keys. The mitigations are the dry-run lint, the real-cloud test suite in the
  defkit-author skill that reads the Secret keys back, and Phase 2's webhook.
- **Operators that only read the credential at bootstrap.** Rotation silently does nothing.
  Each definition states its rotation behaviour; the CloudNativePG shape above uses
  `managed.roles` to avoid it.
- **Credential duplication for Case B providers.** Crossplane writes the password into its
  own Secret as well as reading it from ours. That Secret is the controller's and is not
  part of the contract; we only point at it. Documented as known.
- **Trait-driven `dependsOn`.** If traits cannot add a dependency edge without a controller
  change in Phase 1, the lint in kubevela/kubevela#7718 is the fallback until Phase 2.
- **Scope creep into user management.** The `users` list is kept to naming Secrets. Grants
  and schemas stay with the operator or a separate component.

## Milestones

1. **Contract document and skills.** This KEP, the vocabulary table, and updates to the
   defkit-author and developer skills so new definitions follow the contract. No code.
2. **Phase 1 in the catalog.** The `connect` trait; the Aurora module renamed to the
   contract; a CloudNativePG module as the Case A reference; a Crossplane module as the
   Case B reference using the ExternalSecret projection; `dependsOn` from the trait or the
   lint. Real-cloud and emulator suites that read the keys back.
3. **defkit support.** A `Connection` builder that emits the `-connection` Secret and the
   per-field exporters from one declaration, and a test matcher for the contract.
4. **Phase 2 in vela-core.** The `connection` block on ComponentDefinition, webhook
   validation, `context.connections` for traits, wait-for-keys in the workflow, and the
   diagnosis and dry-run tools reading the declaration. Behind a feature gate for one
   release.
5. **Follow-ups.** Cross-cluster consumers; a `users` component that works across
   databases; vocabulary rows for queues and object stores as the first definitions of each
   land.

## Unresolved questions

1. Does the Percona PostgreSQL operator accept a pre-created credential Secret for a user
   at the pinned version, or must the password be written into the generated Secret? This
   decides whether Percona is Case A or Case C.
2. Where should the Phase 2 block live: a new `spec.connection`, or under the existing
   `spec.status` next to `healthPolicy` and `details`? `spec.connection` reads better;
   `spec.status` keeps every runtime-evaluated CUE in one place.
3. Cross-cluster consumers. Should the `connect` trait copy the two Secrets to the target
   cluster, should a policy do it, or should it stay a documented workflow step as in the
   2022 design? Copying a credentials Secret the Application does not own has ownership
   questions this proposal has not answered.
4. Should `-connection` be allowed to carry `uri` with the password embedded for frameworks
   that only accept a URL, as a declared exception? The current answer is no, and the
   consumer composes the URL.
5. Whether Phase 1's trait-driven `dependsOn` can be done without touching the application
   controller. Needs a prototype.

## References

- kubevela/kubevela#7716, #7717, #7718: exporter and consumer diagnosis, dependency lints.
- kubevela/skills#22: the connection-details rule in the defkit-author skill.
- `design/vela-core/cloud-resource-data-passing.md`: moving a Terraform connection Secret
  to another cluster.
- `servicebinding.io` core specification 1.1.0, "Well-known Secret Entries" and
  "Workload Projection".
- CloudNativePG documentation: bootstrap `initdb.secret`, `managed.roles`, Services
  `-rw`/`-ro`/`-r`.
- AWS Controllers for Kubernetes: `FieldExport`, RDS `DBCluster.spec.masterUserPassword`.
- Crossplane: `writeConnectionSecretToRef`, `masterPasswordSecretRef` on the RDS Cluster.
