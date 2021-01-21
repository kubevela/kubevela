# OAM Rollout Controller Design

- Owner: Ryan Zhang (@ryangzhang-oss)
- Date: 01/14/2021
- Status: Draft

## Table of Contents

- [Introduction](#introduction)
   - [Two flavors of Rollout](#two-flavors-of-rollout)
- [Goals](#goals)
- [Proposal](#proposal)
  - [Registration via Definition/Capability](#registration-via-definitioncapability)
  - [Templating](#templating)
  - [CLI/UI interoperability](#cliui-interoperability)
  - [vela up](#vela-up)
- [Examples](#examples)

## Introduction

`Rollout` or `Upgrade` is one of the most essential "day 2" operation on any application
. KubeVela, as an application centric platform, definitely needs to provide a customized solution
to alleviate the  burden on the application operators. There are several popular rollout solutions
, i.e. [flagger](https://flagger.app/),  in the open source community. However, none of them
work directly with our OAM framework. Therefore, we propose to create an OAM native rollout
 framework that can address all the application rollout/upgrade needs in Kubevela.
    
### Two flavors of Rollout
After hearing from the OAM community, it is clear to us that there are two flavors of rollout
 that we need to support. 
 - One way is through an OAM trait. This flavor works for existing OAM applications that don't
  have many specialized requirements such as interaction with other traits. For
  example, most rollout operations don't work well with scaling operations. Thus, the
  application operator needs to remove any scalar traits from the component before applying the
  rollout trait. Another example is rollout operations usually also involve traffic spliting
  of sorts. Thus, the application operators might also need to manually adjust the related
  traffic trait before and after applying the rollout trait. 
 - The other ways is through a new applicationDeployment CR which directly reference different
  versions of applications instead of workloads. This opens up the possibility for the controller
  to solve conflicts between traits automatically. This resource, however, not a trait and requires
  the application to be immutable. 

### Design Principles and Goals
We design our controllers with the following principles in mind 
  - First, we want all flavors of rollout controllers share the same core rollout
   related logic. The trait and application related logic can be easily encapsulated into its own
   package.
  - Second, the core rollout related logic is easily extensible to support different type of
   workloads, i.e. Deployment, Cloneset, Statefulset, Daemonset or even customized workloads. 
  - Thirdly, the core rollout related logic has a well documented state machine that
   does state transition explicitly.
  - Finally, the controllers can support all the rollout/upgrade needs of an application running
   in a production environment.
   
### Proposal Layout
Here is the rest of the proposal 
  - First, we will present the exact rollout CRD spec.
  - Second, we will give a high level design on how do we plan to implement the controller.
  - Third, we will present the state machine and their transition events. 
  - Finally, we will list common rollout scenarios and their corresponding user experience and
   implementation details.
      
## Rollout API Design
Let's start with the rollout trait spec definition. The applicationDeployment spec is very similar.
```go
// RolloutTraitSpec defines the desired state of RolloutTrait
type RolloutTraitSpec struct {
	// TargetRef references a target resource that contains the newer version
	// of the software. We assumed that new resource already exists.
	// This is the only resource we work on if the resource is a stateful resource (cloneset/statefulset)
	TargetRef runtimev1alpha1.TypedReference `json:"targetRef"`

	// SourceRef references the list of resources that contains the older version
	// of the software. We assume that it's the first time to deploy when we cannot find any source.
	// +optional
	SourceRef []runtimev1alpha1.TypedReference `json:"sourceRef,omitempty"`

	// RolloutPlan is the details on how to rollout the resources
	RolloutPlan RolloutPlan `json:"rolloutPlan"`
}
```
The target and source here are the same as other OAM traits that refers to a workload instance
 with its GVK and name. 
 
We can see that the core part of the rollout logic is encapsulated by the `RolloutPlan` object. This
 allows us to write multiple controllers that share the same logic without duplicating the code
 . Here is the definition of the `RolloutPlan` structure.

```go
// RolloutPlan fines the details of the rollout plan
type RolloutPlan struct {

	// RolloutStrategy defines strategies for the rollout plan
	// +optional
	RolloutStrategy RolloutStrategyType `json:"rolloutStrategy,omitempty"`

	// The size of the target resource. The default is the same
	// as the size of the source resource.
	// +optional
	TargetSize *int32 `json:"targetSize,omitempty"`

	// The number of batches, default = 1
	// mutually exclusive to RolloutBatches
	// +optional
	NumBatches *int32 `json:"numBatches,omitempty"`

	// The exact distribution among batches.
	// mutually exclusive to NumBatches
	// +optional
	RolloutBatches []RolloutBatch `json:"rolloutBatches,omitempty"`

	// All pods in the batches up to the batchPartition (included) will have
	// the target resource specification while the rest still have the source resource
	// This is designed for the operators to manually rollout
	// Default is the the number of batches which will rollout all the batches
	// +optional
	BatchPartition *int32 `json:"lastBatchToRollout,omitempty"`

	// RevertOnDelete revert the rollout when the rollout CR is deleted, default is false 
	//+optional 
	RevertOnDelete bool `json:"revertOnDelete,omitempty"`

	// Paused the rollout, default is false 
	//+optional 
	Paused bool `json:"paused,omitempty"`

	// RolloutWebhooks provides a way for the rollout to interact with an external process
	// +optional
	RolloutWebhooks []RolloutWebhook `json:"rolloutWebhooks,omitempty"`

	// CanaryMetric provides a way for the rollout process to automatically check certain metrics
	// before complete the process
	// +optional
	CanaryMetric []CanaryMetric `json:"canaryMetric,omitempty"`
}
```

## User Experience Workflow
OAM rollout experience is different from flagger in a few different ways and here are the
 implications on its impact on the user experience.
- We assume that the resources it refers to are **_immutable_**. In contrast, flagger watches over a
  target resource and reacts whenever the target's specification changes.
    - The trait version of the controller refers to componentRevision.
    - The application version of the controller refers to immutable application. 
- The rollout logic **_works only once_** and stops after it reaches a terminal state. One can
  still change the rollout plan in the middle of the rollout as long as it does not change the
  pods that are already updated. 
- The applicationDeployment controller only works with applications with 1 component for now.
- Users should rely on the rollout CR to do the actual rollout which means they
 shall set the `replicas` or `partition` field of the new resources to the starting value
 indicated in the [detailed rollout plan design](#rollout-plan-work-with-different-type-of-workloads).
- 
    
 
## Notable implementation level details
Here are some high level implementation decisions

### Rollout on different resources
As we mentioned in the introduction section, we will implement two rollout controllers that work
on different levels. At the end, they both emit an in-memory rollout plan object which includes
references to the target and source kubernetes resources that the rollout planner will execute
upon. For example, the applicationDeployment controller will get the component from the
 application and extract the real workload from it before passing it to the rollout plan object.
 
 With that said, two controllers operate differently to extract the real workload. Here are the
  high level descriptions of how each works. 
 
#### ApplicationDeployment takes control of the application during rolling out phase
When an appDeployment is used to do application level rollout, **_one should apply the
 appDeployment CR before the application itself_**. This is to try to make sure that the
  appDeployment controll has full control of the new application from the beginning.
 However, there is no kubernetes systematic way of acquiring a distributed lock for a controller.  
- The appDeployment controller will make sure it marks itself as the owner of the
 application and changes the appDeployment CR state to be initialized. The application controller
  can be aware of appDeployment and have build-in logics in case.
- The appDeployment controller can change the application fields. For example, it can remove all
 the conflict traits, such as HPA, during the upgrade period. 
- Upon a successful rollout, the appDeployment controller can remove all the traits in the old
  application and leave no pods in the workload.
- Upon a failed rollout, the condition is not determined, it could result in an unstable state.
- Thus, we introduced a `revertOnDelete` field so that a user can delete the appDeployment and
  expect the old application to be intact.  

#### Rollout trait works with componentRevision only
The rollout traits controller only works with componentRevision.
- The component controller emits the new component revision when a new component is created
- The application Configuration controller emits the new component and get its componentRevision
- Upon a successful rollout, the rollout trait will keep the old component revision with no
 pod left.
- Upon a failed rollout,the rollout trait will just stop and leaves the resource mixed. This
 state mostly should still work since the other traits are not touched.

### Rollout plan work with different type of workloads
The rollout plan part of the rollout logic is shared between all rollout controller. It comes
 with references to the target and source workload. The controller is responsible for fetching
the different revisions of the resources. Deployment and cloneset represents the two major types
 of resources in that cloneset can contain both the new and old revisions at a stable state while
 deployment only contains one version when it's stable.
 
#### Rollout plan works with deployment
It's pretty straightforward for the outer controller to create the in-memory target and source
 deployment object since they are just a copy of the kubernetes resources.
- The deployment workload should set the `replicas` field to be zero in the beginning by the user.
- Another options is for it to leave `replicas` field as optional. Since the
 default of `replicas` field is one, this  means the target deployment is created with one pod
 . While not ideal, this cannot be avoided. However, the deployment workload handler can check
  the health of the target before rolling it out.
- We assume that there is no other scalar related traits deployed at the same time. We will use
 `conflict-with` fields in the traitDefinition and webhooks to enforce that.
- If the rollout is successful, the source deployment `replicas` field will be zero and the
 target deployment will be the same as the original source.
- If the rollout failed, we will leave the state as it is
- If the rollout failed and `revertOnDelete` is `true` and the rollout CR is deleted, then the
 source deployment `replicas` field will be turned back to before rollout and the target deployment's `replicas` field will
 be zero.

#### Rollout plan works with cloneset
The outer controller creates the in-memory target and source cloneset object with different image
 ids. The source is only used when we do need to do rollback. 
- The user should set the cloneset workload's **_`partition` field the same as its
 `replicas` field_** before the rollout. This is the only way for the rollout controller to control
  the entire rollout phase.
- The rollout plan mostly just adjusts the  `partition` field in the cloneset and leaves the rest
 of the rollout logic to the cloneset controller.
- If the rollout is successful, the `partition` field will be zero
- If the rollout failed, we will leave the `partition` field as the last time we touch it.
- If the rollout failed and `revertOnDelete` is `true` and the rollout CR is deleted, we will
 perform a revert on the cloneset.  Since the cloneset controller doesn't do rollback when one
 increases the `partition`, the rollout plan can only revert it by replacing the entire
 target cloneset spec with the source spec and `partition` as zero.

### Each rollout plan is an eventual consistency type of logic called in a continues loop goroutine.
- Each loop call basically drives the state of the rollout to a terminal state based on the
     current state of the resource/canary. 
- its state won't move after reaching a terminal state unless the target ref changes. 

### The rollout controller use a finalizer
- revert the rollout if the revertOnDelete field is true, and the rollout status is not
 succeeded.
- return the control back to the application controller.

### We import the implementation of notifiers from flagger through go mod
- We can consider adding an alert rule in the rollout Plan api in the future

### We use webhook to validate whether the change to the rollout CRD is valid.
We will go with strict restrictions on the CRD update in that nothing can be updated other than
the following fields:
- the BatchPartition field can only increase unless the target ref has changed
- the RolloutBatches field can only change the part after the BatchPartition field
- the CanaryMetric/Paused/RevertOnDelete can be modified freely.
- the rollout controller will simply replace the existing rollout CR value in the in-memory map
 which will lead to its in-memory execution to stop. The new CR will kick off a new execution
 loop which will resume the rollout operation based on the rollout and its resources status which
  follows the pre-determined state machine transition.

#### The controller have extension points setup for the following plug-ins:
- workloads. Each workload handler needs to implement the following operations:
    - scale the resources
    - determine the health of the workload
    - report how many replicas are upgraded/ready/available
- (future) metrics provider.
- (future) service mesh provider. Each mesh provider needs to implement the following operations:
     - direct certain percent of the traffic to the source/target workload
     - fetch the current traffic split

## Future work
The applicationDeployment should also work on traits. For example, if someone plans to update the
 HPA traits formula, there should be a way for them to rolling out the HPA change step by step too.
 
