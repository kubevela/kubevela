## Conflicts With

### `Autoscale`

When `Rollout` and `Autoscle` traits are attached to the same service, they two will fight over the number of instances during rollout. Thus, it's by design that `Rollout` will take over replicas control (specified by `.replicas` field) during rollout.

> Note: in up coming releases, KubeVela will introduce a separate section in Appfile to define release phase configurations such as `Rollout`.

## How `Rollout` works?

`Rollout` trait implements progressive release process to rollout your app following [Canary strategy](https://martinfowler.com/bliki/CanaryRelease.html).

In detail, `Rollout` controller will create a canary of your app , and then gradually shift traffic to the canary while measuring key performance indicators like HTTP requests success rate at the same time. 


![alt](../../../../../docs/en/resources/traffic-shifting-analysis.png)

In this sample, for every `10s`, `5%` traffic will be shifted to canary from the primary, until the traffic on canary reached `50%`. At the mean time, the instance number of canary will automatically scale to `replicas: 2` per configured in Appfile.


Based on analysis result of the KPIs during this traffic shifting, a canary will be promoted or aborted if analysis is failed. If promoting, the primary will be upgraded from v1 to v2, and traffic will be fully shifted back to the primary instances. So as result, canary instances will be deleted after the promotion finished.

![alt](../../../../../docs/en/resources/promotion.png)

> Note: KubeVela's `Rollout` trait is implemented with [Weaveworks Flagger](https://flagger.app/) operator.