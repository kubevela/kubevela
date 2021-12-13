# ResourceTracker: Managing Resources behind Application

- Owner: Da Yin (@somefive)
- Reviewer: Jian Li (@leejanee), Jianbo Sun (@wonderflow)
- Date: 12/10/2021
- Status: Implemented

## Intro

As the release of Workflow in KubeVela v1.1, resources dispatched by application can be extremely dynamic.
Instead of simply declaring in Application Component, users can have advanced control for applied resources by leveraging WorkflowSteps such as `ApplyObject` or operators such as `op.Delete`.
Meanwhile, with EnvBinding Policy also released in KubeVela 1.1, users can have multiple same components but deployed in different clusters.
These techniques raise new challenges for tracking and maintaining of resources.

## Goal

To tackle these challenges, a new architecture of ResourceTracker is proposed and implemented.
Generally, there are several major technical changes compared to the previous version:

1. Resources do not use OwnerReference to track their ResourceTracker anymore. In other word, the previous bi-directional binding is simplified into uni-directional, which allows us to have more flexible resource management strategies (such as releasing the control of resources).
2. Resources rendered manifests are recorded in ResourceTracker optionally. Based on that, we can prevent configuration drift by leveraging the reconciling mechanism of the Kubernetes operator pattern. 
3. ResourceTracker deletion now use finalizer and leverage ApplicationController to reconcile. This ensures the deletion of Application truly removes all managed resources. Also, it allows users to manage versioned resource manually. 
4. ResourceTracker in the HubCluster can track resources in ManagedClusters, so that no ResourceTracker is needed anymore in ManagedCluster. Now we can use caches for ResourceTracker again. Additionally, we do not individual multicluster garbage-collect logic anymore.

From the perspective of users, the direct new-incoming capabilities include:
1. Users can prevent configuration drift by default, which is a common usage of the classical Application model. Alternatively, they can only dispatch resources by leveraging [ApplyOnce](../../docs/examples/app-with-policy/apply-once-policy) Policy, which is the mode of Application-as-Workflow.
2. Users can have customized life-cycle control for application resources by leveraging [GarbageCollect](../../docs/examples/app-with-policy/gc-policy) Policy. For example, users might want to keep resources after version updates or application removal.

## Implementations

### ResourceTracker Types

There are several *ResourceTrackers* maintained for one Application.
- **Versioned ResourceTracker**: Each ResourceTracker keeps the record for the resources of one Application generation. Most resources are kept here. When application spec is updated, new versioned ResourceTracker will be created and used. 
- **Root ResourceTracker**: This ResourceTracker keeps the record of the resources that shares the life-cycle with the Application instead of a single version. Resources recorded here will not be recycled until Application is deleted.
- **ControllerRevision ResourceTracker**: This ResourceTracker tracks all the dispatched component revisions. When some components are not in use in new versions, this ResourceTracker can elegantly recycle the revisions for those components. 

### ResourceKeeper

The main implementation of the resource management logic locates at [pkg/resourcekeeper](../../pkg/resourcekeeper).
The **ResourceKeeper** takes charge of the dispatching, tracking, and recycling of all resources.
- **Dispatch**: First record resources in **ResourceTracker**, then apply resources. Depending on the life-cycle of resources, either **Versioned ResourceTracker** or **Root ResourceTracker** will be used.
- **Delete**: First mark resources as deleted in **ResourceTracker**, then delete resources.
- **StateKeep**: Ranging over all managed resources in the latest **Versioned ResourceTracker** and **Root ResourceTracker**, re-apply those resources.
- **GarbageCollect**: Mark outdated or unused ResourceTrackers as deleted and garbage-collect their managed resources. Details will be delivered below.

### Garbage Collection Details

The **GarbageCollect** process includes several steps.
0. **Init**: Scanning over all managed resources in all **Versioned ResourceTrackers** and **Root ResourceTracker** (do not retrieve content from APIServer), aggregating the trackers of each resource and calculate which one RT is responsible for garbage collecting it. 
1. **Mark Stage**: Ranging over all ResourceTrackers. If `KeepLegacyResources` is not enabled, outdated ResourceTrackers will be marked as deleted. If enabled, inactive ResourceTrackers, that have all managed resources removed or managed by newer ResourceTrackers, will be marked as deleted. 
2. **Sweep Stage**: For all ResourceTrackers marked as deleted, check if all inactive managed resources (managed by newer RT or deleted) are removed (do not exist). If true, remove the finalizer of the ResourceTracker (truly remove it).
3. **Finalize Stage**: For all ResourceTrackers marked as deleted, deleting all inactive managed resources.
4. **GarbageCollectComponentRevisionResourceTracker**: Ranging over all resources in active ResourceTrackers and calculate the component usage. For ComponentRevisions whose component is not in-use anymore, remove them.

The **Mark Stage** and **GarbageCollectComponentRevisionResourceTracker** will only run when application workflow succeeded, which means when application is still running workflow or new release is not successful, outdated ResourceTrackers will not be marked and resources will not be recycled.
