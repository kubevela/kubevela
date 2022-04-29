# Apply workload/trait through 3-way-merge-patch

- Owner: Yue Wang(@captainroy-hy), Jianbo Sun(@wonderflow)
- Date: 01/21/2021
- Status: [Implemented](https://github.com/kubevela/kubevela/pull/857)


## Intro

When an ApplicationConfiguration is deployed, 
vela-core will create(apply) corresponding workload/trait instances and keep them stay align with the `spec` defined in ApplicationConfiguration through periodical reconciliation in AppConfig controller. 

In each round of reconciliation, if the configurations rendered from AppConfig are changed comparing to last round reconciliation, it's required to apply all changes to the workloads or traits.
Additionally, it also allows others (anything except AppConfig controller, e.g., trait controllers) to modify workload/trait instances. 


## Goals

Apply should handle three kinds of modification including 
- add a field
- change a field
- remove a field by omitting it

Meanwhile, Apply should have no impact on changes made by others, namely, not eliminate or override those changes UNLESS the change is made upon fields that are rendered from AppConfig originally.


## Implementation

We employed the same mechanism as `kubectl apply`, that is, computing a 3-way diff based on target object's current state, modified state, and last-appied state. 
Specifically, a new annotation, `app.oam.dev/last-applied-configuration`, is introduced to record workload/trait's last-applied state.

Once there's a conflict on field, both changed by AppConfig and others, AppConfig's value will always override others' assignment. 


## Impact on existing system

Before this implementation, vela-core use `JSON-Merge` patch to apply workloads and `update` to apply traits. 
That brought several defects shown in below samples. 
This section introduced a comparison between how old mechanism and new applies workload/trait, also shows how new Apply overcomed the defects. 

### Apply Workloads

The reason why abandon json-merge patch is that, it cannot remove a field through unsetting value in the patched manifest. 

#### Before

For example, apply below deployment as a workload. json-merge patch cannot remove `minReadySeconds` field through applying a modified manifest with `minReadySeconds` omitted .
```yaml
# original workload manifest
apiVersion: apps/v1
kind: Deployment
...
spec:
  minReadySeconds: 60
  replicas: 3
  template:
    spec:
      containers:
      - name: nginx
        image: nginx:1.14.2
---
# modified workload manifest
apiVersion: apps/v1
kind: Deployment
...
spec:
  # minReadySeconds: 60 <=== unset to remove field
  replicas: 3
  template:
    spec:
      containers:
      - name: nginx
        image: nginx:1.14.2
---
# result 
apiVersion: apps/v1
kind: Deployment
...
spec:
  minReadySeconds: 60 # <=== not removed
  replicas: 3
  template:
    spec:
      containers:
      - name: nginx
        image: nginx:1.14.2
```
#### After

By computing a 3-way diff, we can get a patch aware of the field set in last-applied manifest is omitted in the new modified manifest, namely, users wanna remove this field. 
And an annotation, `app.oam.dev/last-applied-configuration`, is used to record last-applied-state of the resource for further use in computing 3-way diff next time.

```yaml
# result 
apiVersion: apps/v1
kind: Deployment
metadata:
  annotations: # v=== record last-applied-state
    app.oam.dev/last-applied-configuration: | 
    {"apiVersion":"apps/v1","kind":"Deployment","metadata":{"name":"nginx-deployment","labels":{"app":"nginx"}},"spec":{"replicas":3,"selector":{"matchLabels":{"app":"nginx"}},"template":{"metadata":{"labels":{"app":"nginx"}},"spec":{"containers":[{"name":"nginx","image":"nginx:1.14.2"}]}}}}
...
spec:
  # minReadySeconds: 60 <=== removed successfully
  replicas: 3
  template:
    spec:
      containers:
      - name: nginx
        image: nginx:1.14.2
```
---

### Apply Traits

The reasons why abandon `update` 

 - update always eliminates all fields set by others (e.g., trait controllers)
 - if trait contains immutable field (e.g., k8s Service), update fails

#### Before
For example, apply below Service as a trait.
```yaml
# original trait manifest
apiVersion: v1
kind: Service
metadata:
  name: my-service
spec:
  selector:
    app: myweb
  ports:
    - protocol: TCP
      port: 80
---
# after applying
apiVersion: v1
kind: Service
...
spec:
  clusterIP: 172.21.7.149 # <=== immutable field
  ports:
  - port: 80
    protocol: TCP
    targetPort: 80
  selector:
    app: myweb
  sessionAffinity: None
  type: ClusterIP
status:
  loadBalancer: {}
---
# update with original manifest fails
# reconciling also fails for cannot applying trait
```
Additionally, if a trait has no immutable field, update will eliminate all fields set by others.
```yaml
# original trait manifest
kind: Bar
spec:
    f1: v1
# someone add a new field to it
kind: Bar
spec:
    f1: v1
    f2: v2 # <=== newly set field
# after reconciling AppConfig
kind: Bar
spec:
    f1: v1
    # f2: v2 <=== removed
```
But as described in [Goals](#goals) section, we expect to keep these changes.

#### After

Applying traits works in the same way as workloads. We use annotation, `app.oam.dev/last-applied-configuration`, to record last-applied manifest.

- Because 3-way diff will ignore the fields not touched in last-applied-state, the immutable fields will not be involved into patch data.
- Because 3-way diff also will ignore the fields set by others (others are not supposed to modify the field rendered from AppConfig), the changes made by others will be retained. 

