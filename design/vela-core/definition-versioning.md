# Versioning Support for KubeVela Definitions

<!-- toc -->
- [Versioning Support for KubeVela Definitions](#versioning-support-for-kubevela-definitions)
  - [Summary](#summary)
  - [Motivation](#motivation)
    - [Goals](#goals)
    - [Non-Goals](#non-goals)
  - [Proposal](#proposal)
    - [How to handle existing components](#how-to-handle-existing-components)
  - [Acceptance Criteria](#acceptance-criteria)
  - [Design Details](#design-details)
<!-- /toc -->

## Summary

Support explicit versioning for KubeVela Definitions and a way to specify which
version of the Definition is to be used in an Application spec.


## Motivation

OAM/KubeVela Definitions (referred to as Definitions or Components for the rest
of the document) are the basic building blocks of the KubeVela platform. They
expose a contract similar to an API contract, which evolves from minor to major
versions. Definitions currently have a version field, but it is automatically
assigned and is just an integer.

Such a versioning scheme does not provide any hint on the type of change
(patch/bug, minor or major)

Applications are composed of definitions that the KubeVela engine stitches
together. KubeVela automatically upgrades/reconciles the application to the
latest when no Definition version is specified. While we don't want ideally
application developers to bother with such details, there are use cases where
upgrading to the latest Definition version will need to be controlled.

- It is an issue faced by many Kubevela community members; see the reference
  below.

- This can be mitigated by specifying a version number, but as the current
  versioning scheme does not provide hints on the type of change, it can hardly
  be automated.

### Goals

- Support Definitions versioning using SemVer
- Allow pinning, specific and non-specific versions of a Definition in the
  application

### Non-Goals

- A complicated versioning feature
- Support for version range in Application. For eg.`type: my-component@>1.2.0`

## Proposal

Definitions to explicitly specify a version as part of the definition as
described below:

    apiVersion: core.oam.dev/v1beta1
    kind: ComponentDefinition
    metadata:
    name: <ComponentDefinition name>
    annotations:
        definition.oam.dev/description: <Function description>
    spec:
        version: <semver>
        workload: # Workload Capability Indicator
            definition:
                apiVersion: <Kubernetes Workload resource group>
                kind: <Kubernetes Workload types>
        schematic:  # Component description
            cue: # Details of components defined by CUE language
                template: <CUE format template>

The `version` must be a valid SemVer without the prefix `v` for example,
`1.1.0`, `1.2.0-rc.0`.

The Application will then have the ability to refer to the version when using a
Definition like this

    apiVersion: core.oam.dev/v1beta1
    kind: Application
    metadata:
        name: app-with-comp-versioning
    spec:
        components:
            - name: backend
              type: my-component-type@1.0.2

When selecting a Definition version in the application spec, it is possible to
select a non-specific version, like `v1.0` or `v1`, the KubeVela will
auto-upgrade the application to use the latest version in the specified version
series. For instance, if the application specifies `type: my-component-type@1.0`
and `v1.0.1` of the Definition is available, KubeVela will re-render the
application using this version.

In case no version is specified then the version is always upgraded to the
latest.

KubeVela will initiate the reconciliation of the application as soon as a new
version of a component is available and the application is eligible (based on
the version specificity) to be upgraded.

> **Proposal Notes:** We are intentionally skipping a flag like auto-upgrade:
> true|false if one does not want to auto-upgrade their component, they should
> always use specific semVer in the application spec. This is also consistent
> with existing behaviour where we always use the latest version of the
> Definition when the application reconciles. We are just providing a way to opt
> out of this behaviour by pinning the component version in the application.
> This is also in line with the philosophy of keeping complexity at the platform
> level instead of the application level.

> **For Definition Maintainers:** Ideally the upgrades to Definition
> should be backwards compatible for a given Major version. Updates to
> Definitions should never force the application spec to change. If a new
> component is changing something significant it should be a Definition with a
> new name and not the new version of the existing Definition.

### How to handle existing components

The existing component will not have the `version` in its spec. We will treat
them under legacy versioning behaviour. They will continue to behave the same
way, including how their usage in an application is upgraded only when it is
modified. A Definition will be *enrolled* to new versioning behaviour as soon
the `version` field is set in the definition spec. All the usages of this
component by existing application will be enrolled for auto-upgrade behaviour of
the new versioning scheme.

> Proposal Notes: At first glance, it seems like switching over to the new the
> versioning scheme can cause chaos by triggering auto-upgrade across clusters,
> but this is again in line with the philosophy of keeping control in the hands
> of the platform team rather than the application developer. The platform team
> should plan carefully when they enrol their component to new versioning. For
> instance avoid changing anything except `version` in the spec when a component
> is enrolled on new versioning.
>

## Acceptance Criteria

**User Story: Definition version specification**

**As a** Definition author
**I should** be able to publish every revision of my Definitions with the Semantic Versioning scheme
**So that** an Application developer can use a specific revision of the Definition

**BDD Acceptance Criteria**

**Given** a Definition spec
**AND** a version field of the spec set to V
**When** the Definition is applied to the KubeVela
**Then** it should be listed as one of the many versions in the Definition list

**User Story: Application Definition version specification**

**As an** Application developer
**I should** be able to specify a revision (complete or partial) for every Definition used
**So that** I can control which revisions are deployed

**BDD Acceptance Criteria**

Scenario: Use the version specified in the application manifest when deploying the service
**Given** a Definition A with version 1.2.3
**And** a Definition B with version 4.5.6
**And** an application composed of A 1.2.2 and B 4.4.2
**When** the application is deployed
**Then** it uses Definition A 1.2.2 and B 4.4.2

**Variant:** use the latest version for the part of the semVer that is not specified.
**And** an application composed of A 1.2 and B 4
**When** the application is deployed
**Then** it uses Definition A 1.2.3 and B 4.5.6

## Design Details

To support versioning we can use the existing `definitionrevisions.core.oam.dev`
resource.

TODO: add more details