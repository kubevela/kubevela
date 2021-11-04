# KubeVela Controller Parameters Reference

|          parameter          |  type  |              default              |                           describe                           |
| :-------------------------: | :----: | :-------------------------------: | :----------------------------------------------------------: |
|         use-webhook         |  bool  |               false               |                   Enable Admission Webhook                   |
|      webhook-cert-dir       | string | /k8s-webhook-server/serving-certs |               Admission webhook cert/key dir.                |
|        webhook-port         |  int   |               9443                |               Admission webhook listen address               |
|        metrics-addr         | string |               :8080               |          The address the metric endpoint binds to.           |
|   enable-leader-election    |  bool  |               false               | Enable leader election for controller manager. Enabling this will ensure there is only one active controller manager. |
|  leader-election-namespace  | string |                ""                 | Determines the namespace in which the leader election configmap will be created. |
|        log-file-path        | string |                ""                 |                  The file to write logs to.                  |
|      log-file-max-size      |  int   |               1024                | Defines the maximum size a log file can grow to, Unit is megabytes. |
|          log-debug          |  bool  |               false               |          Enable debug logs for development purpose           |
|       revision-limit        |  int   |                50                 | revision-limit is the maximum number of revisions that will be maintained. The default value is 50. |
| application-revision-limit  |  int   |                10                 | application-revision-limit is the maximum number of application useless revisions that will be maintained, if the useless revisions exceed this number, older ones will be GCed first.The default value is 10. |
|  definition-revision-limit  |  int   |                20                 | definition-revision-limit is the maximum number of component/trait definition useless revisions that will be maintained, if the useless revisions exceed this number, older ones will be GCed first.The default value is 20. |
|  custom-revision-hook-url   | string |                ""                 | custom-revision-hook-url is a webhook url which will let KubeVela core to call with applicationConfiguration and component info and return a customized component revision |
|    app-config-installed     |  bool  |               true                | app-config-installed indicates if applicationConfiguration CRD is installed |
| autogen-workload-definition |  bool  |               true                | Automatic generated workloadDefinition which componentDefinition refers to |
|         health-addr         | string |               :9440               |          The address the health endpoint binds to.           |
|       apply-once-only       | string |               false               | For the purpose of some production environment that workload or trait should not be affected if no spec change, available options: on, off, force. |
|        disable-caps         | string |                ""                 |           To be disabled builtin capability list.            |
|       storage-driver        | string |               Local               |         Application file save to the storage driver          |
|  informer-re-sync-interval  |  time  |                1h                 | Controller shared informer lister full re-sync period, the interval between two routinely reconciles for one CR (like Application) if no changes made to it. |
|      reconcile-timeout      |  time  |                3m                 |           The timeout for controller reconcile.              |
| system-definition-namespace | string |            vela-system            |     define the namespace of the system-level definition      |
|    concurrent-reconciles    |  int   |                 4                 | The concurrent reconcile number of the controller. You can increase the degree of concurrency if a large number of CPU cores are provided to the controller. |
|        kube-api-qps         |  int   |                50                 | The qps for reconcile k8s clients. Increase it if you have high concurrency. A small number might restrict the requests to the api-server which may cause a long waiting queue when there are a large number of inflight requests. Try to avoid setting it too high since it will cause large burden on apiserver. |
|       kube-api-burst        |  int   |                100                | The burst for reconcile k8s clients. The usage of this parameter is similar to kube-api-qps. Setting it to be larger than kube-api-qps allows accepting more requests temporarily. |
|      depend-check-wait      |  time  |                30s                | depend-check-wait is the time to wait for ApplicationConfiguration's dependent-resource ready. |
|        oam-spec-var         | string |               v0.3                |         the oam spec version controller want to set-up       |
|         pprof-addr          | string |                ""                 | The address for pprof to use while profiling, empty means disable. |
|        perf-enabled         |  bool  |               false               | Enable performance logging for controllers, disabled by default. |

### Recommended Parameters for Scenarios with Various Scale

| Scale |  #Nodes  | #Applications |   #Pods  | concurrent-reconciles | kube-api-qps | kube-api-burst |  CPU  | Memory |
| :---: | -------: | ------------: | -------: | --------------------: | :----------: | -------------: | ----: | -----: |
| Small |  < 200   |   < 3,000     | < 18,000 |              2        |      300     |      500       |   0.5 |   1Gi  |
| Medium | < 500   |   < 5,000     | < 30,000 |              4        |      500     |      800       |     1 |   2Gi  |
| Large | < 1,000  |   < 12,000    | < 72,000 |              4        |      800     |     1,000      |     2 |   4Gi  |

> For details, read KubeVela Performance Test Report
