---
theme: dark
author: KubeVela Community
date: "2026-04-16"
paging: "%d / %d"
---

# KubeVela vNext: KEP 2.13, 2.16, 2.20

**Declarative Addons · Source Resolution · Module Versioning**

Community Call · April 2026

---

## Three KEPs, One Thread

```
KEP-2.20: Module & API Line Versioning
         ↑ defines stable names for definitions

KEP-2.13: Declarative Addon Lifecycle
         ↑ delivers those definitions continuously

KEP-2.16: SourceDefinition & fromSource
         ↑ resolves external data before components render
```

---

# KEP-2.13 - Declarative Addon Lifecycle

## The Problem Today

```bash
vela addon enable aws-s3 --version v1.2.0   # imperative; no CR left behind
vela addon upgrade aws-s3 --version v1.3.0  # definitions replaced in-place, immediately
```

**No GitOps support.** 
- There is no CR representing "aws-s3 v1.2.0 should be installed".
- Nothing to commit to git. 
- Nothing for Flux or Argo CD to reconcile.

**No continuous reconciliation of non-Application resources.**
- Definitions, Views, ConfigTemplates, and Schemas are applied out of band -
outside the owned Application's `spec.components`. 
- The Application controller
never sees them. 
- Delete one manually; nothing heals it.

---

# KEP-2.13 - Declarative Addon Lifecycle

## The Fix: `Addon` CR

```yaml
apiVersion: core.oam.dev/v1beta1
kind: Addon
metadata:
  name: aws-s3
spec:
  version: v1.2.0             # exact tag - pinned (recommended for GitOps)
  # version: ">=1.2.0"        # semver constraint - tracking mode
  # upgradePolicy: Manual     # default: notify via status.availableUpgrade, don't act
  # upgradePolicy: Auto       # upgrade automatically (not GitOps-safe)
  registry: my-registry
  parameters:
    region: us-east-1
    enableV2: true
  clusters:
    - local                   # omit to deploy to all registered clusters
  deletionPolicy: Protect     # default: block delete if any App references a definition
                              # deletionPolicy: Force  # delete immediately regardless of consumers
                              # deletionPolicy: Orphan # release without deleting resources
  overrideDefinitions: false  # reject install if a definition is owned by another addon
```

```bash
# CLI writes a CR, then exits.
# The controller does the actual work.
vela addon enable aws-s3 --version v1.2.0

# Same as applying the YAML above.
# Works identically from git, Flux, Argo CD.
```

---

# KEP-2.13 - Declarative Addon Lifecycle

## Version Selection

```yaml
# Pinned mode - recommended for GitOps
spec:
  version: v1.2.0         # exact tag, never changes autonomously

# Tracking mode - notify but don't auto-upgrade (GitOps-safe)
spec:
  version: ">=1.2.0"
  upgradePolicy: Manual   # writes status.availableUpgrade

# Tracking mode - auto-upgrade (opt-in, not GitOps-safe)
spec:
  version: ">=1.2.0"
  upgradePolicy: Auto     # upgrades on next reconcile
```

```bash
# To upgrade in Manual or pinned mode:
vela addon upgrade aws-s3 --version v1.3.0
# ...or update spec.version in git.
```

---

# KEP-2.13 - Declarative Addon Lifecycle

## Deletion Policies

```yaml
spec:
  deletionPolicy: Protect   # default: block if any Application uses a definition
  # deletionPolicy: Force   # delete immediately - breaks running Apps
  # deletionPolicy: Orphan  # release without deleting; resources stay, unmanaged
```

```bash
# Protect in action:
kubectl delete addon aws-s3
# Error: addon aws-s3 cannot be deleted: Applications [checkout, api-platform]
# reference definitions owned by this addon.
# Remove references or set spec.deletionPolicy: Force to override.

# Check what's blocking:
vela addon status aws-s3
```

---

# KEP-2.13 - Declarative Addon Lifecycle

## Addon-of-Addons: Composing a Platform

