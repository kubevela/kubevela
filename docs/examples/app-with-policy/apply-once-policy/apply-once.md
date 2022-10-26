# How to use ApplyOnce policy

By default, the KubeVela operator will prevent configuration drift for applied resources by reconciling them routinely.
This is useful if you want to keep your application always have the desired configuration in avoid of some unintentional
changes by external modifiers.

However, sometimes, you might want to use KubeVela Application to do the dispatch job and recycle job but want to leave
resources mutable after workflow is finished such as `Horizontal Pod Autoscaler`, etc. In this case, you can use the
following ApplyOnce policy.

```shell
$ cat <<EOF | kubectl apply -f -
apiVersion: core.oam.dev/v1beta1
kind: Application
metadata:
  name: apply-once-app-1
spec:
  components:
    - name: hello-world
      type: webservice
      properties:
        image: crccheck/hello-world
      traits:
        - type: scaler
          properties:
            replicas: 1
  policies:
    - name: apply-once
      type: apply-once
      properties:
        enable: true
EOF
```

In the `apply-once-app-1` case, if you change the replicas of the `hello-world` deployment after Application
enters `running` state, it would be brought back. On the contrary, if you set the `apply-once` policy to be disabled (by
default), any changes to the replicas of `hello-world` application will be brought back in the next reconcile loop.

```shell
$ cat <<EOF | kubectl apply -f -
apiVersion: core.oam.dev/v1beta1
kind: Application
metadata:
  name: apply-once-app-2
spec:
  components:
    - name: hello-world
      type: webservice
      properties:
        image: crccheck/hello-world
      traits:
        - type: scaler
          properties:
            replicas: 1
    - name: hello-cosmos
      type: webservice
      properties:
        image: crccheck/hello-world
      traits:
        - type: scaler
          properties:
            replicas: 1
  policies:
    - name: apply-once
      type: apply-once
      properties:
        enable: true
        rules:
          - selector:
              componentNames: [ "hello-cosmos" ]
              resourceTypes: [ "Deployment" ]
            strategy:
              path: [ "spec.replicas", "spec.template.spec.containers[0].resources" ]
EOF
```

In the `apply-once-app-2` case, any changes to the replicas or containers[0].resources of `hello-cosmos` deployment will
not be brought back in the next reconcile loop. And any changes of `hello-world` component will be brought back in the
next reconcile loop.

```shell
$ cat <<EOF | kubectl apply -f -
apiVersion: core.oam.dev/v1beta1
kind: Application
metadata:
  name: apply-once-app-3
spec:
  components:
    - name: hello-world
      type: webservice
      properties:
        image: crccheck/hello-world
        port: 8080
      traits:
        - type: scaler
          properties:
            replicas: 1
    - name: hello-cosmos
      type: webservice
      properties:
        image: crccheck/hello-world
        port: 8080
      traits:
        - type: scaler
          properties:
            replicas: 1
  policies:
    - name: apply-once
      type: apply-once
      properties:
        enable: true
        rules:
          - selector:
              componentNames: [ "hello-cosmos" ]
              resourceTypes: [ "Deployment" ]
            strategy:
              path: [ "*" ]
EOF
```

In the `apply-once-app-3` case, any changes of `hello-cosmos` deployment will not be brought back and any changes
of `hello-cosmos` service will be brought back in the next reconcile loop. In the same time, any changes
of `hello-world` component will be brought back in the next reconcile loop.

```shell
$ cat <<EOF | kubectl apply -f -
apiVersion: core.oam.dev/v1beta1
kind: Application
metadata:
  name: apply-once-app-4
spec:
  components:
    - name: hello-world
      type: webservice
      properties:
        image: crccheck/hello-world
        port: 8080
      traits:
        - type: scaler
          properties:
            replicas: 1
    - name: hello-cosmos
      type: webservice
      properties:
        image: crccheck/hello-world
        port: 8080
      traits:
        - type: scaler
          properties:
            replicas: 1
  policies:
    - name: apply-once
      type: apply-once
      properties:
        enable: true
        rules:
          - selector:
              componentNames: [ "hello-cosmos" ]
              resourceTypes: [ "Deployment" ]
            strategy:
              affect: onStateKeep
              path: [ "spec.replicas"]
EOF
```

By default, KubeVela executes the apply-once policy in two phases: application update and cycle state maintenance,
allowing configuration drift depending on the policy configuration.

If you have special requirements, you can set the affect to determine the phase of policy execution . 
affect supported configurations: onUpdate/onStateKeep/always (default)

When affect=always, or not set, the policy is executed in two phase.

When affect=onStateKeep, the policy is executed only during the stateKeep phase. In the case of `apply-once-app-4`, any
changes to the deployed copy of `hello-cosmos` will not be brought back to the next state keeping loop, but will be
brought back to the next application update.

When affect=onUpdate, the policy is only executed when the application is updated. In the case of `
apply-once-app-4`, if affect=onUpdate is set, any changes to the deployed copy of `hello-cosmos` will not be brought
back in the next application update, but will be brought back in the next state keeping loop.
