---
title: Multi-Cluster Deployment
---

## Introduction

Modern application infrastructure involves multiple clusters to ensure high availability and maximize service throughput. In this section, we will introduce how to use KubeVela to achieve application deployment across multiple clusters with following features supported:
- Rolling Upgrade: To continuously deploy apps requires to rollout in a safe manner which usually involves step by step rollout batches and analysis.
- Traffic shifting: When rolling upgrade an app, it needs to split the traffic onto both the old and new revisions to verify the new version while preserving service availability.

### AppDeployment

The `AppDeployment` API in KubeVela is provided to satisfy such requirements. Here's an overview of the API:

```yaml
apiVersion: core.oam.dev/v1beta1
kind: AppDeployment
metadata:
  name: sample-appdeploy
spec:
  traffic:
    hosts:
      - example.com

    http:
      - match:
          # match any requests to 'example.com/example-app'
          - uri:
              prefix: "/example-app"

        # split traffic 50/50 on v1/v2 versions of the app
        weightedTargets:
          - revisionName: example-app-v1
            componentName: testsvc
            port: 80
            weight: 50
          - revisionName: example-app-v2
            componentName: testsvc
            port: 80
            weight: 50

  appRevisions:
    - # Name of the AppRevision.
      # Each modification to Application would generate a new AppRevision.
      revisionName: example-app-v1

      # Cluster specific workload placement config
      placement:
        - clusterSelector:
            # You can select Clusters by name or labels.
            # If multiple clusters is selected, one will be picked via a unique hashing algorithm.
            labels:
              tier: production
            name: prod-cluster-1

          distribution:
            replicas: 5

        - # If no clusterSelector is given, it will use the host cluster in which this CR exists
          distribution:
            replicas: 5

    - revisionName: example-app-v2
      placement:
        - clusterSelector:
            labels:
              tier: production
            name: prod-cluster-1
          distribution:
            replicas: 5
        - distribution:
            replicas: 5
```

### Cluster

The clusters selected in the `placement` part from above is defined in Cluster CRD. Here's what it looks like:

```yaml
apiVersion: core.oam.dev/v1beta1
kind: Cluster
metadata:
  name: prod-cluster-1
  labels:
    tier: production
spec:
  kubeconfigSecretRef:
    name: kubeconfig-cluster-1 # the secret name
```

The secret must contain the kubeconfig credentials in `config` field:

```yaml
apiVersion: v1
kind: Secret
metadata:
  name: kubeconfig-cluster-1
data:
  config: ... # kubeconfig data
```

## Quickstart

Here's a step-by-step tutorial for you to try out. All of the yaml files are from [`docs/examples/appdeployment/`](https://github.com/oam-dev/kubevela/tree/master/docs/examples/appdeployment).
You must run all commands in that directory.

1. Create an Application

   ```bash
   $ cat <<EOF | kubectl apply -f -
   apiVersion: core.oam.dev/v1beta1
   kind: Application
   metadata:
     name: example-app
     annotations:
       app.oam.dev/revision-only: "true"
   spec:
     components:
       - name: testsvc
         type: webservice
         properties:
           addRevisionLabel: true
           image: crccheck/hello-world
           port: 8000
   EOF
   ```

   This will create `example-app-v1` AppRevision. Check it:

   ```bash
   $ kubectl get applicationrevisions.core.oam.dev
   NAME             AGE
   example-app-v1   116s
   ```

   > Note: with `app.oam.dev/revision-only: "true"` annotation, above `Application` resource won't create any pod instances and leave the real deployment process to `AppDeployment`.

1. Then use the above AppRevision to create an AppDeployment.

   ```bash
   $ kubectl apply -f appdeployment-1.yaml
   ```

   > Note: in order to AppDeployment to work, your workload object must have a `spec.replicas` field for scaling.

1. Now you can check that there will 1 deployment and 2 pod instances deployed

   ```bash
   $ kubectl get deploy
   NAME         READY   UP-TO-DATE   AVAILABLE   AGE
   testsvc-v1   2/2     2            0           27s
   ```

1. Update Application properties:

   ```bash
   $ cat <<EOF | kubectl apply -f -
   apiVersion: core.oam.dev/v1beta1
   kind: Application
   metadata:
     name: example-app
     annotations:
       app.oam.dev/revision-only: "true"
   spec:
     components:
       - name: testsvc
         type: webservice
         properties:
           addRevisionLabel: true
           image: nginx
           port: 80
   EOF
   ```

   This will create a new `example-app-v2` AppRevision. Check it:

   ```bash
   $ kubectl get applicationrevisions.core.oam.dev
   NAME
   example-app-v1
   example-app-v2
   ```

1. Then use the two AppRevisions to update the AppDeployment:

   ```bash
   $ kubectl apply -f appdeployment-2.yaml
   ```

   (Optional) If you have Istio installed, you can apply the AppDeployment with traffic split:

   ```bash
   # set up gateway if not yet
   $ kubectl apply -f gateway.yaml

   $ kubectl apply -f appdeployment-2-traffic.yaml
   ```

   Note that for traffic split to work, your must set the following pod labels in workload cue templates (see [webservice.cue](https://github.com/oam-dev/kubevela/blob/master/hack/vela-templates/cue/webservice.cue)):

   ```shell
   "app.oam.dev/component": context.name
   "app.oam.dev/appRevision": context.appRevision
   ```

1. Now you can check that there will 1 deployment and 1 pod per revision.

   ```bash
   $ kubectl get deploy
   NAME         READY   UP-TO-DATE   AVAILABLE   AGE
   testsvc-v1   1/1     1            1           2m14s
   testsvc-v2   1/1     1            1           8s
   ```

   (Optional) To verify traffic split:

   ```bash
   # run this in another terminal
   $ kubectl -n istio-system port-forward service/istio-ingressgateway 8080:80
   Forwarding from 127.0.0.1:8080 -> 8080
   Forwarding from [::1]:8080 -> 8080

   # The command should return pages of either docker whale or nginx in 50/50
   $ curl -H "Host: example-app.example.com" http://localhost:8080/
   ```

1. Cleanup:

   ```bash
   kubectl delete appdeployments.core.oam.dev  --all
   kubectl delete applications.core.oam.dev --all
   ```
