# v1.2.2


## What's Changed
* Feat: add JFrog webhook trigger by @chwetion in https://github.com/oam-dev/kubevela/pull/3104
* Fix: trait/comp command output without a new line by @chivalryq in https://github.com/oam-dev/kubevela/pull/3112
* Feat: support wild match for env patch by @Somefive in https://github.com/oam-dev/kubevela/pull/3111
* Fix: fix revision will change when add new trait with skiprevisionaffect to application by @chwetion in https://github.com/oam-dev/kubevela/pull/3032
* Fix: add app samples for Terraform definition by @zzxwill in https://github.com/oam-dev/kubevela/pull/3118
* Feat: add port name in webservice by @FogDong in https://github.com/oam-dev/kubevela/pull/3110
* Fix: add imagePullSecrets for helm templates  to support private docker registry by @StevenLeiZhang in https://github.com/oam-dev/kubevela/pull/3122
* Fix: workflow skip executing all steps occasionally by @leejanee in https://github.com/oam-dev/kubevela/pull/3025
* Fix: support generate Terraform ComponentDefinition from local HCL file by @zzxwill in https://github.com/oam-dev/kubevela/pull/3132
* Fix: prioritize namespace flag for `vela up` by @devholic in https://github.com/oam-dev/kubevela/pull/3135
* Fix: handle workflow cache reconcile by @FogDong in https://github.com/oam-dev/kubevela/pull/3128
* Feat: extend gateway trait to set class in spec by @devholic in https://github.com/oam-dev/kubevela/pull/3138
* Fix: add providerRef in generated ComponentDefinition by @zzxwill in https://github.com/oam-dev/kubevela/pull/3142
* Fix: retrieve Terraform variables from variables.tf by @zzxwill in https://github.com/oam-dev/kubevela/pull/3149
* Feat: addon parameter support ui-shcema by @wangyikewxgm in https://github.com/oam-dev/kubevela/pull/3154
* Fix: vela addnon enable cannot support '=' by @wangyikewxgm in https://github.com/oam-dev/kubevela/pull/3156
* Fix: add context parameters into the error message by @zeed-w-beez in https://github.com/oam-dev/kubevela/pull/3145
* Feat: support vela show for workflow step definition by @FogDong in https://github.com/oam-dev/kubevela/pull/3140

## New Contributors
* @devholic made their first contribution in https://github.com/oam-dev/kubevela/pull/3135

**Full Changelog**: https://github.com/oam-dev/kubevela/compare/v1.2.1...v1.2.2

# v1.2.1

## What's Changed

* Fix: can't query data from the MongoDB by @barnettZQG in https://github.com/oam-dev/kubevela/pull/3095
* Fix: use personel token of vela-bot instead of github token for homebrew update by @wonderflow in https://github.com/oam-dev/kubevela/pull/3096
* Fix: acr image no version by @wangyikewxgm in https://github.com/oam-dev/kubevela/pull/3100
* Fix: support generate cloud resource docs in Chinese by @zzxwill in https://github.com/oam-dev/kubevela/pull/3079
* Fix: clear old data in mongodb unit test case by @barnettZQG in https://github.com/oam-dev/kubevela/pull/3103
* Feat: support external revision in patch component by @Somefive in https://github.com/oam-dev/kubevela/pull/3106
* Fix:  file under the path of github addon registry is not ignored by @StevenLeiZhang in https://github.com/oam-dev/kubevela/pull/3099
* Fix: Vela is crashed, when disabling addon, which needs namespace vela-system by @StevenLeiZhang in https://github.com/oam-dev/kubevela/pull/3109
* Fix: rollout workload namespace not aligned with rollout by @Somefive in https://github.com/oam-dev/kubevela/pull/3107

## New Contributors
* @StevenLeiZhang made their first contribution in https://github.com/oam-dev/kubevela/pull/3099

**Full Changelog**: https://github.com/oam-dev/kubevela/compare/v1.2.0...v1.2.1


# v1.2.0

❤️  KubeVela v1.2.0 released ! ❤️

Docs have been updated about the release at https://kubevela.io/docs/next/ .

