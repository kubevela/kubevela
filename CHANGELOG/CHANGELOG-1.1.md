# v1.1.3

## What's Changed
* Fix: remove ocm addon enable in Makefile by @Somefive in https://github.com/oam-dev/kubevela/pull/2327
* Chore(cli): remove useless deploy.yaml by @chivalryq in https://github.com/oam-dev/kubevela/pull/2335
* Fix: do not override the workload name if its specified by @FogDong in https://github.com/oam-dev/kubevela/pull/2336
* Fix: remove appcontext CRD and controller by @wonderflow in https://github.com/oam-dev/kubevela/pull/2270
* Feat: add revisionHistoryLimit to helm chart by @haugom in https://github.com/oam-dev/kubevela/pull/2343
* Chore: deprecate 'vela dashboard' apiserver by @chivalryq in https://github.com/oam-dev/kubevela/pull/2341
* Chore: remove e2e-api-test in rollout test to speed up by @chivalryq in https://github.com/oam-dev/kubevela/pull/2345
* Docs: rollout demo by @wangyikewxgm in https://github.com/oam-dev/kubevela/pull/2348
* Feat: add vela minimal chart by @FogDong in https://github.com/oam-dev/kubevela/pull/2340
* Fix: runc security issue by @Somefive in https://github.com/oam-dev/kubevela/pull/2350
* Docs: add a WeChat QR code by @barnettZQG in https://github.com/oam-dev/kubevela/pull/2351
* Feat: Initialize api for vela dashboard and CLI by @barnettZQG in https://github.com/oam-dev/kubevela/pull/2339
* Fix(helm chart): fix startup args for apiserver by @yangsoon in https://github.com/oam-dev/kubevela/pull/2362
* Fix: dockerfile e2e test command lack environment configuration by @Somefive in https://github.com/oam-dev/kubevela/pull/2231
* Feat: inputs support setting value in array by @leejanee in https://github.com/oam-dev/kubevela/pull/2358
* Feat: support rollout controller for StatefulSet by @whichxjy in https://github.com/oam-dev/kubevela/pull/1969
* Fix: delete deprecated vela dashboard in e2e setup by @FogDong in https://github.com/oam-dev/kubevela/pull/2379
* Fix: try fix CI unit test by @wonderflow in https://github.com/oam-dev/kubevela/pull/2376
* Feat: Addon REST API by @hongchaodeng in https://github.com/oam-dev/kubevela/pull/2369
* Fix: fix built in workflow steps by @FogDong in https://github.com/oam-dev/kubevela/pull/2378
* Feat: add vela minimal in make manifests by @FogDong in https://github.com/oam-dev/kubevela/pull/2389
* Chore(deps): bump go.mongodb.org/mongo-driver from 1.3.2 to 1.5.1 by @wonderflow in https://github.com/oam-dev/kubevela/pull/2391
* Fix: use aliyun oss istio chart by @wangyikewxgm in https://github.com/oam-dev/kubevela/pull/2392
* Support remote git repo for Terraform configuration by @zzxwill in https://github.com/oam-dev/kubevela/pull/2337
* Feat: add inputs test cases and optimize code by @leejanee in https://github.com/oam-dev/kubevela/pull/2388
* Fix: pass owner to workload if rollout failed by @wangyikewxgm in https://github.com/oam-dev/kubevela/pull/2397
* Feat: bootstrap multicluster testing by @Somefive in https://github.com/oam-dev/kubevela/pull/2368
* Feat(workflow): add depends on in workflow by @FogDong in https://github.com/oam-dev/kubevela/pull/2387
* Fix: make the name of Terraform credential secret same to component name by @zzxwill in https://github.com/oam-dev/kubevela/pull/2399
* Fix: revision GC in workflow mode by @FogDong in https://github.com/oam-dev/kubevela/pull/2355
* Fix: Applied Resources Statistics  Error by @leejanee in https://github.com/oam-dev/kubevela/pull/2398


