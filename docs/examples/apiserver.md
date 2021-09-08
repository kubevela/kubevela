# How to use apiserver

## Preparation

The apiserver is enabled by default, but you need some extra works to make it accessible outside the k8s cluster.

1. For development environment, you can use `kubectl port-forward` to access the KubeVela apiserver.

```shell
kubectl port-forward --namespace vela-system service/kubevela-vela-core-apiserver 8000:80
```

2. For production environment, you can set up Ingress to expose the KubeVela apiserver.

```shell
cat <<EOF | kubectl apply -f -
apiVersion: networking.k8s.io/v1
kind: Ingress
metadata:
  name: kubevela-vela-core-apiserver
  namespace: vela-system
spec:
  defaultBackend:
    service:
      name: kubevela-vela-core-apiserver
      port:
        number: 80
EOF
```

```shell
$ kubectl get ingress -n vela-system
NAME                           CLASS    HOSTS   ADDRESS         PORTS   AGE
kubevela-vela-core-apiserver   <none>   *       <your ip>       80      99m
```

## Api

### Create or Update Application

1. URL

```
POST /v1/namespaces/<namespace>/applications/<application name>
```

2. Request Body Example

```json
{
  "components": [
    {
      "name": "express-server",
      "type": "webservice",
      "properties": {
        "image": "crccheck/hello-world",
        "port": 8000
      },
      "traits": [
        {
          "type": "ingress",
          "properties": {
            "domain": "testsvc.example.com",
            "http": {
              "/": 8000
            }
          }
        }
      ]
    }
  ]
}
```

### Delete Application

```
DELETE /v1/namespaces/<namespace>/applications/<application name>
```