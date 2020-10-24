# Rollout Trait Design  

## Flagger rollout flow
Here is the 10,000 ft overview of the flagger rollout flow

The overall controller Run function starts the processNextItem every second in the background. The processNextItme basically calls SyncHandler on each item in the queue.
It calls the scheduleCanary() function in a for loop. 

SyncHandler
1. Set the canary status as **Initializing** if canary is not deleted
2. Finalize (revert) the canary process if the canary has "revertOnDelete" flag. This means reverting the canary process when the canary object is deleted. OAM should NEVER set this to true.  
3. Each canary will be mapped to a job which runs the "AdvanceCanary" logic below forever until stopped

AdvanceCanary
1. Create Canary controller/Service router/Mesh router/ through a factory
2. CanaryController initialize the primary resource by copying from the target
3. ServiceRouter initialize creates the serivce for the canary/primary resources
4. Check if the canary has changed if the primary is initialized successfully. Just return if not.
5. Check if the rollout webhook confirms to start
6. Check if the Primary is ready, return if not
7. MeshRouter starts to calculate the traffic weight between primary/canary
8. Set the canary status as **Initialized** if it's initializing and return. Otherwise, set it to **Progressing** if it determined to advance
9. Check if the canary revision (spec) changes or dependency changes during rollout, if so, reset to **Progressing**
10. Check if no need to do analysis, go straight to finish the rollout if not. The default is to route 100% to primary which is not what we want. OAM should NEVER set this to true.
11. Check with rollback webhooks, rollback if something is wrong
12. Check if the status is promoting, call controller promote which just copy canary spec to primary. This will set the status to **Finalizing** if there is no error.
13. Check if the status is finalizing. Finalize delete the canary and set the canary status as **Succeeded**
14. Roll back if the number of failed cases cross certain threshold.
15. Pick a strategy based on the spec.Analysis (A/B testing, Blue/Green or Canary). The canary status is set as **Promoting** at the end of those phase.


## Problems with the flagger design in OAM
The problem with the current flagger design is that it wants to be strictly GitOps compatible. Thus, it 
cannot touch the image related fields in the "target" object. That's why it has to create a "primary" deployment
and treat the "target" as the "canary" while doing the rollout. It finally "promotes" the primary 
by copying everything from the "canary" to the "primary" after it 's done.

We don't need to stick to that scheme, but we face a similar problem as our component/workload controller
will constantly reconcile the target component too. 
1. One way is to pin the component to a specific revision in the application configuration. The user then can modify 
the component and that can be automatically picked up by the rollout trait. The draw-back of this approach is that
the upgrade will end up with the workload with the latest version not the workload in the applcation configuration. 
2. Another way is to not allow the component to specify the "replicas" by the user. In this way, a user can modify
the container related fields which leads to a new workload of size 0 created. The rollout trait 
can pick up the "last" revision of the component and its corresponding workload as the source. The nice part of this 
approach is the new component will be the canary, thus keeping it under the application controller's control
and ready to be upgraded again.

We decide to take the second approach as it matches with the flow better.

## Proposed OAM Design
- Add a new "meshProvider" type "OAM", the scheduler will automatically set the meshProvider as the "routeTrait" implementation
which is SMI for now.
- OAM can fill the workloadRef to the `targetRef` automatically. This will point the trait to the workload 
which has its "componentRevisionName" in its label.
- Write an OAM flagger resource controller that works with the podSpecWorkload/containerizedWorkload/Deployment
    - Find the last live component revision as the canary if the user didn't explicitly spell out the revisionName
    - We need to use the workload itself as the canary, and the "source" as the primary   
    - The Initialize function will be no-op
    - The controller will adjust the replica size between the canary and the primary if there is HPA associated with the workload
    - The Promote function will simply reduce the primary (source) replicas zero and bump the canary (target)  
- It seems that we don't need an OAM specific router (even if the service/routing name is a bit mis-matched) 
- Remove all the hard coded string literal "primary" from the scheduler.go file

## What is not covered
- This design does not cover the case that HPA is involved. This requires some changes at the OAM 
runtime to keep the HPA trait on the old workload.
- We need a way to translate the status of the flagger trait status back to the rollout output if we 
don't plan to introduce a rollout CRD.
- The rollout experience needs to be combined with the metris/route/autoscaler traits together to have
a merely complete CD experience. The very basic case is for the canary to emit Prometheus metrics. The rollout
trait needs to setup metrics checkers and threatholds to move to the next stages. 