# v1.1.2

This is a bug fix release.
Since the big v1.1.1 release, many users had given it try for our new features. We sincerely appreciate your enthusiasm and amazing feedback.

There are some small issues found by our users and we have fixed them. Most notably:

- The Charts of addons (prometheus, etc.) are moved to OSS to provide better accessibility and network speed.
- The FluxCD and Terraform addons are not enabled by default. Users can install them via `vela addon enable ...`.

We have located more small issues around templates as well and fixed them, and decided a bug fix release ASAP.

Users are highly recommended to use the v1.1.2 release instead. We want to thank all of our users sincerely! ❤️ ❤️ ❤️

## What's Changed
* Fix(rollout): improve rollback experience by @hongchaodeng in https://github.com/oam-dev/kubevela/pull/2294
* Fix: fix typo by @hughxia in https://github.com/oam-dev/kubevela/pull/2317
* Fix: fix cluster-gateway image tag in chart by @Somefive in https://github.com/oam-dev/kubevela/pull/2318
* Fix: workflow example by @leejanee in https://github.com/oam-dev/kubevela/pull/2323
* Fix: fix multicluster values bug by @Somefive in https://github.com/oam-dev/kubevela/pull/2326
* Fix(helm): Do not install fluxcd and terraform by default by @yangsoon in https://github.com/oam-dev/kubevela/pull/2328
* Fix: move charts from github repo to Alibaba Cloud OSS repo by @zzxwill in https://github.com/oam-dev/kubevela/pull/2324
* Fix: add comments and adjust helm typed component's spec by @zzxwill in https://github.com/oam-dev/kubevela/pull/2332
* Fix: fix multicluster template bug by @Somefive in https://github.com/oam-dev/kubevela/pull/2333
* Feat: add args for init-contianer and sidecar by @Gallardot in https://github.com/oam-dev/kubevela/pull/2331

# v1.1.1

Users are highly recommended to use the v1.1.2 release instead.

# Changes since v1.1.0

