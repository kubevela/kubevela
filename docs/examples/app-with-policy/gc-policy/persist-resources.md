# How to persist resources

By leveraging the garbage-collect policy, users can persist some resources, which skip the normal garbage-collect process when application is updated.

Take the following app as an example, in the garbage-collect policy, a rule is added which marks all the resources created by the `expose` trait to use the `onAppDelete` strategy. This will keep those services until application is deleted.
```shell
$ cat <<EOF | kubectl apply -f -
apiVersion: core.oam.dev/v1beta1
kind: Application
metadata:
  name: garbage-collect-app
spec:
  components:
    - name: hello-world
      type: webservice
      properties:
        image: crccheck/hello-world
      traits:
        - type: expose
          properties:
            port: [8000]
  policies:
    - name: garbage-collect
      type: garbage-collect
      properties:
        rules:
          - selector:
              traitTypes:
                - expose
            strategy: onAppDelete
EOF
```

You can find deployment and service are created.
```shell
$ kubectl get deployment
NAME          READY   UP-TO-DATE   AVAILABLE   AGE
hello-world   1/1     1            1           74s
$ kubectl get service   
NAME          TYPE        CLUSTER-IP      EXTERNAL-IP   PORT(S)    AGE
hello-world   ClusterIP   10.96.160.208   <none>        8000/TCP   78s
```

If you upgrade the application and use a different component, you will find the old versioned deployment is deleted by the service is kept.
```shell
$ cat <<EOF | kubectl apply -f -
apiVersion: core.oam.dev/v1beta1
kind: Application
metadata:
  name: garbage-collect-app
spec:
  components:
    - name: hello-world-new
      type: webservice
      properties:
        image: crccheck/hello-world
      traits:
        - type: expose
          properties:
            port: [8000]
  policies:
    - name: garbage-collect
      type: garbage-collect
      properties:
        rules:
          - selector:
              traitTypes:
                - expose
            strategy: onAppDelete
EOF

$ kubectl get deployment
NAME              READY   UP-TO-DATE   AVAILABLE   AGE
hello-world-new   1/1     1            1           10s
$ kubectl get service   
NAME              TYPE        CLUSTER-IP      EXTERNAL-IP   PORT(S)    AGE
hello-world       ClusterIP   10.96.160.208   <none>        8000/TCP   5m56s
hello-world-new   ClusterIP   10.96.20.4      <none>        8000/TCP   13s
```

Users can also keep component if they are deploying job-like components. Resources dispatched by `job-like-component` type component will be kept after application is deleted.

```yaml
apiVersion: core.oam.dev/v1beta1
kind: Application
metadata:
  name: garbage-collect-app
spec:
  components:
    - name: hello-world-new
      type: job-like-component
  policies:
    - name: garbage-collect
      type: garbage-collect
      properties:
        rules:
          - selector:
              componentTypes:
                - webservice
            strategy: never
```

A more straightforward way is to specify `compNames` to match specified components.
```yaml
apiVersion: core.oam.dev/v1beta1
kind: Application
metadata:
  name: create-ns-app
spec:
  components:
    - name: example-addon-namespace
      type: k8s-objects
      properties:
        objects:
          - apiVersion: v1
            kind: Namespace
  policies:
    - name: garbage-collect
      type: garbage-collect
      properties:
        rules:
          - selector:
              componentNames:
                - example-addon-namespace
            strategy: never
```
