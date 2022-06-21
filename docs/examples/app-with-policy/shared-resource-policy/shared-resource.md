## How to share resources across applications

Sometimes, you may want different applications to share the same resource.
For example, you might have various applications that needs the same namespace to exist.
In this case, you can use the `shared-resource` policy to declare which resources should be shared.

### Usage

```yaml
apiVersion: core.oam.dev/v1beta1
kind: Application
metadata:
  name: app1
spec:
  components:
    - name: ns1
      type: k8s-objects
      properties:
        objects:
          - apiVersion: v1
            kind: Namespace
            metadata:
              name: example
    - name: cm1
      type: k8s-objects
      properties:
        objects:
          - apiVersion: v1
            kind: ConfigMap
            metadata:
              name: cm1
              namespace: example
            data:
              key: value1
  policies:
    - name: shared-resource
      type: shared-resource
      properties:
        rules:
          - selector:
              resourceTypes: ["Namespace"]
```

```yaml
apiVersion: core.oam.dev/v1beta1
kind: Application
metadata:
  name: app2
spec:
  components:
    - name: ns2
      type: k8s-objects
      properties:
        objects:
          - apiVersion: v1
            kind: Namespace
            metadata:
              name: example
    - name: cm2
      type: k8s-objects
      properties:
        objects:
          - apiVersion: v1
            kind: ConfigMap
            metadata:
              name: cm2
              namespace: example
            data:
              key: value2
  policies:
    - name: shared-resource
      type: shared-resource
      properties:
        rules:
          - selector:
              resourceTypes: ["Namespace"]
```

The above two applications will dispatch the same namespace "example".
They will create two different ConfigMap inside namespace "example" respectively.

Both application use the `shared-resource` policy and declared the namespace resource as shared.
In this way, there will be no conflict for creating the same namespace.
If the `shared-resource` policy is not used, the second application will report error after it finds that the namespace "example" is managed by the first application. 

The namespace will only be recycled when both applications are removed.

### Working Detail

One of the problem for sharing resource is that what will happen if different application holds different configuration for the shared resource.
In the `shared-resource` policy, all sharers will be recorded by time order. The first sharer will be able to write the resource while other sharers can only read it. After the first sharer is deleted, it will give the control of the resource to the next sharer. If no sharer is handling it, the resource will be finally removed.
