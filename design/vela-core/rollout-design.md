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
	TargetRef corev1.ObjectReference `json:"targetRef"`

	// SourceRef references the list of resources that contains the older version
	// of the software. We assume that it's the first time to deploy when we cannot find any source.
	// +optional
	SourceRef []corev1.ObjectReference `json:"sourceRef,omitempty"`

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
OAM rollout experience is different from flagger in some key areas and here are the
 implications on its impact on the user experience.
- We assume that the resources it refers to are **immutable**. In contrast, flagger watches over a
  target resource and reacts whenever the target's specification changes.
    - The trait version of the controller refers to componentRevision.
    - The application version of the controller refers to immutable application. 
- The rollout logic **works only once** and stops after it reaches a terminal state. One can
  still change the rollout plan in the middle of the rollout as long as it does not change the
  pods that are already updated. 
- The applicationDeployment controller only rollout one component in the applications for now.
- Users in general should rely on the rollout CR to do the actual rollout which means they
 shall set the `replicas` or `partition` field of the new resources to the starting value
 indicated in the [detailed rollout plan design](#rollout-plan-work-with-different-type-of-workloads).
    
 
## Notable implementation level design decisions
Here are some high level implementation design decisions that will impact the user experience of
 rolling out.

### Rollout workflows
As we mentioned in the introduction section, we will implement two rollout controllers that work
on different levels. At the end, they both emit an in-memory rollout plan object which includes
references to the target and source kubernetes resources that the rollout planner will execute
upon. For example, the applicationDeployment controller will get the component from the
 application and extract the real workload from it before passing it to the rollout plan object.
 
 With that said, two controllers operate differently to extract the real workload. Here are the
  high level descriptions of how each works. 
 
 #### Application inplace upgrade workflow 
 The most natural way to upgrade an application is to upgrade it in-place which means the users
just change the application, and the system will pick up the change, then apply to the runtime
. The implementation of this type of upgrade looks like this:
- The application controller compute a hash value of the applicationConfiguration. The
 application controller **always** use the component revision name in the AC it generates. This
  guaranteed that the AC also changes when the component changes.
- The application controller creates the applicationConfiguration with a new name (with a suffix
) upon changing of its hash value and with a pre-determined annotation 
 "app.oam.dev/appconfig-rollout" set to true. 
- The AC controller have special handle logic in the apply part of the logic. The exact logic
 depends on the workload type and we will list each in the 
 [rollout with different workload](#Rollout plan work with different type of workloads) section
 . This special AC logic is also the real magic for the other rollout scenario to work as AC
  controller is the only entity that is directly responsible for emitting the workload to the k8s.
   
   
#### ApplicationDeployment workflow 
When an appDeployment is used to do application level rollout, **the target application
is not reconciled by the application controller yet**. This is to make sure  the
appDeployment controller has the full control of the new application from the beginning.
We will use a pre-defined annotation "app.oam.dev/rollout-template" that equals to "true" to facilitate
 that. We expect any system, such as the [kubevela apiserver](APIServer-Catalog.md), that
  utilizes an appDeployment object to follow this rule.
- Upon creation, the appDeployment controller marks itself as the owner of the application. The
 application controller will have built-in logic to ignore any applications that has the
 "app.oam.dev/rollout-template" annotation set to true.
- the appDeployment controller will also add another annotation "app.oam.dev/creating" to the
 application to be passed down to the ApplicationConfiguration CR it generates to mark 
 that the AC is reconciled for the first time.
- The ApplicationConfiguration controller recognizes this annotation, and it will see if there is
 anything it needs to do before emitting the workload to the k8s. The AC controller removes this
  annotation at the end of a successful reconcile.
- The appDeployment controller can change the target application fields. For example, 
   - It might remove all the conflict traits, such as HPA during upgrade. 
   - It might modify the label selectors fields in the services to make sure there are ways to
    differentiate traffic routing to the old and new application resources.
- The appDeployment controller will return the control of the new application back to the
 application controller after it makes the initial adjustment of the application by removing the
  annotation.
  - We will use a webhook to ensure that the "rollout" annotation cannot be added back once removed.
- Upon a successful rollout, the appDeployment controller leaves no pods running for the old
 application.
- Upon a failed rollout, the condition is not determined, it could result in an unstable state
 since both the old and new applications have been modified. 
- Thus, we introduced a `revertOnDelete` field so that a user can delete the appDeployment and
  expect the old application to be intact, and the new application takes no effect.

#### Rollout trait workflow
The rollout traits controller only works with componentRevision.
- The component controller emits the new component revision when a new component is created or
 updated.
- The application configuration controller emits the new component and assign the
 componentRevision as the source and target of rollout trait. 
- We assume that there is no other scalar related traits deployed at the same time. We will use
 `conflict-with` fields in the traitDefinition and webhooks to enforce that.
- Upon a successful rollout, the rollout trait will keep the old component revision with no
 pod left.
- Upon a failed rollout,the rollout trait will just stop and leaves the resource mixed. This
 state mostly should still work since the other traits are not touched.

### Rollout plan work with different type of workloads
The rollout plan part of the rollout logic is shared between all rollout controller. It comes
 with references to the target and source workload. The controller is responsible for fetching
the different revisions of the resources. Deployment and Cloneset represents the two major types
 of resources in that Cloneset can contain both the new and old revisions at a stable state while
 deployment only contains one version when it's stable.
 
#### Rollout plan works with deployment
It's pretty straightforward for the outer controller to create the in-memory target and source
 deployment object since they are just a copy of the kubernetes resources.
- The deployment workload should set the **`Paused` field to be true** by the user in the
 appDeployment case.
- Another options is for the user to leave the `replicas` field as 0 if the rollout does not have
 access to that field.
- If the rollout is successful, the source deployment `replicas` field will be zero and the
 target deployment will be the same as the original source.
- If the rollout failed, we will leave the state as it is.
- If the rollout failed and `revertOnDelete` is `true` and the rollout CR is deleted, then the
 source deployment `replicas` field will be turned back to before rollout and the target deployment's `replicas` field will
 be zero.

#### Rollout plan works with cloneset
The outer controller creates the in-memory target and source cloneset object with different image
 ids. The source is only used when we do need to do rollback.
- The user should set the Cloneset workload's **`Paused` field to be true** by the user in the
  appDeployment case.
- Another options is for the user to leave the `partition` field in a value that effectively stop
 upgrade if the rollout does not have access to that field.
- The rollout plan mostly just adjusts the  `partition` field in the Cloneset and leaves the rest
 of the rollout logic to the Cloneset controller.
- If the rollout is successful, the `partition` field will be zero
- If the rollout failed, we will leave the `partition` field as the last time we touch it.
- If the rollout failed and `revertOnDelete` is `true` and the rollout CR is deleted, we will
 perform a revert on the Cloneset. Note that only the latest Cloneset controller allows rollback
 when one increases the `partition` field.

### Operational features
- We will use the same service mesh model as flagger in the sense that user needs to provide the
 service mesh provider type and give us the reference to an ingress object. 
    - We plan to directly import the various flagger mesh implementation.
- We plan to import the implementation of notifiers from flagger too
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

### The controller have extension points setup for the following plug-ins:
- workloads. Each workload handler needs to implement the following operations:
    - scale the resources
    - determine the health of the workload
    - report how many replicas are upgraded/ready/available
- (future) metrics provider.
- (future) service mesh provider. Each mesh provider needs to implement the following operations:
     - direct certain percent of the traffic to the source/target workload
     - fetch the current traffic split

## State Transition
Here is the state transition graph

![](https://raw.githubusercontent.com/oam-dev/kubevela.io/main/docs/resources/approllout-status-transition.jpg)

Here are the various top-level states of the rollout 
```go
	// VerifyingSpecState indicates that the rollout is in the stage of verifying the rollout settings
    // and the controller can locate both the target and the source
    VerifyingSpecState RollingState = "verifyingSpec"
    // InitializingState indicates that the rollout is initializing all the new resources
    InitializingState RollingState = "initializing"
    // RollingInBatchesState indicates that the rollout starts rolling
    RollingInBatchesState RollingState = "rollingInBatches"
    // FinalisingState indicates that the rollout is finalizing, possibly clean up the old resources, adjust traffic
    FinalisingState RollingState = "finalising"
    // RolloutFailingState indicates that the rollout is failing
    // one needs to finalize it before mark it as failed by cleaning up the old resources, adjust traffic
    RolloutFailingState RollingState = "rolloutFailing"
    // RolloutSucceedState indicates that rollout successfully completed to match the desired target state
    RolloutSucceedState RollingState = "rolloutSucceed"
    // RolloutAbandoningState indicates that the rollout is abandoned, can be restarted. This is a terminal state
    RolloutAbandoningState RollingState = "rolloutAbandoned"
    // RolloutFailedState indicates that rollout is failed, the target replica is not reached
    // we can not move forward anymore, we will let the client to decide when or whether to revert.
    RolloutFailedState RollingState = "rolloutFailed"
)
```

These are the sub-states of the rollout when its in the rolling state.
```go
	// BatchInitializingState still rolling the batch, the batch rolling is not completed yet
    BatchInitializingState BatchRollingState = "batchInitializing"
    // BatchInRollingState still rolling the batch, the batch rolling is not completed yet
    BatchInRollingState BatchRollingState = "batchInRolling"
    // BatchVerifyingState verifying if the application is ready to roll.
    BatchVerifyingState BatchRollingState = "batchVerifying"
    // BatchRolloutFailedState indicates that the batch didn't get the manual or automatic approval
    BatchRolloutFailedState BatchRollingState = "batchVerifyFailed"
    // BatchFinalizingState indicates that all the pods in the are available, we can move on to the next batch
    BatchFinalizingState BatchRollingState = "batchFinalizing"
    // BatchReadyState indicates that all the pods in the are upgraded and its state is ready
    BatchReadyState BatchRollingState = "batchReady"
)
```

## Future work
The applicationDeployment should also work on traits. For example, if someone plans to update the
 HPA traits formula, there should be a way for them to rolling out the HPA change step by step too.
 