**Changelog Between v1.2.0-rc.2~v1.2.0**: https://github.com/oam-dev/kubevela/compare/v1.2.0-rc.2...v1.2.0

## What's New

### UI Console Supported

Check how to use the GUI by this [how-to document](https://kubevela.io/docs/next/how-to/dashboard/application/create-application).

**GUI frontend code repo is here: https://github.com/oam-dev/velaux**
**API Server Code is here: https://github.com/oam-dev/kubevela/tree/master/pkg/apiserver**

We also add a [VelaQL](https://kubevela.io/docs/next/platform-engineers/system-operation/velaql) feature that could allow apiserver to interact with K8s Object in an extended way.

### Addon System

We add a new addon system in v1.2, this helps KubeVela install it's extension including more than X-Definition files.

The community has already supported some built-in addons here（https://github.com/oam-dev/catalog/tree/master/addons ）, there're also some experimental addons here （https://github.com/oam-dev/catalog/tree/master/experimental/addons）。

You can learn how to use it [from docs](https://kubevela.io/docs/next/how-to/cli/addon/addon).

You can [build and contribute](https://kubevela.io/docs/next/platform-engineers/addon/intro) your own addons.

### CI/CD Integration

You can use triggers to integrate with different CI and image registry systems on VelaUX.

* Feat: add ACR webhook trigger for CI/CD (#3044)
* Feat: add Harbor image registry webhook trigger for CI/CD (#3065)
* Feat: add DockerHub webhook trigger for CI/CD (#3081)

### Cloud Resources Enhancement

* Feature: support terraform/provider-azure addon by @zzxwill in https://github.com/oam-dev/kubevela/pull/2402
* Fix: aws/azure Terraform provider are broken by @zzxwill in https://github.com/oam-dev/kubevela/pull/2513 ,  https://github.com/oam-dev/kubevela/pull/2520 , https://github.com/oam-dev/kubevela/pull/2465
* Feat: Add Terraform Azure Storage Account by @maciejgwizdala in https://github.com/oam-dev/kubevela/pull/2646
* Docs: add vpc and vswitch cloud resource templates of alicloud by @lowkeyrd in https://github.com/oam-dev/kubevela/pull/2663
* Fix: allow external cloud resources to be kept when Application is deleted by @zzxwill in https://github.com/oam-dev/kubevela/pull/2698
* Feat: add alibaba cloud redis definition by @chivalryq in https://github.com/oam-dev/kubevela/pull/2507
* Feat: envbinding support cloud resource deploy and share by @Somefive in https://github.com/oam-dev/kubevela/pull/2734
* Fix: support naming a terraform provider by @zzxwill in https://github.com/oam-dev/kubevela/pull/2794


### Multi-Cluster Enhancement

* Feat: multicluster support ServiceAccountToken by @Somefive in https://github.com/oam-dev/kubevela/pull/2356
* Feat: add secure tls for cluster-gateway by @Somefive in https://github.com/oam-dev/kubevela/pull/2426
* Feat: add support for envbinding with namespace selector by @Somefive in https://github.com/oam-dev/kubevela/pull/2432
* Feat: upgrade cluster gateway to support remote debug by @Somefive in https://github.com/oam-dev/kubevela/pull/2673
* Feat: set multicluster enabled by default  by @Somefive in #2930

### Workflow Enhancement

* Feat: add apply raw built in workflow steps by @FogDong in https://github.com/oam-dev/kubevela/pull/2420
* Feat: add read object step def by @FogDong in https://github.com/oam-dev/kubevela/pull/2480
* Feat: add export config and secret def for workflow by @FogDong in https://github.com/oam-dev/kubevela/pull/2484
* Feat: support secret in webhook notification by @FogDong in https://github.com/oam-dev/kubevela/pull/2509
* Feat: Record workflow execution state by @leejanee in https://github.com/oam-dev/kubevela/pull/2479
* Fix(cli): use flag instead of env in workflow cli by @FogDong in https://github.com/oam-dev/kubevela/pull/2512
* Not update resource if render hash equal. by @leejanee in https://github.com/oam-dev/kubevela/pull/2522
* Feat: add email support in webhook notification by @FogDong in https://github.com/oam-dev/kubevela/pull/2535
* Feat: add render component and apply component remaining by @FogDong in https://github.com/oam-dev/kubevela/pull/2587
* Feat: add list application records api by @FogDong in https://github.com/oam-dev/kubevela/pull/2757
* Feat: component-pod-view support filter resource by cluster name and cluster namespace by @yangsoon in https://github.com/oam-dev/kubevela/pull/2754
* Feat: workflow support update by @barnettZQG in https://github.com/oam-dev/kubevela/pull/2760
* Feat: add workflow reconciling backoff time and failed limit times by @FogDong in #2881

### Component/Trait Enhancement

* Feat: add health check and custom status for helm type component  by @chivalryq in https://github.com/oam-dev/kubevela/pull/2499
* Feat: add nocalhost dev config trait definition by @yuyicai in https://github.com/oam-dev/kubevela/pull/2545
* Feat(rollout): fill rolloutBatches if empty when scale up/down by @wangyikewxgm in https://github.com/oam-dev/kubevela/pull/2569
* Feat: allow import package in custom status cue template by @Chwetion in https://github.com/oam-dev/kubevela/pull/2585
* Feat: add trait service-account by @yangsoon in https://github.com/oam-dev/kubevela/pull/2878
* Fix: add ingress class as arguments by @Somefive in https://github.com/oam-dev/kubevela/pull/2445
* Fix: add libgit2 support for gitops by @FogDong in https://github.com/oam-dev/kubevela/pull/2477
* Fix: make nginx class to be default value and allow pvc trait to attach more than once by @wonderflow in https://github.com/oam-dev/kubevela/pull/2466
* Feat: add imagePullPolicy/imagePullSecret to task def by @chivalryq in https://github.com/oam-dev/kubevela/pull/2503


### Vela CLI Enhancement

* Feat: add vela prob to test cluster by @wonderflow in https://github.com/oam-dev/kubevela/pull/2635
* Feat: vela logs support multicluster by @chivalryq in https://github.com/oam-dev/kubevela/pull/2593
* Feat: vela-cli support use ocm to join/list/detach cluster by @yangsoon in https://github.com/oam-dev/kubevela/pull/2599
* Feat: add vela exec for multi cluster by @wonderflow in https://github.com/oam-dev/kubevela/pull/2299
* Feat: multicluster vela status/exec/port-forward by @Somefive in https://github.com/oam-dev/kubevela/pull/2662
* Fix: support `-n` flag for all commands to specify namespace by @chivalryq in https://github.com/oam-dev/kubevela/pull/2719
* Feat: vela delete add wait and force options by @yangsoon in https://github.com/oam-dev/kubevela/pull/2747
* Feat: add workflow rollback cli by @FogDong in https://github.com/oam-dev/kubevela/pull/2795
* Refactor: refine cli commands && align kubectl-vela with vela && use getnamespaceAndEnv for all by @wonderflow in #3048


### Overall Enhancements

* Feat: ResourceTracker new architecture by @Somefive in https://github.com/oam-dev/kubevela/pull/2849
* New Resource Management Model: Garbage Collection and Resource State Keeper [Desigin Doc](https://github.com/oam-dev/kubevela/blob/master/design/vela-core/resourcetracker_design.md)
* Feat:  output log with structured tag & add step duration metrics by @leejanee in https://github.com/oam-dev/kubevela/pull/2683
* Feat: support user defined image registry that allows private installation by @wonderflow in https://github.com/oam-dev/kubevela/pull/2623
* Feat: add a built in garbage-collect policy to application by @yangsoon in https://github.com/oam-dev/kubevela/pull/2575
* Feat: health scope controller support check trait-managing workload by @wangyikewxgm in https://github.com/oam-dev/kubevela/pull/2527
* Chore:  add homebrew bump to support `brew install kubevela` by @basefas in https://github.com/oam-dev/kubevela/pull/2434
* Chore: Update release action to support build binaries for more platform by @basefas in https://github.com/oam-dev/kubevela/pull/2537
* Feat: add reconcile timeout configuration for vela-core by @Somefive in https://github.com/oam-dev/kubevela/pull/2630
* Chore: push docker images to Alibaba Cloud ACR by @zzxwill in https://github.com/oam-dev/kubevela/pull/2601


## What's Changed/Deprecated

* Deprecated: containerized workload by @reetasingh in https://github.com/oam-dev/kubevela/pull/2330
* Deprecated: initializer CRD controller by @chivalryq in https://github.com/oam-dev/kubevela/pull/2491
* Deprecated: remove envbinding controller, use #ApplyComponent for EnvBinding by @Somefive in https://github.com/oam-dev/kubevela/pull/2556 , https://github.com/oam-dev/kubevela/pull/2382
* Deprecated(cli): CLI vela config by @chivalryq in https://github.com/oam-dev/kubevela/pull/2037
* Deprecated: remove addon with no definitions by @chivalryq in https://github.com/oam-dev/kubevela/pull/2574
* Refactor: remove apiserver component from the chart, users should use velaux addon instead by @barnettZQG in https://github.com/oam-dev/kubevela/pull/2838
* Refactor: all addons are migrated from initializer to application objects by @chivalryq in https://github.com/oam-dev/kubevela/pull/2444
* Refactor: change rollout's json tag so the status of rollout will be optional by @GingoBang in https://github.com/oam-dev/kubevela/pull/2314
* Deprecated: deprecate CRD discovery for CUE import in Definition to prevent memory leak and OOM crash (#2925)
* Deprecated: delete approllout related code #3040
* Deprecate: delete appDeployment related logic #3050

## Bugfix

* Feat: rework resource tracker to solve bugs by @Somefive in https://github.com/oam-dev/kubevela/pull/2797
* Fix: change raw extension to pointer by @FogDong in https://github.com/oam-dev/kubevela/pull/2451
* Fix: show reconcile error log by @wonderflow in https://github.com/oam-dev/kubevela/pull/2626
* Fix: Closure Bug In newValue by @leejanee in https://github.com/oam-dev/kubevela/pull/2437
* Fix: resourceTracker compatibility bug by @wangyikewxgm in https://github.com/oam-dev/kubevela/pull/2467
* Fix(application): nil pointer for component properties by @kinsolee in https://github.com/oam-dev/kubevela/pull/2481
* Fix: Commit step-generate data  without success by @leejanee in https://github.com/oam-dev/kubevela/pull/2539
* Fix(cli): client-side throttling in vela CLI by @chivalryq in https://github.com/oam-dev/kubevela/pull/2581
* Fix: fix delete a component from application not delete workload by @wangyikewxgm in https://github.com/oam-dev/kubevela/pull/2680
* Fix: lookupByScript don't support `import` by @leejanee in https://github.com/oam-dev/kubevela/pull/2788
* Fix: resourcetracker do not garbage collect legacyRTs correctly by @Somefive in https://github.com/oam-dev/kubevela/pull/2817
* Fix: application conditions confusion. by @leejanee in https://github.com/oam-dev/kubevela/pull/2834

## New Contributors
* @GingoBang made their first contribution in https://github.com/oam-dev/kubevela/pull/2314
* @basefas made their first contribution in https://github.com/oam-dev/kubevela/pull/2434
* @yuyicai made their first contribution in https://github.com/oam-dev/kubevela/pull/2540
* @maciejgwizdala made their first contribution in https://github.com/oam-dev/kubevela/pull/2646
* @lowkeyrd made their first contribution in https://github.com/oam-dev/kubevela/pull/2663
* @zxbyoyoyo made their first contribution in https://github.com/oam-dev/kubevela/pull/2703
* @yue9944882 made their first contribution in https://github.com/oam-dev/kubevela/pull/2751
* @snyk-bot made their first contribution in https://github.com/oam-dev/kubevela/pull/2857
* @yunjianzhong made their first contribution in https://github.com/oam-dev/kubevela/pull/3005
* @songminglong made their first contribution in https://github.com/oam-dev/kubevela/pull/3064
* @basuotian made their first contribution in https://github.com/oam-dev/kubevela/pull/3059
* @K1ngram4 made their first contribution in https://github.com/oam-dev/kubevela/pull/3065

**Full Changelog**: https://github.com/oam-dev/kubevela/compare/v1.1.3...v1.2.0