# v1.0.7

This is a minor fix for release-1.0, please refer to release-1.1.x for the latest feature.

1. Fix podDisruptive field for inner traits #1844


# v1.0.6

1. fix bug: When the Component contains multiple traits of the same type, the status of the trait in the Application is reported incorrectly (#1731) (#1743)
2. Fix terraform component can't work normally, generate OpenAPI JSON schema for Terraform Component (#1738) (#1753)
3. Improve the logging system #1735 #1758
4. add ConcurrentReconciles for setting the concurrent reconcile number of the controller  #1775 

# v1.0.5

1. Fix Terraform application status issue (#1611)
2. application supports specifying different versions of Definition (#1597)
3. Enable Dynamic Admission Control for Application (#1619)
4. Update inner samples for "vela show xxx --web" (#1616)
5. fix empty rolloutBatch will panic whole controller bug (#1646)
6. Use stricter syntax check for CUE (#1643)
7. make ResourceTracker to own cluster-scope resource (#1634)
8. update docs

# v1.0.4

## Upgrade to this release

**Please update Application CRD to upgrade from v1.0.3 to this release**

```
kubectl apply -f https://raw.githubusercontent.com/kubevela/kubevela/master/charts/vela-core/crds/core.oam.dev_applications.yaml
```

**Check the upgrade docs to upgrade from other release: https://kubevela.io/docs/advanced-install#upgrade**


## Changelog

1. add more PVC volume traits and docs (#1524)
2. automatically sync vela api code to the repo([kubevela-core-api](https://github.com/oam-dev/kubevela-core-api)) on release, you can use this repo as import package for kubevela integration (#1523)
3. fix cue template of worker and ingress with more accurate error info (#1532)
4. add critical path k8s event for Application (#1463)
5. support K8s Deployment for AppRollout #1539 #1557
6. vela cli: enable "vela show" to support namespaced capability (#1521)
7. Add scpoe reference in Application object `status.Service` (#1540)
8. vela cli: `vela show` support list the parameter of ComponentDefinition created by helm charts (#1543)
9. Add revision mechanism for Component/Trait Definition and default revision histroy will keep 20 revisions #1531
10. fix CRD for legacy K8s cluser(<=1.14) (#1531)
11. fix duplate key in kubevela chart webhook yaml (#1571)
12. Check whether parameter.cmd is nill for `sidecar` trait (#1575)
13. add e2e-test into test coverage report (#1553)
14. support krew install for kubectl vela plugin #1582
15. fix controller cannot start due to the format error of the third-party CRD (#1584)
16. use accelerate domain for helm chart repo to speed up for global users (#1585)
17.  embed rollout in an application, now you can use rolloutPlan in Application (#1568)
18. Support server-side Terraform as cloud resource provider #1519

# v1.0.3

More end user guide was added in `Application Deployment` section.

1. add helm test to verify the chart of KubeVela have been installed successfully  (#1415)
2. fix bug which Component/TraitDefinition won't work when contains “`_|_`” in value (#1450)
3. add volumes definition in worker/webservice (#1459)
4. Remove local kind binary dependency #1458
5. ignore error not found when deleting resourceTracker (#1462)
6. add context.appRevisionNum as runtime context (#1466)
7. implement cli `vela system live-diff` to check diff before upgrade (#1419)
8. add webhook validation on CUE template outputs name (#1460)
9. Fix helm chart about wrong webhook policy (#1483)
10. Remove trait-injector from controller options (#1490)
12. add app name as label for AppRevision (#1488)
13. Introduce vela as a kubectl plugin (#1485)
14. update status of appContext by patch to avoid resourceVersion conflict error (#1500)
15. add workloadDefinitionRef to application status.services (#1471)
16. Add garbage collection mechanism for AppRevision, it will only keep 10 revisions by default (#1501)
17. Remove AGE in definition crd print columns (#1509)


# v1.0.2

1. remove no used ingress notes in KubeVela charts (#1405)
2. fix import inner package in the format of third party package path and add docs (#1412  #1417)
3. vela cli support use "vela system cue-packages" to list cue-package (#1417)
4. Fix bug that the registered k8s built-in gvk does not exist in third party package path (#1414)
5. Fix bug that patchKey not work when strategyUnify function not work with close call (#1430)
6. add podDisruptive to traitdefinition to notify wether a trait update will cause restart of pod or not (#1192)
7. Add a new cloneset scale controller (#1301)
8. Support garbage collection for across-namespace workloads and traits  (#1421)
9. Add short name for crds && Remove redundant and ambiguous short names #1434
10. Refresh built-in packages when component/trait definition are registered (#1402)


**You should upgrade following CRDs to upgrade from v1.0.1, all CRDs changes are backward compatible**:

```
kubectl apply -f https://raw.githubusercontent.com/oam-dev/kubevela/master/charts/vela-core/crds/core.oam.dev_resourcetrackers.yaml
kubectl apply -f https://raw.githubusercontent.com/oam-dev/kubevela/master/charts/vela-core/crds/standard.oam.dev_rollouttraits.yaml
kubectl apply -f https://raw.githubusercontent.com/oam-dev/kubevela/master/charts/vela-core/crds/core.oam.dev_traitdefinitions.yaml
kubectl apply -f https://raw.githubusercontent.com/oam-dev/kubevela/master/charts/vela-core/crds/core.oam.dev_applications.yaml
kubectl apply -f https://raw.githubusercontent.com/oam-dev/kubevela/master/charts/vela-core/crds/core.oam.dev_approllouts.yaml
kubectl apply -f https://raw.githubusercontent.com/oam-dev/kubevela/master/charts/vela-core/crds/core.oam.dev_applicationrevisions.yaml
```


# v1.0.1

There are some fixes contained for release v1.0.0:

1. add initial finalizer and abandon support for app rollout, you can revert quickly now(#1362)
2. fix vela show fail to get component definition (#1366)
3. fix application context controller should not own application object (#1370)
4. fix: "system-definition-namespace" chart args not work in vela chart (#1371)
5. fix resources created in different namespace can not be updated (#1374)
6. fix automatically generate schema for helm values fail in array list value (#1375)
7. upgrade API version of mutate/validate webhook to v1 (#1383)
8. fix webhook not work by helm install kubevela without cert-manager #1267
7. remove create cert-manager issuer in vela CLI env command (#1267)
8. refine CRD print results: add additional print column and short Name for CRD (#1377)

Many other docs improvements.

Thanks for all the contributors!

# v1.0.0

We're excited to announce the release of KubeVela 1.0.0! 🎉🎉🎉🎉

Thanks to all the new and existing contributors who helped make this release happen!

You may already noticed the awesome community has shipped a brand new KubeVela website https://kubevela.io ! 🎉🎉

If you're new to KubeVela, feel free to start with its [getting started page](https://kubevela.io/docs/quick-start) and learning about [its core concepts](https://kubevela.io/docs/concepts). The full feature of vela is explained in [platform builder guide](https://kubevela.io/docs/platform-engineers/overview).

For existing adopters, please follow the [installing](https://kubevela.io/docs/install) or [upgrading](https://kubevela.io/docs/install#upgrade) KubeVela to version 1.0.0.

## Acknowledgements ❤️

Thanks to everyone who made this release possible!

@captainroy-hy @sunny0826 @leejanee  @yangsoon @wangyikewxgm @hongchaodeng @zzxwill @ryanzhang-oss @resouer @wonderflow  @hprotzek @vnzongzna  @majian159 @Cweiping @mengjiao-liu @kushthedude @unknwon @Ghostbaby @mosesyou @dylandee @wangkai1994 @LeoLiuYan @just-do1 @hoopoe61 @Incubator4th @TomorJM @hahchenchen @zeed-w-beez @allenhaozi @mason1kwok @kinsolee @shikanon @96RadhikaJadhav

# What's New

## API version upgraded to `v1beta1`

All user facing APIs have been upgraded to `v1beta1`, you could learn more details in the [API Changes](#API-Changes) section below.

## `ComponentDefinition`

The [`ComponentDefinition`](https://kubevela.io/docs/platform-engineers/definition-and-templates) now takes the responsibility of defining encapsulation and abstraction for your app components. And you are free to choose to use Helm chart or CUE to define them. This leaves `WorkloadDefinition` focusing on declaring workload characteristic such as `replicable`, `childResource`  etc, so the `spec.schematic` field in `WorkloadDefinition` could be deprecated in next few releases.

## Application Versioning and Progressive Rollout

* A rolling style upgrade was supported by the object called [`AppRollout`](https://kubevela.io/docs/rollout/rollout/). It can help you to upgrade an Application from source revision to the target and support Blue/Green, Canary and A/B testing rollout strategy.
* Multi-Version, Multi-Cluster Application Deployment was supported by the object called [`AppDeployment`](https://kubevela.io/docs/rollout/appdeploy). It can help you to deploy multiple revision apps to multiple clusters with leverage of Service Mesh.

## Visualization Enhancement

KubeVela now automatically generates Open-API-v3 Schema for all the definition abstractions including CUE, Helm and raw Kubernetes resource templates. You can integrate KubeVela with your own dashboard and [generate forms from definitions](https://kubevela.io/docs/platform-engineers/openapi-v3-json-schema) at ease!

## Application Abstraction

There're several major updates on the `Application` abstraction itself:
* [Helm based abstraction](https://kubevela.io/docs/helm/component) was supported with few [limitations](https://kubevela.io/docs/helm/known-issues). In other words, you can now declare any existing Helm chart as an app component in KubeVela. The most exciting part is the trait system of KubeVela works seamlessly with the Helm based components, yes, just [attach trait](https://kubevela.io/docs/helm/trait) to it!
* [Raw Kubernetes resource templates](https://kubevela.io/docs/kube/component) was still supported, that's simpler but less powerful comparing to [the CUE way](https://kubevela.io/docs/cue/component). Of course, the trait system also [works seamless](https://kubevela.io/docs/kube/trait) with it.


## CUE Template Enhancement

* [Runtime information context](https://kubevela.io/docs/cue/component#full-available-information-in-cue-context) was supported, you could use this information to render the resources in CUE template.
* [Data passing](https://kubevela.io/docs/cue/advanced#data-passing) was supported during CUE rendering. Specifically, the `context.output` contains the rendered workload API resource and the `context.outputs.<xx>` contains all the other rendered API resources.
* [K8s API resources are now built-in packages](https://kubevela.io/docs/cue/basic#import-kube-package): the K8s built-in API including CRD will be discovered by KubeVela and automatically built as CUE packages, you can use it in your CUE template. This is very helpful in validation especially on writing new CUE templates.
* [Dry-run Application](https://kubevela.io/docs/platform-engineers/debug-test-cue) was supported along with a debug and test guide for building CUE template. You can create CUE based definitions with confidence now!
* [Deploy resources in different namespaces](https://kubevela.io/docs/cue/cross-namespace-resource/) was supported now, you can specify namespace in your CUE template.

## Declare and Consume Cloud Resources

* [Declare and consume cloud resources](https://kubevela.io/docs/platform-engineers/cloud-services/) were supported now in KubeVela, you can easily register cloud resources by `ComponentDefinition` and bind the service into the applications.

## A brand new website

We have upgraded our website [kubevela.io](https://github.com/oam-dev/kubevela.io) based on "Docusaurus". All docs is automatically generated from [KubeVela](https://github.com/oam-dev/kubevela/tree/master/docs) while the blogs are on [kubevela.io/blogs](https://github.com/oam-dev/kubevela.io/tree/main/blog).


# Changes

## API Changes

1. Change definition from cluster scope to namespace scope #1085 the cluster scope CRD was still compatible.
2. Application Spec changes.
    - `spec.components[x].settings` in v1alpha2 was changed to `spec.components[x].properties` in v1beta1
    - `spec.components[x].traits[x].name` in v1alpha2 was changed to `spec.components[x].traits[x].type` in v1beta1

Example of the v1alpha2 Spec:

```
apiVersion: core.oam.dev/v1alpha2
kind: Application
metadata:
  name: first-vela-app
spec:
  components:
    - name: express-server
      type: webservice
      settings:
        ...
      traits:
        - name: ingress
          properties:
            ...
```

Example of the v1beta1 Spec:

```
apiVersion: core.oam.dev/v1beta1
kind: Application
metadata:
  name: first-vela-app
spec:
  components:
    - name: express-server
      type: webservice
      properties:
        ...
      traits:
        - type: ingress
          properties:
            ...
```


## Deprecation

1. route/autoscaler/metrics these three traits and their controllers were moved out from the vela core.  #1172 You could still find and use them from https://github.com/oam-dev/catalog.
2. the dashboard was deprecated in KubeVela and we will merge these features and create a new in [velacp](https://github.com/oam-dev/velacp) soon.
3. vela CLI will only support run/modify an app from appfile by using `vela up`, so some other commands related were deprecated, such as `vela svc deploy`, `vela <trait> ...`


## Other Notable changes

1. `prometheus` and `certmanager` CRD are not required in installation #1005
2. Parent overrides child when annotation/labels conflicts && one revision will apply once only in force mode && AC.status CRD updated #1109
3. `ApplicationRevision` CRD Object was introduced as revision of Application #1214
4. KubeVela chart image pull policy was changed to `Always` from `IfNotPresent` #1228
   5.` Application` Controller will use `AppContext` to manage the resources generation #1245, in other word, you can run KubeVela `Application Controller` without any `v1apha2 Object`.
6. The regular time for all events automatically sync changed from 5min to 1 hour   #1285
7. `vela system dry-run` will print raw K8s resources in a better format  #1246


# Known Issues

1. Built-in CUE package was not supported now for K8s Cluster v1.20, we will support in the next release. #1313
2. Resources created in different namespace from application will only be garbage collected (GC) when the application deleted, an update will not trigger GC for now, we will fix it in the next release. #1339


Thanks again to all the contributors!