```yaml
apiVersion: core.oam.dev/v1beta1
kind: Application
metadata:
  name: xp-installation
  namespace: vela-system
spec:
  components:
    - name: crossplane
      type: addon
      properties:
        version: v1.15.0

    - name: crossplane-aws
      type: addon
      properties:
        version: v1.3.0
        parameters:
          region: us-east-1

    - name: crossplane-gcp
      type: addon
      properties:
        version: v1.2.0
        parameters:
          project: my-gcp-project

  workflow:
    steps:
      - name: install-crossplane
        type: apply-component
        properties:
          component: crossplane

      - name: install-providers
        type: step-group
        dependsOn: [install-crossplane]   # providers only start once crossplane is healthy
        subSteps:
          - name: install-crossplane-aws
            type: apply-component
            properties:
              component: crossplane-aws
          - name: install-crossplane-gcp
            type: apply-component
            properties:
              component: crossplane-gcp
```

---

# KEP-2.16 - SourceDefinitions

## The Problem Today

```yaml
# To use external data in a component, you need a workflow step.
spec:
  workflow:
    steps:
      - name: fetch-cluster-config
        type: http
        properties:
          url: http://config-service/cluster/us-east-1
          method: GET
      - name: deploy-api
        type: apply-component
        properties:
          component: api
        dependsOn: [fetch-cluster-config]
```

- Repetitive orchestration logic leaks into every Application
- Breaks the abstraction simplicity offered by KubeVela
- No caching - every reconcile hits the external system
- No schema enforcement - any field path, no type safety
- Platform team can't control what gets fetched or how

---

# KEP-2.16 - SourceDefinitions

## The Fix: `SourceDefinition`

Platform engineer writes **once**. Application authors bind and consume.

```cue
// cluster-config-reader.cue  (platform engineer authors this)

"cluster-config-reader": {
  type: "source"
  attributes: { scope: "spoke" }
}

schema: {
  region:      string
  environment: string
  vpcId?:      string   // optional - may be absent
  accountId:   string
}

storage: {
  key:        "cluster-config-reader-\(context.cluster)"
  storageTTL: "15m"
}

template: {
  _cfg: ex.#Read & {
    $params: { 
      apiVersion: "v1", 
      kind: "ConfigMap",       
      metadata: { 
        name: "cluster-config", 
        namespace: "platform-data"  
      } 
    }
  }
  output: {
    region:      _cfg.$returns.data.region
    environment: _cfg.$returns.data.environment
    vpcId:       _cfg.$returns.data.vpcId
    accountId:   _cfg.$returns.data.accountId
  }
}
```

---

# KEP-2.16 - SourceDefinitions

## Application Author: Binding & Consuming

```yaml
apiVersion: core.oam.dev/v1beta1
kind: Application
spec:
  sources:
    - name: cluster-info
      type: cluster-config-reader

  components:
    - name: api
      type: webservice
      properties:
        region:
          fromSource: cluster-info.region       # shorthand: source.field
        accountId:
          fromSource: cluster-info.accountId
        vpcId:
          fromSource:
            name: cluster-info
            path: vpcId
            default: "my-vpc"                  # required: vpcId is optional in schema...
        image: myapp:v1                        # but mandatory in component
```

---

# KEP-2.16 - SourceDefinitions

## The Cache

```
fromSource: cluster-info.region

    ↓

Evaluate storage: key = "cluster-config-reader-us-east-1"

    ↓

Layer 1: In-memory LRU hit?  → return immediately (no I/O)

    ↓ miss

Layer 2: Config object fresh? (age < storageTTL)  → return cached value

    ↓ miss or expired

Execute CueX template: (reads ConfigMap from cluster)

    ↓

Write Config object, populate LRU cache, return value

    ↓

Substitute into component properties

    ↓

Component CUE template runs with concrete inputs
```

CueX execution only happens on cache miss or TTL expiry.

---

# KEP-2.16 - SourceDefinitions

## Source Chaining

