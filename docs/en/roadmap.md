# KubeVela Roadmap

![alt](../resources/KubeVela-01.png)

KubeVela is a young project and still have a lot to achieve. This page will highlight some notable ideas and tasks that the community is working on. For detailed view into the roadmap including bug fixes and feature set of the certain release, please check the [GitHub project board](https://github.com/oam-dev/kubevela/projects/1).

Overall, KubeVela alpha release will mainly focus on:
1. Making the project more stable,
2. Complete the feature set of capability extensibility,
3. Developer-centric experience around Appfile (Application-as-Code).

# Notable Tasks

KubeVela Controller:
- [Moving CUE based abstraction layer to kubevela core instead of cli side](https://github.com/oam-dev/kubevela/projects/1#card-48198530).
- [Compatibility checking between workload types and traits](https://github.com/oam-dev/kubevela/projects/1#card-48199349) and [`conflictsWith` feature](https://github.com/oam-dev/kubevela/projects/1#card-48199465)
- [Simplify revision mechanism in kubevela core](https://github.com/oam-dev/kubevela/projects/1#card-48199829)
- [Capability Center (i.e. ddon registry)](https://github.com/oam-dev/kubevela/projects/1#card-48203470)
- [CRD registry to manage the third-party dependencies easier](https://github.com/oam-dev/kubevela/projects/1#card-48200758)

KubeVela DevEx:
- [Smart Dashboard based on CUE schema](https://github.com/oam-dev/kubevela/projects/1#card-48200031)
- [Make defining CUE templates easier](https://github.com/oam-dev/kubevela/projects/1#card-48200509)
- [Generate reference doc automatically for capability based on CUE schema](https://github.com/oam-dev/kubevela/projects/1#card-48200195)
- [Developer-centric experience around Appfile](https://github.com/oam-dev/kubevela/projects/1#card-47565777)
- [Better application observability](https://github.com/oam-dev/kubevela/projects/1#card-47134946)

Tech Debts:
- [Contributing KEDA bug fixes to KEDA upstream](https://github.com/oam-dev/kubevela/projects/1#card-48199538)
- [Contributing the modularizing Flagger changes to Flagger upstream](https://github.com/oam-dev/kubevela/projects/1#card-48198830)