1. rollout trait change IncreaseFirst to DecreaseFirst (#2142)
2. Feat(definition): add built-in dingtalk workflow step definition (#2152)
3. Fix(dryrun): add default name and namespace in dry run (#2150)
4. Docs: fix typo about workflow rollout (#2163)
5. Fix: traitdefinition controller reconcile in a infinite loop (#2157)
6. Refactor: change the ownerReference of configMap which store the parameter for each revision to definitionRevision (#2164)
7. Fix: add fluxcd dashbaords (#2130)
8. Feat: modify apply component cue action to support skipWorkload trait (#2167)
9.  Trait: Add TraitDefinition for PVC (#2158)
10. initilize KubeVela codeowner file (#2178)
11. Feat(cue): support access components artifacts in cue template context (#2161)
12. Feat(addon): add default enable addon (#2172)
13. Feat(envbinding): add resourceTracker for envBinding (#2179)
14. Fix: align all CUE template keyword to use parameter (#2181)
15. Feat: add vela live-diff , dry-run, cue-packages into vela commands (#2182)
16. Fix: move Terraform defintions charts/vela-core/templates/definitions (#2176)
17. Fix: add patchkey to volumes (#2191)
18. Feat(workflow): add depends-on workflow step definition (#2190)
19. Feat: add pprof (#2192)
20. Feat: add more registry traits as internal ones (#2184)
21. Fix: support more Terraform variable types (#2194)
22. Fix: update help message of ingress trait (#2198)
23. Refactor(#2185): remove unused config options in Makefile (#2200)
24. Docs: update environment design (#2199)
25. Fix: modify service-binding with more accurate type (#2209)
26. Feat(healthscope): add health-scope-binding policy and e2e test for health scope (#2205)
27. Feat(workflow): support dingding and slack in webhook notification (#2213)
28. Feat(workflow): add apply application workflow step definition (#2186)
29. Feat(workflow): input.ParameterKey described like paths notation (#2214)
30. Fix(upgrade): upgrade controller-tools from 0.2 to 0.6.2 (#2215)
31. Fix(app): When only the policy is specified, the resources in the app need to be rendered and created (#2197)
32. Feat(workflow): outputs support script notation (#2218)
33. Fix(addon): rename clonset-service to clonse (#2219)
34. Feat(workflow): Add op.#Task action (#2220)
35. Fix(webhook): only check the uniqueness of component names under the same namespace (#2222)
36. Feat(apiserver): add apiserver service to helm chart (#2225)
37. Fix: add flag --label to filer components and traits (#2217)
38. Fix(addons): remove kruise addon (#2226)
39. Feat: add pressure-test parameter optimize (#2230)
40. Fix: align the envbind-app name with the original application name (#2232)
41. Feat(workflow): add status check for workflow mutil-env deploy (#2229)
42. Refactor: move from io/ioutil to io and os package (#2234)
43. Feat(trait): annotation and labels trait should also affect the workload object along with pod (#2237)
44. Feat(app): show health status from HealthScope in application (#2228)
45. Fix: kustomize json patch trait definition (#2239)
46. Docs: canary rollout demo (rollout part only) (#2177)
47. Feat: vela show annotations display undefined should be refined (#2244)
48. Feat: support code-generator and sync to kubevela-core-api (#2174)
49. Feat: add image auto update for gitops (#2251)
50. Fix: fix the output DB_PASSWORD for rds definition (#2267)
51. Fix: add alibaba eip cloud resource (#2268)
52. Refactor application code to make it run as Dag workflow (#2236)
53. Fix: remove podspecworkload controller and CRD (#2269)
54. Feat: add more options for leader election configuration to avoid pressure on apiserver
55. Feat: istio addon and use case demo (#2276)
56. Fix: patch any key using retainKeys strategy (#2280)
57. Fix: add exponential backoff wait time for workflow reconciling (#2279)
58. Refactor: change field exportKey to valueFrom (#2284)
59. Fix(helm): enable apiserver by default (#2249)
60. Feat: alibaba provider addon (#2243)
61. Support MultiCluster EnvBinding with cluster-gateway (#2247)
62. Fix: fix apply application workflow step (#2288)
63. Fix: fix alibaba cloud rds module (#2293)
64. Feat: add commit msg in kustomize (#2296)
65. Feat: allow user specify alibaba provider addon's region (#2297)
66. Fix: generate service in canary-traffic trait (#2300)
67. Fix: imagePullSecrets error from cloneset (#2305)
68. Fix: add application logging dashboard (#2301)
69. Feat: Make applicationComponent can be modified in step (#2304)
70. Fix: generate service in canary-traffic trait (#2307)


# v1.1.0

Note: the documents (https://kubevela.io/)  for v1.1.0 is still WIP, so we mark it as pre-release. The ETA for documents is next 2 weeks.

We would like to extend our thanks to all [the new and existing contributors](https://github.com/oam-dev/kubevela/graphs/contributors) who helped make this release happen.

Please follow the guide to [install](https://kubevela.io/docs/next/getting-started/quick-install) or [upgrade](https://kubevela.io/docs/next/platform-engineers/advanced-install/) KubeVela to version v1.1.0.

## What's New

- **Hybrid Environment App Delivery Control plane**
    - In the new release, we have fully upgraded KubeVela to a multi-cluster/hybrid-cloud/multi-cloud app delivery control plane with leverage of OAM as the consistent app delivery model across clouds and infrastructures.
- **Workflow**
    - KubeVela has added a Workflow mechanism that empowers users to glue any operational tasks to customize the control logic to build more complex operations. Workflow is modular by design and each module is mainly composed in CUE -- so you can define complex operations in a declarative, data-driven manner.
- **Environment**
    - KubeVela added an Initializer which allow users to define what constructs the environment. The environment Initialized by the Initilizer could contain different kinds of resources include K8s cluster, system components, policies and almost everything. Of course, you can destry an environment very easily with the help of Initializer.
- **Out of Box Addons**
    - With the help of Initilizer, KubeVela has support lots of out of box addons. You can list/enable/disable them by `vela addon` command. Each addon is an Initializer that deloy the CRD Controllers and other resources related.
- **Cloud Resources Support**
    - We also support terraform to provision almost every cloud resources and pass through to other components defined in KubeVela application.
- **Tools to edit and manage X-Definition**
    - We also provide the `vela def` tool sets to provide unified CUE based capability to manage X-Definition.
- **Others**
    - Allow specify name for component revision auto-generated by Application. Allow specify name for auto generated Definition revision.
    - Controller runtime dependency upgrade that can compatible with Kubernetes v1.21 . KubeVela support Kubernetes v1.18~v1.21.
    - Other details you could read changelog in the release history.
    
## Change log since v1.1-rc2

1. fix configmap patchkey bug (#2080)
2. Merge velacp to apiserver branch in oam repo (#2039) (#2127)  (#2087)
3. support rollout controller seprated and install as helm chart in runtime cluster  (#2075)
4. fix bug that KubeVela can not be installed in specified namespace (#2083)
5. enable vela def to use import decl (#2084)
6. enhance envbinding: support apply resources to cluster (#2093)
7. Add obsevability addon (#2091)
8. Feat(vela): add vela workflow suspend command (#2108)
9. feat(def): add built-in workflow definitions (#2094)
10. Feat(vela): add vela workflow resume command (#2114)
11. upgrade openkruise version to v0.9.0 (#2076)
12. Fix(workflow): set workload name in configmap if the name is not specified (#2119)
13. helm component support OSS bucket (#2104)
14. add rollout demo with Workflow (#2121)
15. Support script as parameter and make the WorkflowStepDefinition more universal  (#2124)
16. fix(cli) fix bug when vela show componetdefinition's workload type is AutoDetectWorkloadDefinition (#2125)
17. Fix(workflow): set the namespace to app's ns if it's not specified (#2133)
18. fix specify external revision bug (#2126)
19. add CUE-based health check in HealthScope controller (#1956)
20. Feat(addon): Add source and patch to kustomize definition (#2138)
21. Feat(vela): add vela workflow terminate and restart command (#2131)


# v1.1.0-rc.2

1. Allow users to specify component revision name in Application (#1929) the new field `externalRevision` can specify the revision name.

```
kind: Application
spec:
  components:
  - name: mycomp
    type: webservice
    externalRevision: my-revision-v1
    properties:
      ...
```

2. Add more workflow demo and fix some demos #2042  #2059 #2060 #2064
3. Add cloneset ComponentDefinition into kruise addon (#2050)
4. definitions support specify the revision name (#2044), you can specify the name by adding an annotation `definitionrevision.oam.dev/name`

```
apiVersion: core.oam.dev/v1beta1
kind: ComponentDefinition
metadata:
  name: worker
  annotations:
    # you can specify the revision name in annotations
    definitionrevision.oam.dev/name: "1.1.3"
spec:
  ...
```

5. fix definition controller log error cause by openapi schema generation error (#2063)
6. Add add-on input go-template implementation (#2049)

# v1.1.0-rc.1


1. Workflow support specify Order Steps by Field Tag (#2022)
2. support application policy (#2011)
3. add OCM multi cluster demo (#1992)
4. Fix(volume): seperate volume to trait (#2027)
5. allow application skip gc resource and leave workload ownerReference controlled by rollout(#2024)
6. Store component parameters in context (#2030)
7. Allow specify chart values for helm trait(#2033)
8. workflow support http provider (#2029)
9.  Use vela def commands to replace mergedef.sh for internal definition generation (#2031)


# Other release histories

Refer to https://github.com/oam-dev/kubevela/releases