```yaml
spec:
  sources:
    # Resolved first
    - name: cluster-info
      definition: cluster-config-reader

    # Resolved second - uses cluster-info output as input
    - name: app-config
      definition: app-config-reader
      properties:
        region:
          fromSource: cluster-info.region       # already resolved above
        environment:
          fromSource: cluster-info.environment

  components:
    - name: api
      type: webservice
      properties:
        dbEndpoint:
          fromSource: app-config.dbEndpoint
```

---

# KEP-2.16 - SourceDefinitions

## Platform Pattern: Governance via Labels

```cue
// governance-metadata.cue  (no parameters - driven entirely by app labels)

storage: {
  key:        "governance-\(context.appLabels["example.org/service-name"])-\(context.cluster)"
  storageTTL: "1h"
}
```

```yaml
# Application author only needs the label convention.
metadata:
  labels:
    example.org/service-name: checkout

spec:
  sources:
    - name: governance
      definition: governance-metadata   # no properties needed

  components:
    - name: api
      properties:
        costCentre:
          fromSource: governance.costCentre
```

Platform policies can inject the source binding - authors don't even need to declare it.

---

# KEP-2.16 - SourceDefinitions

## Trust Model

```
Platform engineer (high trust)
    Authors SourceDefinition: arbitrary CueX, HTTP calls, cluster reads
    RBAC (or internal policies) restrict who can create/update SourceDefinition

Application author (narrower trust)
    Binds a named SourceDefinition, supplies properties
    Reads only fields declared in schema:
    Cannot alter resolution logic
    Cannot access fields outside schema:
```

Admission webhook enforces the boundary at `kubectl apply` time:
- Every `fromSource` path validated against `schema:`
- `SubjectAccessReview` checked for each referenced `SourceDefinition`
- Unknown paths, missing `default:` rejected before any resolution occurs

---

# KEP-2.20 - Module & API Line Versioning

## The Problem Today

```bash
# Platform team ships a new version of the postgres addon.
# The PostgreSQL definition gets a new required field.
vela addon upgrade postgres --version v2.0.0

# Every Application using type: database now breaks.
# There is no migration window.
# There is no warning.
# There is no v1 to fall back to.
```

Common workaround: name the new definition `database-v2`.
- No convention, no tooling support, no deprecation lifecycle
- Old `database` name never goes away (nobody dares delete it)

---

# KEP-2.20 - Module & API Line Versioning

## Semver is for Releases. API Lines are for Contracts.

Users don't deploy addons. They deploy Applications that reference **definitions**.

```yaml
# Today: the only identity a user has is a bare name.
components:
  - name: my-db
    type: database        # which database? which version of its schema?
```

```yaml
# Or pin to a specific DefinitionRevision:
components:
  - name: my-db
    type: database@v1.2.3   
    # binds to a specific revision of the database definition
    # Fragile: only a limited number of DefinitionRevisions are retained.
    # On a fresh cluster, v1.2.3 may simply not exist depending on GitOps strategy.
```

Definition semver (`@v1.2.3`) tracks which revision was published, not
whether the parameter contract is stable or what breaking changes it contains.

```yaml
# With API lines: the type reference carries the stability contract.
components:
  - name: my-db
    type: postgres/v1/database   # "I bind to the v1 parameter contract."
```

`v1` is a **promise** from the definition author:
the parameter schema remains fulfillable for the lifetime of this line.
Addon releases come and go underneath it. The user never notices.

With that promise in place, the platform team can ship freely:

Breaking the contract requires a new line (`v2`), not an in-place edit.

---

# KEP-2.20 - Module & API Line Versioning

## The Fix: Module Identity + API Lines

```
module: postgres
  └── v1/                ← API line (stable contract)
  │     └── database     ← ComponentDefinition: postgres-v1-database
  └── v2/                ← new API line (breaking change)
        └── database     ← ComponentDefinition: postgres-v2-database
```

