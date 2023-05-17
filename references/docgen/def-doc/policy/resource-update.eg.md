`resource-update` policy can allow users to customize the update behavior for selected resources.


```yaml
apiVersion: core.oam.dev/v1beta1
kind: Application
metadata:
  name: recreate
spec:
  components:
    - type: k8s-objects
      name: recreate
      properties:
        objects:
          - apiVersion: v1
            kind: Secret
            metadata:
              name: recreate
            data:
              key: dgo=
            immutable: true
  policies:
    - type: resource-update
      name: resource-update
      properties:
        rules:
          - selector:
              resourceTypes: ["Secret"]
            strategy:
              recreateFields: ["data.key"]
```
By specifying `recreateFields`, the application will recreate the target resource (**Secret** here) when the field changes (`data.key` here). If the field is not changed, the application will use the normal update (**patch** here).

```yaml
apiVersion: core.oam.dev/v1beta1
kind: Application
metadata:
  name: recreate
spec:
  components:
    - type: k8s-objects
      name: recreate
      properties:
        objects:
          - apiVersion: v1
            kind: ConfigMap
            metadata:
              name: recreate
            data:
              key: val
  policies:
    - type: resource-update
      name: resource-update
      properties:
        rules:
          - selector:
              resourceTypes: ["ConfigMap"]
            strategy:
              op: replace
```
By specifying `op` to `replace`, the application will update the given resource (ConfigMap here) by replace. Compared to **patch**, which leverages three-way merge patch to only modify the fields managed by KubeVela application, "replace" will update the object as a whole and wipe out other fields even if it is not managed by the KubeVela application. It can be seen as an "application-level" *ApplyResourceByReplace*.