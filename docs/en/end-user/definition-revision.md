---
title:  Specify Definition Revision in Application
---

Every time the platform-builder update ComponentDefinition/TraitDefinition, a corresponding DefinitionRevision will be generated.
And the DefinitionRevision can be regarded as a snapshot of ComponentDefinition/TraitDefinition.

## Usage of Definition

Suppose we need a `worker` to run a background program. And the platform-builder has implemented a `worker` 
ComponentDefinition for end-user(The ComponentDefinition `worker` may have been updated multiple times).


First, the platform-builder registered the `v1` version of the `worker`, let's see how to use this version of the `worker`.

**Click to see how to register the v1 version of the worker**
<details>

```shell
kubectl apply -f https://raw.githubusercontent.com/oam-dev/kubevela/master/docs/examples/definition-revision/worker-v1.yaml
```

</details>

We can use `kubectl vela show` to show the reference doc for `worker`.

```shell
$ kubectl vela show worker
# Properties
+-------+----------------------------------------------------+----------+----------+---------+
| NAME  |                    DESCRIPTION                     |   TYPE   | REQUIRED | DEFAULT |
+-------+----------------------------------------------------+----------+----------+---------+
| cmd   | Commands to run in the container                   | []string | false    |         |
| image | Which image would you like to use for your service | string   | true     |         |
+-------+----------------------------------------------------+----------+----------+---------+
```

Next the platform-builder created the `v2` version of the `worker`.

**Click to see how to register the v2 version of the worker**
<details>

```shell
kubectl apply -f https://raw.githubusercontent.com/oam-dev/kubevela/master/docs/examples/definition-revision/worker-v2.yaml
```

</details>

The latest worker ComponentDefinition adds the `port` parameter, allowing users to specify the port exposed by the container.

```shell
$ kubectl vela show worker
# Properties
+-------+----------------------------------------------------+----------+----------+---------+
| NAME  |                    DESCRIPTION                     |   TYPE   | REQUIRED | DEFAULT |
+-------+----------------------------------------------------+----------+----------+---------+
| cmd   | Commands to run in the container                   | []string | false    |         |
| image | Which image would you like to use for your service | string   | true     |         |
| port  | Which port do you want customer traffic sent to    | int      | true     |         |
+-------+----------------------------------------------------+----------+----------+---------+
```

After the platform-builder has updated the two versions of the worker, the corresponding DefinitionRevision will 
be generated to store the snapshot information.

```shell
$ kubectl get definitionrevision -l="componentdefinition.oam.dev/name=worker"
NAME        REVISION   HASH               TYPE
worker-v1   1          76486234845427dc   Component
worker-v2   2          cb22fdc3b037702e   Component
```

## Specify Definition Version in Application

We can specify the Component to use a specific version of the Definition in the Application,
If no special declaration is made, the app will use the latest Definition to render the Component.

Application `testapp` use the latest `worker` ComponentDefinition to specify the exposed port for the service.

```yaml
# testapp.yaml
apiVersion: core.oam.dev/v1beta1
kind: Application
metadata:
  name: testapp
spec:
  components:
    - name: backend
      type: worker
      properties:
        image: oamdev/testapp:v1
        port: 8080
        cmd: ["node", "server.js"]
```

```shell
kubectl apply -f testapp.yaml
```

After deploy the Application `testapp`, we can see that the resources `Deployment` is generated.

```shell
$ kubectl get deployment
NAME      READY   UP-TO-DATE   AVAILABLE   AGE
backend   1/1     1            1           8s

$ kubectl get deployment backend -o jsonpath="{.spec.template.spec.containers[0].ports[0].containerPort}"
8080
```

Sometimes the latest Definition may not meet the user's requirement, so we want to use some old version like `v1` of the worker to render the Component.
we can specify the version of Definition in format `definitionName@version`.

```yaml
# testapp.yaml
apiVersion: core.oam.dev/v1beta1
kind: Application
metadata:
  name: testapp
spec:
  components:
    - name: backend
      type: worker@v1
      properties:
        image: crccheck/hello-world
```

```shell
kubectl apply -f testapp.yaml
```

The `v1` version of the `worker` only allows the container to expose port `8000`.

```shell
$ kubectl get deployment
NAME      READY   UP-TO-DATE   AVAILABLE   AGE
backend   1/1     1            1           3m7s

$ kubectl get deployment backend -o jsonpath="{.spec.template.spec.containers[0].ports[0].containerPort}"
8000
```