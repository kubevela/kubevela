# KubeVela Controller Parameters Reference

|          parameter          |  type  |              default              |                           describe                           |
| :-------------------------: | :----: | :-------------------------------: | :----------------------------------------------------------: |
|         use-webhook         |  bool  |               false               |                   Enable Admission Webhook                   |
|     use-trait-injector      |  bool  |               false               |                     Enable TraitInjector                     |
|      webhook-cert-dir       | string | /k8s-webhook-server/serving-certs |               Admission webhook cert/key dir.                |
|        metrics-addr         | string |               :8080               |          The address the metric endpoint binds to.           |
|   enable-leader-election    |  bool  |               false               | Enable leader election for controller manager. Enabling this will ensure there is only one active controller manager. |
|  leader-election-namespace  | string |                ""                 | Determines the namespace in which the leader election configmap will be created. |
|        log-file-path        | string |                ""                 |                  The file to write logs to.                  |
|       log-retain-date       |  int   |                 7                 |        The number of days of logs history to retain.         |
|        log-compress         |  bool  |               true                |           Enable compression on the rotated logs.            |
|       revision-limit        |  int   |                50                 | revision-limit is the maximum number of revisions that will be maintained. The default value is 50. |
|         health-addr         | string |               :9440               |          The address the health endpoint binds to.           |
|       apply-once-only       | string |               false               | For the purpose of some production environment that workload or trait should not be affected if no spec change, available options: on, off, force. |
|  custom-revision-hook-url   | string |                ""                 | custom-revision-hook-url is a webhook url which will let KubeVela core to call with applicationConfiguration and component info and return a customized component revision |
|        disable-caps         | string |                ""                 |           To be disabled builtin capability list.            |
|       storage-driver        | string |               Local               |         Application file save to the storage driver          |
|  informer-re-sync-interval  |  time  |                2h                 |    controller shared informer lister full re-sync period     |
| system-definition-namespace | string |            vela-system            |     define the namespace of the system-level definition      |
|          long-wait          |  time  |                1m                 | long-wait is controller next reconcile interval time like 30s, 2m etc. The default value is 1m, you can set it to 0 for no reconcile routine after success |
|    concurrent-reconciles    |  int   |                 4                 | concurrent-reconciles is the concurrent reconcile number of the controller. |
|      depend-check-wait      |  time  |                30s                | depend-check-wait is the time to wait for ApplicationConfiguration's dependent-resource ready. |
