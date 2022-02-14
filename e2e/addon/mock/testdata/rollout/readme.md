
# Supported workload type

Rollout Trait supports following component types: webservice、worker and cloneset.

When using webservice/worker as Workload type with Rollout Trait, Workload's name will be controllerRevision's name. And when Workload's type is cloneset, because of clonset support in-place update Workload's name will always be component's name.

