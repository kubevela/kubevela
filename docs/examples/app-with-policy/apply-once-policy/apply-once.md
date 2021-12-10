# How to use ApplyOnce policy

By default, the KubeVela operator will prevent configuration drift for applied resources by reconciling them routinely. This is useful if you want to keep your application always have the desired configuration in avoid of some unintentional changes by external modifiers.

However, sometimes, you might want to use KubeVela Application to do the dispatch job and recycle job but want to leave resources mutable after workflow is finished. In this case, you can use the following ApplyOnce policy. 

```shell
$ cat <<EOF | kubectl apply -f -
apiVersion: core.oam.dev/v1beta1
kind: Application
metadata:
  name: apply-once-app
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

In this case, if you change the replicas of the `hello-world` deployment after Application enters `running` state, it would be brought back. On the contrary, if you set the `apply-once` policy to be disabled (by default), any changes to the replicas of `hello-world` application will be brought back in the next reconcile loop.
