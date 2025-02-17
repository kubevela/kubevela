# Versioning Support for KubeVela Definitions

<!-- toc -->
- [Versioning Support for KubeVela Definitions](#versioning-support-for-kubevela-definitions)
  - [Summary](#summary)
  - [Scope](#scope)
  - [Motivation](#motivation)
    - [Goals](#goals)
    - [Non-Goals](#non-goals)
  - [Acceptance Criteria](#acceptance-criteria)
  - [Current Implementation](#current-implementation)
    - [Versioning](#versioning)
    - [Auto Upgrade](#auto-upgrade)
      - [Reference:](#reference)
  - [Proposal](#proposal)
    - [Details](#details)
    - [Issues](#issues)
  - [Examples](#examples)
<!-- /toc -->

## Summary

Support Semantic versioning for KubeVela Components and a way to allow fine control over auto-upgrades of KubeVela Applications to new versions of a Component. The implementation should include support for consistent versioning across environments/clusters, meaning specific Revisions/Versions of a Component should have consistent behaviour.

## Scope

Although, this document limits the scope of discussion to ComponentDefinition Revisions/Versions, due to the current implementation, the changes will most likely apply to all [`Definition`](https://kubevela.io/docs/getting-started/definition/) types. These changes are planned to be explored and validated as part of the implementation.

## Motivation

OAM/KubeVela Definitions (referred to as ComponentDefinitions of Components in the rest
of the document) are the basic building blocks of the KubeVela platform. They
expose a contract similar to an API contract, which evolves from minor to major
versions. Applications are composed of Components that the KubeVela engine stitches
together.

KubeVela creates a `DefinitionRevision` for all changes in a Component `spec`.  
Currently, Applications can refer to a particular Revision of a Component. 
But, this versioning scheme has the following issues: 

- The `DefinitionRevision` does not denote the type of the change (patch/bug, minor or major). This hinders automation of automatic upgrades.
- The current scheme also doesn't allow much control over automatic upgrades to new Component Revisions. KubeVela automatically upgrades/reconciles the Application to the
latest when no Component Revision is specified. 
  > While we don't ideally want Application developers to bother with such details, there are use cases where
  > an automatic upgrade to the latest Component version is not desired.


### Goals

- Support Component versioning with Semantic Versions.
- Allow pinning specific and non-specific versions of a Component in the
  KubeVela Application.

### Non-Goals

- Support for version range in Application. For eg. "type: my-component@>1.2.0"

## Acceptance Criteria

**User Story: Component version specification**

>**AS A** Component author\
>**I SHOULD** be able to publish every version of my Component with the Semantic Versioning scheme\
>**SO THAT** an Application developer can use a specific version of the Component.

**BDD Acceptance Criteria**

>**GIVEN** an updated ComponentDefinition Specification \
>**AND** a version denoted by the ComponentDefinition is set to V\
>**WHEN** the Component is applied to KubeVela\
>**THEN** `V` should be listed as one of the many versions in the DefinitionRevision list

**User Story: Application Component version specification**

>**AS AN** Application developer\
>**I SHOULD** be able to specify a version (complete or partial) for every Component used\
>**SO THAT** I can control which version are deployed.

**BDD Acceptance Criteria**

>Scenario 1: Use the version specified in the Application manifest when deploying the service\
>**GIVEN** a Component A with versions 1.2.2 | 1.2.3\
>**AND** a Component B with versions 4.4.2 | 4.5.6\
>**AND** an Application composed of A 1.2.2 and B 4.4.2\
>**WHEN** the Application is deployed\
>**THEN** it uses Component A 1.2.2 and B 4.4.2
>
>**Variant:** Use the latest version for the part of the SemVer that is not specified.\
>**GIVEN** component A latest version is 1.2.3
>**AND** Component B latest version is 4.5.6
>**AND** an Application composed of A 1.2 and B 4\
>**WHEN** the Application is deployed\
>**THEN** it uses Component A 1.2.3 and B 4.5.6

> Scenario 2: Behaviour when auto-upgrade is disabled \
> **GIVEN** a component A with version 1.2.3\
> **AND** an Application composed of A-1.2.3\
> **IF** Auto-upgrade is disabled\
> **WHEN** a new version of Component A (A-1.2.5) is released\
> **THEN** the Application should continue to use A-1.2.3
>
> **Variant** Behaviour when auto-upgrade is disabled and exact version is unavailable.\
> **GIVEN** a component A with version 1.2.3\
> **AND** a new Application composed of A-1.2.2\
> **IF** Auto-upgrade is disabled\
> **WHEN** the Application is applied\
> **THEN** the Application deployment should fail.
>
> **Variant** Behaviour when auto-upgrade is disabled and exact version is unavailable.\
> **GIVEN** a component A with version 1.2.3\
> **AND** a new Application composed of A-1.2\
> **IF** Auto-upgrade is disabled\
> **WHEN** the Application is applied\
> **THEN** the Application deployment should fail.

> Scenario 3: Behaviour when auto-upgrade is enabled \
> **GIVEN** a component A with version 1.2.3\
> **AND** an Application composed of A-1.2\
> **IF** Auto-upgrade is enabled\
> **THEN** the Application should use A-1.2.3
> **AND WHEN** a new version of Component A (A-1.2.5) is released\
> **THEN** the Application should update to use A-1.2.5
>
> **Variant** Behaviour when auto-upgrade is enabled and exact version is unavailable \
> **GIVEN** a component A with version 1.2.3\
> **AND** a new Application composed of A-1.2.2\
> **IF** Auto-upgrade is enabled\
> **WHEN** the Application is applied\
> **THEN** the Application deployment should fail.
>
> **Variant** Behaviour when auto-upgrade is enabled and exact version is unavailable \
> **GIVEN** a component A with version 1.2.3\
> **AND** a new Application composed of A-1.2\
> **IF** Auto-upgrade is enabled\
> **WHEN** the Application is applied\
> **THEN** the Application deployment should use A-1.2.3.

> Scenario 4:  Expectations of consistent versioning across Environments/Clusters. \
> **GIVEN** a component A with versions 1.2.1|1.2.2|2.2.1\
> **AND** an Application composed of A-1.2.2\
> **IF** the Application needs to be deployed across Environments (Dev, Prod etc)\
> **OR** the Application needs to be deployed in multiple clusters managed independently\
> **WHEN** the Application  is deployed across Environments/Clusters \
> **THEN** The Application should behave consistently, as in all the clusters A-1.2.2 map to the same ComponentDefinition changes.

## Current Implementation

### Versioning

Currently, KubeVela has some support for controlling Definition versions based on K8s annotations and DefinitionRevisions. The annotation `definitionrevision.oam.dev/name` can be used to version the ComponentDefinition. For example if the following annotation is added to a ComponentDefinition, it produces a new DefinitionRevision and names the ComponentDefinition as `component-name-v4.4` .

> definitionrevision.oam.dev/name: "4.4"

This Component can then be referred in the Application as follows:

>"component-name@v4.4" - `NamedDefinitionRevision`

Alternatively, since DefinitionRevisions are maintained even if a **"named"** Revision is not specified via the annotation `definitionrevision.oam.dev/name`, Applications can still refer to a particular Revision of a Component via the auto-incrementing Revision numbers.

>"component-name@v2" - `DefinitionRevision`

![version](./kubevela-version.png)

This versioning scheme, although convenient, has the following issues:

- Applications which do not explicitly specify a target Revision of a ComponentDefinition, the "latest" applied revision of the ComponentDefinition is used. In scenarios where a cluster has to be replicated or re-created, this means that the sequence in which revisions of a ComponentDefinition are applied becomes important. Implicitly, this also means that the Component maintainers need to keep all Revisions of a ComponentDefinition in their deployment pipeline.\
If `definitionrevision.oam.dev/name` annotation is not added to ComponentDefinitions, even if the Applications are explicit about a Component Revision, there is currently no guarantee that the Application behaviour will be consistent across Environments/Clusters. For example, a `Dev` environment will typically have more churn in Revisions than a `Prod` one and a reference to Component Revision `v3` in an Application will not be the same in both environments.


### Auto Upgrade
KubeVela utilises the annotation `app.oam.dev/autoUpdate` for automatic upgrade.

Application reconciliation behaviour when the `app.oam.dev/autoUpdate` annotation is specified in the Application: 
- If a ComponentDefinition Revision is not specified, the Application will always use the latest available Revision.
- If a ComponentDefinition Revision is specified and a new Revision is released after the Application was created, the latest changes will not reflect in the Application.

Note: This feature is not documented in KubeVela documentation.

#### Reference:

- https://kubevela.io/docs/platform-engineers/x-def-version/
- [Auto Upgrade PR](https://github.com/kubevela/kubevela/pull/3217)


## Proposal

### Introduce `spec.version` as an optional field in the Definition

- Add an optional field `version` in the Definition `spec` and use it to generate the ComponentDefinition Revisions.

- Update the auto-upgrade behaviour to also allow limiting upgrades for an Application within a specified Definition version range. The existing annotation `app.oam.dev/autoUpdate` for enabling automatic updates will be used for this new behaviour and will maintain backward compatibility.

- Implement Validating webhook to:
  - Ensure that the values of the annotation `definitionrevision.oam.dev/version`, `definitionrevision.oam.dev/name` or `spec.version` field adhere to semantic versioning.
  - Ensure that the `definitionrevision.oam.dev/name` annotation and the `spec.version` field are not present together in the ComponentDefinition to avoid conflicts.
  - Ensure that `app.oam.dev/publishVersion` and `app.oam.dev/autoUpdate` both annotation are not present in Application to avoid conflicts.

### Issues

The following issues assume adherence to strict backward compatibility, meaning the `definitionrevision.oam.dev/name` annotation should continue to work as is.

- It does not resolve inconsistent versioning behaviour across Environments/Clusters when explicit versions are not specified or named DefinitionRevisions are not used.

## Examples
1. Create a `configmap-component` ComponentDefinition with `1.2.5` version 
```
apiVersion: core.oam.dev/v1beta1
kind: ComponentDefinition
metadata:
  name: configmap-component
  namespace: vela-system
spec:
  version: 1.2.5
  schematic:
    cue:
      template: |
        output: {
        	apiVersion: "v1"
        	kind:       "ConfigMap"
        	metadata: {
        		name:      "comptest"
        	}
        	data: {
            version: "125"
        	}
        }

  workload:
    definition:
      apiVersion: v1
      kind: ConfigMap
```

2. Create a `configmap-component` ComponentDefinition with `2.0.5` version
```apiVersion: core.oam.dev/v1beta1
kind: ComponentDefinition
metadata:
  name: configmap-component
  namespace: vela-system
spec:
  version: 2.5.0
  schematic:
    cue:
      template: |
        output: {
        	apiVersion: "v1"
        	kind:       "ConfigMap"
        	metadata: {
        		name:      "comptest"
        	}
        	data: {
            version: "250"
        	}
        }

  workload:
    definition:
      apiVersion: v1
      kind: ConfigMap
```
3. List DefinitionRevisions
```
kubectl get definitionrevision -n vela-system | grep -i my-component
my-component-v1.2.5                1          1a4f3ac77e4fcfef   Component
my-component-v2.5.0                2          e61e9b5e55b01c2b   Component
```

4. Create Application using `configmap-component@v1.2` version and enable the Auto Update using `app.oam.dev/autoUpdate` annotation.
```apiVersion: core.oam.dev/v1beta1
kind: Application
metadata:
  name: test-app
  namespace: test
  annotations:
    app.oam.dev/autoUpdate: "true"
spec:
  components:
    - name: test
      type: my-component@v1
```

    Expected Behavior:
    - Application will use `configmap-component@v1.2.5`, as `1.2.5` is highest version in specified range(`1`).

5. Create a `configmap-component` ComponentDefinition with `1.2.7` version 
```
apiVersion: core.oam.dev/v1beta1
kind: ComponentDefinition
metadata:
  name: configmap-component
  namespace: vela-system
spec:
  version: 1.2.7
  schematic:
    cue:
      template: |
        output: {
        	apiVersion: "v1"
        	kind:       "ConfigMap"
        	metadata: {
        		name:      "comptest"
        	}
        	data: {
            version: "127"
        	}
        }

  workload:
    definition:
      apiVersion: v1
      kind: ConfigMap
```

    Expected Behavior:
    - After the Application is reconciled, it will use `configmap-component@v1.2.7`, as `1.2.7` is the latest version within the specified range (1).

6. List Definitionrevision 
```kubectl get definitionrevision -n vela-system | grep -i my-component
my-component-v1.2.5                1          1a4f3ac77e4fcfef   Component
my-component-v1.2.7                3          86d7fb1a36566dea   Component
my-component-v2.5.0                2          e61e9b5e55b01c2b   Component```