```yaml
# Application author pins to an API line, not a definition name.
spec:
  components:
    - name: my-db
      type: postgres/v1/database    # stable - survives addon upgrades
      properties:
        storage: 50Gi
```

`v1` is the contract. As long as `v1` is installed, this Application renders correctly.

---

# KEP-2.20 - Module & API Line Versioning

## Definition Naming Convention

```
{module}-{apiVersion}-{definition-name}

Examples:
  postgres-v1-database
  postgres-v2-database
  aws-s3-v1-bucket
  aws-s3-v1beta1-encryption-policy
```

`spec.module` and `spec.apiVersion` are new fields on all X-Definitions.
The addon controller stamps them at install time from `_module.cue`.

```bash
kubectl get componentdefinition postgres-v1-database -o yaml | grep -A2 spec:
# spec:
#   module: postgres
#   apiVersion: v1
#   version: 1.2.3    # could be 2.x.x but still v1 conformant
```

---

# KEP-2.20 - Module & API Line Versioning

## The `_module.cue` Contract

```cue
// modules/postgres/_module.cue

module: "postgres"      // globally unique module identifier

lines: {
  v1: {
    enabled: true
  }
  v2: {
    enabled: len(context.cluster) > 0   // CueX expression: context-aware
  }
}
```

```
modules/
  postgres/
    _module.cue         ← module identity + line configuration
    v1/
      _version.cue      ← source references for this line
      database.cue      ← ComponentDefinition template
      auxiliary/
        composition.yaml
    v2/
      _version.cue
      database.cue
```

---

# KEP-2.20 - Module & API Line Versioning

## v1 and v2 Live Side by Side

```bash
# Addon v2.0.0 ships v1 and v2 API lines simultaneously.
vela addon upgrade postgres --version v2.0.0

kubectl get componentdefinitions | grep postgres
# postgres-v1-database    # still installed - existing Apps unaffected
# postgres-v2-database    # new - available for migration
```

```yaml
# Existing Application: unchanged, still works.
components:
  - name: db
    type: postgres/v1/database
    properties:
      storage: 50Gi

# New Application: opts in to v2.
components:
  - name: db
    type: postgres/v2/database
    properties:
      storage: 50Gi
      replicationMode: async   # new v2 field
```

---

# KEP-2.20 - Module & API Line Versioning

## Deprecation Lifecycle

```cue
// modules/postgres/_module.cue  (addon v3.0.0)

lines: {
  v1: {
    enabled:           false   // setting false (from true) triggers deprecation
    deprecationReason: "Use postgres/v2/database - v1 will be removed in postgre v4.0.0"
  }
  v2: {
    enabled: true
  }
}
```

```bash
kubectl get componentdefinition postgres-v1-database -o yaml | grep deprecated
# annotations:
#   definition.oam.dev/deprecated: "true"

# Admission webhook warns on any new Application using deprecated lines.
# Existing Applications continue to work until they migrate.
```

---

# KEP-2.20 - Module & API Line Versioning

## Summary: How They Connect

```
Module author writes postgres/v1/database template
    └─► vela module publish  →  Module registry

Platform team bundles it in an Addon (or modules can exist in-line)
    └─► vela addon publish   →  Addon registry

GitOps declares desired cluster state:
  Addon CR (KEP-2.13): continuously reconciles definitions
  Addon-of-addons Application: composes whole platform

Application author uses the result:
  type: postgres/v1/database  (stable across addon upgrades - KEP-2.20)
  fromSource: cluster-info.region  (no workflow step needed - KEP-2.16)

Definition removed only after all consumers migrate (KEP-2.20 deprecation).
```

---

# Status

All three KEPs are **Ready for Review**.

| KEP | Title | Status |
|-----|-------|--------|
| 2.13 | Declarative Addon Lifecycle | Ready for Review |
| 2.16 | SourceDefinition & fromSource | Ready for Review |
| 2.20 | Module & API Line Versioning | Ready for Review |

Questions, feedback, concerns - please comment on the PR or raise in the community channel.